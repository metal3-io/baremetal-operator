package controllers

import (
	goctx "context"
	"testing"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"k8s.io/apimachinery/pkg/runtime"

	ctrl "sigs.k8s.io/controller-runtime"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner/demo"
)

func newDemoReconciler(initObjs ...runtime.Object) *BareMetalHostReconciler {
	clientBuilder := fakeclient.NewClientBuilder().WithRuntimeObjects(initObjs...)
	for _, v := range initObjs {
		clientBuilder = clientBuilder.WithStatusSubresource(v.(client.Object))
	}
	c := clientBuilder.Build()

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
// a registration error.
func TestDemoRegistrationError(t *testing.T) {
	host := newDefaultNamedHost(t, demo.RegistrationErrorHost)
	r := newDemoReconciler(host)

	tryReconcile(t, r, host,
		func(host *metal3api.BareMetalHost, result reconcile.Result) bool {
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
// that it is being registered.
func TestDemoRegistering(t *testing.T) {
	host := newDefaultNamedHost(t, demo.RegisteringHost)
	r := newDemoReconciler(host)

	tryReconcile(t, r, host,
		func(host *metal3api.BareMetalHost, result reconcile.Result) bool {
			t.Logf("Status: %q State: %q ErrorMessage: %q",
				host.OperationalStatus(),
				host.Status.Provisioning.State,
				host.Status.ErrorMessage,
			)
			return host.Status.Provisioning.State == metal3api.StateRegistering
		},
	)
}

// TestDemoInspecting tests that a host with the right name reports
// that it is being inspected.
func TestDemoInspecting(t *testing.T) {
	host := newDefaultNamedHost(t, demo.InspectingHost)
	r := newDemoReconciler(host)

	tryReconcile(t, r, host,
		func(host *metal3api.BareMetalHost, result reconcile.Result) bool {
			t.Logf("Status: %q State: %q ErrorMessage: %q",
				host.OperationalStatus(),
				host.Status.Provisioning.State,
				host.Status.ErrorMessage,
			)
			return host.Status.Provisioning.State == metal3api.StateInspecting
		},
	)
}

func TestDemoPreparing(t *testing.T) {
	host := newDefaultNamedHost(t, demo.PreparingHost)
	host.Spec.Image = &metal3api.Image{
		URL:      "a-url",
		Checksum: "a-checksum",
	}
	host.Spec.Online = true
	r := newDemoReconciler(host)

	tryReconcile(t, r, host,
		func(host *metal3api.BareMetalHost, result reconcile.Result) bool {
			t.Logf("Status: %q State: %q ErrorMessage: %q",
				host.OperationalStatus(),
				host.Status.Provisioning.State,
				host.Status.ErrorMessage,
			)
			return host.Status.Provisioning.State == metal3api.StatePreparing
		},
	)
}

func TestDemoPreparingError(t *testing.T) {
	host := newDefaultNamedHost(t, demo.PreparingErrorHost)
	host.Spec.Image = &metal3api.Image{
		URL:      "a-url",
		Checksum: "a-checksum",
	}
	host.Spec.Online = true
	r := newDemoReconciler(host)

	tryReconcile(t, r, host,
		func(host *metal3api.BareMetalHost, result reconcile.Result) bool {
			t.Logf("Status: %q State: %q ErrorMessage: %q",
				host.OperationalStatus(),
				host.Status.Provisioning.State,
				host.Status.ErrorMessage,
			)
			return host.Status.Provisioning.State == metal3api.StatePreparing
		},
	)
}

// TestDemoAvailable tests that a host with the right name reports
// that it is available to be provisioned.
func TestDemoAvailable(t *testing.T) {
	host := newDefaultNamedHost(t, demo.AvailableHost)
	r := newDemoReconciler(host)

	tryReconcile(t, r, host,
		func(host *metal3api.BareMetalHost, result reconcile.Result) bool {
			t.Logf("Status: %q State: %q ErrorMessage: %q",
				host.OperationalStatus(),
				host.Status.Provisioning.State,
				host.Status.ErrorMessage,
			)
			return host.Status.Provisioning.State == metal3api.StateAvailable
		},
	)
}

// TestDemoProvisioning tests that a host with the right name reports
// that it is being provisioned.
func TestDemoProvisioning(t *testing.T) {
	host := newDefaultNamedHost(t, demo.ProvisioningHost)
	host.Spec.Image = &metal3api.Image{
		URL:      "a-url",
		Checksum: "a-checksum",
	}
	host.Spec.Online = true
	r := newDemoReconciler(host)

	tryReconcile(t, r, host,
		func(host *metal3api.BareMetalHost, result reconcile.Result) bool {
			t.Logf("Status: %q State: %q ErrorMessage: %q",
				host.OperationalStatus(),
				host.Status.Provisioning.State,
				host.Status.ErrorMessage,
			)
			return host.Status.Provisioning.State == metal3api.StateProvisioning
		},
	)
}

// TestDemoProvisioned tests that a host with the right name reports
// that it has been provisioned.
func TestDemoProvisioned(t *testing.T) {
	host := newDefaultNamedHost(t, demo.ProvisionedHost)
	host.Spec.Image = &metal3api.Image{
		URL:      "a-url",
		Checksum: "a-checksum",
	}
	host.Spec.Online = true
	r := newDemoReconciler(host)

	tryReconcile(t, r, host,
		func(host *metal3api.BareMetalHost, result reconcile.Result) bool {
			t.Logf("Status: %q State: %q ErrorMessage: %q",
				host.OperationalStatus(),
				host.Status.Provisioning.State,
				host.Status.ErrorMessage,
			)
			return host.Status.Provisioning.State == metal3api.StateProvisioned
		},
	)
}

// TestDemoValidationError tests that a host with the right name
// reports that it had and error while being provisioned.
func TestDemoValidationError(t *testing.T) {
	host := newDefaultNamedHost(t, demo.ValidationErrorHost)
	host.Spec.Image = &metal3api.Image{
		URL:      "a-url",
		Checksum: "a-checksum",
	}
	host.Spec.Online = true
	r := newDemoReconciler(host)

	tryReconcile(t, r, host,
		func(host *metal3api.BareMetalHost, result reconcile.Result) bool {
			t.Logf("Status: %q State: %q ErrorMessage: %q",
				host.OperationalStatus(),
				host.Status.Provisioning.State,
				host.Status.ErrorMessage,
			)
			return host.Status.ErrorMessage != ""
		},
	)
}
