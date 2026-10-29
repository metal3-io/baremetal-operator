package webhooks

import (
	"errors"
	"testing"

	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestHostSerialConsoleAdmission(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, metal3api.AddToScheme(scheme))
	host := &metal3api.BareMetalHost{ObjectMeta: metav1.ObjectMeta{Name: "worker-0", Namespace: "metal3"}}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(host).Build()
	webhook := &HostSerialConsole{Client: c}
	console := &metal3api.HostSerialConsole{ObjectMeta: host.ObjectMeta}
	_, err := webhook.ValidateCreate(t.Context(), console)
	require.NoError(t, err)
	console.Namespace = "other-namespace"
	_, err = webhook.ValidateCreate(t.Context(), console)
	require.ErrorContains(t, err, "same namespace")
	// Closing access and finalizing requests must work after the host is gone.
	_, err = webhook.ValidateDelete(t.Context(), console)
	require.NoError(t, err)
	_, err = webhook.ValidateUpdate(t.Context(), console, console)
	require.NoError(t, err)
	_, err = webhook.ValidateCreate(t.Context(), nil)
	require.Error(t, err)
}

func TestHostSerialConsoleAdmissionDetached(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, metal3api.AddToScheme(scheme))
	host := &metal3api.BareMetalHost{ObjectMeta: metav1.ObjectMeta{Name: "worker-0", Namespace: "metal3", Annotations: map[string]string{metal3api.DetachedAnnotation: ""}}}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(host).Build()
	webhook := &HostSerialConsole{Client: c}
	_, err := webhook.ValidateCreate(t.Context(), &metal3api.HostSerialConsole{ObjectMeta: metav1.ObjectMeta{Name: host.Name, Namespace: host.Namespace}})
	require.ErrorContains(t, err, "detached")
}

func TestBMHSerialConsoleAddressValidation(t *testing.T) {
	webhook := &BareMetalHost{}
	for _, tc := range []struct{ address, management, wantError string }{
		{address: "ipmi://192.0.2.20:623?cipher_suite=3"},
		{},
		{address: "https://bmc.example", wantError: "must be an ipmi://"},
		{address: "ipmi://user:password@bmc.example", wantError: "without credentials"},
		{address: "ipmi://bmc.example:0", wantError: "invalid serial console IPMI port"},
		{address: "ipmi://192.0.2.20", management: "ipmi://bmc.example", wantError: "only supported with non-IPMI management"},
	} {
		t.Run(tc.address+tc.management, func(t *testing.T) {
			host := &metal3api.BareMetalHost{ObjectMeta: metav1.ObjectMeta{Name: "worker-0"}}
			host.Spec.BMC.Address = "redfish://bmc.example/redfish/v1/Systems/1"
			if tc.management != "" {
				host.Spec.BMC.Address = tc.management
			}
			host.Spec.BMC.SerialConsoleAddress = tc.address
			err := errors.Join(webhook.validateHost(host)...)
			if tc.wantError == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tc.wantError)
			}
		})
	}
}
