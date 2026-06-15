/*

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package webhooks

import (
	"context"
	"fmt"
	"strings"
	"testing"

	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestValidateAttachment(t *testing.T) {
	testCases := []struct {
		name          string
		attachment    *metal3api.HostNetworkAttachment
		expectedError bool
		errorContains string
	}{
		{
			name: "valid-access-mode",
			attachment: &metal3api.HostNetworkAttachment{
				Spec: metal3api.HostNetworkAttachmentSpec{
					Mode:       metal3api.SwitchportModeAccess,
					NativeVLAN: 100,
				},
			},
			expectedError: false,
		},
		{
			name: "valid-trunk-mode-with-vlans",
			attachment: &metal3api.HostNetworkAttachment{
				Spec: metal3api.HostNetworkAttachmentSpec{
					Mode:         metal3api.SwitchportModeTrunk,
					NativeVLAN:   1,
					AllowedVLANs: []int{10, 20, 30},
				},
			},
			expectedError: false,
		},
		{
			name: "valid-hybrid-mode",
			attachment: &metal3api.HostNetworkAttachment{
				Spec: metal3api.HostNetworkAttachmentSpec{
					Mode:         metal3api.SwitchportModeHybrid,
					NativeVLAN:   100,
					AllowedVLANs: []int{200, 300},
				},
			},
			expectedError: false,
		},
		{
			name: "valid-mtu",
			attachment: &metal3api.HostNetworkAttachment{
				Spec: metal3api.HostNetworkAttachmentSpec{
					Mode:       metal3api.SwitchportModeAccess,
					NativeVLAN: 1,
					MTU:        ptr.To(9000),
				},
			},
			expectedError: false,
		},
		{
			name: "invalid-access-mode-with-allowed-vlans",
			attachment: &metal3api.HostNetworkAttachment{
				Spec: metal3api.HostNetworkAttachmentSpec{
					Mode:         metal3api.SwitchportModeAccess,
					NativeVLAN:   100,
					AllowedVLANs: []int{200},
				},
			},
			expectedError: true,
			errorContains: "allowedVlans cannot be specified for access mode",
		},
		// VLAN range (min/max) and mode enum validation are now handled by
		// CRD schema markers and tested at the API server admission level.
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			webhook := &HostNetworkAttachment{}
			errs := webhook.validateAttachment(tc.attachment)

			if tc.expectedError {
				assert.NotEmpty(t, errs, "expected validation errors")
				if tc.errorContains != "" {
					found := false
					for _, err := range errs {
						if strings.Contains(err.Error(), tc.errorContains) {
							found = true
							break
						}
					}
					assert.True(t, found, "expected error to contain: %s", tc.errorContains)
				}
			} else {
				assert.Empty(t, errs, "expected no validation errors")
			}
		})
	}
}

func TestFindBMHReferences(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = metal3api.AddToScheme(scheme)

	attachment := &metal3api.HostNetworkAttachment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-attachment",
			Namespace: "test-ns",
		},
	}

	bmhWithReference := &metal3api.BareMetalHost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "host-with-ref",
			Namespace: "test-ns",
		},
		Spec: metal3api.BareMetalHostSpec{
			NetworkInterfaces: []metal3api.NetworkInterface{
				{
					Name: "eth0",
					HostNetworkAttachment: metal3api.HostNetworkAttachmentRef{
						Name: "test-attachment",
					},
				},
			},
		},
	}

	bmhWithoutReference := &metal3api.BareMetalHost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "host-without-ref",
			Namespace: "test-ns",
		},
		Spec: metal3api.BareMetalHostSpec{},
	}

	bmhWithDifferentAttachment := &metal3api.BareMetalHost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "host-different-ref",
			Namespace: "test-ns",
		},
		Spec: metal3api.BareMetalHostSpec{
			NetworkInterfaces: []metal3api.NetworkInterface{
				{
					Name: "eth0",
					HostNetworkAttachment: metal3api.HostNetworkAttachmentRef{
						Name: "other-attachment",
					},
				},
			},
		},
	}

	testCases := []struct {
		name              string
		bmhs              []metal3api.BareMetalHost
		expectedRefsCount int
		expectedRefs      []string
	}{
		{
			name:              "no-bmhs",
			bmhs:              []metal3api.BareMetalHost{},
			expectedRefsCount: 0,
		},
		{
			name:              "one-bmh-with-reference",
			bmhs:              []metal3api.BareMetalHost{*bmhWithReference},
			expectedRefsCount: 1,
			expectedRefs:      []string{"test-ns/host-with-ref[eth0]"},
		},
		{
			name:              "one-bmh-without-reference",
			bmhs:              []metal3api.BareMetalHost{*bmhWithoutReference},
			expectedRefsCount: 0,
		},
		{
			name:              "mixed-bmhs",
			bmhs:              []metal3api.BareMetalHost{*bmhWithReference, *bmhWithoutReference, *bmhWithDifferentAttachment},
			expectedRefsCount: 1,
			expectedRefs:      []string{"test-ns/host-with-ref[eth0]"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			objs := []runtime.Object{attachment}
			for i := range tc.bmhs {
				objs = append(objs, &tc.bmhs[i])
			}

			webhook := &HostNetworkAttachment{
				Client: fakeclient.NewClientBuilder().
					WithScheme(scheme).
					WithRuntimeObjects(objs...).
					WithIndex(&metal3api.BareMetalHost{}, bmhNetworkAttachmentIndexField, func(obj client.Object) []string {
						bmh, _ := obj.(*metal3api.BareMetalHost)
						var attachments []string
						for _, iface := range bmh.Spec.NetworkInterfaces {
							if iface.HostNetworkAttachment.Name != "" {
								ns := iface.HostNetworkAttachment.Namespace
								if ns == "" {
									ns = bmh.Namespace
								}
								key := fmt.Sprintf("%s/%s", ns, iface.HostNetworkAttachment.Name)
								attachments = append(attachments, key)
							}
						}
						return attachments
					}).
					Build(),
			}

			refs, err := webhook.findBMHReferences(context.TODO(), attachment)
			require.NoError(t, err)
			assert.Len(t, refs, tc.expectedRefsCount)
			if tc.expectedRefs != nil {
				assert.Equal(t, tc.expectedRefs, refs)
			}
		})
	}
}

func TestHNAValidateUpdate(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = metal3api.AddToScheme(scheme)

	oldAttachment := &metal3api.HostNetworkAttachment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-attachment",
			Namespace: "test-ns",
		},
		Spec: metal3api.HostNetworkAttachmentSpec{
			Mode:       metal3api.SwitchportModeAccess,
			NativeVLAN: 100,
		},
	}

	newAttachmentNoChange := &metal3api.HostNetworkAttachment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-attachment",
			Namespace: "test-ns",
		},
		Spec: metal3api.HostNetworkAttachmentSpec{
			Mode:       metal3api.SwitchportModeAccess,
			NativeVLAN: 100,
		},
	}

	newAttachmentChanged := &metal3api.HostNetworkAttachment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-attachment",
			Namespace: "test-ns",
		},
		Spec: metal3api.HostNetworkAttachmentSpec{
			Mode:       metal3api.SwitchportModeTrunk,
			NativeVLAN: 1,
		},
	}

	newAttachmentInvalid := &metal3api.HostNetworkAttachment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-attachment",
			Namespace: "test-ns",
		},
		Spec: metal3api.HostNetworkAttachmentSpec{
			Mode:         metal3api.SwitchportModeAccess,
			NativeVLAN:   100,
			AllowedVLANs: []int{200}, // Invalid: access mode cannot have allowedVLANs
		},
	}

	bmhWithReference := &metal3api.BareMetalHost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "host-with-ref",
			Namespace: "test-ns",
		},
		Spec: metal3api.BareMetalHostSpec{
			NetworkInterfaces: []metal3api.NetworkInterface{
				{
					Name: "eth0",
					HostNetworkAttachment: metal3api.HostNetworkAttachmentRef{
						Name: "test-attachment",
					},
				},
			},
		},
	}

	testCases := []struct {
		name          string
		oldAttachment *metal3api.HostNetworkAttachment
		newAttachment *metal3api.HostNetworkAttachment
		bmhs          []metal3api.BareMetalHost
		expectedError bool
		errorContains string
	}{
		{
			name:          "no-spec-change",
			oldAttachment: oldAttachment,
			newAttachment: newAttachmentNoChange,
			bmhs:          []metal3api.BareMetalHost{*bmhWithReference},
			expectedError: false,
		},
		{
			name:          "spec-changed-no-references",
			oldAttachment: oldAttachment,
			newAttachment: newAttachmentChanged,
			bmhs:          []metal3api.BareMetalHost{},
			expectedError: false,
		},
		{
			name:          "spec-changed-with-references",
			oldAttachment: oldAttachment,
			newAttachment: newAttachmentChanged,
			bmhs:          []metal3api.BareMetalHost{*bmhWithReference},
			expectedError: true,
			errorContains: "immutable while referenced",
		},
		{
			name:          "invalid-new-spec",
			oldAttachment: oldAttachment,
			newAttachment: newAttachmentInvalid,
			bmhs:          []metal3api.BareMetalHost{},
			expectedError: true,
			errorContains: "allowedVlans cannot be specified for access mode",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			objs := []runtime.Object{tc.oldAttachment}
			for i := range tc.bmhs {
				objs = append(objs, &tc.bmhs[i])
			}

			webhook := &HostNetworkAttachment{
				Client: fakeclient.NewClientBuilder().
					WithScheme(scheme).
					WithRuntimeObjects(objs...).
					WithIndex(&metal3api.BareMetalHost{}, bmhNetworkAttachmentIndexField, func(obj client.Object) []string {
						bmh, _ := obj.(*metal3api.BareMetalHost)
						var attachments []string
						for _, iface := range bmh.Spec.NetworkInterfaces {
							if iface.HostNetworkAttachment.Name != "" {
								ns := iface.HostNetworkAttachment.Namespace
								if ns == "" {
									ns = bmh.Namespace
								}
								key := fmt.Sprintf("%s/%s", ns, iface.HostNetworkAttachment.Name)
								attachments = append(attachments, key)
							}
						}
						return attachments
					}).
					Build(),
			}

			warnings, err := webhook.validateUpdate(context.TODO(), tc.oldAttachment, tc.newAttachment)
			_ = warnings // warnings not checked in these tests

			if tc.expectedError {
				require.Error(t, err)
				if tc.errorContains != "" {
					assert.Contains(t, err.Error(), tc.errorContains)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestHNAValidateDelete(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = metal3api.AddToScheme(scheme)

	attachment := &metal3api.HostNetworkAttachment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-attachment",
			Namespace: "test-ns",
		},
	}

	bmhWithReference := &metal3api.BareMetalHost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "host-with-ref",
			Namespace: "test-ns",
		},
		Spec: metal3api.BareMetalHostSpec{
			NetworkInterfaces: []metal3api.NetworkInterface{
				{
					Name: "eth0",
					HostNetworkAttachment: metal3api.HostNetworkAttachmentRef{
						Name: "test-attachment",
					},
				},
			},
		},
	}

	testCases := []struct {
		name          string
		bmhs          []metal3api.BareMetalHost
		expectedError bool
		errorContains string
	}{
		{
			name:          "no-references",
			bmhs:          []metal3api.BareMetalHost{},
			expectedError: false,
		},
		{
			name:          "with-references",
			bmhs:          []metal3api.BareMetalHost{*bmhWithReference},
			expectedError: true,
			errorContains: "cannot delete attachment while referenced",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			objs := []runtime.Object{attachment}
			for i := range tc.bmhs {
				objs = append(objs, &tc.bmhs[i])
			}

			webhook := &HostNetworkAttachment{
				Client: fakeclient.NewClientBuilder().
					WithScheme(scheme).
					WithRuntimeObjects(objs...).
					WithIndex(&metal3api.BareMetalHost{}, bmhNetworkAttachmentIndexField, func(obj client.Object) []string {
						bmh, _ := obj.(*metal3api.BareMetalHost)
						var attachments []string
						for _, iface := range bmh.Spec.NetworkInterfaces {
							if iface.HostNetworkAttachment.Name != "" {
								ns := iface.HostNetworkAttachment.Namespace
								if ns == "" {
									ns = bmh.Namespace
								}
								key := fmt.Sprintf("%s/%s", ns, iface.HostNetworkAttachment.Name)
								attachments = append(attachments, key)
							}
						}
						return attachments
					}).
					Build(),
			}

			warnings, err := webhook.validateDelete(context.TODO(), attachment)
			_ = warnings // warnings not checked in these tests

			if tc.expectedError {
				require.Error(t, err)
				if tc.errorContains != "" {
					assert.Contains(t, err.Error(), tc.errorContains)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestFindBMHReferencesCrossNamespace(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = metal3api.AddToScheme(scheme)

	attachment := &metal3api.HostNetworkAttachment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "shared-attachment",
			Namespace: "infra-ns",
		},
	}

	bmhCrossNS := &metal3api.BareMetalHost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "host-cross-ns",
			Namespace: "tenant-ns",
		},
		Spec: metal3api.BareMetalHostSpec{
			NetworkInterfaces: []metal3api.NetworkInterface{
				{
					Name: "eth0",
					HostNetworkAttachment: metal3api.HostNetworkAttachmentRef{
						Name:      "shared-attachment",
						Namespace: "infra-ns",
					},
				},
			},
		},
	}

	bmhSameNSNoMatch := &metal3api.BareMetalHost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "host-same-ns",
			Namespace: "infra-ns",
		},
		Spec: metal3api.BareMetalHostSpec{
			NetworkInterfaces: []metal3api.NetworkInterface{
				{
					Name: "eth0",
					HostNetworkAttachment: metal3api.HostNetworkAttachmentRef{
						Name: "other-attachment",
					},
				},
			},
		},
	}

	objs := []runtime.Object{attachment, bmhCrossNS, bmhSameNSNoMatch}

	webhook := &HostNetworkAttachment{
		Client: fakeclient.NewClientBuilder().
			WithScheme(scheme).
			WithRuntimeObjects(objs...).
			WithIndex(&metal3api.BareMetalHost{}, bmhNetworkAttachmentIndexField, func(obj client.Object) []string {
				bmh, _ := obj.(*metal3api.BareMetalHost)
				var attachments []string
				for _, iface := range bmh.Spec.NetworkInterfaces {
					if iface.HostNetworkAttachment.Name != "" {
						ns := iface.HostNetworkAttachment.Namespace
						if ns == "" {
							ns = bmh.Namespace
						}
						key := fmt.Sprintf("%s/%s", ns, iface.HostNetworkAttachment.Name)
						attachments = append(attachments, key)
					}
				}
				return attachments
			}).
			Build(),
	}

	refs, err := webhook.findBMHReferences(context.TODO(), attachment)
	require.NoError(t, err)
	assert.Len(t, refs, 1)
	assert.Equal(t, []string{"tenant-ns/host-cross-ns[eth0]"}, refs)
}

func TestHNAValidateUpdateFailClosed(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = metal3api.AddToScheme(scheme)

	oldAttachment := &metal3api.HostNetworkAttachment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-attachment",
			Namespace: "test-ns",
		},
		Spec: metal3api.HostNetworkAttachmentSpec{
			Mode:       metal3api.SwitchportModeAccess,
			NativeVLAN: 100,
		},
	}

	newAttachment := &metal3api.HostNetworkAttachment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-attachment",
			Namespace: "test-ns",
		},
		Spec: metal3api.HostNetworkAttachmentSpec{
			Mode:       metal3api.SwitchportModeTrunk,
			NativeVLAN: 1,
		},
	}

	// Build a client without the field index — List with field selector will fail
	webhook := &HostNetworkAttachment{
		Client: fakeclient.NewClientBuilder().
			WithScheme(scheme).
			WithRuntimeObjects(oldAttachment).
			Build(),
	}

	_, err := webhook.validateUpdate(context.TODO(), oldAttachment, newAttachment)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to check BMH references, cannot safely allow update")
}

func TestHNAValidateDeleteFailClosed(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = metal3api.AddToScheme(scheme)

	attachment := &metal3api.HostNetworkAttachment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-attachment",
			Namespace: "test-ns",
		},
	}

	// Build a client without the field index — List with field selector will fail
	webhook := &HostNetworkAttachment{
		Client: fakeclient.NewClientBuilder().
			WithScheme(scheme).
			WithRuntimeObjects(attachment).
			Build(),
	}

	_, err := webhook.validateDelete(context.TODO(), attachment)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to check BMH references")
}

func TestHNAValidateUpdateWarnings(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = metal3api.AddToScheme(scheme)

	oldAttachment := &metal3api.HostNetworkAttachment{
		ObjectMeta: metav1.ObjectMeta{Name: "test-attachment", Namespace: "test-ns"},
		Spec:       metal3api.HostNetworkAttachmentSpec{Mode: metal3api.SwitchportModeAccess, NativeVLAN: 100},
	}
	newAttachment := &metal3api.HostNetworkAttachment{
		ObjectMeta: metav1.ObjectMeta{Name: "test-attachment", Namespace: "test-ns"},
		Spec:       metal3api.HostNetworkAttachmentSpec{Mode: metal3api.SwitchportModeTrunk, NativeVLAN: 1},
	}
	bmh := &metal3api.BareMetalHost{
		ObjectMeta: metav1.ObjectMeta{Name: "host1", Namespace: "test-ns"},
		Spec: metal3api.BareMetalHostSpec{
			NetworkInterfaces: []metal3api.NetworkInterface{
				{Name: "eth0", HostNetworkAttachment: metal3api.HostNetworkAttachmentRef{Name: "test-attachment"}},
			},
		},
	}

	webhook := &HostNetworkAttachment{
		Client: fakeclient.NewClientBuilder().
			WithScheme(scheme).
			WithRuntimeObjects(oldAttachment, bmh).
			WithIndex(&metal3api.BareMetalHost{}, bmhNetworkAttachmentIndexField, func(obj client.Object) []string {
				b, _ := obj.(*metal3api.BareMetalHost)
				var attachments []string
				for _, iface := range b.Spec.NetworkInterfaces {
					if iface.HostNetworkAttachment.Name != "" {
						ns := iface.HostNetworkAttachment.Namespace
						if ns == "" {
							ns = b.Namespace
						}
						attachments = append(attachments, fmt.Sprintf("%s/%s", ns, iface.HostNetworkAttachment.Name))
					}
				}
				return attachments
			}).
			Build(),
	}

	warnings, err := webhook.validateUpdate(context.TODO(), oldAttachment, newAttachment)
	require.Error(t, err)
	require.Len(t, warnings, 1)
	assert.Contains(t, warnings[0], "host1")
	assert.Contains(t, warnings[0], "Cannot modify attachment while in use")
}

func TestHNAValidateDeleteWarnings(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = metal3api.AddToScheme(scheme)

	attachment := &metal3api.HostNetworkAttachment{
		ObjectMeta: metav1.ObjectMeta{Name: "test-attachment", Namespace: "test-ns"},
	}
	bmh := &metal3api.BareMetalHost{
		ObjectMeta: metav1.ObjectMeta{Name: "host1", Namespace: "test-ns"},
		Spec: metal3api.BareMetalHostSpec{
			NetworkInterfaces: []metal3api.NetworkInterface{
				{Name: "eth0", HostNetworkAttachment: metal3api.HostNetworkAttachmentRef{Name: "test-attachment"}},
			},
		},
	}

	webhook := &HostNetworkAttachment{
		Client: fakeclient.NewClientBuilder().
			WithScheme(scheme).
			WithRuntimeObjects(attachment, bmh).
			WithIndex(&metal3api.BareMetalHost{}, bmhNetworkAttachmentIndexField, func(obj client.Object) []string {
				b, _ := obj.(*metal3api.BareMetalHost)
				var attachments []string
				for _, iface := range b.Spec.NetworkInterfaces {
					if iface.HostNetworkAttachment.Name != "" {
						ns := iface.HostNetworkAttachment.Namespace
						if ns == "" {
							ns = b.Namespace
						}
						attachments = append(attachments, fmt.Sprintf("%s/%s", ns, iface.HostNetworkAttachment.Name))
					}
				}
				return attachments
			}).
			Build(),
	}

	warnings, err := webhook.validateDelete(context.TODO(), attachment)
	require.Error(t, err)
	require.Len(t, warnings, 1)
	assert.Contains(t, warnings[0], "host1")
	assert.Contains(t, warnings[0], "referenced by")
}

func TestFindBMHReferencesMultipleInterfacesSameBMH(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = metal3api.AddToScheme(scheme)

	attachment := &metal3api.HostNetworkAttachment{
		ObjectMeta: metav1.ObjectMeta{Name: "shared-attachment", Namespace: "test-ns"},
	}
	bmh := &metal3api.BareMetalHost{
		ObjectMeta: metav1.ObjectMeta{Name: "multi-ref-host", Namespace: "test-ns"},
		Spec: metal3api.BareMetalHostSpec{
			NetworkInterfaces: []metal3api.NetworkInterface{
				{Name: "eth0", HostNetworkAttachment: metal3api.HostNetworkAttachmentRef{Name: "shared-attachment"}},
				{Name: "eth1", HostNetworkAttachment: metal3api.HostNetworkAttachmentRef{Name: "shared-attachment"}},
			},
		},
	}

	webhook := &HostNetworkAttachment{
		Client: fakeclient.NewClientBuilder().
			WithScheme(scheme).
			WithRuntimeObjects(attachment, bmh).
			WithIndex(&metal3api.BareMetalHost{}, bmhNetworkAttachmentIndexField, func(obj client.Object) []string {
				b, _ := obj.(*metal3api.BareMetalHost)
				var attachments []string
				for _, iface := range b.Spec.NetworkInterfaces {
					if iface.HostNetworkAttachment.Name != "" {
						ns := iface.HostNetworkAttachment.Namespace
						if ns == "" {
							ns = b.Namespace
						}
						attachments = append(attachments, fmt.Sprintf("%s/%s", ns, iface.HostNetworkAttachment.Name))
					}
				}
				return attachments
			}).
			Build(),
	}

	refs, err := webhook.findBMHReferences(context.TODO(), attachment)
	require.NoError(t, err)
	assert.Len(t, refs, 2)
	assert.Contains(t, refs, "test-ns/multi-ref-host[eth0]")
	assert.Contains(t, refs, "test-ns/multi-ref-host[eth1]")
}

// VLAN ID range validation is now handled by CRD schema markers
// (+kubebuilder:validation:Minimum/Maximum on the type definition).
