package controllers

import (
	goctx "context"
	"testing"

	"k8s.io/apimachinery/pkg/runtime"

	ctrl "sigs.k8s.io/controller-runtime"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metal3v1alpha1 "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner/demo"
)

func newDemoReconciler(initObjs ...runtime.Object) *BareMetalHostReconciler {

	c := fakeclient.NewFakeClient(initObjs...)

	// Add a default secret that can be used by most hosts.
	bmcSecret := newSecret(defaultSecretName, map[string]string{"username": "User", "password": "Pass"})
	c.Create(goctx.TODO(), bmcSecret)

	return &BareMetalHostReconciler{
		Client:             c,
		ProvisionerFactory: &demo.Demo{},
		Log:                ctrl.Log.WithName("controller").WithName("BareMetalHost"),
	}
}

// TestDemoRegistrationError tests that a host with the right name reports
// a registration error
func TestDemoRegistrationError(t *testing.T) {
	host := newDefaultNamedHost(demo.RegistrationErrorHost, t)
	r := newDemoReconciler(host)

	tryReconcile(t, r, host,
		func(host *metal3v1alpha1.BareMetalHost, result reconcile.Result) bool {
			t.Logf("Status: %q State: %q ErrorMessage: %q",
				host.OperationalStatus(),
				host.Status.Provisioning.State,
				host.Status.ErrorMessage,
			)
			return host.Status.ErrorMessage != ""
		},
	)
}

// TestDemoRegistering tests that a host with the right name reports
// that it is being registered
func TestDemoRegistering(t *testing.T) {
	host := newDefaultNamedHost(demo.RegisteringHost, t)
	r := newDemoReconciler(host)

	tryReconcile(t, r, host,
		func(host *metal3v1alpha1.BareMetalHost, result reconcile.Result) bool {
			t.Logf("Status: %q State: %q ErrorMessage: %q",
				host.OperationalStatus(),
				host.Status.Provisioning.State,
				host.Status.ErrorMessage,
			)
			return host.Status.Provisioning.State == metal3v1alpha1.StateRegistering
		},
	)
}

// TestDemoInspecting tests that a host with the right name reports
// that it is being inspected
func TestDemoInspecting(t *testing.T) {
	host := newDefaultNamedHost(demo.InspectingHost, t)
	r := newDemoReconciler(host)

	tryReconcile(t, r, host,
		func(host *metal3v1alpha1.BareMetalHost, result reconcile.Result) bool {
			t.Logf("Status: %q State: %q ErrorMessage: %q",
				host.OperationalStatus(),
				host.Status.Provisioning.State,
				host.Status.ErrorMessage,
			)
			return host.Status.Provisioning.State == metal3v1alpha1.StateInspecting
		},
	)
}

func TestDemoPreparing(t *testing.T) {
	host := newDefaultNamedHost(demo.PreparingHost, t)
	host.Spec.Image = &metal3v1alpha1.Image{
		URL:      "a-url",
		Checksum: "a-checksum",
	}
	host.Spec.Online = true
	r := newDemoReconciler(host)

	tryReconcile(t, r, host,
		func(host *metal3v1alpha1.BareMetalHost, result reconcile.Result) bool {
			t.Logf("Status: %q State: %q ErrorMessage: %q",
				host.OperationalStatus(),
				host.Status.Provisioning.State,
				host.Status.ErrorMessage,
			)
			return host.Status.Provisioning.State == metal3v1alpha1.StatePreparing
		},
	)
}

func TestDemoPreparingError(t *testing.T) {
	host := newDefaultNamedHost(demo.PreparingErrorHost, t)
	host.Spec.Image = &metal3v1alpha1.Image{
		URL:      "a-url",
		Checksum: "a-checksum",
	}
	host.Spec.Online = true
	r := newDemoReconciler(host)

	tryReconcile(t, r, host,
		func(host *metal3v1alpha1.BareMetalHost, result reconcile.Result) bool {
			t.Logf("Status: %q State: %q ErrorMessage: %q",
				host.OperationalStatus(),
				host.Status.Provisioning.State,
				host.Status.ErrorMessage,
			)
			return host.Status.Provisioning.State == metal3v1alpha1.StatePreparing
		},
	)
}

// TestDemoAvailable tests that a host with the right name reports
// that it is available to be provisioned
func TestDemoAvailable(t *testing.T) {
	host := newDefaultNamedHost(demo.AvailableHost, t)
	r := newDemoReconciler(host)

	tryReconcile(t, r, host,
		func(host *metal3v1alpha1.BareMetalHost, result reconcile.Result) bool {
			t.Logf("Status: %q State: %q ErrorMessage: %q",
				host.OperationalStatus(),
				host.Status.Provisioning.State,
				host.Status.ErrorMessage,
			)
			return host.Status.Provisioning.State == metal3v1alpha1.StateAvailable
		},
	)
}

// TestDemoProvisioning tests that a host with the right name reports
// that it is being provisioned
func TestDemoProvisioning(t *testing.T) {
	host := newDefaultNamedHost(demo.ProvisioningHost, t)
	host.Spec.Image = &metal3v1alpha1.Image{
		URL:      "a-url",
		Checksum: "a-checksum",
	}
	host.Spec.Online = true
	r := newDemoReconciler(host)

	tryReconcile(t, r, host,
		func(host *metal3v1alpha1.BareMetalHost, result reconcile.Result) bool {
			t.Logf("Status: %q State: %q ErrorMessage: %q",
				host.OperationalStatus(),
				host.Status.Provisioning.State,
				host.Status.ErrorMessage,
			)
			return host.Status.Provisioning.State == metal3v1alpha1.StateProvisioning
		},
	)
}

// TestDemoProvisioned tests that a host with the right name reports
// that it has been provisioned
func TestDemoProvisioned(t *testing.T) {
	host := newDefaultNamedHost(demo.ProvisionedHost, t)
	host.Spec.Image = &metal3v1alpha1.Image{
		URL:      "a-url",
		Checksum: "a-checksum",
	}
	host.Spec.Online = true
	r := newDemoReconciler(host)

	tryReconcile(t, r, host,
		func(host *metal3v1alpha1.BareMetalHost, result reconcile.Result) bool {
			t.Logf("Status: %q State: %q ErrorMessage: %q",
				host.OperationalStatus(),
				host.Status.Provisioning.State,
				host.Status.ErrorMessage,
			)
			return host.Status.Provisioning.State == metal3v1alpha1.StateProvisioned
		},
	)
}

// TestDemoValidationError tests that a host with the right name
// reports that it had and error while being provisioned
func TestDemoValidationError(t *testing.T) {
	host := newDefaultNamedHost(demo.ValidationErrorHost, t)
	host.Spec.Image = &metal3v1alpha1.Image{
		URL:      "a-url",
		Checksum: "a-checksum",
	}
	host.Spec.Online = true
	r := newDemoReconciler(host)

	tryReconcile(t, r, host,
		func(host *metal3v1alpha1.BareMetalHost, result reconcile.Result) bool {
			t.Logf("Status: %q State: %q ErrorMessage: %q",
				host.OperationalStatus(),
				host.Status.Provisioning.State,
				host.Status.ErrorMessage,
			)
			return host.Status.ErrorMessage != ""
		},
	)
}
