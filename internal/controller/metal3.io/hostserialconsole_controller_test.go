package controllers

import (
	"context"
	"errors"
	"net"
	"testing"

	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/event"
)

type consoleMock struct {
	*mockProvisioner
	state              provisioner.SerialConsoleState
	stateErr           error
	endpointErr        error
	setErr             error
	endpointCalls      int
	serialConsole      *metal3api.SerialConsoleEndpoint
	serialConsoleCalls []bool
}

func (p *consoleMock) GetSerialConsoleState(context.Context) (provisioner.SerialConsoleState, error) {
	return p.state, p.stateErr
}
func (p *consoleMock) GetSerialConsole(context.Context) (*metal3api.SerialConsoleEndpoint, error) {
	p.endpointCalls++
	return p.serialConsole, p.endpointErr
}
func (p *consoleMock) SetSerialConsole(_ context.Context, enabled bool) error {
	p.serialConsoleCalls = append(p.serialConsoleCalls, enabled)
	return p.setErr
}

type consoleFactory struct {
	prov provisioner.Provisioner
	data []provisioner.HostData
	err  error
}

func (f *consoleFactory) NewProvisioner(_ context.Context, data provisioner.HostData, _ provisioner.EventPublisher) (provisioner.Provisioner, error) {
	f.data = append(f.data, data)
	return f.prov, f.err
}

func newConsoleTest(t *testing.T) (*HostSerialConsoleReconciler, *consoleMock, ctrl.Request) {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, metal3api.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, discoveryv1.AddToScheme(scheme))
	host := &metal3api.BareMetalHost{ObjectMeta: metav1.ObjectMeta{Name: "worker-0", Namespace: "metal3", UID: "host-uid"}}
	host.Spec.BMC.CredentialsName = "bmc-secret"
	host.Status.Provisioning.State = metal3api.StateProvisioned
	host.Status.Provisioning.ID = "node-id"
	console := &metal3api.HostSerialConsole{ObjectMeta: metav1.ObjectMeta{Name: host.Name, Namespace: host.Namespace, UID: "console-uid"}}
	secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "bmc-secret", Namespace: host.Namespace}, Data: map[string][]byte{"username": []byte("admin"), "password": []byte("password")}}
	c := fake.NewClientBuilder().WithScheme(scheme).
		WithStatusSubresource(host, console).WithObjects(host, console, secret).Build()
	p := &consoleMock{mockProvisioner: newMockProvisioner()}
	p.state = provisioner.SerialConsoleState{ID: "node-id/conductor/2023"}
	p.serialConsole = &metal3api.SerialConsoleEndpoint{Host: "192.0.2.10", Port: 2023}
	return &HostSerialConsoleReconciler{Client: c, APIReader: c, ProvisionerFactory: &consoleFactory{prov: p}}, p, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(console)}
}

func consoleObject(t *testing.T, r *HostSerialConsoleReconciler, req ctrl.Request) *metal3api.HostSerialConsole {
	t.Helper()
	obj := &metal3api.HostSerialConsole{}
	require.NoError(t, r.Get(t.Context(), req.NamespacedName, obj))
	return obj
}

func TestHostSerialConsoleLifecycle(t *testing.T) {
	r, p, req := newConsoleTest(t)
	ctx := t.Context()
	_, err := r.Reconcile(ctx, req)
	require.NoError(t, err)
	require.Equal(t, []bool{true}, p.serialConsoleCalls)
	obj := consoleObject(t, r, req)
	require.Equal(t, "node-id", obj.Status.ProvisionerID)
	require.Contains(t, obj.Finalizers, metal3api.HostSerialConsoleFinalizer)
	require.Equal(t, "Enabling", meta.FindStatusCondition(obj.Status.Conditions, "Ready").Reason)

	p.state.Enabled = true
	_, err = r.Reconcile(ctx, req)
	require.NoError(t, err)
	obj = consoleObject(t, r, req)
	assert.True(t, meta.IsStatusConditionTrue(obj.Status.Conditions, "Ready"))
	assert.Equal(t, "tcp://serial-console-console-uid.metal3.svc:2023", obj.Status.ConsoleURL)
	svc := &corev1.Service{}
	key := types.NamespacedName{Namespace: req.Namespace, Name: obj.Status.ServiceName}
	require.NoError(t, r.Get(ctx, key, svc))
	assert.Equal(t, corev1.ServiceTypeClusterIP, svc.Spec.Type)
	assert.Empty(t, svc.Spec.Selector)
	slice := &discoveryv1.EndpointSlice{}
	require.NoError(t, r.Get(ctx, key, slice))
	assert.Empty(t, validation.IsValidLabelValue(slice.Labels[discoveryv1.LabelManagedBy]))
	assert.Equal(t, []string{"192.0.2.10"}, slice.Endpoints[0].Addresses)
	assert.Equal(t, int32(2023), *slice.Ports[0].Port)

	// Monitoring, controller restarts, and Service repair must not call the
	// disruptive console API or restart the backend console.
	require.NoError(t, r.Delete(ctx, svc))
	for range 3 {
		_, err = r.Reconcile(ctx, req)
		require.NoError(t, err)
	}
	assert.Equal(t, 1, p.endpointCalls)
	assert.Equal(t, []bool{true}, p.serialConsoleCalls)
	require.NoError(t, r.Get(ctx, key, svc))

	// Deletion withdraws the Service immediately, but the finalizer waits for
	// confirmed disable completion, not just acceptance of the PUT request.
	require.NoError(t, r.Delete(ctx, obj))
	_, err = r.Reconcile(ctx, req)
	require.NoError(t, err)
	assert.True(t, k8serrors.IsNotFound(r.Get(ctx, key, &corev1.Service{})))
	assert.True(t, k8serrors.IsNotFound(r.Get(ctx, key, &discoveryv1.EndpointSlice{})))
	obj = consoleObject(t, r, req)
	assert.Contains(t, obj.Finalizers, metal3api.HostSerialConsoleFinalizer)
	assert.Equal(t, []bool{true, false}, p.serialConsoleCalls)
	p.state.Enabled = false
	_, err = r.Reconcile(ctx, req)
	require.NoError(t, err)
	assert.True(t, k8serrors.IsNotFound(r.Get(ctx, req.NamespacedName, &metal3api.HostSerialConsole{})))
}

func TestHostSerialConsoleBackendReplacement(t *testing.T) {
	r, p, req := newConsoleTest(t)
	ctx := t.Context()
	p.state.Enabled = true
	_, err := r.Reconcile(ctx, req)
	require.NoError(t, err)
	p.state.ID = "node-id/other-conductor/3023"
	p.serialConsole = &metal3api.SerialConsoleEndpoint{Host: "192.0.2.11", Port: 3023}
	_, err = r.Reconcile(ctx, req)
	require.NoError(t, err)
	assert.Equal(t, 2, p.endpointCalls)
	obj := consoleObject(t, r, req)
	slice := &discoveryv1.EndpointSlice{}
	require.NoError(t, r.Get(ctx, types.NamespacedName{Name: obj.Status.ServiceName, Namespace: req.Namespace}, slice))
	assert.Equal(t, []string{"192.0.2.11"}, slice.Endpoints[0].Addresses)
	assert.Equal(t, int32(3023), *slice.Ports[0].Port)
}

func TestHostSerialConsoleCleanupWithoutHostOrSecret(t *testing.T) {
	for _, missingNode := range []bool{false, true} {
		t.Run(map[bool]string{false: "node-exists", true: "node-missing"}[missingNode], func(t *testing.T) {
			r, p, req := newConsoleTest(t)
			p.state.Enabled = true
			_, err := r.Reconcile(t.Context(), req)
			require.NoError(t, err)
			obj := consoleObject(t, r, req)
			require.NoError(t, r.Delete(t.Context(), obj))
			host := &metal3api.BareMetalHost{}
			require.NoError(t, r.Get(t.Context(), req.NamespacedName, host))
			require.NoError(t, r.Delete(t.Context(), host))
			require.NoError(t, r.Delete(t.Context(), &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "bmc-secret", Namespace: req.Namespace}}))
			if missingNode {
				p.stateErr = provisioner.ErrNeedsRegistration
			}
			_, err = r.Reconcile(t.Context(), req)
			require.NoError(t, err)
			if !missingNode {
				require.Equal(t, []bool{false}, p.serialConsoleCalls, "disable must work after deleting the host and credentials")
				require.Contains(t, consoleObject(t, r, req).Finalizers, metal3api.HostSerialConsoleFinalizer)
				p.state.Enabled = false
				_, err = r.Reconcile(t.Context(), req)
				require.NoError(t, err)
			}
			assert.True(t, k8serrors.IsNotFound(r.Get(t.Context(), req.NamespacedName, &metal3api.HostSerialConsole{})))
		})
	}
}

func TestHostSerialConsoleErrorsAndHostLifecycle(t *testing.T) {
	cases := []struct {
		name       string
		change     func(*metal3api.BareMetalHost, *consoleMock)
		deleting   bool
		wantReason string
	}{
		{name: "busy", change: func(_ *metal3api.BareMetalHost, p *consoleMock) { p.state.Busy = true }},
		{name: "enable-error", change: func(_ *metal3api.BareMetalHost, p *consoleMock) { p.setErr = errors.New("unsupported") }, wantReason: "ConsoleUnavailable"},
		{name: "endpoint-error", change: func(_ *metal3api.BareMetalHost, p *consoleMock) {
			p.state.Enabled = true
			p.endpointErr = errors.New("endpoint failed")
		}, wantReason: "ConsoleUnavailable"},
		{name: "paused", change: func(h *metal3api.BareMetalHost, _ *consoleMock) {
			h.Annotations = map[string]string{metal3api.PausedAnnotation: ""}
		}, wantReason: "Paused"},
		{name: "deprovisioning", change: func(h *metal3api.BareMetalHost, _ *consoleMock) {
			h.Status.Provisioning.State = metal3api.StateDeprovisioning
		}, deleting: true},
		{name: "detached", change: func(h *metal3api.BareMetalHost, _ *consoleMock) {
			h.Annotations = map[string]string{metal3api.DetachedAnnotation: ""}
		}, deleting: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r, p, req := newConsoleTest(t)
			h := &metal3api.BareMetalHost{}
			require.NoError(t, r.Get(t.Context(), req.NamespacedName, h))
			tc.change(h, p)
			state := h.Status.DeepCopy()
			require.NoError(t, r.Update(t.Context(), h))
			h.Status = *state
			require.NoError(t, r.Status().Update(t.Context(), h))
			_, err := r.Reconcile(t.Context(), req)
			require.NoError(t, err)
			if tc.deleting {
				assert.True(t, k8serrors.IsNotFound(r.Get(t.Context(), req.NamespacedName, &metal3api.HostSerialConsole{})))
				return
			}
			obj := consoleObject(t, r, req)
			if tc.wantReason != "" {
				require.Equal(t, tc.wantReason, meta.FindStatusCondition(obj.Status.Conditions, "Ready").Reason)
			}
			assert.Empty(t, obj.Status.ConsoleURL)
		})
	}
}

func TestHostSerialConsoleDoesNotAdoptForeignService(t *testing.T) {
	r, p, req := newConsoleTest(t)
	p.state.Enabled = true
	_, err := r.Reconcile(t.Context(), req)
	require.NoError(t, err)
	obj := consoleObject(t, r, req)
	key := types.NamespacedName{Name: obj.Status.ServiceName, Namespace: req.Namespace}
	owned := &corev1.Service{}
	require.NoError(t, r.Get(t.Context(), key, owned))
	require.NoError(t, r.Delete(t.Context(), owned))
	foreign := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: key.Name, Namespace: key.Namespace, CreationTimestamp: metav1.Now()},
		Spec: corev1.ServiceSpec{Type: corev1.ServiceTypeClusterIP, Ports: []corev1.ServicePort{{Port: 9999, Protocol: corev1.ProtocolTCP}}}}
	require.NoError(t, r.Create(t.Context(), foreign))
	_, err = r.Reconcile(t.Context(), req)
	require.NoError(t, err)
	require.Contains(t, meta.FindStatusCondition(consoleObject(t, r, req).Status.Conditions, "Ready").Message, "refusing to adopt")
	require.NoError(t, r.Get(t.Context(), client.ObjectKeyFromObject(foreign), foreign))
	assert.Empty(t, foreign.OwnerReferences)
	assert.Equal(t, int32(9999), foreign.Spec.Ports[0].Port)
}

func TestHostSerialConsoleDisableFailureWithdrawsService(t *testing.T) {
	r, p, req := newConsoleTest(t)
	p.state.Enabled = true
	_, err := r.Reconcile(t.Context(), req)
	require.NoError(t, err)
	obj := consoleObject(t, r, req)
	require.NoError(t, r.Delete(t.Context(), obj))
	p.setErr = errors.New("backend unavailable")
	_, err = r.Reconcile(t.Context(), req)
	require.NoError(t, err)
	obj = consoleObject(t, r, req)
	require.Contains(t, obj.Finalizers, metal3api.HostSerialConsoleFinalizer)
	require.Equal(t, "ConsoleUnavailable", meta.FindStatusCondition(obj.Status.Conditions, "Ready").Reason)
	require.Empty(t, obj.Status.ConsoleURL)
	services := &corev1.ServiceList{}
	require.NoError(t, r.List(t.Context(), services))
	require.Empty(t, services.Items)
}

func TestHostSerialConsoleIPv6(t *testing.T) {
	r, p, req := newConsoleTest(t)
	p.state.Enabled = true
	p.serialConsole = &metal3api.SerialConsoleEndpoint{Host: "2001:db8::10", Port: 3023}
	_, err := r.Reconcile(t.Context(), req)
	require.NoError(t, err)
	obj := consoleObject(t, r, req)
	svc := &corev1.Service{}
	key := types.NamespacedName{Namespace: req.Namespace, Name: obj.Status.ServiceName}
	require.NoError(t, r.Get(t.Context(), key, svc))
	require.Equal(t, []corev1.IPFamily{corev1.IPv6Protocol}, svc.Spec.IPFamilies)
	slice := &discoveryv1.EndpointSlice{}
	require.NoError(t, r.Get(t.Context(), key, slice))
	require.Equal(t, discoveryv1.AddressTypeIPv6, slice.AddressType)
	require.Equal(t, []string{"2001:db8::10"}, slice.Endpoints[0].Addresses)
}

func TestHostSerialConsoleNodeReplacementCleansOldNode(t *testing.T) {
	r, p, req := newConsoleTest(t)
	p.state.Enabled = true
	_, err := r.Reconcile(t.Context(), req)
	require.NoError(t, err)
	host := &metal3api.BareMetalHost{}
	require.NoError(t, r.Get(t.Context(), req.NamespacedName, host))
	host.Status.Provisioning.ID = "new-node-id"
	require.NoError(t, r.Status().Update(t.Context(), host))
	factory, ok := r.ProvisionerFactory.(*consoleFactory)
	require.True(t, ok)
	_, err = r.Reconcile(t.Context(), req)
	require.NoError(t, err)
	require.Equal(t, "node-id", factory.data[len(factory.data)-1].ProvisionerID)
	require.Equal(t, []bool{false}, p.serialConsoleCalls)
	require.Empty(t, consoleObject(t, r, req).Status.ServiceName)
	p.state.Enabled = false
	_, err = r.Reconcile(t.Context(), req)
	require.NoError(t, err)
	require.Empty(t, consoleObject(t, r, req).Status.ProvisionerID)
	_, err = r.Reconcile(t.Context(), req)
	require.NoError(t, err)
	require.Equal(t, "new-node-id", factory.data[len(factory.data)-1].ProvisionerID)
}

func TestHostSerialConsoleRejectsLoopbackEndpoint(t *testing.T) {
	r, p, req := newConsoleTest(t)
	p.state.Enabled = true
	p.serialConsole.Host = "127.0.0.1"
	_, err := r.Reconcile(t.Context(), req)
	require.NoError(t, err)
	obj := consoleObject(t, r, req)
	require.Equal(t, "ConsoleUnavailable", meta.FindStatusCondition(obj.Status.Conditions, "Ready").Reason)
	require.Empty(t, obj.Status.ServiceName)
}

func TestSerialConsoleRequestChanged(t *testing.T) {
	old := &metal3api.HostSerialConsole{ObjectMeta: metav1.ObjectMeta{Generation: 1}}
	updated := old.DeepCopy()
	updated.Status.ServiceName = "console"
	require.False(t, serialConsoleRequestChanged(event.UpdateEvent{ObjectOld: old, ObjectNew: updated}))
	now := metav1.Now()
	updated.DeletionTimestamp = &now
	require.True(t, serialConsoleRequestChanged(event.UpdateEvent{ObjectOld: old, ObjectNew: updated}))
	updated = old.DeepCopy()
	updated.Finalizers = []string{metal3api.HostSerialConsoleFinalizer}
	require.True(t, serialConsoleRequestChanged(event.UpdateEvent{ObjectOld: old, ObjectNew: updated}))
}

func TestSerialConsoleConditionPatchWithStaleVersion(t *testing.T) {
	r, _, req := newConsoleTest(t)
	stale := consoleObject(t, r, req)
	current := stale.DeepCopy()
	current.Labels = map[string]string{"updated": "true"}
	require.NoError(t, r.Update(t.Context(), current))
	_, err := r.setConsoleCondition(t.Context(), stale, "ConsoleUnavailable", "backend is not ready", metav1.ConditionFalse)
	require.NoError(t, err)
	latest := consoleObject(t, r, req)
	require.Equal(t, "true", latest.Labels["updated"])
	require.Equal(t, "ConsoleUnavailable", meta.FindStatusCondition(latest.Status.Conditions, "Ready").Reason)
}

func TestHostSerialConsoleUnsupportedProvisionerDoesNotAcquireFinalizer(t *testing.T) {
	r, _, req := newConsoleTest(t)
	factory, ok := r.ProvisionerFactory.(*consoleFactory)
	require.True(t, ok)
	factory.prov = newMockProvisioner() // Implements Provisioner, not the optional console capability.
	_, err := r.Reconcile(t.Context(), req)
	require.NoError(t, err)
	console := consoleObject(t, r, req)
	require.Equal(t, "ConsoleUnsupported", meta.FindStatusCondition(console.Status.Conditions, "Ready").Reason)
	require.NotContains(t, console.Finalizers, metal3api.HostSerialConsoleFinalizer)
	require.Empty(t, console.Status.ProvisionerID)
	require.Nil(t, console.Status.HostDeprovisioningGeneration)
	require.NoError(t, r.Delete(t.Context(), console))
	_, err = r.Reconcile(t.Context(), req)
	require.NoError(t, err)
	require.True(t, k8serrors.IsNotFound(r.Get(t.Context(), req.NamespacedName, &metal3api.HostSerialConsole{})))
	require.Len(t, factory.data, 1, "deletion must not schedule unsupported backend cleanup")
}

func TestHostSerialConsoleBackendCapabilityChangeRetainsCleanup(t *testing.T) {
	r, p, req := newConsoleTest(t)
	p.state.Enabled = true
	_, err := r.Reconcile(t.Context(), req)
	require.NoError(t, err)
	console := consoleObject(t, r, req)
	factory, ok := r.ProvisionerFactory.(*consoleFactory)
	require.True(t, ok)
	factory.prov = newMockProvisioner()
	require.NoError(t, r.Delete(t.Context(), console))
	_, err = r.Reconcile(t.Context(), req)
	require.NoError(t, err)
	console = consoleObject(t, r, req)
	require.Contains(t, console.Finalizers, metal3api.HostSerialConsoleFinalizer)
	require.Contains(t, meta.FindStatusCondition(console.Status.Conditions, "Ready").Message, "previously managed console")
}

func TestHostSerialConsoleRoutingOwnershipErrorStillDisablesBackend(t *testing.T) {
	for _, resource := range []string{"Service", "EndpointSlice"} {
		t.Run(resource, func(t *testing.T) {
			r, p, req := newConsoleTest(t)
			p.state.Enabled = true
			_, err := r.Reconcile(t.Context(), req)
			require.NoError(t, err)
			console := consoleObject(t, r, req)
			key := types.NamespacedName{Namespace: req.Namespace, Name: console.Status.ServiceName}
			var unowned, other client.Object = &corev1.Service{}, &discoveryv1.EndpointSlice{}
			if resource == "EndpointSlice" {
				unowned, other = other, unowned
			}
			require.NoError(t, r.Get(t.Context(), key, unowned))
			unowned.SetOwnerReferences(nil)
			require.NoError(t, r.Update(t.Context(), unowned))
			require.NoError(t, r.Delete(t.Context(), console))
			_, err = r.Reconcile(t.Context(), req)
			require.ErrorContains(t, err, "refusing to delete unowned")
			require.Equal(t, []bool{false}, p.serialConsoleCalls)
			require.NoError(t, r.Get(t.Context(), key, unowned), "do not delete a foreign routing object")
			require.True(t, k8serrors.IsNotFound(r.Get(t.Context(), key, other)), "still withdraw the routing object we own")
			require.Contains(t, consoleObject(t, r, req).Finalizers, metal3api.HostSerialConsoleFinalizer)

			p.state.Enabled = false
			_, err = r.Reconcile(t.Context(), req)
			require.ErrorContains(t, err, "refusing to delete unowned")
			require.Contains(t, consoleObject(t, r, req).Finalizers, metal3api.HostSerialConsoleFinalizer)
			// An administrator resolves the ownership conflict. Only now may the
			// request finalize; backend disable was not delayed by that conflict.
			require.NoError(t, r.Delete(t.Context(), unowned))
			_, err = r.Reconcile(t.Context(), req)
			require.NoError(t, err)
			require.True(t, k8serrors.IsNotFound(r.Get(t.Context(), req.NamespacedName, &metal3api.HostSerialConsole{})))
		})
	}
}

func TestHostSerialConsoleMissedDeprovisioningExpiresRequest(t *testing.T) {
	r, p, req := newConsoleTest(t)
	p.state.Enabled = true
	_, err := r.Reconcile(t.Context(), req)
	require.NoError(t, err)
	console := consoleObject(t, r, req)
	require.NotNil(t, console.Status.HostDeprovisioningGeneration)
	require.Zero(t, *console.Status.HostDeprovisioningGeneration)
	host := &metal3api.BareMetalHost{}
	require.NoError(t, r.Get(t.Context(), req.NamespacedName, host))
	hsm := newHostStateMachine(host, testNewReconciler(host), p, true)
	info := &reconcileInfo{host: host, log: ctrl.Log.WithName("console-lifecycle-test")}
	hsm.NextState = metal3api.StateDeprovisioning
	require.Nil(t, hsm.updateHostStateFrom(t.Context(), metal3api.StateProvisioned, info))
	require.Equal(t, int64(1), host.Status.Provisioning.DeprovisioningGeneration)
	require.NoError(t, r.Status().Update(t.Context(), host))
	hsm.NextState = metal3api.StateAvailable
	require.Nil(t, hsm.updateHostStateFrom(t.Context(), metal3api.StateDeprovisioning, info))
	require.NoError(t, r.Status().Update(t.Context(), host))
	// The console controller never observed deprovisioning. BMH UID and
	// Ironic node ID are unchanged, but the durable workload boundary expired it.
	_, err = r.Reconcile(t.Context(), req)
	require.NoError(t, err)
	require.False(t, consoleObject(t, r, req).DeletionTimestamp.IsZero())
	_, err = r.Reconcile(t.Context(), req)
	require.NoError(t, err)
	require.Equal(t, []bool{false}, p.serialConsoleCalls)
	p.state.Enabled = false
	_, err = r.Reconcile(t.Context(), req)
	require.NoError(t, err)
	require.True(t, k8serrors.IsNotFound(r.Get(t.Context(), req.NamespacedName, &metal3api.HostSerialConsole{})))

	// A new explicit request may bind to the next workload generation.
	replacement := &metal3api.HostSerialConsole{ObjectMeta: metav1.ObjectMeta{Name: req.Name, Namespace: req.Namespace, UID: "next-console-uid"}}
	require.NoError(t, r.Create(t.Context(), replacement))
	_, err = r.Reconcile(t.Context(), req)
	require.NoError(t, err)
	replacement = consoleObject(t, r, req)
	require.NotNil(t, replacement.Status.HostDeprovisioningGeneration)
	require.Equal(t, int64(1), *replacement.Status.HostDeprovisioningGeneration)
}

func TestHostSerialConsoleObservationFailurePreservesRouting(t *testing.T) {
	for _, failure := range []string{"factory", "node-state", "DNS"} {
		t.Run(failure, func(t *testing.T) {
			r, p, req := newConsoleTest(t)
			p.state.Enabled = true
			_, err := r.Reconcile(t.Context(), req)
			require.NoError(t, err)
			before := consoleObject(t, r, req)
			key := types.NamespacedName{Name: before.Status.ServiceName, Namespace: req.Namespace}
			service := &corev1.Service{}
			require.NoError(t, r.Get(t.Context(), key, service))
			service.Spec.ClusterIP = "10.0.0.20"
			require.NoError(t, r.Update(t.Context(), service))
			factory, ok := r.ProvisionerFactory.(*consoleFactory)
			require.True(t, ok)
			switch failure {
			case "factory":
				factory.err = provisioner.ErrNotReady
			case "node-state":
				p.stateErr = errors.New("temporary Ironic API outage")
			case "DNS":
				r.LookupIPAddr = func(context.Context, string) ([]net.IPAddr, error) { return nil, errors.New("temporary DNS outage") }
			}
			_, err = r.Reconcile(t.Context(), req)
			require.NoError(t, err)
			after := consoleObject(t, r, req)
			require.Equal(t, before.Status.ConsoleURL, after.Status.ConsoleURL)
			require.Equal(t, metav1.ConditionUnknown, meta.FindStatusCondition(after.Status.Conditions, "Ready").Status)
			persisted := &corev1.Service{}
			require.NoError(t, r.Get(t.Context(), key, persisted))
			require.Equal(t, service.Spec, persisted.Spec)
			require.NoError(t, r.Get(t.Context(), key, &discoveryv1.EndpointSlice{}))
			factory.err, p.stateErr, r.LookupIPAddr = nil, nil, nil
			_, err = r.Reconcile(t.Context(), req)
			require.NoError(t, err)
			require.True(t, meta.IsStatusConditionTrue(consoleObject(t, r, req).Status.Conditions, "Ready"))
			require.Equal(t, 1, p.endpointCalls, "recovery must not call console show again")
		})
	}
}

func TestHostSerialConsoleConfirmedMissingNodeWithdrawsRouting(t *testing.T) {
	r, p, req := newConsoleTest(t)
	p.state.Enabled = true
	_, err := r.Reconcile(t.Context(), req)
	require.NoError(t, err)
	before := consoleObject(t, r, req)
	p.stateErr = provisioner.ErrNeedsRegistration
	_, err = r.Reconcile(t.Context(), req)
	require.NoError(t, err)
	after := consoleObject(t, r, req)
	require.Empty(t, after.Status.ConsoleURL)
	require.Nil(t, after.Status.BackendEndpoint)
	key := types.NamespacedName{Name: before.Status.ServiceName, Namespace: req.Namespace}
	require.True(t, k8serrors.IsNotFound(r.Get(t.Context(), key, &corev1.Service{})))
	require.True(t, k8serrors.IsNotFound(r.Get(t.Context(), key, &discoveryv1.EndpointSlice{})))
}

func TestHostSerialConsolePausePreservesEndpointAndAllowsDeletion(t *testing.T) {
	r, p, req := newConsoleTest(t)
	p.state.Enabled = true
	_, err := r.Reconcile(t.Context(), req)
	require.NoError(t, err)
	before := consoleObject(t, r, req)
	host := &metal3api.BareMetalHost{}
	require.NoError(t, r.Get(t.Context(), req.NamespacedName, host))
	host.Annotations = map[string]string{metal3api.PausedAnnotation: ""}
	require.NoError(t, r.Update(t.Context(), host))
	factory, ok := r.ProvisionerFactory.(*consoleFactory)
	require.True(t, ok)
	previousCalls := len(factory.data)
	_, err = r.Reconcile(t.Context(), req)
	require.NoError(t, err)
	after := consoleObject(t, r, req)
	require.Equal(t, before.Status.ConsoleURL, after.Status.ConsoleURL)
	require.Equal(t, before.Status.BackendEndpoint, after.Status.BackendEndpoint)
	require.Equal(t, "Paused", meta.FindStatusCondition(after.Status.Conditions, "Ready").Reason)
	require.Equal(t, metav1.ConditionUnknown, meta.FindStatusCondition(after.Status.Conditions, "Ready").Status)
	require.Len(t, factory.data, previousCalls, "paused reconciliation must not access the backend")
	require.NoError(t, r.Delete(t.Context(), after))
	_, err = r.Reconcile(t.Context(), req)
	require.NoError(t, err)
	require.Equal(t, []bool{false}, p.serialConsoleCalls)
	p.state.Enabled = false
	_, err = r.Reconcile(t.Context(), req)
	require.NoError(t, err)
	require.True(t, k8serrors.IsNotFound(r.Get(t.Context(), req.NamespacedName, &metal3api.HostSerialConsole{})))
}
