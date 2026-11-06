package controllers

import (
	"context"
	"errors"
	"testing"

	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	"github.com/metal3-io/baremetal-operator/pkg/hardwareutils/bmc"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner/fixture"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// Test support for HostFirmwareComponents in the HostFirmwareComponentsReconciler.
func getTestHFCReconciler(host *metal3api.HostFirmwareComponents) *HostFirmwareComponentsReconciler {
	c := fakeclient.NewClientBuilder().WithRuntimeObjects(host).WithStatusSubresource(host).Build()

	reconciler := &HostFirmwareComponentsReconciler{
		Client: c,
		Log:    ctrl.Log.WithName("test_reconciler").WithName("HostFirmwareComponents"),
	}

	return reconciler
}

func getMockHFCProvisioner(host *metal3api.BareMetalHost, components []metal3api.FirmwareComponentStatus) provisioner.Provisioner {
	state := fixture.Fixture{
		HostFirmwareComponents: fixture.HostFirmwareComponentsMock{
			Components: components,
		},
	}
	p, _ := state.NewProvisioner(context.TODO(), provisioner.BuildHostData(*host, bmc.Credentials{}),
		func(reason, message string) {})
	return p
}

// Mock components to return from provisioner.
func getCurrentComponents(updatedComponents string) []metal3api.FirmwareComponentStatus {
	var components []metal3api.FirmwareComponentStatus
	switch updatedComponents {
	case "bmc":
		components = []metal3api.FirmwareComponentStatus{
			{
				Component:          "bmc",
				InitialVersion:     "1.0.0",
				CurrentVersion:     "1.1.0",
				LastVersionFlashed: "1.1.0",
			},
			{
				Component:      "bios",
				InitialVersion: "1.0.1",
				CurrentVersion: "1.0.1",
			},
		}
	case "bios":
		components = []metal3api.FirmwareComponentStatus{
			{
				Component:      "bmc",
				InitialVersion: "1.0.0",
				CurrentVersion: "1.0.0",
			},
			{
				Component:          "bios",
				InitialVersion:     "1.0.1",
				CurrentVersion:     "1.1.10",
				LastVersionFlashed: "1.1.10",
			},
		}
	default:
		components = []metal3api.FirmwareComponentStatus{
			{
				Component:          "bmc",
				InitialVersion:     "1.0.0",
				CurrentVersion:     "1.1.0",
				LastVersionFlashed: "1.1.0",
			},
			{
				Component:          "bios",
				InitialVersion:     "1.0.1",
				CurrentVersion:     "1.1.10",
				LastVersionFlashed: "1.1.10",
			},
		}
	}

	return components
}

// Create the baremetalhost reconciler and use that to create bmh in same namespace.
func createBaremetalHostHFC() *metal3api.BareMetalHost {
	bmh := &metal3api.BareMetalHost{}
	bmh.ObjectMeta = metav1.ObjectMeta{Name: hostName, Namespace: hostNamespace}
	c := fakeclient.NewFakeClient(bmh)

	reconciler := &BareMetalHostReconciler{
		Client:             c,
		ProvisionerFactory: nil,
		Log:                ctrl.Log.WithName("bmh_reconciler").WithName("BareMetalHost"),
	}
	_ = reconciler.Create(context.TODO(), bmh)

	return bmh
}

// Create and HFC with input spec components.
func getHFC(spec metal3api.HostFirmwareComponentsSpec) *metal3api.HostFirmwareComponents {
	hfc := &metal3api.HostFirmwareComponents{}

	hfc.Status = metal3api.HostFirmwareComponentsStatus{
		Components: []metal3api.FirmwareComponentStatus{
			{
				Component:      "bmc",
				InitialVersion: "1.0.0",
				CurrentVersion: "1.0.0",
			},
			{
				Component:      "bios",
				InitialVersion: "1.0.1",
				CurrentVersion: "1.0.1",
			},
		},
	}

	hfc.TypeMeta = metav1.TypeMeta{
		Kind:       "HostFirmwareComponents",
		APIVersion: "metal3.io/v1alpha1"}
	hfc.ObjectMeta = metav1.ObjectMeta{
		Name:      hostName,
		Namespace: hostNamespace}

	hfc.Spec = spec
	return hfc
}

// Test the hostfirmwarecomponents reconciler functions.
func TestStoreHostFirmwareComponents(t *testing.T) {
	testCases := []struct {
		Scenario           string
		UpdatedComponents  string
		CurrentHFCResource *metal3api.HostFirmwareComponents
		ExpectedComponents *metal3api.HostFirmwareComponents
	}{
		{
			Scenario:          "update bmc",
			UpdatedComponents: "bmc",
			CurrentHFCResource: &metal3api.HostFirmwareComponents{
				TypeMeta: metav1.TypeMeta{
					Kind:       "HostFirmwareComponents",
					APIVersion: "metal3.io/v1alpha1"},
				ObjectMeta: metav1.ObjectMeta{
					Name:            "hostName",
					Namespace:       "hostNamespace",
					ResourceVersion: "1"},
				Spec: metal3api.HostFirmwareComponentsSpec{
					Updates: []metal3api.FirmwareUpdate{
						{
							Component: "bmc",
							URL:       "https://myurl/newbmcfirmware",
						},
					},
				},
				Status: metal3api.HostFirmwareComponentsStatus{
					Components: []metal3api.FirmwareComponentStatus{
						{
							Component:          "bmc",
							InitialVersion:     "1.0.0",
							CurrentVersion:     "1.1.0",
							LastVersionFlashed: "1.1.0",
						},
						{
							Component:      "bios",
							InitialVersion: "1.0.1",
							CurrentVersion: "1.0.1",
						},
					},
				},
			},
			ExpectedComponents: &metal3api.HostFirmwareComponents{
				Spec: metal3api.HostFirmwareComponentsSpec{
					Updates: []metal3api.FirmwareUpdate{
						{
							Component: "bmc",
							URL:       "https://myurl/newbmcfirmware",
						},
					},
				},
				Status: metal3api.HostFirmwareComponentsStatus{
					Updates: []metal3api.FirmwareUpdate{
						{
							Component: "bmc",
							URL:       "https://myurl/newbmcfirmware",
						},
					},
					Components: []metal3api.FirmwareComponentStatus{
						{
							Component:          "bmc",
							InitialVersion:     "1.0.0",
							CurrentVersion:     "1.1.0",
							LastVersionFlashed: "1.1.0",
						},
						{
							Component:      "bios",
							InitialVersion: "1.0.1",
							CurrentVersion: "1.0.1",
						},
					},
					Conditions: []metav1.Condition{
						{Type: "ChangeDetected", Status: "True", Reason: "OK"},
						{Type: "Valid", Status: "True", Reason: "OK"},
					},
				},
			},
		},
		{
			Scenario:          "update bios",
			UpdatedComponents: "bios",
			CurrentHFCResource: &metal3api.HostFirmwareComponents{
				TypeMeta: metav1.TypeMeta{
					Kind:       "HostFirmwareComponents",
					APIVersion: "metal3.io/v1alpha1"},
				ObjectMeta: metav1.ObjectMeta{
					Name:            "hostName",
					Namespace:       "hostNamespace",
					ResourceVersion: "1"},
				Spec: metal3api.HostFirmwareComponentsSpec{
					Updates: []metal3api.FirmwareUpdate{
						{
							Component: "bios",
							URL:       "https://myurl/newbiosfirmware",
						},
					},
				},
				Status: metal3api.HostFirmwareComponentsStatus{
					Components: []metal3api.FirmwareComponentStatus{
						{
							Component:      "bmc",
							InitialVersion: "1.0.0",
							CurrentVersion: "1.0.0",
						},
						{
							Component:          "bios",
							InitialVersion:     "1.0.1",
							CurrentVersion:     "1.1.10",
							LastVersionFlashed: "1.1.10",
						},
					},
				},
			},
			ExpectedComponents: &metal3api.HostFirmwareComponents{
				Spec: metal3api.HostFirmwareComponentsSpec{
					Updates: []metal3api.FirmwareUpdate{
						{
							Component: "bios",
							URL:       "https://myurl/newbiosfirmware",
						},
					},
				},
				Status: metal3api.HostFirmwareComponentsStatus{
					Updates: []metal3api.FirmwareUpdate{
						{
							Component: "bios",
							URL:       "https://myurl/newbiosfirmware",
						},
					},
					Components: []metal3api.FirmwareComponentStatus{
						{
							Component:      "bmc",
							InitialVersion: "1.0.0",
							CurrentVersion: "1.0.0",
						},
						{
							Component:          "bios",
							InitialVersion:     "1.0.1",
							CurrentVersion:     "1.1.10",
							LastVersionFlashed: "1.1.10",
						},
					},
					Conditions: []metav1.Condition{
						{Type: "ChangeDetected", Status: "True", Reason: "OK"},
						{Type: "Valid", Status: "True", Reason: "OK"},
					},
				},
			},
		},
		{
			Scenario:          "update all",
			UpdatedComponents: "all",
			CurrentHFCResource: &metal3api.HostFirmwareComponents{
				TypeMeta: metav1.TypeMeta{
					Kind:       "HostFirmwareComponents",
					APIVersion: "metal3.io/v1alpha1"},
				ObjectMeta: metav1.ObjectMeta{
					Name:            "hostName",
					Namespace:       "hostNamespace",
					ResourceVersion: "1"},
				Spec: metal3api.HostFirmwareComponentsSpec{
					Updates: []metal3api.FirmwareUpdate{
						{
							Component: "bmc",
							URL:       "https://myurl/newbmcfirmware",
						},
						{
							Component: "bios",
							URL:       "https://myurl/newbiosfirmware",
						},
					},
				},
				Status: metal3api.HostFirmwareComponentsStatus{
					Components: []metal3api.FirmwareComponentStatus{
						{
							Component:          "bmc",
							InitialVersion:     "1.0.0",
							CurrentVersion:     "1.1.0",
							LastVersionFlashed: "1.1.0",
						},
						{
							Component:          "bios",
							InitialVersion:     "1.0.1",
							CurrentVersion:     "1.1.10",
							LastVersionFlashed: "1.1.10",
						},
					},
				},
			},
			ExpectedComponents: &metal3api.HostFirmwareComponents{
				Spec: metal3api.HostFirmwareComponentsSpec{
					Updates: []metal3api.FirmwareUpdate{
						{
							Component: "bmc",
							URL:       "https://myurl/newbmcfirmware",
						},
						{
							Component: "bios",
							URL:       "https://myurl/newbiosfirmware",
						},
					},
				},
				Status: metal3api.HostFirmwareComponentsStatus{
					Updates: []metal3api.FirmwareUpdate{
						{
							Component: "bmc",
							URL:       "https://myurl/newbmcfirmware",
						},
						{
							Component: "bios",
							URL:       "https://myurl/newbiosfirmware",
						},
					},
					Components: []metal3api.FirmwareComponentStatus{
						{
							Component:          "bmc",
							InitialVersion:     "1.0.0",
							CurrentVersion:     "1.1.0",
							LastVersionFlashed: "1.1.0",
						},
						{
							Component:          "bios",
							InitialVersion:     "1.0.1",
							CurrentVersion:     "1.1.10",
							LastVersionFlashed: "1.1.10",
						},
					},
					Conditions: []metav1.Condition{
						{Type: "ChangeDetected", Status: "True", Reason: "OK"},
						{Type: "Valid", Status: "True", Reason: "OK"},
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Scenario, func(t *testing.T) {
			ctx := t.Context()

			tc.ExpectedComponents.TypeMeta = metav1.TypeMeta{
				Kind:       "HostFirmwareComponents",
				APIVersion: "metal3.io/v1alpha1"}
			tc.ExpectedComponents.ObjectMeta = metav1.ObjectMeta{
				Name:            hostName,
				Namespace:       hostNamespace,
				ResourceVersion: "2"}

			hfc := tc.CurrentHFCResource
			r := getTestHFCReconciler(hfc)
			// Create a bmh resource needed by hfc reconciler
			bmh := createBaremetalHostHFC()

			prov := getMockHFCProvisioner(bmh, getCurrentComponents(tc.UpdatedComponents))

			info := &rhfcInfo{
				log: logf.Log.WithName("controllers").WithName("HostFirmwareComponents"),
				hfc: tc.CurrentHFCResource,
				bmh: bmh,
			}

			components, err := prov.GetFirmwareComponents(ctx)
			require.NoError(t, err)

			err = r.updateHostFirmware(ctx, info, components)
			require.NoError(t, err)

			// Check that resources get created or updated
			key := client.ObjectKey{
				Namespace: hfc.ObjectMeta.Namespace, Name: hfc.ObjectMeta.Name}
			actual := &metal3api.HostFirmwareComponents{}
			err = r.Client.Get(ctx, key, actual)
			require.NoError(t, err)

			assert.Equal(t, tc.ExpectedComponents.Spec.Updates, actual.Spec.Updates)
			assert.Equal(t, tc.ExpectedComponents.Status.Components, actual.Status.Components)

			currentTime := metav1.Now()
			tc.ExpectedComponents.Status.LastUpdated = &currentTime
			actual.Status.LastUpdated = &currentTime
			for i := range tc.ExpectedComponents.Status.Conditions {
				tc.ExpectedComponents.Status.Conditions[i].LastTransitionTime = currentTime
				actual.Status.Conditions[i].LastTransitionTime = currentTime
			}
			assert.Equal(t, tc.ExpectedComponents.Status.LastUpdated, actual.Status.LastUpdated)
			assert.Equal(t, tc.ExpectedComponents.Status.Conditions, actual.Status.Conditions)
		})
	}
}

// Test that the reconciler does not return an error when the HFC resource
// has been deleted but the BMH still exists (e.g. during namespace deletion).
func TestHFCReconcileDeletedHFC(t *testing.T) {
	bmh := &metal3api.BareMetalHost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      hostName,
			Namespace: hostNamespace,
		},
		Status: metal3api.BareMetalHostStatus{
			Provisioning: metal3api.ProvisionStatus{
				State: metal3api.StateDeprovisioning,
			},
		},
	}
	// Build a client with only the BMH, no HFC
	c := fakeclient.NewClientBuilder().WithRuntimeObjects(bmh).Build()

	reconciler := &HostFirmwareComponentsReconciler{
		Client: c,
		Log:    ctrl.Log.WithName("test_reconciler").WithName("HostFirmwareComponents"),
	}

	request := ctrl.Request{NamespacedName: client.ObjectKey{Name: hostName, Namespace: hostNamespace}}
	result, err := reconciler.Reconcile(t.Context(), request)

	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
}

// Test the function to validate the components in the Spec.
func TestValidadeHostFirmwareComponents(t *testing.T) {
	testCases := []struct {
		Scenario       string
		SpecUpdates    metal3api.HostFirmwareComponentsSpec
		ExpectedErrors []string
	}{
		{
			Scenario: "valid spec - all components",
			SpecUpdates: metal3api.HostFirmwareComponentsSpec{
				Updates: []metal3api.FirmwareUpdate{
					{Component: "bmc", URL: "https://myurl/mybmcfw"},
					{Component: "bios", URL: "https://myurl/mybiosfw"},
				},
			},
			ExpectedErrors: []string{""},
		},
		{
			Scenario: "valid spec - only bios",
			SpecUpdates: metal3api.HostFirmwareComponentsSpec{
				Updates: []metal3api.FirmwareUpdate{
					{Component: "bios", URL: "https://myurl/mybiosfw"},
				},
			},
			ExpectedErrors: []string{""},
		},
		{
			Scenario: "valid spec - only bmc",
			SpecUpdates: metal3api.HostFirmwareComponentsSpec{
				Updates: []metal3api.FirmwareUpdate{
					{Component: "bmc", URL: "https://myurl/mybmcfw"},
				},
			},
			ExpectedErrors: []string{""},
		},
		{
			Scenario: "valid spec - with nic components",
			SpecUpdates: metal3api.HostFirmwareComponentsSpec{
				Updates: []metal3api.FirmwareUpdate{
					{Component: "bmc", URL: "https://myurl/mybmcfw"},
					{Component: "nic:NIC.1", URL: "https://myurl/mynicfw"},
					{Component: "nic:AD007", URL: "https://myurl/mynic2fw"},
				},
			},
			ExpectedErrors: []string{""},
		},
		{
			Scenario: "invalid something component",
			SpecUpdates: metal3api.HostFirmwareComponentsSpec{
				Updates: []metal3api.FirmwareUpdate{
					{Component: "something", URL: "https://myurl/myfw"},
				},
			},
			ExpectedErrors: []string{"component something is invalid, only 'bmc', 'bios', or names starting with 'nic:' are allowed as update names"},
		},
		{
			Scenario: "invalid something component with other valid components",
			SpecUpdates: metal3api.HostFirmwareComponentsSpec{
				Updates: []metal3api.FirmwareUpdate{
					{Component: "bmc", URL: "https://myurl/mybmcfw"},
					{Component: "bios", URL: "https://myurl/mybiosfw"},
					{Component: "something", URL: "https://myurl/myfw"},
				},
			},
			ExpectedErrors: []string{"component something is invalid, only 'bmc', 'bios', or names starting with 'nic:' are allowed as update names"},
		},
		{
			Scenario: "component not in lowercase",
			SpecUpdates: metal3api.HostFirmwareComponentsSpec{
				Updates: []metal3api.FirmwareUpdate{
					{Component: "BMC", URL: "https://myurl/mybmcfw"},
					{Component: "BIOS", URL: "https://myurl/mybiosfw"},
				},
			},
			ExpectedErrors: []string{
				"component BMC is invalid, only 'bmc', 'bios', or names starting with 'nic:' are allowed as update names",
				"component BIOS is invalid, only 'bmc', 'bios', or names starting with 'nic:' are allowed as update names",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Scenario, func(t *testing.T) {
			hfc := getHFC(tc.SpecUpdates)
			r := getTestHFCReconciler(hfc)
			info := rhfcInfo{
				log: logf.Log.WithName("controllers").WithName("HostFirmwareComponents"),
				hfc: hfc,
			}
			errors := r.validateHostFirmwareComponents(&info)
			if len(errors) == 0 {
				assert.Empty(t, tc.ExpectedErrors[0])
			} else {
				for i := range errors {
					assert.Equal(t, tc.ExpectedErrors[i], errors[i].Error())
				}
			}
		})
	}
}

// HFCDoneFunc is a callback passed to tryHFCReconcile.
// Return true when the condition you are testing has been met.
type HFCDoneFunc func(hfc *metal3api.HostFirmwareComponents, result ctrl.Result) bool

// tryHFCReconcile drives the reconciler in a loop until
// isDone returns true, or until 25 iterations are exceeded.
func tryHFCReconcile(t *testing.T, r *HostFirmwareComponentsReconciler, hfc *metal3api.HostFirmwareComponents, isDone HFCDoneFunc) {
	t.Helper()
	req := ctrl.Request{NamespacedName: client.ObjectKey{Name: hfc.Name, Namespace: hfc.Namespace}}
	const maxIter int = 25
	for i := range maxIter {
		t.Logf("Iteration %d", i)

		result, err := r.Reconcile(t.Context(), req)
		if err != nil {
			t.Fatal(err)
		}

		// Re-fetch so isDone sees the latest written status
		current := &metal3api.HostFirmwareComponents{}
		if err = r.Get(t.Context(), req.NamespacedName, current); err != nil {
			t.Fatal(err)
		}

		current.DeepCopyInto(hfc)

		if isDone(hfc, result) {
			t.Logf("Done at iteration %d", i)
			return
		}

		if !result.Requeue && result.RequeueAfter == 0 {
			t.Fatalf("Reconcile stopped at iteration %d without isDone returning true", i)
		}
	}

	t.Fatalf("Exceeded %d iterations", maxIter)
}

// newHFCReconciler builds a HostFirmwareComponentsReconciler with a fake client
// containing the given BMH and HFC, and the given fixture as the provisioner.
func newHFCReconciler(t *testing.T, bmh *metal3api.BareMetalHost, hfc *metal3api.HostFirmwareComponents, fix *fixture.Fixture) *HostFirmwareComponentsReconciler {
	t.Helper()
	c := fakeclient.NewClientBuilder().
		WithRuntimeObjects(bmh, hfc).
		WithStatusSubresource(bmh, hfc).
		Build()
	return &HostFirmwareComponentsReconciler{
		Client:             c,
		Log:                ctrl.Log.WithName("test").WithName("HostFirmwareComponents"),
		ProvisionerFactory: fix,
	}
}

// newProvisionedBMH returns a BareMetalHost in the Provisioned state,
// which is required for the HFC reconciler to proceed past the skip-guard.
func newProvisionedBMH(name, ns string) *metal3api.BareMetalHost {
	return &metal3api.BareMetalHost{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Status: metal3api.BareMetalHostStatus{
			Provisioning: metal3api.ProvisionStatus{
				ID:    "ironic-id-1",
				State: metal3api.StateProvisioned,
			},
		},
	}
}

// newHFC returns a HostFirmwareComponents resource with the given metadata and spec.
func newHFC(name, ns string, spec metal3api.HostFirmwareComponentsSpec) *metal3api.HostFirmwareComponents {
	return &metal3api.HostFirmwareComponents{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Spec:       spec,
	}
}

// Scenario 1: Provisioner returns components that match a pending spec update.
// Expected: ChangeDetected=True, Valid=True, Status.Components populated.
func TestHFCReconcile_ChangeDetected(t *testing.T) {
	ns, name := "test-ns", "node-1"

	bmh := newProvisionedBMH(name, ns)
	hfc := newHFC(name, ns, metal3api.HostFirmwareComponentsSpec{
		Updates: []metal3api.FirmwareUpdate{
			{Component: "bmc", URL: "https://fw.example/bmc.bin"},
		},
	})
	fix := &fixture.Fixture{
		HostFirmwareComponents: fixture.HostFirmwareComponentsMock{
			Components: getCurrentComponents("bmc"),
		},
	}

	r := newHFCReconciler(t, bmh, hfc, fix)

	tryHFCReconcile(t, r, hfc, func(hfc *metal3api.HostFirmwareComponents, _ ctrl.Result) bool {
		return len(hfc.Status.Components) == 2 &&
			meta.IsStatusConditionTrue(hfc.Status.Conditions, string(metal3api.HostFirmwareComponentsChangeDetected)) &&
			meta.IsStatusConditionTrue(hfc.Status.Conditions, string(metal3api.HostFirmwareComponentsValid))
	})
}

// Scenario 2: Spec has no updates — provisioner returns components but there
// is nothing requested to change.
// Expected: ChangeDetected=False, Valid=True, Status.Components still populated.
func TestHFCReconcile_NoChange(t *testing.T) {
	ns, name := "test-ns", "node-2"

	bmh := newProvisionedBMH(name, ns)
	hfc := newHFC(name, ns, metal3api.HostFirmwareComponentsSpec{}) // Empty spec — no updates requested.
	fix := &fixture.Fixture{
		HostFirmwareComponents: fixture.HostFirmwareComponentsMock{
			Components: []metal3api.FirmwareComponentStatus{ // No change in components
				{Component: "bmc", InitialVersion: "1.0.0", CurrentVersion: "1.0.0"},
				{Component: "bios", InitialVersion: "2.0.0", CurrentVersion: "2.0.0"},
			},
		},
	}

	r := newHFCReconciler(t, bmh, hfc, fix)

	tryHFCReconcile(t, r, hfc, func(hfc *metal3api.HostFirmwareComponents, _ ctrl.Result) bool {
		return len(hfc.Status.Components) == 2 &&
			meta.IsStatusConditionFalse(hfc.Status.Conditions, string(metal3api.HostFirmwareComponentsChangeDetected)) &&
			meta.IsStatusConditionTrue(hfc.Status.Conditions, string(metal3api.HostFirmwareComponentsValid))
	})
}

// Scenario 3: Spec contains an invalid component name (not bmc/bios/nic:*).
// Expected: Valid=False, ChangeDetected condition reflects the invalid spec.
func TestHFCReconcile_InvalidSpec(t *testing.T) {
	ns, name := "test-ns", "node-3"

	bmh := newProvisionedBMH(name, ns)
	hfc := newHFC(name, ns, metal3api.HostFirmwareComponentsSpec{
		Updates: []metal3api.FirmwareUpdate{
			{Component: "INVALID", URL: "https://fw.example/bad.bin"},
		},
	})
	fix := &fixture.Fixture{
		HostFirmwareComponents: fixture.HostFirmwareComponentsMock{
			Components: []metal3api.FirmwareComponentStatus{
				{Component: "bmc", InitialVersion: "1.0.0", CurrentVersion: "1.0.0"},
			},
		},
	}

	r := newHFCReconciler(t, bmh, hfc, fix)

	tryHFCReconcile(t, r, hfc, func(hfc *metal3api.HostFirmwareComponents, _ ctrl.Result) bool {
		return meta.IsStatusConditionFalse(hfc.Status.Conditions, string(metal3api.HostFirmwareComponentsValid))
	})
}

// Test that reconcile exits cleanly when HFC exists but the corresponding
// BareMetalHost does not exist.
func TestHFCReconcileBMHNotFound(t *testing.T) {
	ns, name := "test-ns", "node-4"

	hfc := newHFC(name, ns, metal3api.HostFirmwareComponentsSpec{})
	c := fakeclient.NewClientBuilder().WithRuntimeObjects(hfc).Build()

	reconciler := &HostFirmwareComponentsReconciler{
		Client: c,
		Log:    ctrl.Log.WithName("test_reconciler").WithName("HostFirmwareComponents"),
	}

	request := ctrl.Request{NamespacedName: client.ObjectKey{Name: name, Namespace: ns}}
	result, err := reconciler.Reconcile(t.Context(), request)

	require.NoError(t, err)
	require.Equal(t, ctrl.Result{}, result)
}

type errorOnBMHGetClient struct {
	client.Client
	err error
}

type errorProvisionerFactory struct {
	err error
}

type firmwareComponentsErrorFactory struct {
	baseFactory provisioner.Factory
	err         error
}

type firmwareComponentsErrorProvisioner struct {
	provisioner.Provisioner
	err error
}

func (f *errorProvisionerFactory) NewProvisioner(_ context.Context, _ provisioner.HostData, _ provisioner.EventPublisher) (provisioner.Provisioner, error) {
	return nil, f.err
}

func (f *firmwareComponentsErrorFactory) NewProvisioner(ctx context.Context, hostData provisioner.HostData, publish provisioner.EventPublisher) (provisioner.Provisioner, error) {
	p, err := f.baseFactory.NewProvisioner(ctx, hostData, publish)
	if err != nil {
		return nil, err
	}

	return &firmwareComponentsErrorProvisioner{Provisioner: p, err: f.err}, nil
}

func (p *firmwareComponentsErrorProvisioner) GetFirmwareComponents(_ context.Context) ([]metal3api.FirmwareComponentStatus, error) {
	return nil, p.err
}

func (c *errorOnBMHGetClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	if _, isBMH := obj.(*metal3api.BareMetalHost); isBMH {
		return c.err
	}
	return c.Client.Get(ctx, key, obj, opts...)
}

// Test that reconcile requeues when fetching BareMetalHost fails with a non-not-found error.
func TestHFCReconcileBMHFetchError(t *testing.T) {
	ns, name := "test-ns", "node-5"
	t.Log("simulate BareMetalHost fetch error")

	baseClient := fakeclient.NewClientBuilder().Build()
	expectedErr := errors.New("temporary api error")
	reconciler := &HostFirmwareComponentsReconciler{
		Client: &errorOnBMHGetClient{
			Client: baseClient,
			err:    expectedErr,
		},
		Log: ctrl.Log.WithName("test_reconciler").WithName("HostFirmwareComponents"),
	}

	request := ctrl.Request{NamespacedName: client.ObjectKey{Name: name, Namespace: ns}}
	result, err := reconciler.Reconcile(t.Context(), request)
	t.Logf("reconcile result: %+v", result)

	require.Error(t, err)
	require.ErrorContains(t, err, expectedErr.Error())
	require.True(t, result.Requeue)
	require.Equal(t, resourceNotAvailableRetryDelay, result.RequeueAfter)
}

// Test that reconcile requeues with unmanagedRetryDelay when subresource
// reconciliation must be skipped for an unmanaged host.
func TestHFCReconcileSkipReconcileSubresource(t *testing.T) {
	ns, name := "test-ns", "node-6"

	bmh := &metal3api.BareMetalHost{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Status: metal3api.BareMetalHostStatus{
			Provisioning: metal3api.ProvisionStatus{State: metal3api.StateUnmanaged},
		},
	}
	hfc := newHFC(name, ns, metal3api.HostFirmwareComponentsSpec{})
	c := fakeclient.NewClientBuilder().WithRuntimeObjects(bmh, hfc).Build()

	reconciler := &HostFirmwareComponentsReconciler{
		Client: c,
		Log:    ctrl.Log.WithName("test_reconciler").WithName("HostFirmwareComponents"),
	}

	request := ctrl.Request{NamespacedName: client.ObjectKey{Name: name, Namespace: ns}}
	result, err := reconciler.Reconcile(t.Context(), request)

	require.NoError(t, err)
	require.True(t, result.Requeue)
	require.Equal(t, unmanagedRetryDelay, result.RequeueAfter)
}

// Test that reconcile returns a wrapped error when provisioner creation fails.
func TestHFCReconcileProvisionerCreationFailure(t *testing.T) {
	ns, name := "test-ns", "node-7"

	bmh := newProvisionedBMH(name, ns)
	hfc := newHFC(name, ns, metal3api.HostFirmwareComponentsSpec{})
	c := fakeclient.NewClientBuilder().WithRuntimeObjects(bmh, hfc).Build()

	expectedErr := errors.New("factory boom")
	reconciler := &HostFirmwareComponentsReconciler{
		Client:             c,
		Log:                ctrl.Log.WithName("test_reconciler").WithName("HostFirmwareComponents"),
		ProvisionerFactory: &errorProvisionerFactory{err: expectedErr},
	}

	request := ctrl.Request{NamespacedName: client.ObjectKey{Name: name, Namespace: ns}}
	result, err := reconciler.Reconcile(t.Context(), request)

	require.Error(t, err)
	require.ErrorContains(t, err, "failed to create provisioner")
	require.ErrorContains(t, err, expectedErr.Error())
	require.Equal(t, ctrl.Result{}, result)
}

// Test that reconcile requeues with provisionerRetryDelay when provisioner
// initialization is not ready yet.
func TestHFCReconcileProvisionerNotReady(t *testing.T) {
	ns, name := "test-ns", "node-8"

	bmh := newProvisionedBMH(name, ns)
	hfc := newHFC(name, ns, metal3api.HostFirmwareComponentsSpec{})
	fix := &fixture.Fixture{BecomeReadyCounter: 2}

	reconciler := newHFCReconciler(t, bmh, hfc, fix)

	request := ctrl.Request{NamespacedName: client.ObjectKey{Name: name, Namespace: ns}}
	result, err := reconciler.Reconcile(t.Context(), request)

	require.NoError(t, err)
	require.True(t, result.Requeue)
	require.Equal(t, provisionerRetryDelay, result.RequeueAfter)
}

// Test that reconcile exits cleanly when GetFirmwareComponents returns
// ErrFirmwareUpdateUnsupported.
func TestHFCReconcileFirmwareUpdateUnsupported(t *testing.T) {
	ns, name := "test-ns", "node-9"

	bmh := newProvisionedBMH(name, ns)
	hfc := newHFC(name, ns, metal3api.HostFirmwareComponentsSpec{})

	baseFixture := &fixture.Fixture{}
	reconciler := newHFCReconciler(t, bmh, hfc, baseFixture)
	reconciler.ProvisionerFactory = &firmwareComponentsErrorFactory{
		baseFactory: baseFixture,
		err:         provisioner.ErrFirmwareUpdateUnsupported,
	}

	request := ctrl.Request{NamespacedName: client.ObjectKey{Name: name, Namespace: ns}}
	result, err := reconciler.Reconcile(t.Context(), request)

	require.NoError(t, err)
	require.Equal(t, ctrl.Result{}, result)
}

// Test that reconcile requeues with provisionerRetryDelay when
// GetFirmwareComponents returns a generic error.
func TestHFCReconcileFirmwareComponentsGenericError(t *testing.T) {
	ns, name := "test-ns", "node-10"

	bmh := newProvisionedBMH(name, ns)
	hfc := newHFC(name, ns, metal3api.HostFirmwareComponentsSpec{})

	baseFixture := &fixture.Fixture{}
	reconciler := newHFCReconciler(t, bmh, hfc, baseFixture)
	reconciler.ProvisionerFactory = &firmwareComponentsErrorFactory{
		baseFactory: baseFixture,
		err:         errors.New("temporary components error"),
	}

	request := ctrl.Request{NamespacedName: client.ObjectKey{Name: name, Namespace: ns}}
	result, err := reconciler.Reconcile(t.Context(), request)

	require.NoError(t, err)
	require.True(t, result.Requeue)
	require.Equal(t, provisionerRetryDelay, result.RequeueAfter)
}

// Test that reconcile publishes a ValidationFailed event when the firmware
// update spec is invalid.
func TestHFCReconcilePublishesValidationFailedEvent(t *testing.T) {
	ns, name := "test-ns", "node-11"

	bmh := newProvisionedBMH(name, ns)
	hfc := newHFC(name, ns, metal3api.HostFirmwareComponentsSpec{
		Updates: []metal3api.FirmwareUpdate{
			{Component: "INVALID", URL: "https://fw.example/bad.bin"},
		},
	})
	fix := &fixture.Fixture{
		HostFirmwareComponents: fixture.HostFirmwareComponentsMock{
			Components: []metal3api.FirmwareComponentStatus{
				{Component: "bmc", InitialVersion: "1.0.0", CurrentVersion: "1.0.0"},
			},
		},
	}

	r := newHFCReconciler(t, bmh, hfc, fix)
	request := ctrl.Request{NamespacedName: client.ObjectKey{Name: name, Namespace: ns}}

	result, err := r.Reconcile(t.Context(), request)

	require.NoError(t, err)
	require.True(t, result.Requeue)

	eventList := &corev1.EventList{}
	err = r.List(t.Context(), eventList, client.InNamespace(ns))
	require.NoError(t, err)
	require.NotEmpty(t, eventList.Items)

	found := false
	for _, ev := range eventList.Items {
		if ev.Reason == "ValidationFailed" {
			found = true
			require.Contains(t, ev.Message, "Invalid Firmware Components")
			break
		}
	}

	require.True(t, found, "expected ValidationFailed event to be published")
}
