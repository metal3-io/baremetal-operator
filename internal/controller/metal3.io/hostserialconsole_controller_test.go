package controllers

import (
	"context"
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
