package baremetalhost

import (
	goctx "context"
	"testing"

	"k8s.io/apimachinery/pkg/runtime"

	"k8s.io/client-go/kubernetes/scheme"

	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"

	metalkubeapis "github.com/metal3-io/baremetal-operator/pkg/apis"
	metalkubev1alpha1 "github.com/metal3-io/baremetal-operator/pkg/apis/metalkube/v1alpha1"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner/demo"
)

func init() {
	logf.SetLogger(logf.ZapLogger(true))
	// Register our package types with the global scheme
	metalkubeapis.AddToScheme(scheme.Scheme)
}

func newDemoReconciler(initObjs ...runtime.Object) *ReconcileBareMetalHost {

	c := fakeclient.NewFakeClient(initObjs...)

	// Add a default secret that can be used by most hosts.
	c.Create(goctx.TODO(), newSecret(defaultSecretName, "User", "Pass"))

	return &ReconcileBareMetalHost{
		client:             c,
		scheme:             scheme.Scheme,
		provisionerFactory: demo.New,
	}
}

// TestDemoRegistrationError tests that a host with the right name reports
// a registration error
func TestDemoRegistrationError(t *testing.T) {
	host := newDefaultNamedHost(demo.RegistrationErrorHost, t)
	r := newDemoReconciler(host)

	tryReconcile(t, r, host,
		func(host *metalkubev1alpha1.BareMetalHost, result reconcile.Result) bool {
			t.Logf("Status: %q State: %q ErrorMessage: %q",
				host.OperationalStatus(),
				host.Status.Provisioning.State,
				host.Status.ErrorMessage,
			)
			return host.HasError()
		},
	)
}

// TestDemoRegistering tests that a host with the right name reports
// that it is being registered
func TestDemoRegistering(t *testing.T) {
	host := newDefaultNamedHost(demo.RegisteringHost, t)
	r := newDemoReconciler(host)

	tryReconcile(t, r, host,
		func(host *metalkubev1alpha1.BareMetalHost, result reconcile.Result) bool {
			t.Logf("Status: %q State: %q ErrorMessage: %q",
				host.OperationalStatus(),
				host.Status.Provisioning.State,
				host.Status.ErrorMessage,
			)
			return host.Status.Provisioning.State == metalkubev1alpha1.StateRegistering
		},
	)
}

// TestDemoInspecting tests that a host with the right name reports
// that it is being inspected
func TestDemoInspecting(t *testing.T) {
	host := newDefaultNamedHost(demo.InspectingHost, t)
	r := newDemoReconciler(host)

	tryReconcile(t, r, host,
		func(host *metalkubev1alpha1.BareMetalHost, result reconcile.Result) bool {
			t.Logf("Status: %q State: %q ErrorMessage: %q",
				host.OperationalStatus(),
				host.Status.Provisioning.State,
				host.Status.ErrorMessage,
			)
			return host.Status.Provisioning.State == metalkubev1alpha1.StateInspecting
		},
	)
}

// TestDemoReady tests that a host with the right name reports
// that it is ready to be provisioned
func TestDemoReady(t *testing.T) {
	host := newDefaultNamedHost(demo.ReadyHost, t)
	r := newDemoReconciler(host)

	tryReconcile(t, r, host,
		func(host *metalkubev1alpha1.BareMetalHost, result reconcile.Result) bool {
			t.Logf("Status: %q State: %q ErrorMessage: %q",
				host.OperationalStatus(),
				host.Status.Provisioning.State,
				host.Status.ErrorMessage,
			)
			return host.Status.Provisioning.State == metalkubev1alpha1.StateReady
		},
	)
}

// TestDemoProvisioning tests that a host with the right name reports
// that it is being provisioned
func TestDemoProvisioning(t *testing.T) {
	host := newDefaultNamedHost(demo.ProvisioningHost, t)
	host.Spec.Image = &metalkubev1alpha1.Image{
		URL:      "a-url",
		Checksum: "a-checksum",
	}
	host.Spec.Online = true
	r := newDemoReconciler(host)

	tryReconcile(t, r, host,
		func(host *metalkubev1alpha1.BareMetalHost, result reconcile.Result) bool {
			t.Logf("Status: %q State: %q ErrorMessage: %q",
				host.OperationalStatus(),
				host.Status.Provisioning.State,
				host.Status.ErrorMessage,
			)
			return host.Status.Provisioning.State == metalkubev1alpha1.StateProvisioning
		},
	)
}

// TestDemoProvisioned tests that a host with the right name reports
// that it has been provisioned
func TestDemoProvisioned(t *testing.T) {
	host := newDefaultNamedHost(demo.ProvisionedHost, t)
	host.Spec.Image = &metalkubev1alpha1.Image{
		URL:      "a-url",
		Checksum: "a-checksum",
	}
	host.Spec.Online = true
	r := newDemoReconciler(host)

	tryReconcile(t, r, host,
		func(host *metalkubev1alpha1.BareMetalHost, result reconcile.Result) bool {
			t.Logf("Status: %q State: %q ErrorMessage: %q",
				host.OperationalStatus(),
				host.Status.Provisioning.State,
				host.Status.ErrorMessage,
			)
			return host.Status.Provisioning.State == metalkubev1alpha1.StateProvisioned
		},
	)
}

// TestDemoValidationError tests that a host with the right name
// reports that it had and error while being provisioned
func TestDemoValidationError(t *testing.T) {
	host := newDefaultNamedHost(demo.ValidationErrorHost, t)
	host.Spec.Image = &metalkubev1alpha1.Image{
		URL:      "a-url",
		Checksum: "a-checksum",
	}
	host.Spec.Online = true
	r := newDemoReconciler(host)

	tryReconcile(t, r, host,
		func(host *metalkubev1alpha1.BareMetalHost, result reconcile.Result) bool {
			t.Logf("Status: %q State: %q ErrorMessage: %q",
				host.OperationalStatus(),
				host.Status.Provisioning.State,
				host.Status.ErrorMessage,
			)
			return host.HasError()
		},
	)
}
