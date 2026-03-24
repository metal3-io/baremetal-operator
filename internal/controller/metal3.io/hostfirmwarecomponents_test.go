package controllers

import (
	"testing"

	"github.com/go-logr/logr"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner/fixture"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestHFCReconcile(t *testing.T) {

	testBMH := &metal3api.BareMetalHost{
		ObjectMeta: metav1.ObjectMeta{Name: hostName, Namespace: hostNamespace},
		Status: metal3api.BareMetalHostStatus{
			Provisioning: metal3api.ProvisionStatus{
				State: metal3api.StateAvailable,
			},
		},
	}

	testCases := map[string]struct {
		apiObjects []client.Object                   // Objects to seed in the mock Kubernetes API, before reconcile
		want       *metal3api.HostFirmwareComponents // Expected HFC after reconciliation
		retResult  ctrl.Result                       // Expected reconcile return result
	}{
		"update bmc": {
			apiObjects: []client.Object{
				&metal3api.HostFirmwareComponents{
					ObjectMeta: metav1.ObjectMeta{
						Name:      hostName,
						Namespace: hostNamespace,
					},
					Spec: metal3api.HostFirmwareComponentsSpec{
						Updates: []metal3api.FirmwareUpdate{
							{
								Component: "bmc",
								URL:       "https://myurl/newbmcfirmware",
							},
						},
					},
				},
				testBMH,
			},
			want: &metal3api.HostFirmwareComponents{
				ObjectMeta: metav1.ObjectMeta{Name: hostName, Namespace: hostNamespace},
				Spec: metal3api.HostFirmwareComponentsSpec{
					Updates: []metal3api.FirmwareUpdate{
						{
							Component: "bmc",
							URL:       "https://myurl/newbmcfirmware",
						},
					},
				},
				Status: metal3api.HostFirmwareComponentsStatus{
					Components: []metal3api.FirmwareComponentStatus{}, // Overwritten by mockComponents
					Conditions: []metav1.Condition{
						{Type: "ChangeDetected", Status: "True", Reason: "OK", Message: ""},
						{Type: "Valid", Status: "True", Reason: "OK", Message: ""},
					},
				},
			},
			retResult: ctrl.Result{Requeue: true, RequeueAfter: reconcilerRequeueDelayChangeDetected},
		},
		"update bios": {
			apiObjects: []client.Object{
				&metal3api.HostFirmwareComponents{
					ObjectMeta: metav1.ObjectMeta{
						Name:      hostName,
						Namespace: hostNamespace,
					},
					Spec: metal3api.HostFirmwareComponentsSpec{
						Updates: []metal3api.FirmwareUpdate{
							{
								Component: "bios",
								URL:       "https://myurl/newbiosfirmware",
							},
						},
					},
				},
				testBMH,
			},
			want: &metal3api.HostFirmwareComponents{
				ObjectMeta: metav1.ObjectMeta{
					Name:      hostName,
					Namespace: hostNamespace,
				},
				Spec: metal3api.HostFirmwareComponentsSpec{
					Updates: []metal3api.FirmwareUpdate{
						{
							Component: "bios",
							URL:       "https://myurl/newbiosfirmware",
						},
					},
				},
				Status: metal3api.HostFirmwareComponentsStatus{
					Components: []metal3api.FirmwareComponentStatus{}, // Overwritten by mockComponents
					Conditions: []metav1.Condition{
						{Type: "ChangeDetected", Status: "True", Reason: "OK", Message: ""},
						{Type: "Valid", Status: "True", Reason: "OK", Message: ""},
					},
				},
			},
			retResult: ctrl.Result{Requeue: true, RequeueAfter: reconcilerRequeueDelayChangeDetected},
		},
		"update nic": {
			apiObjects: []client.Object{
				&metal3api.HostFirmwareComponents{
					ObjectMeta: metav1.ObjectMeta{
						Name:            hostName,
						Namespace:       hostNamespace,
						ResourceVersion: "1"},
					Spec: metal3api.HostFirmwareComponentsSpec{
						Updates: []metal3api.FirmwareUpdate{
							{
								Component: "nic:identifier",
								URL:       "https://myurl/newnicfirmware",
							},
						},
					},
				},
				testBMH,
			},
			want: &metal3api.HostFirmwareComponents{
				ObjectMeta: metav1.ObjectMeta{
					Name:      hostName,
					Namespace: hostNamespace,
				},
				Spec: metal3api.HostFirmwareComponentsSpec{
					Updates: []metal3api.FirmwareUpdate{
						{
							Component: "nic:identifier",
							URL:       "https://myurl/newnicfirmware",
						},
					},
				},
				Status: metal3api.HostFirmwareComponentsStatus{
					Updates: []metal3api.FirmwareUpdate{
						{
							Component: "nic:identifier",
							URL:       "https://myurl/newnicfirmware",
						},
					},
					Components: []metal3api.FirmwareComponentStatus{}, //overwrite
					Conditions: []metav1.Condition{
						{Type: "ChangeDetected", Status: "True", Reason: "OK", Message: ""},
						{Type: "Valid", Status: "True", Reason: "OK", Message: ""},
					},
				},
			},
			retResult: ctrl.Result{Requeue: true, RequeueAfter: reconcilerRequeueDelayChangeDetected},
		},
		"update all": {
			apiObjects: []client.Object{
				&metal3api.HostFirmwareComponents{
					ObjectMeta: metav1.ObjectMeta{
						Name:      hostName,
						Namespace: hostNamespace,
					},
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
							{
								Component: "nic:identifier",
								URL:       "https://myurl/newnicfirmware",
							},
						},
					},
				},
				testBMH,
			},
			want: &metal3api.HostFirmwareComponents{
				ObjectMeta: metav1.ObjectMeta{
					Name:      hostName,
					Namespace: hostNamespace,
				},
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
						{
							Component: "nic:identifier",
							URL:       "https://myurl/newnicfirmware",
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
						{
							Component: "nic:identifier",
							URL:       "https://myurl/newnicfirmware",
						},
					},
					Components: []metal3api.FirmwareComponentStatus{}, // Overwritten by mockComponents
					Conditions: []metav1.Condition{
						{Type: "ChangeDetected", Status: "True", Reason: "OK", Message: ""},
						{Type: "Valid", Status: "True", Reason: "OK", Message: ""},
					},
				},
			},
			retResult: ctrl.Result{Requeue: true, RequeueAfter: reconcilerRequeueDelayChangeDetected},
		},
		"invalid component": {
			apiObjects: []client.Object{
				&metal3api.HostFirmwareComponents{
					ObjectMeta: metav1.ObjectMeta{
						Name:      hostName,
						Namespace: hostNamespace,
					},
					Spec: metal3api.HostFirmwareComponentsSpec{
						Updates: []metal3api.FirmwareUpdate{
							{
								Component: "something",
								URL:       "https://myurl/myfw",
							},
						},
					},
				},
				testBMH,
			},
			want: &metal3api.HostFirmwareComponents{
				ObjectMeta: metav1.ObjectMeta{
					Name:      hostName,
					Namespace: hostNamespace,
				},
				Spec: metal3api.HostFirmwareComponentsSpec{
					Updates: []metal3api.FirmwareUpdate{
						{
							Component: "something",
							URL:       "https://myurl/myfw",
						},
					},
				},
				Status: metal3api.HostFirmwareComponentsStatus{
					Components: []metal3api.FirmwareComponentStatus{}, // Overwritten by mockComponents
					Conditions: []metav1.Condition{
						{Type: "ChangeDetected", Status: "True", Reason: "OK", Message: ""},
						{Type: "Valid", Status: "False", Reason: "InvalidComponent", Message: "Invalid Firmware Components: ['something' is not a valid component name, allowed: 'bmc', 'bios', 'nic', or names starting with 'nic:']"},
					},
				},
			},
			retResult: ctrl.Result{Requeue: true, RequeueAfter: reconcilerRequeueDelayChangeDetected},
		},
		"bmh not found": {
			apiObjects: []client.Object{
				&metal3api.HostFirmwareComponents{
					ObjectMeta: metav1.ObjectMeta{
						Name:      hostName,
						Namespace: hostNamespace,
					},
					Spec: metal3api.HostFirmwareComponentsSpec{
						Updates: []metal3api.FirmwareUpdate{
							{
								Component: "bmc",
								URL:       "https://myurl/newbmcfirmware",
							},
						},
					},
				},
			},
			retResult: ctrl.Result{},
		},
		"hfc deleted": {
			// BMH exists (deprovisioning), HFC has been deleted — reconciler must not error
			apiObjects: []client.Object{
				&metal3api.BareMetalHost{
					ObjectMeta: metav1.ObjectMeta{Name: hostName, Namespace: hostNamespace},
					Status: metal3api.BareMetalHostStatus{
						Provisioning: metal3api.ProvisionStatus{
							State: metal3api.StateDeprovisioning,
						},
					},
				},
			},
			want:      nil,
			retResult: ctrl.Result{},
		},
	}

	for scenario, tc := range testCases {
		t.Run(scenario, func(t *testing.T) {
			ctx := t.Context()

			builder := fakeclient.NewClientBuilder().WithScheme(scheme.Scheme)
			for _, o := range tc.apiObjects {
				builder = builder.WithRuntimeObjects(o).WithStatusSubresource(o)
			}
			fakeClient := builder.Build()

			// mockComponents is arbitrary - the test verifies that whatever the provisioner
			// returns gets copied to Status.Components, not the specific values
			mockComponents := []metal3api.FirmwareComponentStatus{
				{
					Component:          "any",
					InitialVersion:     "1.0.0",
					CurrentVersion:     "1.2.0",
					LastVersionFlashed: "1.2.0",
				},
			}
			fakeProvisioner := &fixture.Fixture{
				HostFirmwareComponents: fixture.HostFirmwareComponentsMock{
					Components: mockComponents,
				},
			}
			if tc.want != nil {
				tc.want.Status.Components = mockComponents
			}

			r := &HostFirmwareComponentsReconciler{
				Client:             fakeClient,
				ProvisionerFactory: fakeProvisioner,
				Log:                logr.Discard(),
			}

			request := ctrl.Request{
				NamespacedName: client.ObjectKey{
					Namespace: hostNamespace,
					Name:      hostName,
				},
			}

			result, err := r.Reconcile(ctx, request)
			require.NoError(t, err)

			if diff := cmp.Diff(tc.retResult, result); diff != "" {
				t.Errorf("Reconcile result mismatch (-want +got):\n%s", diff)
			}

			if tc.want != nil {
				key := client.ObjectKey{Namespace: hostNamespace, Name: hostName}
				got := &metal3api.HostFirmwareComponents{}
				err = r.Get(ctx, key, got)
				require.NoError(t, err)

				diff := cmp.Diff(tc.want, got,
					cmpopts.IgnoreFields(metav1.ObjectMeta{}, "ResourceVersion"),
					cmpopts.IgnoreFields(metav1.Condition{}, "LastTransitionTime"),
					cmpopts.IgnoreFields(metal3api.HostFirmwareComponentsStatus{}, "LastUpdated"),
					cmpopts.IgnoreFields(metal3api.HostFirmwareComponentsStatus{}, "Updates"),
				)
				if diff != "" {
					t.Errorf("HostFirmwareComponents mismatch (-want +got):\n%s", diff)
				}
			}
		})
	}
}
