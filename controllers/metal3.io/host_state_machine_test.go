package controllers

import (
	"context"
	"testing"
	"time"

	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"

	promutil "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1/profile"
	"github.com/metal3-io/baremetal-operator/pkg/hardwareutils/bmc"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner"
)

func testStateMachine(host *metal3api.BareMetalHost) *hostStateMachine {
	r := newTestReconciler()
	p, _ := r.ProvisionerFactory.NewProvisioner(context.TODO(), provisioner.BuildHostData(*host, bmc.Credentials{}),
		func(reason, message string) {})
	return newHostStateMachine(host, r, p, true)
}

// Create a reconciler with a fake client to satisfy states that use the client.
func testNewReconciler(host *metal3api.BareMetalHost) *BareMetalHostReconciler {
	reconciler := &BareMetalHostReconciler{
		Client:             fakeclient.NewClientBuilder().WithObjects(host).Build(),
		ProvisionerFactory: nil,
		Log:                ctrl.Log.WithName("host_state_machine").WithName("BareMetalHost"),
	}

	return reconciler
}

func TestProvisioningCapacity(t *testing.T) {
	testCases := []struct {
		Scenario string

		HasProvisioningCapacity bool
		Host                    *metal3api.BareMetalHost

		ExpectedProvisioningState metal3api.ProvisioningState
		ExpectedDelayed           bool
	}{
		{
			Scenario:                "transition-to-inspecting-delayed",
			Host:                    host(metal3api.StateRegistering).build(),
			HasProvisioningCapacity: false,

			ExpectedProvisioningState: metal3api.StateRegistering,
			ExpectedDelayed:           true,
		},
		{
			Scenario:                "transition-to-provisioning-delayed",
			Host:                    host(metal3api.StateAvailable).SaveHostProvisioningSettings().build(),
			HasProvisioningCapacity: false,

			ExpectedProvisioningState: metal3api.StateAvailable,
			ExpectedDelayed:           true,
		},
		{
			Scenario:                "transition-to-provisioning-delayed-deprecated-ready",
			Host:                    host(metal3api.StateReady).SaveHostProvisioningSettings().build(),
			HasProvisioningCapacity: false,

			ExpectedProvisioningState: metal3api.StateReady,
			ExpectedDelayed:           true,
		},
		{
			Scenario:                "transition-to-inspecting-ok",
			Host:                    host(metal3api.StateRegistering).build(),
			HasProvisioningCapacity: true,

			ExpectedProvisioningState: metal3api.StateInspecting,
			ExpectedDelayed:           false,
		},
		{
			Scenario:                "transition-to-provisioning-ok",
			Host:                    host(metal3api.StateAvailable).SaveHostProvisioningSettings().build(),
			HasProvisioningCapacity: true,

			ExpectedProvisioningState: metal3api.StateProvisioning,
			ExpectedDelayed:           false,
		},

		{
			Scenario:                "already-delayed-delayed",
			Host:                    host(metal3api.StateAvailable).SetOperationalStatus(metal3api.OperationalStatusDelayed).build(),
			HasProvisioningCapacity: false,

			ExpectedProvisioningState: metal3api.StateAvailable,
			ExpectedDelayed:           true,
		},
		{
			Scenario:                "already-delayed-ok",
			Host:                    host(metal3api.StateAvailable).SetOperationalStatus(metal3api.OperationalStatusDelayed).build(),
			HasProvisioningCapacity: true,

			ExpectedProvisioningState: metal3api.StateAvailable,
			ExpectedDelayed:           false,
		},

		{
			Scenario:                "untracked-inspecting-delayed",
			Host:                    host(metal3api.StateInspecting).build(),
			HasProvisioningCapacity: false,

			ExpectedProvisioningState: metal3api.StateInspecting,
			ExpectedDelayed:           true,
		},
		{
			Scenario:                "untracked-inspecting-ok",
			Host:                    host(metal3api.StateInspecting).build(),
			HasProvisioningCapacity: true,

			ExpectedProvisioningState: metal3api.StatePreparing,
			ExpectedDelayed:           false,
		},
		{
			Scenario:                "untracked-inspecting-delayed",
			Host:                    host(metal3api.StateProvisioning).build(),
			HasProvisioningCapacity: false,

			ExpectedProvisioningState: metal3api.StateProvisioning,
			ExpectedDelayed:           true,
		},
		{
			Scenario:                "untracked-provisioning-ok",
			Host:                    host(metal3api.StateProvisioning).build(),
			HasProvisioningCapacity: true,

			ExpectedProvisioningState: metal3api.StateProvisioned,
			ExpectedDelayed:           false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Scenario, func(t *testing.T) {
			prov := newMockProvisioner()
			prov.setHasCapacity(tc.HasProvisioningCapacity)
			reconciler := testNewReconciler(tc.Host)
			hsm := newHostStateMachine(tc.Host, reconciler, prov, true)
			info := makeDefaultReconcileInfo(tc.Host)
			delayedProvisioningHostCounters.Reset()

			result := hsm.ReconcileState(info)

			assert.Equal(t, tc.ExpectedProvisioningState, tc.Host.Status.Provisioning.State)
			assert.Equal(t, tc.ExpectedDelayed, metal3api.OperationalStatusDelayed == tc.Host.Status.OperationalStatus, "Expected OperationalStatusDelayed")
			assert.Equal(t, tc.ExpectedDelayed, assert.ObjectsAreEqual(actionDelayed{}, result), "Expected actionDelayed")

			if tc.ExpectedDelayed {
				counter, _ := delayedProvisioningHostCounters.GetMetricWith(hostMetricLabels(info.request))
				initialCounterValue := promutil.ToFloat64(counter)
				for _, sb := range info.postSaveCallbacks {
					sb()
				}
				assert.Greater(t, promutil.ToFloat64(counter), initialCounterValue)
			}
		})
	}
}

func TestDeprovisioningCapacity(t *testing.T) {
	testCases := []struct {
		Scenario string

		HasDeprovisioningCapacity bool
		Host                      *metal3api.BareMetalHost

		ExpectedDeprovisioningState metal3api.ProvisioningState
		ExpectedDelayed             bool
	}{
		{
			Scenario:                  "transition-to-deprovisionig-ready",
			Host:                      host(metal3api.StateDeprovisioning).build(),
			HasDeprovisioningCapacity: true,

			ExpectedDeprovisioningState: metal3api.StateAvailable,
			ExpectedDelayed:             false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Scenario, func(t *testing.T) {
			prov := newMockProvisioner()
			prov.setHasCapacity(tc.HasDeprovisioningCapacity)
			reconciler := testNewReconciler(tc.Host)
			hsm := newHostStateMachine(tc.Host, reconciler, prov, true)
			info := makeDefaultReconcileInfo(tc.Host)
			delayedDeprovisioningHostCounters.Reset()

			result := hsm.ReconcileState(info)

			assert.Equal(t, tc.ExpectedDeprovisioningState, tc.Host.Status.Provisioning.State)
			assert.Equal(t, tc.ExpectedDelayed, metal3api.OperationalStatusDelayed == tc.Host.Status.OperationalStatus, "Expected OperationalStatusDelayed")
			assert.Equal(t, tc.ExpectedDelayed, assert.ObjectsAreEqual(actionDelayed{}, result), "Expected actionDelayed")

			if tc.ExpectedDelayed {
				counter, _ := delayedDeprovisioningHostCounters.GetMetricWith(hostMetricLabels(info.request))
				initialCounterValue := promutil.ToFloat64(counter)
				for _, sb := range info.postSaveCallbacks {
					sb()
				}
				assert.Greater(t, promutil.ToFloat64(counter), initialCounterValue)
			}
		})
	}
}

func TestDetach(t *testing.T) {
	testCases := []struct {
		Scenario                  string
		Host                      *metal3api.BareMetalHost
		HasDetachedAnnotation     bool
		ExpectedDetach            bool
		ExpectedDirty             bool
		ExpectedOperationalStatus metal3api.OperationalStatus
		ExpectedState             metal3api.ProvisioningState
	}{
		{
			Scenario:                  "ProvisionedHost",
			Host:                      host(metal3api.StateProvisioned).build(),
			ExpectedDetach:            false,
			ExpectedDirty:             false,
			ExpectedOperationalStatus: metal3api.OperationalStatusOK,
			ExpectedState:             metal3api.StateProvisioned,
		},
		{
			Scenario:                  "DetachProvisionedHost",
			Host:                      host(metal3api.StateProvisioned).build(),
			HasDetachedAnnotation:     true,
			ExpectedDetach:            true,
			ExpectedDirty:             true,
			ExpectedOperationalStatus: metal3api.OperationalStatusDetached,
			ExpectedState:             metal3api.StateProvisioned,
		},
		{
			Scenario:                  "DeleteDetachedProvisionedHost",
			Host:                      host(metal3api.StateProvisioned).SetOperationalStatus(metal3api.OperationalStatusDetached).setDeletion().withFinalizer().build(),
			HasDetachedAnnotation:     true,
			ExpectedDetach:            false,
			ExpectedDirty:             true,
			ExpectedOperationalStatus: metal3api.OperationalStatusDetached,
			// Should move to Deleting without any Deprovisioning
			ExpectedState: metal3api.StateDeleting,
		},
		{
			Scenario:                  "ExternallyProvisionedHost",
			Host:                      host(metal3api.StateExternallyProvisioned).SetExternallyProvisioned().build(),
			HasDetachedAnnotation:     false,
			ExpectedDetach:            false,
			ExpectedDirty:             false,
			ExpectedOperationalStatus: metal3api.OperationalStatusOK,
			ExpectedState:             metal3api.StateExternallyProvisioned,
		},
		{
			Scenario:                  "DetachExternallyProvisionedHost",
			Host:                      host(metal3api.StateExternallyProvisioned).SetExternallyProvisioned().build(),
			HasDetachedAnnotation:     true,
			ExpectedDetach:            true,
			ExpectedDirty:             true,
			ExpectedOperationalStatus: metal3api.OperationalStatusDetached,
			ExpectedState:             metal3api.StateExternallyProvisioned,
		},
		{
			Scenario:                  "NoneHost",
			Host:                      host(metal3api.StateNone).build(),
			HasDetachedAnnotation:     true,
			ExpectedDetach:            false,
			ExpectedDirty:             true,
			ExpectedOperationalStatus: metal3api.OperationalStatusDiscovered,
			ExpectedState:             metal3api.StateUnmanaged,
		},
		{
			Scenario:                  "UnmanagedHost",
			Host:                      host(metal3api.StateUnmanaged).build(),
			HasDetachedAnnotation:     true,
			ExpectedDetach:            false,
			ExpectedDirty:             false,
			ExpectedOperationalStatus: metal3api.OperationalStatusOK,
			ExpectedState:             metal3api.StateUnmanaged,
		},
		{
			Scenario:                  "RegisteringHost",
			Host:                      host(metal3api.StateRegistering).build(),
			HasDetachedAnnotation:     true,
			ExpectedDetach:            false,
			ExpectedDirty:             true,
			ExpectedOperationalStatus: metal3api.OperationalStatusOK,
			ExpectedState:             metal3api.StateInspecting,
		},
		{
			Scenario:                  "InspectingHost",
			Host:                      host(metal3api.StateInspecting).build(),
			HasDetachedAnnotation:     true,
			ExpectedDetach:            false,
			ExpectedDirty:             true,
			ExpectedOperationalStatus: metal3api.OperationalStatusOK,
			ExpectedState:             metal3api.StatePreparing,
		},
		{
			Scenario:                  "DetachAvailableHost",
			Host:                      host(metal3api.StateAvailable).SetImageURL("").SetStatusPoweredOn(false).build(),
			HasDetachedAnnotation:     true,
			ExpectedDetach:            true,
			ExpectedDirty:             true,
			ExpectedOperationalStatus: metal3api.OperationalStatusDetached,
			ExpectedState:             metal3api.StateAvailable,
		},
		{
			Scenario:                  "AttachAvailableHost",
			Host:                      host(metal3api.StateAvailable).SetImageURL("").SetStatusPoweredOn(false).build(),
			HasDetachedAnnotation:     false,
			ExpectedDetach:            false,
			ExpectedDirty:             true,
			ExpectedOperationalStatus: metal3api.OperationalStatusOK,
			ExpectedState:             metal3api.StateAvailable,
		},
		{
			Scenario:                  "AvailableHost",
			Host:                      host(metal3api.StateAvailable).build(),
			HasDetachedAnnotation:     false,
			ExpectedDetach:            false,
			ExpectedDirty:             true,
			ExpectedOperationalStatus: metal3api.OperationalStatusOK,
			ExpectedState:             metal3api.StateProvisioning,
		},
		{
			Scenario:                  "DeprecatedReadyHost",
			Host:                      host(metal3api.StateReady).build(),
			HasDetachedAnnotation:     false,
			ExpectedDetach:            false,
			ExpectedDirty:             true,
			ExpectedOperationalStatus: metal3api.OperationalStatusOK,
			ExpectedState:             metal3api.StateProvisioning,
		},
		{
			Scenario:                  "PreparingHost",
			Host:                      host(metal3api.StatePreparing).build(),
			HasDetachedAnnotation:     true,
			ExpectedDetach:            false,
			ExpectedDirty:             true,
			ExpectedOperationalStatus: metal3api.OperationalStatusOK,
			ExpectedState:             metal3api.StateAvailable,
		},
		{
			Scenario:                  "DetachAvailableHost",
			Host:                      host(metal3api.StateAvailable).SetImageURL("").SetStatusPoweredOn(false).build(),
			HasDetachedAnnotation:     true,
			ExpectedDetach:            true,
			ExpectedDirty:             true,
			ExpectedOperationalStatus: metal3api.OperationalStatusDetached,
			ExpectedState:             metal3api.StateAvailable,
		},
		{
			Scenario:                  "AttachAvailableHost",
			Host:                      host(metal3api.StateAvailable).SetImageURL("").SetStatusPoweredOn(false).build(),
			HasDetachedAnnotation:     false,
			ExpectedDetach:            false,
			ExpectedDirty:             true,
			ExpectedOperationalStatus: metal3api.OperationalStatusOK,
			ExpectedState:             metal3api.StateAvailable,
		},
		{
			Scenario:                  "AvailableHost",
			Host:                      host(metal3api.StateAvailable).build(),
			HasDetachedAnnotation:     false,
			ExpectedDetach:            false,
			ExpectedDirty:             true,
			ExpectedOperationalStatus: metal3api.OperationalStatusOK,
			ExpectedState:             metal3api.StateProvisioning,
		},
		{
			Scenario:                  "ProvisioningHost",
			Host:                      host(metal3api.StateProvisioning).build(),
			HasDetachedAnnotation:     true,
			ExpectedDetach:            false,
			ExpectedDirty:             true,
			ExpectedOperationalStatus: metal3api.OperationalStatusOK,
			ExpectedState:             metal3api.StateProvisioned,
		},
		{
			Scenario:                  "DeprovisioningHost",
			Host:                      host(metal3api.StateDeprovisioning).build(),
			HasDetachedAnnotation:     true,
			ExpectedDetach:            false,
			ExpectedDirty:             true,
			ExpectedOperationalStatus: metal3api.OperationalStatusOK,
			ExpectedState:             metal3api.StateAvailable,
		},
		{
			Scenario:                  "DeletingHost",
			Host:                      host(metal3api.StateDeleting).setDeletion().withFinalizer().build(),
			HasDetachedAnnotation:     true,
			ExpectedDetach:            false,
			ExpectedDirty:             false,
			ExpectedOperationalStatus: metal3api.OperationalStatusOK,
			ExpectedState:             metal3api.StateDeleting,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.Scenario, func(t *testing.T) {
			if tc.HasDetachedAnnotation {
				tc.Host.Annotations = map[string]string{
					metal3api.DetachedAnnotation: "true",
				}
			}
			prov := newMockProvisioner()
			reconciler := testNewReconciler(tc.Host)
			hsm := newHostStateMachine(tc.Host, reconciler, prov, true)
			info := makeDefaultReconcileInfo(tc.Host)
			result := hsm.ReconcileState(info)

			assert.Equal(t, tc.ExpectedDetach, prov.calledNoError("Detach"), "ExpectedDetach mismatch")
			assert.Equal(t, tc.ExpectedDirty, result.Dirty(), "ExpectedDirty mismatch")
			assert.Equal(t, tc.ExpectedOperationalStatus, info.host.OperationalStatus())
			assert.Equal(t, tc.ExpectedState, info.host.Status.Provisioning.State)
		})
	}
}

func TestDetachError(t *testing.T) {
	testCases := []struct {
		Scenario                  string
		Host                      *metal3api.BareMetalHost
		ExpectedOperationalStatus metal3api.OperationalStatus
		ExpectedState             metal3api.ProvisioningState
		ClearError                bool
		RemoveAnnotation          bool
	}{
		{
			Scenario:                  "ProvisionerTemporaryError",
			Host:                      host(metal3api.StateProvisioned).build(),
			ExpectedOperationalStatus: metal3api.OperationalStatusDetached,
			ExpectedState:             metal3api.StateProvisioned,
			ClearError:                true,
		},
		{
			Scenario:                  "AnnotationRemovedAfterDetachError",
			Host:                      host(metal3api.StateProvisioned).build(),
			ExpectedOperationalStatus: metal3api.OperationalStatusOK,
			ExpectedState:             metal3api.StateProvisioned,
			RemoveAnnotation:          true,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.Scenario, func(t *testing.T) {
			tc.Host.Annotations = map[string]string{
				metal3api.DetachedAnnotation: "true",
			}
			prov := newMockProvisioner()
			reconciler := testNewReconciler(tc.Host)
			hsm := newHostStateMachine(tc.Host, reconciler, prov, true)
			info := makeDefaultReconcileInfo(tc.Host)

			prov.setNextError("Detach", "some error")
			result := hsm.ReconcileState(info)
			assert.True(t, result.Dirty())
			assert.Equal(t, 1, tc.Host.Status.ErrorCount)
			assert.Equal(t, metal3api.OperationalStatusError, info.host.OperationalStatus())
			assert.Equal(t, metal3api.DetachError, info.host.Status.ErrorType)
			assert.Equal(t, tc.ExpectedState, info.host.Status.Provisioning.State)

			if tc.ClearError {
				prov.clearNextError("Detach")
			}
			if tc.RemoveAnnotation {
				tc.Host.Annotations = map[string]string{}
			}
			result = hsm.ReconcileState(info)
			assert.Equal(t, 0, tc.Host.Status.ErrorCount)
			assert.True(t, result.Dirty())
			assert.Equal(t, tc.ExpectedOperationalStatus, info.host.OperationalStatus())
			assert.Equal(t, tc.ExpectedState, info.host.Status.Provisioning.State)
			assert.Empty(t, info.host.Status.ErrorType)
		})
	}
}

func TestProvisioningCancelled(t *testing.T) {
	testCases := []struct {
		Scenario string
		Host     metal3api.BareMetalHost
		Expected bool
	}{
		{
			Scenario: "with image url, unprovisioned",
			Host: metal3api.BareMetalHost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "myhost",
					Namespace: "myns",
				},
				Spec: metal3api.BareMetalHostSpec{
					Image: &metal3api.Image{
						URL: "not-empty",
					},
					Online: true,
				},
			},
			Expected: false,
		},

		{
			Scenario: "with custom deploy, unprovisioned",
			Host: metal3api.BareMetalHost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "myhost",
					Namespace: "myns",
				},
				Spec: metal3api.BareMetalHostSpec{
					CustomDeploy: &metal3api.CustomDeploy{
						Method: "install_everything",
					},
					Online: true,
				},
			},
			Expected: false,
		},

		{
			Scenario: "with image, unprovisioned",
			Host: metal3api.BareMetalHost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "myhost",
					Namespace: "myns",
				},
				Spec: metal3api.BareMetalHostSpec{
					Image:  &metal3api.Image{},
					Online: true,
				},
			},
			Expected: true,
		},

		{
			Scenario: "without, unprovisioned",
			Host: metal3api.BareMetalHost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "myhost",
					Namespace: "myns",
				},
				Spec: metal3api.BareMetalHostSpec{
					Online: true,
				},
			},
			Expected: true,
		},

		{
			Scenario: "with image url, offline",
			Host: metal3api.BareMetalHost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "myhost",
					Namespace: "myns",
				},
				Spec: metal3api.BareMetalHostSpec{
					Image: &metal3api.Image{
						URL: "not-empty",
					},
					Online: false,
				},
			},
			Expected: false,
		},

		{
			Scenario: "with custom deploy, offline",
			Host: metal3api.BareMetalHost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "myhost",
					Namespace: "myns",
				},
				Spec: metal3api.BareMetalHostSpec{
					CustomDeploy: &metal3api.CustomDeploy{
						Method: "install_everything",
					},
					Online: false,
				},
			},
			Expected: false,
		},

		{
			Scenario: "provisioned with image",
			Host: metal3api.BareMetalHost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "myhost",
					Namespace: "myns",
				},
				Spec: metal3api.BareMetalHostSpec{
					Image: &metal3api.Image{
						URL: "same",
					},
					Online: true,
				},
				Status: metal3api.BareMetalHostStatus{
					Provisioning: metal3api.ProvisionStatus{
						Image: metal3api.Image{
							URL: "same",
						},
					},
				},
			},
			Expected: false,
		},

		{
			Scenario: "provisioned with error",
			Host: metal3api.BareMetalHost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "myhost",
					Namespace: "myns",
				},
				Spec: metal3api.BareMetalHostSpec{
					Image: &metal3api.Image{
						URL: "same",
					},
					Online: true,
				},
				Status: metal3api.BareMetalHostStatus{
					ErrorType:    metal3api.ProvisionedRegistrationError,
					ErrorMessage: "Adoption failed",
					ErrorCount:   1,
					Provisioning: metal3api.ProvisionStatus{
						Image: metal3api.Image{
							URL: "same",
						},
					},
				},
			},
			Expected: false,
		},

		{

			Scenario: "provisioned with custom deploy",
			Host: metal3api.BareMetalHost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "myhost",
					Namespace: "myns",
				},
				Spec: metal3api.BareMetalHostSpec{
					CustomDeploy: &metal3api.CustomDeploy{
						Method: "install_everything",
					},
					Online: true,
				},
				Status: metal3api.BareMetalHostStatus{
					Provisioning: metal3api.ProvisionStatus{
						CustomDeploy: &metal3api.CustomDeploy{
							Method: "install_everything",
						},
					},
				},
			},
			Expected: false,
		},

		{
			Scenario: "removed image",
			Host: metal3api.BareMetalHost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "myhost",
					Namespace: "myns",
				},
				Spec: metal3api.BareMetalHostSpec{
					Online: true,
				},
				Status: metal3api.BareMetalHostStatus{
					Provisioning: metal3api.ProvisionStatus{
						Image: metal3api.Image{
							URL: "same",
						},
					},
				},
			},
			Expected: true,
		},

		{
			Scenario: "removed custom deploy",
			Host: metal3api.BareMetalHost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "myhost",
					Namespace: "myns",
				},
				Spec: metal3api.BareMetalHostSpec{
					Online: true,
				},
				Status: metal3api.BareMetalHostStatus{
					Provisioning: metal3api.ProvisionStatus{
						CustomDeploy: &metal3api.CustomDeploy{
							Method: "install_everything",
						},
					},
				},
			},
			Expected: true,
		},

		{
			Scenario: "changed image",
			Host: metal3api.BareMetalHost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "myhost",
					Namespace: "myns",
				},
				Spec: metal3api.BareMetalHostSpec{
					Image: &metal3api.Image{
						URL: "not-empty",
					},
					Online: true,
				},
				Status: metal3api.BareMetalHostStatus{
					Provisioning: metal3api.ProvisionStatus{
						Image: metal3api.Image{
							URL: "also-not-empty",
						},
					},
				},
			},
			Expected: true,
		},

		{
			Scenario: "changed custom deploy",
			Host: metal3api.BareMetalHost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "myhost",
					Namespace: "myns",
				},
				Spec: metal3api.BareMetalHostSpec{
					CustomDeploy: &metal3api.CustomDeploy{
						Method: "install_not_everything",
					},
					Online: true,
				},
				Status: metal3api.BareMetalHostStatus{
					Provisioning: metal3api.ProvisionStatus{
						CustomDeploy: &metal3api.CustomDeploy{
							Method: "install_everything",
						},
					},
				},
			},
			Expected: true,
		},

		{
			Scenario: "changed image with custom deploy",
			Host: metal3api.BareMetalHost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "myhost",
					Namespace: "myns",
				},
				Spec: metal3api.BareMetalHostSpec{
					Image: &metal3api.Image{
						URL: "not-empty",
					},
					CustomDeploy: &metal3api.CustomDeploy{
						Method: "install_everything",
					},
					Online: true,
				},
				Status: metal3api.BareMetalHostStatus{
					Provisioning: metal3api.ProvisionStatus{
						Image: metal3api.Image{
							URL: "also-not-empty",
						},
						CustomDeploy: &metal3api.CustomDeploy{
							Method: "install_everything",
						},
					},
				},
			},
			Expected: true,
		},

		{
			Scenario: "changed custom deploy with image",
			Host: metal3api.BareMetalHost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "myhost",
					Namespace: "myns",
				},
				Spec: metal3api.BareMetalHostSpec{
					Image: &metal3api.Image{
						URL: "not-empty",
					},
					CustomDeploy: &metal3api.CustomDeploy{
						Method: "install_not_everything",
					},
					Online: true,
				},
				Status: metal3api.BareMetalHostStatus{
					Provisioning: metal3api.ProvisionStatus{
						Image: metal3api.Image{
							URL: "not-empty",
						},
						CustomDeploy: &metal3api.CustomDeploy{
							Method: "install_everything",
						},
					},
				},
			},
			Expected: true,
		},

		{
			Scenario: "removed custom deploy with image",
			Host: metal3api.BareMetalHost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "myhost",
					Namespace: "myns",
				},
				Spec: metal3api.BareMetalHostSpec{
					Image: &metal3api.Image{
						URL: "not-empty",
					},
					Online: true,
				},
				Status: metal3api.BareMetalHostStatus{
					Provisioning: metal3api.ProvisionStatus{
						Image: metal3api.Image{
							URL: "not-empty",
						},
						CustomDeploy: &metal3api.CustomDeploy{
							Method: "install_everything",
						},
					},
				},
			},
			Expected: true,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.Scenario, func(t *testing.T) {
			host := tc.Host
			hsm := testStateMachine(&host)
			actual := hsm.provisioningCancelled()
			if tc.Expected && !actual {
				t.Error("expected to need deprovisioning")
			}
			if !tc.Expected && actual {
				t.Error("did not expect to need deprovisioning")
			}
		})
	}
}

func TestErrorCountIncreasedOnActionFailure(t *testing.T) {
	defaultError := "some error"
	poweroffError := "some details"
	tests := []struct {
		Scenario           string
		Host               *metal3api.BareMetalHost
		ProvisionerErrorOn string
		originalError      string
		ExpectedError      string
	}{
		{
			Scenario:           "registration",
			Host:               host(metal3api.StateRegistering).build(),
			ProvisionerErrorOn: "ValidateManagementAccess",
			originalError:      defaultError,
			ExpectedError:      defaultError,
		},
		{
			Scenario:           "inspecting",
			Host:               host(metal3api.StateInspecting).build(),
			ProvisionerErrorOn: "InspectHardware",
			originalError:      defaultError,
			ExpectedError:      defaultError,
		},
		{
			Scenario:           "provisioning",
			Host:               host(metal3api.StateProvisioning).SetImageURL("imageSpecUrl").build(),
			ProvisionerErrorOn: "Provision",
			originalError:      defaultError,
			ExpectedError:      defaultError,
		},
		{
			Scenario:           "deprovisioning",
			Host:               host(metal3api.StateDeprovisioning).build(),
			ProvisionerErrorOn: "Deprovision",
			originalError:      defaultError,
			ExpectedError:      defaultError,
		},
		{
			Scenario:           "available-power-on",
			Host:               host(metal3api.StateAvailable).SetImageURL("").SetStatusPoweredOn(false).build(),
			ProvisionerErrorOn: "PowerOn",
			originalError:      defaultError,
			ExpectedError:      defaultError,
		},
		{
			Scenario:           "available-power-off",
			Host:               host(metal3api.StateAvailable).SetImageURL("").SetOnline(false).build(),
			ProvisionerErrorOn: "PowerOff",
			originalError:      poweroffError,
			ExpectedError:      clarifySoftPoweroffFailure + poweroffError,
		},
		{
			Scenario:           "deprecated-ready-power-on",
			Host:               host(metal3api.StateReady).SetImageURL("").SetStatusPoweredOn(false).build(),
			ProvisionerErrorOn: "PowerOn",
			originalError:      defaultError,
			ExpectedError:      defaultError,
		},
		{
			Scenario:           "deprecated-ready-power-off",
			Host:               host(metal3api.StateReady).SetImageURL("").SetOnline(false).build(),
			ProvisionerErrorOn: "PowerOff",
			originalError:      poweroffError,
			ExpectedError:      clarifySoftPoweroffFailure + poweroffError,
		},
		{
			Scenario:           "externally-provisioned-adopt-failed",
			Host:               host(metal3api.StateExternallyProvisioned).SetExternallyProvisioned().build(),
			ProvisionerErrorOn: "Adopt",
			originalError:      defaultError,
			ExpectedError:      defaultError,
		},
		{
			Scenario:           "provisioned-adopt-failed",
			Host:               host(metal3api.StateProvisioned).build(),
			ProvisionerErrorOn: "Adopt",
			originalError:      defaultError,
			ExpectedError:      defaultError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.Scenario, func(t *testing.T) {
			prov := newMockProvisioner()
			reconciler := testNewReconciler(tt.Host)
			hsm := newHostStateMachine(tt.Host, reconciler, prov, true)
			info := makeDefaultReconcileInfo(tt.Host)

			prov.setNextError(tt.ProvisionerErrorOn, tt.originalError)
			result := hsm.ReconcileState(info)

			assert.Equal(t, 1, tt.Host.Status.ErrorCount)
			assert.Equal(t, tt.ExpectedError, tt.Host.Status.ErrorMessage)
			assert.True(t, result.Dirty())
		})
	}
}

func TestErrorCountClearedOnStateTransition(t *testing.T) {
	tests := []struct {
		Scenario                     string
		Host                         *metal3api.BareMetalHost
		TargetState                  metal3api.ProvisioningState
		PreserveErrorCountOnComplete bool
	}{
		{
			Scenario:    "registering-to-inspecting",
			Host:        host(metal3api.StateRegistering).build(),
			TargetState: metal3api.StateInspecting,
		},
		{
			Scenario:    "registering-to-preparing",
			Host:        host(metal3api.StateRegistering).DisableInspection().build(),
			TargetState: metal3api.StatePreparing,
		},
		{
			Scenario:    "inspecting-to-preparing",
			Host:        host(metal3api.StateInspecting).build(),
			TargetState: metal3api.StatePreparing,
		},
		{
			Scenario:    "matchprofile-to-preparing",
			Host:        host(metal3api.StateMatchProfile).build(),
			TargetState: metal3api.StatePreparing,
		},
		{
			Scenario:    "preparing-to-ready",
			Host:        host(metal3api.StatePreparing).build(),
			TargetState: metal3api.StateAvailable,
		},
		{
			Scenario:    "provisioning-to-provisioned",
			Host:        host(metal3api.StateProvisioning).build(),
			TargetState: metal3api.StateProvisioned,
		},
		{
			Scenario:    "deprovisioning-to-ready",
			Host:        host(metal3api.StateDeprovisioning).build(),
			TargetState: metal3api.StateAvailable,
		},
		{
			Scenario:    "deprovisioning-to-powering-off",
			Host:        host(metal3api.StateDeprovisioning).setDeletion().withFinalizer().build(),
			TargetState: metal3api.StatePoweringOffBeforeDelete,
		},
		{
			Scenario:    "powering-off-to-deleting",
			Host:        host(metal3api.StatePoweringOffBeforeDelete).setDeletion().withFinalizer().build(),
			TargetState: metal3api.StateDeleting,
		},
	}
	for _, tt := range tests {
		t.Run(tt.Scenario, func(t *testing.T) {
			prov := newMockProvisioner()
			reconciler := testNewReconciler(tt.Host)
			hsm := newHostStateMachine(tt.Host, reconciler, prov, true)
			info := makeDefaultReconcileInfo(tt.Host)

			info.host.Status.ErrorCount = 1
			hsm.ReconcileState(info)

			assert.Equal(t, tt.TargetState, info.host.Status.Provisioning.State)
			assert.Equal(t, info.host.Status.ErrorCount, 0)
		})
	}
}

func TestErrorClean(t *testing.T) {
	tests := []struct {
		Scenario    string
		Host        *metal3api.BareMetalHost
		SecretName  string
		ExpectError bool
	}{
		{
			Scenario: "clean-after-registration-error",
			Host: host(metal3api.StateInspecting).
				SetStatusError(metal3api.OperationalStatusError, metal3api.RegistrationError, "some error", 1).
				build(),
		},
		{
			Scenario: "clean-after-creds-change",
			Host: host(metal3api.StateAvailable).
				SetStatusError(metal3api.OperationalStatusError, metal3api.InspectionError, "some error", 1).
				build(),
			SecretName: "NewCreds",
		},
	}
	for _, tt := range tests {
		t.Run(tt.Scenario, func(t *testing.T) {
			prov := newMockProvisioner()
			hsm := newHostStateMachine(tt.Host, &BareMetalHostReconciler{
				Client: fakeclient.NewFakeClient(),
			}, prov, true)

			info := makeDefaultReconcileInfo(tt.Host)
			if tt.SecretName != "" {
				info.bmcCredsSecret.Name = tt.SecretName
			}

			hsm.ReconcileState(info)

			if tt.ExpectError {
				assert.Equal(t, metal3api.ProvisionedRegistrationError, tt.Host.Status.ErrorType)
				assert.NotEmpty(t, tt.Host.Status.ErrorMessage)
			} else {
				assert.Equal(t, metal3api.OperationalStatusOK, tt.Host.Status.OperationalStatus)
				assert.Empty(t, tt.Host.Status.ErrorType)
				assert.Empty(t, tt.Host.Status.ErrorMessage)
			}
		})
	}
}

func TestDeleteWaitsForDetach(t *testing.T) {
	tests := []struct {
		Scenario                  string
		Host                      *metal3api.BareMetalHost
		ExpectedState             metal3api.ProvisioningState
		ExpectedOperationalStatus metal3api.OperationalStatus
	}{
		{
			Scenario: "detached-delay",
			Host: host(metal3api.StateProvisioned).
				SetOperationalStatus(metal3api.OperationalStatusDetached).
				setDeletion().
				setDetached("{\"deleteAction\": \"delay\"}").
				build(),
			ExpectedState:             metal3api.StateProvisioned,
			ExpectedOperationalStatus: metal3api.OperationalStatusDetached,
		},
		{
			Scenario: "detached-delete",
			Host: host(metal3api.StateProvisioned).
				SetOperationalStatus(metal3api.OperationalStatusDetached).
				setDeletion().
				setDetached("{\"deleteAction\": \"delete\"}").
				build(),
			ExpectedState:             metal3api.StateDeleting,
			ExpectedOperationalStatus: metal3api.OperationalStatusDetached,
		},
		{
			Scenario: "detached-not-json",
			Host: host(metal3api.StateProvisioned).
				SetOperationalStatus(metal3api.OperationalStatusDetached).
				setDeletion().
				setDetached("true").
				build(),
			ExpectedState:             metal3api.StateDeleting,
			ExpectedOperationalStatus: metal3api.OperationalStatusDetached,
		},
		{
			Scenario: "detached-no-annotation",
			Host: host(metal3api.StateProvisioned).
				SetOperationalStatus(metal3api.OperationalStatusDetached).
				setDeletion().
				build(),
			ExpectedState:             metal3api.StateProvisioned,
			ExpectedOperationalStatus: metal3api.OperationalStatusOK,
		},
		{
			Scenario: "attached",
			Host: host(metal3api.StateProvisioned).
				setDeletion().
				build(),
			ExpectedState:             metal3api.StateDeprovisioning,
			ExpectedOperationalStatus: metal3api.OperationalStatusOK,
		},
	}
	for _, tt := range tests {
		t.Run(tt.Scenario, func(t *testing.T) {
			prov := newMockProvisioner()
			hsm := newHostStateMachine(tt.Host, &BareMetalHostReconciler{
				Client: fakeclient.NewFakeClient(),
			}, prov, true)

			info := makeDefaultReconcileInfo(tt.Host)
			hsm.ReconcileState(info)

			assert.Equal(t, tt.ExpectedState, tt.Host.Status.Provisioning.State)
			assert.Equal(t, tt.ExpectedOperationalStatus, tt.Host.Status.OperationalStatus)
		})
	}
}

type hostBuilder struct {
	metal3api.BareMetalHost
}

func host(state metal3api.ProvisioningState) *hostBuilder {
	creds := metal3api.CredentialsStatus{
		Reference: &corev1.SecretReference{
			Name:      "secretRefName",
			Namespace: "secretNs",
		},
		Version: "100",
	}

	return &hostBuilder{
		metal3api.BareMetalHost{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "foo",
				Namespace: "bar",
			},
			Spec: metal3api.BareMetalHostSpec{
				Online: true,
				Image: &metal3api.Image{
					URL: "not-empty",
				},
				RootDeviceHints: &metal3api.RootDeviceHints{},
			},
			Status: metal3api.BareMetalHostStatus{
				HardwareProfile: profile.DefaultProfileName,
				Provisioning: metal3api.ProvisionStatus{
					State:           state,
					BootMode:        metal3api.DefaultBootMode,
					RootDeviceHints: &metal3api.RootDeviceHints{},
					Image: metal3api.Image{
						URL: "", // needs provisioning
					},
					RAID: &metal3api.RAIDConfig{
						SoftwareRAIDVolumes: []metal3api.SoftwareRAIDVolume{},
					},
				},
				GoodCredentials:   creds,
				TriedCredentials:  creds,
				OperationalStatus: metal3api.OperationalStatusOK,
				PoweredOn:         true,
			},
		},
	}
}

func (hb *hostBuilder) build() *metal3api.BareMetalHost {
	return &hb.BareMetalHost
}

func (hb *hostBuilder) SaveHostProvisioningSettings() *hostBuilder {
	info := makeDefaultReconcileInfo(&hb.BareMetalHost)
	saveHostProvisioningSettings(&hb.BareMetalHost, info)
	return hb
}

func (hb *hostBuilder) SetTriedCredentials() *hostBuilder {
	hb.Status.TriedCredentials = hb.Status.GoodCredentials
	return hb
}

func (hb *hostBuilder) SetExternallyProvisioned() *hostBuilder {
	hb.Spec.ExternallyProvisioned = true
	return hb
}

func (hb *hostBuilder) SetImageURL(url string) *hostBuilder {
	hb.Spec.Image = &metal3api.Image{
		URL: url,
	}
	return hb
}

func (hb *hostBuilder) SetStatusError(opStatus metal3api.OperationalStatus, errType metal3api.ErrorType, errMsg string, errCount int) *hostBuilder {
	hb.Status.OperationalStatus = opStatus
	hb.Status.ErrorType = errType
	hb.Status.ErrorMessage = errMsg
	hb.Status.ErrorCount = errCount

	return hb
}

func (hb *hostBuilder) SetStatusImageURL(url string) *hostBuilder {
	hb.Status.Provisioning.Image.URL = url
	return hb
}

func (hb *hostBuilder) SetStatusPoweredOn(status bool) *hostBuilder {
	hb.Status.PoweredOn = status
	return hb
}

func (hb *hostBuilder) SetOnline(status bool) *hostBuilder {
	hb.Spec.Online = status
	return hb
}

func (hb *hostBuilder) SetOperationalStatus(status metal3api.OperationalStatus) *hostBuilder {
	hb.Status.OperationalStatus = status
	return hb
}

func (hb *hostBuilder) DisableInspection() *hostBuilder {
	if hb.Annotations == nil {
		hb.Annotations = make(map[string]string, 1)
	}
	hb.Annotations[metal3api.InspectAnnotationPrefix] = "disabled"
	return hb
}

func (hb *hostBuilder) setDeletion() *hostBuilder {
	date := metav1.Date(2021, time.January, 18, 10, 18, 0, 0, time.UTC)
	hb.DeletionTimestamp = &date
	return hb
}

func (hb *hostBuilder) withFinalizer() *hostBuilder {
	hb.Finalizers = []string{"test"}
	return hb
}

func (hb *hostBuilder) setDetached(val string) *hostBuilder {
	if hb.Annotations == nil {
		hb.Annotations = make(map[string]string, 1)
	}
	hb.Annotations[metal3api.DetachedAnnotation] = val
	return hb
}

func makeDefaultReconcileInfo(host *metal3api.BareMetalHost) *reconcileInfo {
	return &reconcileInfo{
		log:     logf.Log.WithName("controllers").WithName("BareMetalHost").WithName("host_state_machine"),
		host:    host,
		request: ctrl.Request{},
		bmcCredsSecret: &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:            host.Status.GoodCredentials.Reference.Name,
				Namespace:       host.Status.GoodCredentials.Reference.Namespace,
				ResourceVersion: host.Status.GoodCredentials.Version,
			},
		},
	}
}

func newMockProvisioner() *mockProvisioner {
	return &mockProvisioner{
		hasCapacity:  true,
		nextResults:  make(map[string]provisioner.Result),
		callsNoError: make(map[string]bool),
	}
}

type mockProvisioner struct {
	hasCapacity  bool
	nextResults  map[string]provisioner.Result
	callsNoError map[string]bool
}

func (m *mockProvisioner) getNextResultByMethod(name string) (result provisioner.Result) {
	if value, ok := m.nextResults[name]; ok {
		result = value
	} else {
		m.callsNoError[name] = true
	}
	return
}

func (m *mockProvisioner) setHasCapacity(hasCapacity bool) {
	m.hasCapacity = hasCapacity
}

func (m *mockProvisioner) HasCapacity() (result bool, err error) {
	return m.hasCapacity, nil
}

func (m *mockProvisioner) setNextError(methodName, msg string) {
	m.nextResults[methodName] = provisioner.Result{
		ErrorMessage: msg,
	}
}

func (m *mockProvisioner) clearNextError(methodName string) {
	m.nextResults[methodName] = provisioner.Result{}
}

func (m *mockProvisioner) calledNoError(methodName string) bool {
	return m.callsNoError[methodName]
}

func (m *mockProvisioner) ValidateManagementAccess(_ provisioner.ManagementAccessData, _, _ bool) (result provisioner.Result, provID string, err error) {
	return m.getNextResultByMethod("ValidateManagementAccess"), "", err
}

func (m *mockProvisioner) PreprovisioningImageFormats() ([]metal3api.ImageFormat, error) {
	return nil, nil
}

func (m *mockProvisioner) InspectHardware(_ provisioner.InspectData, _, _, _ bool) (result provisioner.Result, started bool, details *metal3api.HardwareDetails, err error) {
	details = &metal3api.HardwareDetails{}
	return m.getNextResultByMethod("InspectHardware"), true, details, err
}

func (m *mockProvisioner) UpdateHardwareState() (hwState provisioner.HardwareState, err error) {
	return
}

func (m *mockProvisioner) Prepare(_ provisioner.PrepareData, _ bool, _ bool) (result provisioner.Result, started bool, err error) {
	return m.getNextResultByMethod("Prepare"), m.nextResults["Prepare"].Dirty, err
}

func (m *mockProvisioner) Adopt(_ provisioner.AdoptData, _ bool) (result provisioner.Result, err error) {
	return m.getNextResultByMethod("Adopt"), err
}

func (m *mockProvisioner) Provision(_ provisioner.ProvisionData, _ bool) (result provisioner.Result, err error) {
	return m.getNextResultByMethod("Provision"), err
}

func (m *mockProvisioner) Deprovision(_ bool) (result provisioner.Result, err error) {
	return m.getNextResultByMethod("Deprovision"), err
}

func (m *mockProvisioner) Delete() (result provisioner.Result, err error) {
	return m.getNextResultByMethod("Delete"), err
}

func (m *mockProvisioner) Detach() (result provisioner.Result, err error) {
	res := m.getNextResultByMethod("Detach")
	return res, err
}

func (m *mockProvisioner) PowerOn(_ bool) (result provisioner.Result, err error) {
	return m.getNextResultByMethod("PowerOn"), err
}

func (m *mockProvisioner) PowerOff(_ metal3api.RebootMode, _ bool) (result provisioner.Result, err error) {
	return m.getNextResultByMethod("PowerOff"), err
}

func (m *mockProvisioner) TryInit() (result bool, err error) {
	return
}

func (m *mockProvisioner) GetFirmwareSettings(_ bool) (settings metal3api.SettingsMap, schema map[string]metal3api.SettingSchema, err error) {
	return
}

func (m *mockProvisioner) AddBMCEventSubscriptionForNode(_ *metal3api.BMCEventSubscription, _ provisioner.HTTPHeaders) (result provisioner.Result, err error) {
	return result, nil
}

func (m *mockProvisioner) RemoveBMCEventSubscriptionForNode(_ metal3api.BMCEventSubscription) (result provisioner.Result, err error) {
	return result, nil
}

func (p *mockProvisioner) GetFirmwareComponents() (components []metal3api.FirmwareComponentStatus, err error) {
	return components, nil
}

func TestUpdateBootModeStatus(t *testing.T) {
	testCases := []struct {
		Scenario       string
		SpecValue      metal3api.BootMode
		StatusValue    metal3api.BootMode
		ExpectedValue  metal3api.BootMode
		ExpectedChange bool
	}{
		{
			Scenario:       "default",
			SpecValue:      "",
			StatusValue:    "",
			ExpectedValue:  metal3api.DefaultBootMode,
			ExpectedChange: true,
		},

		{
			Scenario:       "set UEFI",
			SpecValue:      metal3api.UEFI,
			StatusValue:    "",
			ExpectedValue:  metal3api.UEFI,
			ExpectedChange: true,
		},

		{
			Scenario:       "already UEFI",
			SpecValue:      metal3api.UEFI,
			StatusValue:    metal3api.UEFI,
			ExpectedValue:  metal3api.UEFI,
			ExpectedChange: false,
		},

		{
			Scenario:       "set Legacy",
			SpecValue:      metal3api.Legacy,
			StatusValue:    "",
			ExpectedValue:  metal3api.Legacy,
			ExpectedChange: true,
		},

		{
			Scenario:       "already Legacy",
			SpecValue:      metal3api.Legacy,
			StatusValue:    metal3api.Legacy,
			ExpectedValue:  metal3api.Legacy,
			ExpectedChange: false,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.Scenario, func(t *testing.T) {
			host := metal3api.BareMetalHost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "myhost",
					Namespace: "myns",
				},
				Spec: metal3api.BareMetalHostSpec{
					Image: &metal3api.Image{
						URL: "not-empty",
					},
					Online:   true,
					BootMode: tc.SpecValue,
				},
				Status: metal3api.BareMetalHostStatus{
					Provisioning: metal3api.ProvisionStatus{
						Image: metal3api.Image{
							URL: "also-not-empty",
						},
						BootMode: tc.StatusValue,
					},
				},
			}
			changed := updateBootModeStatus(&host)
			assert.Equal(t, tc.ExpectedChange, changed, "unexpected change response")
			assert.Equal(t, tc.ExpectedValue, host.Status.Provisioning.BootMode)
		})
	}
}
