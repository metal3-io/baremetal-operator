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
	"errors"
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
					AllowedVLANs: []string{"10", "20", "30"},
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
					AllowedVLANs: []string{"200", "300"},
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
					AllowedVLANs: []string{"200"},
				},
			},
			expectedError: true,
			errorContains: "allowedVLANs cannot be specified for access mode",
		},
		{
			name: "valid-vlan-range",
			attachment: &metal3api.HostNetworkAttachment{
				Spec: metal3api.HostNetworkAttachmentSpec{
					Mode:         metal3api.SwitchportModeTrunk,
					NativeVLAN:   1,
					AllowedVLANs: []string{"10", "20-30", "100", "200-400"},
				},
			},
			expectedError: false,
		},
		{
			name: "valid-boundary-vlans",
			attachment: &metal3api.HostNetworkAttachment{
				Spec: metal3api.HostNetworkAttachmentSpec{
					Mode:         metal3api.SwitchportModeTrunk,
					NativeVLAN:   1,
					AllowedVLANs: []string{"1", "4094", "1-4094"},
				},
			},
			expectedError: false,
		},
		{
			name: "invalid-vlan-zero",
			attachment: &metal3api.HostNetworkAttachment{
				Spec: metal3api.HostNetworkAttachmentSpec{
					Mode:         metal3api.SwitchportModeTrunk,
					NativeVLAN:   1,
					AllowedVLANs: []string{"0"},
				},
			},
			expectedError: true,
			errorContains: "out of range 1-4094",
		},
		{
			name: "invalid-vlan-above-max",
			attachment: &metal3api.HostNetworkAttachment{
				Spec: metal3api.HostNetworkAttachmentSpec{
					Mode:         metal3api.SwitchportModeTrunk,
					NativeVLAN:   1,
					AllowedVLANs: []string{"4095"},
				},
			},
			expectedError: true,
			errorContains: "out of range 1-4094",
		},
		{
			name: "invalid-range-end-above-max",
			attachment: &metal3api.HostNetworkAttachment{
				Spec: metal3api.HostNetworkAttachmentSpec{
					Mode:         metal3api.SwitchportModeTrunk,
					NativeVLAN:   1,
					AllowedVLANs: []string{"1-5000"},
				},
			},
			expectedError: true,
			errorContains: "out of range 1-4094",
		},
		{
			name: "invalid-reversed-range",
			attachment: &metal3api.HostNetworkAttachment{
				Spec: metal3api.HostNetworkAttachmentSpec{
					Mode:         metal3api.SwitchportModeTrunk,
					NativeVLAN:   1,
					AllowedVLANs: []string{"200-100"},
				},
			},
			expectedError: true,
			errorContains: "must be less than end",
		},
		{
			name: "invalid-degenerate-range",
			attachment: &metal3api.HostNetworkAttachment{
				Spec: metal3api.HostNetworkAttachmentSpec{
					Mode:         metal3api.SwitchportModeTrunk,
					NativeVLAN:   1,
					AllowedVLANs: []string{"100-100"},
				},
			},
			expectedError: true,
			errorContains: "must be less than end",
		},
		{
			name: "invalid-non-numeric",
			attachment: &metal3api.HostNetworkAttachment{
				Spec: metal3api.HostNetworkAttachmentSpec{
					Mode:         metal3api.SwitchportModeTrunk,
					NativeVLAN:   1,
					AllowedVLANs: []string{"abc"},
				},
			},
			expectedError: true,
			errorContains: "invalid VLAN ID",
		},
		{
			name: "invalid-range-non-numeric-end",
			attachment: &metal3api.HostNetworkAttachment{
				Spec: metal3api.HostNetworkAttachmentSpec{
					Mode:         metal3api.SwitchportModeTrunk,
					NativeVLAN:   1,
					AllowedVLANs: []string{"10-abc"},
				},
			},
			expectedError: true,
			errorContains: "invalid range end",
		},
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
	require.NoError(t, metal3api.AddToScheme(scheme))

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
		expectedReference bool
	}{
		{
			name: "no-bmhs",
			bmhs: []metal3api.BareMetalHost{},
		},
		{
			name:              "one-bmh-with-reference",
			bmhs:              []metal3api.BareMetalHost{*bmhWithReference},
			expectedReference: true,
		},
		{
			name: "one-bmh-without-reference",
			bmhs: []metal3api.BareMetalHost{*bmhWithoutReference},
		},
		{
			name:              "mixed-bmhs",
			bmhs:              []metal3api.BareMetalHost{*bmhWithReference, *bmhWithoutReference, *bmhWithDifferentAttachment},
			expectedReference: true,
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
			webhook.APIReader = webhook.Client

			referenced, err := webhook.findBMHReferences(context.TODO(), attachment)
			require.NoError(t, err)
			assert.Equal(t, tc.expectedReference, referenced)
		})
	}
}

type recordingHNAAPIReader struct {
	client.Reader
	reads []client.ObjectKey
	fail  bool
}

func (reader *recordingHNAAPIReader) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	reader.reads = append(reader.reads, key)
	if reader.fail {
		return errors.New("API read failed")
	}
	return reader.Reader.Get(ctx, key, obj, opts...)
}

func TestHNAValidateDeleteChecksLiveBMH(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, metal3api.AddToScheme(scheme))

	attachment := &metal3api.HostNetworkAttachment{
		ObjectMeta: metav1.ObjectMeta{Name: "test-attachment", Namespace: "test-ns"},
	}
	cachedBMH := &metal3api.BareMetalHost{
		ObjectMeta: metav1.ObjectMeta{Name: "host-with-ref", Namespace: "test-ns"},
		Spec: metal3api.BareMetalHostSpec{
			NetworkInterfaces: []metal3api.NetworkInterface{
				{Name: "eth0", HostNetworkAttachment: metal3api.HostNetworkAttachmentRef{Name: attachment.Name}},
			},
		},
	}
	differentAttachment := cachedBMH.DeepCopy()
	differentAttachment.Spec.NetworkInterfaces[0].HostNetworkAttachment.Name = "other-attachment"
	differentNamespace := cachedBMH.DeepCopy()
	differentNamespace.Spec.NetworkInterfaces[0].HostNetworkAttachment.Namespace = "other-ns"
	multipleReferences := cachedBMH.DeepCopy()
	multipleReferences.Spec.NetworkInterfaces = append(multipleReferences.Spec.NetworkInterfaces,
		metal3api.NetworkInterface{Name: "eth1", HostNetworkAttachment: metal3api.HostNetworkAttachmentRef{Name: attachment.Name}})

	testCases := []struct {
		name          string
		cachedBMH     *metal3api.BareMetalHost
		noCachedBMH   bool
		liveBMH       *metal3api.BareMetalHost
		readerFails   bool
		expectedReads int
		expectedError string
	}{
		{name: "no cached BMH", noCachedBMH: true, readerFails: true},
		{name: "cached BMH without references", cachedBMH: &metal3api.BareMetalHost{
			ObjectMeta: cachedBMH.ObjectMeta,
		}, readerFails: true},
		{name: "cached reference to different attachment", cachedBMH: differentAttachment, readerFails: true},
		{name: "cached reference to different namespace", cachedBMH: differentNamespace, readerFails: true},
		{name: "deleted BMH", expectedReads: 1},
		{name: "live BMH dropped reference", liveBMH: &metal3api.BareMetalHost{ObjectMeta: cachedBMH.ObjectMeta}, expectedReads: 1},
		{name: "still referenced", liveBMH: cachedBMH, expectedReads: 1, expectedError: "cannot delete attachment while referenced"},
		{name: "multiple interfaces referencing attachment", cachedBMH: multipleReferences, liveBMH: multipleReferences,
			expectedReads: 1, expectedError: "one or more BMHs"},
		{name: "API read error", readerFails: true, expectedReads: 1, expectedError: "API read failed"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cachedClientBuilder := fakeclient.NewClientBuilder().
				WithScheme(scheme).
				WithIndex(&metal3api.BareMetalHost{}, bmhNetworkAttachmentIndexField, func(client.Object) []string {
					// Include nonconflicting candidates to verify the API reader is
					// only used after checking references in the cached object.
					return []string{"test-ns/test-attachment"}
				})
			if !tc.noCachedBMH {
				candidate := cachedBMH
				if tc.cachedBMH != nil {
					candidate = tc.cachedBMH
				}
				cachedClientBuilder.WithRuntimeObjects(candidate.DeepCopy())
			}

			liveClientBuilder := fakeclient.NewClientBuilder().WithScheme(scheme)
			if tc.liveBMH != nil {
				liveClientBuilder.WithRuntimeObjects(tc.liveBMH.DeepCopy())
			}
			apiReader := &recordingHNAAPIReader{
				Reader: liveClientBuilder.Build(),
				fail:   tc.readerFails,
			}

			webhook := &HostNetworkAttachment{Client: cachedClientBuilder.Build(), APIReader: apiReader}
			_, err := webhook.validateDelete(context.Background(), attachment)
			require.Len(t, apiReader.reads, tc.expectedReads)
			if tc.expectedReads > 0 {
				assert.Equal(t, client.ObjectKeyFromObject(cachedBMH), apiReader.reads[0])
			}
			if tc.expectedError == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tc.expectedError)
			}
		})
	}
}

func TestHNAFindBMHReferencesShortCircuits(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, metal3api.AddToScheme(scheme))

	attachment := &metal3api.HostNetworkAttachment{
		ObjectMeta: metav1.ObjectMeta{Name: "test-attachment", Namespace: "test-ns"},
	}
	newBMH := func(name string) *metal3api.BareMetalHost {
		return &metal3api.BareMetalHost{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "test-ns"},
			Spec: metal3api.BareMetalHostSpec{
				NetworkInterfaces: []metal3api.NetworkInterface{
					{Name: "eth0", HostNetworkAttachment: metal3api.HostNetworkAttachmentRef{Name: attachment.Name}},
				},
			},
		}
	}
	bmhA, bmhB := newBMH("host-a"), newBMH("host-b")

	indexFn := func(obj client.Object) []string {
		b, _ := obj.(*metal3api.BareMetalHost)
		var keys []string
		for _, iface := range b.Spec.NetworkInterfaces {
			if iface.HostNetworkAttachment.Name != "" {
				keys = append(keys, fmt.Sprintf("%s/%s", b.Namespace, iface.HostNetworkAttachment.Name))
			}
		}
		return keys
	}

	cachedClient := fakeclient.NewClientBuilder().
		WithScheme(scheme).
		WithRuntimeObjects(bmhA.DeepCopy(), bmhB.DeepCopy()).
		WithIndex(&metal3api.BareMetalHost{}, bmhNetworkAttachmentIndexField, indexFn).
		Build()
	apiReader := &recordingHNAAPIReader{
		Reader: fakeclient.NewClientBuilder().WithScheme(scheme).
			WithRuntimeObjects(bmhA.DeepCopy(), bmhB.DeepCopy()).Build(),
	}

	webhook := &HostNetworkAttachment{Client: cachedClient, APIReader: apiReader}
	referenced, err := webhook.findBMHReferences(context.Background(), attachment)
	require.NoError(t, err)
	require.True(t, referenced)
	// A single confirmed live reference is enough to block the operation, so the
	// second candidate is never read from the API reader.
	require.Len(t, apiReader.reads, 1)
}

func TestHNAValidateUpdate(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, metal3api.AddToScheme(scheme))

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
			AllowedVLANs: []string{"200"}, // Invalid: access mode cannot have allowedVLANs
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
			errorContains: "allowedVLANs cannot be specified for access mode",
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
			webhook.APIReader = webhook.Client

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
	require.NoError(t, metal3api.AddToScheme(scheme))

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
			webhook.APIReader = webhook.Client

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
	require.NoError(t, metal3api.AddToScheme(scheme))

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
	webhook.APIReader = webhook.Client

	referenced, err := webhook.findBMHReferences(context.TODO(), attachment)
	require.NoError(t, err)
	assert.True(t, referenced)
}

func TestHNAValidateUpdateFailClosed(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, metal3api.AddToScheme(scheme))

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
	require.NoError(t, metal3api.AddToScheme(scheme))

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
	require.NoError(t, metal3api.AddToScheme(scheme))

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
	webhook.APIReader = webhook.Client

	warnings, err := webhook.validateUpdate(context.TODO(), oldAttachment, newAttachment)
	require.Error(t, err)
	assert.Empty(t, warnings)
	assert.Contains(t, err.Error(), "immutable while referenced")
}

func TestHNAValidateDeleteWarnings(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, metal3api.AddToScheme(scheme))

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
	webhook.APIReader = webhook.Client

	warnings, err := webhook.validateDelete(context.TODO(), attachment)
	require.Error(t, err)
	require.Len(t, warnings, 1)
	assert.Contains(t, warnings[0], "one or more BMHs")
}

func TestFindBMHReferencesMultipleInterfacesSameBMH(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, metal3api.AddToScheme(scheme))

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
	webhook.APIReader = webhook.Client

	referenced, err := webhook.findBMHReferences(context.TODO(), attachment)
	require.NoError(t, err)
	assert.True(t, referenced)
}

// VLAN ID range validation is now handled by CRD schema markers
// (+kubebuilder:validation:Minimum/Maximum on the type definition).
