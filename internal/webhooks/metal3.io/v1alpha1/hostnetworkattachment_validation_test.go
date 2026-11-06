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
	"strings"
	"testing"

	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
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

func TestHNAValidateUpdate(t *testing.T) {
	oldAttachment := &metal3api.HostNetworkAttachment{
		ObjectMeta: metav1.ObjectMeta{Name: "test-attachment", Namespace: "test-ns"},
		Spec: metal3api.HostNetworkAttachmentSpec{
			Mode:       metal3api.SwitchportModeAccess,
			NativeVLAN: 100,
		},
	}

	testCases := []struct {
		name          string
		newAttachment *metal3api.HostNetworkAttachment
		errorContains string
	}{
		{
			name: "unchanged-spec",
			newAttachment: &metal3api.HostNetworkAttachment{
				ObjectMeta: metav1.ObjectMeta{Name: "test-attachment", Namespace: "test-ns"},
				Spec: metal3api.HostNetworkAttachmentSpec{
					Mode:       metal3api.SwitchportModeAccess,
					NativeVLAN: 100,
				},
			},
		},
		{
			name: "valid-spec-change",
			newAttachment: &metal3api.HostNetworkAttachment{
				ObjectMeta: metav1.ObjectMeta{Name: "test-attachment", Namespace: "test-ns"},
				Spec: metal3api.HostNetworkAttachmentSpec{
					Mode:         metal3api.SwitchportModeTrunk,
					NativeVLAN:   1,
					AllowedVLANs: []string{"10", "20-30"},
				},
			},
		},
		{
			name: "invalid-spec-change",
			newAttachment: &metal3api.HostNetworkAttachment{
				ObjectMeta: metav1.ObjectMeta{Name: "test-attachment", Namespace: "test-ns"},
				Spec: metal3api.HostNetworkAttachmentSpec{
					Mode:         metal3api.SwitchportModeAccess,
					NativeVLAN:   100,
					AllowedVLANs: []string{"200"},
				},
			},
			errorContains: "allowedVLANs cannot be specified for access mode",
		},
	}

	webhook := &HostNetworkAttachment{}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := webhook.ValidateUpdate(context.Background(), oldAttachment, tc.newAttachment)
			if tc.errorContains != "" {
				require.ErrorContains(t, err, tc.errorContains)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestHNAValidateDelete(t *testing.T) {
	attachment := &metal3api.HostNetworkAttachment{
		ObjectMeta: metav1.ObjectMeta{Name: "test-attachment", Namespace: "test-ns"},
	}

	warnings, err := (&HostNetworkAttachment{}).ValidateDelete(context.Background(), attachment)
	require.NoError(t, err)
	assert.Empty(t, warnings)
}
