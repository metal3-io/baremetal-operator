//go:build integration

package controllers

import (
	"context"
	"net"
	"path/filepath"
	"testing"
	"time"

	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

// Fake-client tests cannot check API defaulting or schema/metadata validation.
// This test exercises the actual resources produced by the reconciler without
// a running cluster, Ironic, a garbage collector, or a network connection to a BMC.
func TestHostSerialConsoleAPIServer(t *testing.T) {
	for _, tc := range []struct {
		family      corev1.IPFamily
		serviceCIDR string
	}{
		{family: corev1.IPv4Protocol, serviceCIDR: "10.96.0.0/24"},
		{family: corev1.IPv6Protocol, serviceCIDR: "fd00:96::/108"},
	} {
		t.Run(string(tc.family), func(t *testing.T) { testConsoleAPIServer(t, tc.family, tc.serviceCIDR) })
	}
}

func testConsoleAPIServer(t *testing.T, family corev1.IPFamily, serviceCIDR string) {
	t.Helper()
	crds := filepath.Join("..", "..", "..", "config", "base", "crds", "bases")
	environment := &envtest.Environment{
		UseExistingCluster: ptr.To(false),
		CRDDirectoryPaths: []string{
			filepath.Join(crds, "metal3.io_baremetalhosts.yaml"),
			filepath.Join(crds, "metal3.io_hostserialconsoles.yaml"),
		},
		ErrorIfCRDPathMissing:    true,
		ControlPlaneStartTimeout: time.Minute,
	}
	environment.ControlPlane.GetAPIServer().Configure().Set("service-cluster-ip-range", serviceCIDR)
	if family == corev1.IPv6Protocol {
		environment.ControlPlane.GetAPIServer().Configure().Set("advertise-address", "::1")
	}
	config, err := environment.Start()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, environment.Stop()) })
	scheme := runtime.NewScheme()
	require.NoError(t, metal3api.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, discoveryv1.AddToScheme(scheme))
	api, err := client.New(config, client.Options{Scheme: scheme})
	require.NoError(t, err)
	ctx := t.Context()
	require.NoError(t, api.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "console-test"}}))
	host := &metal3api.BareMetalHost{ObjectMeta: metav1.ObjectMeta{Name: "worker-0", Namespace: "console-test"}}
	host.Spec.BMC.Address = "ipmi://192.0.2.20"
	host.Spec.BMC.CredentialsName = "bmc-secret"
	require.NoError(t, api.Create(ctx, host))
	host.Status.Provisioning.State = metal3api.StateProvisioned
	host.Status.Provisioning.ID = "node-id"
	host.Status.OperationalStatus = metal3api.OperationalStatusOK
	require.NoError(t, api.Status().Update(ctx, host))
	console := &metal3api.HostSerialConsole{ObjectMeta: metav1.ObjectMeta{Name: host.Name, Namespace: host.Namespace}}
	require.NoError(t, api.Create(ctx, console))
	p := &consoleMock{mockProvisioner: newMockProvisioner(), state: provisioner.SerialConsoleState{Enabled: true, ID: "node-id/conductor/3023"},
		serialConsole: &metal3api.SerialConsoleEndpoint{Host: "conductor.example", Port: 3023}}
	addresses := []net.IPAddr{{IP: net.ParseIP("2001:db8::10")}, {IP: net.ParseIP("192.0.2.10")},
		{IP: net.ParseIP("2001:db8::11")}, {IP: net.ParseIP("192.0.2.11")}}
	r := &HostSerialConsoleReconciler{Client: api, APIReader: api, ProvisionerFactory: &consoleFactory{prov: p},
		LookupIPAddr: func(context.Context, string) ([]net.IPAddr, error) {
			return addresses, nil
		}}
	req := ctrl.Request{NamespacedName: client.ObjectKeyFromObject(console)}
	_, err = r.Reconcile(ctx, req)
	require.NoError(t, err)
	require.NoError(t, api.Get(ctx, req.NamespacedName, console))
	require.True(t, meta.IsStatusConditionTrue(console.Status.Conditions, "Ready"), "%+v", console.Status.Conditions)
	require.NotNil(t, console.Status.HostDeprovisioningGeneration)
	require.Zero(t, *console.Status.HostDeprovisioningGeneration)
	key := types.NamespacedName{Name: console.Status.ServiceName, Namespace: console.Namespace}
	service := &corev1.Service{}
	require.NoError(t, api.Get(ctx, key, service))
	require.NotEmpty(t, service.Spec.ClusterIP)
	require.Equal(t, []corev1.IPFamily{family}, service.Spec.IPFamilies)
	slice := &discoveryv1.EndpointSlice{}
	require.NoError(t, api.Get(ctx, key, slice))
	require.Equal(t, discoveryv1.AddressType(family), slice.AddressType)
	address := "192.0.2.10"
	if family == corev1.IPv6Protocol {
		address = "2001:db8::10"
	}
	require.Equal(t, []string{address}, slice.Endpoints[0].Addresses)
	require.Equal(t, int32(3023), *slice.Ports[0].Port)
	// Changing DNS answer order must not replace or update an established
	// Service, nor call the disruptive endpoint API again.
	previousIP, previousVersion := service.Spec.ClusterIP, service.ResourceVersion
	previousSliceVersion := slice.ResourceVersion
	addresses[0], addresses[2] = addresses[2], addresses[0]
	addresses[1], addresses[3] = addresses[3], addresses[1]
	_, err = r.Reconcile(ctx, req)
	require.NoError(t, err)
	require.NoError(t, api.Get(ctx, key, service))
	require.Equal(t, previousIP, service.Spec.ClusterIP)
	require.Equal(t, previousVersion, service.ResourceVersion)
	require.NoError(t, api.Get(ctx, key, slice))
	require.Equal(t, previousSliceVersion, slice.ResourceVersion)
	require.Equal(t, []string{address}, slice.Endpoints[0].Addresses)
	require.Equal(t, 1, p.endpointCalls)
	invalid := slice.DeepCopy()
	invalid.Name, invalid.ResourceVersion, invalid.UID = "invalid-console", "", ""
	invalid.Labels[discoveryv1.LabelManagedBy] = "metal3.io/not-a-label-value"
	require.True(t, k8serrors.IsInvalid(api.Create(ctx, invalid)), "the API must reject the previously broken manager label")
	console.Status.BackendEndpoint.Port = 0
	require.True(t, k8serrors.IsInvalid(api.Status().Update(ctx, console)), "the CRD must reject invalid backend ports")
	require.NoError(t, api.Get(ctx, req.NamespacedName, console))
	require.NoError(t, api.Delete(ctx, console))
	_, err = r.Reconcile(ctx, req)
	require.NoError(t, err)
	require.NoError(t, api.Get(ctx, req.NamespacedName, console), "accepted disable is not confirmed cleanup")
	require.Equal(t, []bool{false}, p.serialConsoleCalls)
	p.state.Enabled = false
	_, err = r.Reconcile(ctx, req)
	require.NoError(t, err)
	require.True(t, k8serrors.IsNotFound(api.Get(ctx, req.NamespacedName, console)))
	require.True(t, k8serrors.IsNotFound(api.Get(ctx, key, service)))
	require.True(t, k8serrors.IsNotFound(api.Get(ctx, key, slice)))
}
