/*
Copyright 2026 The Metal3 Authors.

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
	"strings"
	"testing"

	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func errorArrContainsPrefix(out []error, want string) bool {
	if want == "" {
		return len(out) == 0
	}
	for _, err := range out {
		if strings.HasPrefix(err.Error(), want) {
			return true
		}
	}
	return false
}

func TestValidateHostClaimSelectorCreate(t *testing.T) {
	tests := []struct {
		name      string
		selector  *metal3api.HostSelector
		wantedErr string
	}{
		{
			name: "valid",
			selector: &metal3api.HostSelector{
				MatchLabels: map[string]string{"key": "value"},
				MatchExpressions: []metal3api.HostSelectorRequirement{
					{
						Key:      "key",
						Operator: "=",
						Values:   []string{"v"},
					},
				},
			},
			wantedErr: "",
		},
		{
			name: "invalid key matchLabel",
			selector: &metal3api.HostSelector{
				MatchLabels: map[string]string{"-key-": "value"},
			},
			wantedErr: "-key-=value: name part",
		},
		{
			name: "invalid value matchLabel",
			selector: &metal3api.HostSelector{
				MatchLabels: map[string]string{"key": "-value-"},
			},
			wantedErr: "key=-value-: a valid label",
		},
		{
			name: "invalid key matchExpr",
			selector: &metal3api.HostSelector{
				MatchExpressions: []metal3api.HostSelectorRequirement{
					{
						Key:      "-key-",
						Operator: "=",
						Values:   []string{"v"},
					},
				},
			},
			wantedErr: "matchExpr 1: name part ",
		},
		{
			name: "bad arity = matchExpr",
			selector: &metal3api.HostSelector{
				MatchExpressions: []metal3api.HostSelectorRequirement{
					{
						Key:      "key",
						Operator: "=",
						Values:   []string{},
					},
				},
			},
			wantedErr: "matchExpr 1: exactly one value",
		},
		{
			name: "bad value = matchExpr",
			selector: &metal3api.HostSelector{
				MatchExpressions: []metal3api.HostSelectorRequirement{
					{
						Key:      "key",
						Operator: "=",
						Values:   []string{"-vvv-"},
					},
				},
			},
			wantedErr: "matchExpr 1 (value \"-vvv-\"): a valid label ",
		},
		{
			name: "bad arity exists matchExpr",
			selector: &metal3api.HostSelector{
				MatchExpressions: []metal3api.HostSelectorRequirement{
					{
						Key:      "key",
						Operator: "exists",
						Values:   []string{"vvv"},
					},
				},
			},
			wantedErr: "matchExpr 1: values not authorized",
		},
		{
			name: "bad value in matchExpr",
			selector: &metal3api.HostSelector{
				MatchExpressions: []metal3api.HostSelectorRequirement{
					{
						Key:      "key",
						Operator: "in",
						Values:   []string{"vvv", "-bad-"},
					},
				},
			},
			wantedErr: "matchExpr 1 (value \"-bad-\"): a valid",
		},
		{
			name: "bad operator",
			selector: &metal3api.HostSelector{
				MatchExpressions: []metal3api.HostSelectorRequirement{
					{
						Key:      "key",
						Operator: "badOp",
						Values:   []string{"vvv"},
					},
				},
			},
			wantedErr: "matchExpr 1: invalid operation badOp in",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			webhook := &HostClaimWebhook{}
			hostclaim := &metal3api.HostClaim{
				TypeMeta:   metav1.TypeMeta{},
				ObjectMeta: metav1.ObjectMeta{},
				Spec: metal3api.HostClaimSpec{
					HostSelector: *tt.selector,
				},
			}
			if err := webhook.validateHostClaim(hostclaim); !errorArrContainsPrefix(err, tt.wantedErr) {
				t.Errorf("metal3api.HostClaimWebhook HostSelector error = %v, wantErr %v", err, tt.wantedErr)
			}
		})
	}
}

func TestValidateHostClaimImageCreate(t *testing.T) {
	tests := []struct {
		name      string
		image     *metal3api.Image
		wantedErr string
	}{
		{
			name: "valid image",
			image: &metal3api.Image{
				URL:      "https://example.com/image",
				Checksum: "be254ebfd73e66ca91f6d91f5050aa2ee1ec4813ee65ba472f608ed340cbff09",
			},
		},
		{
			name: "invalid image",
			image: &metal3api.Image{
				URL:      "test1",
				Checksum: "be254ebfd73e66ca91f6d91f5050aa2ee1ec4813ee65ba472f608ed340cbff09",
			},
			wantedErr: "image URL test1 is invalid: parse \"test1\": invalid URI for request",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			webhook := &HostClaimWebhook{}
			hostclaim := &metal3api.HostClaim{
				TypeMeta:   metav1.TypeMeta{},
				ObjectMeta: metav1.ObjectMeta{},
				Spec: metal3api.HostClaimSpec{
					Image: tt.image,
				},
			}
			if err := webhook.validateHostClaim(hostclaim); !errorArrContainsPrefix(err, tt.wantedErr) {
				t.Errorf("metal3api.HostClaimWebhook Image error = %v, wantErr %v", err, tt.wantedErr)
			}
		})
	}
}

func TestValidateHostClaimSecretReference(t *testing.T) {
	tm := metav1.TypeMeta{
		Kind:       "HostClaim",
		APIVersion: "metal3.io/v1alpha1",
	}

	om := metav1.ObjectMeta{
		Name:      "test",
		Namespace: "test-namespace",
	}

	tests := []struct {
		name      string
		hostClaim *metal3api.HostClaim
		wantedErr string
	}{
		{
			name: "crossNamespaceUserData",
			hostClaim: &metal3api.HostClaim{
				TypeMeta:   tm,
				ObjectMeta: om,
				Spec: metal3api.HostClaimSpec{
					UserData: &corev1.SecretReference{
						Name:      "test-secret",
						Namespace: "different-namespace", // Different from host's namespace
					},
				},
			},
			wantedErr: "hostclaims.metal3.io \"test\" is forbidden: userData: cross-namespace Secret references are not allowed",
		},
		{
			name: "crossNamespaceNetworkData",
			hostClaim: &metal3api.HostClaim{
				TypeMeta:   tm,
				ObjectMeta: om,
				Spec: metal3api.HostClaimSpec{
					NetworkData: &corev1.SecretReference{
						Name:      "test-secret",
						Namespace: "different-namespace", // Different from host's namespace
					},
				},
			},
			wantedErr: "hostclaims.metal3.io \"test\" is forbidden: networkData: cross-namespace Secret references are not allowed",
		},
		{
			name: "crossNamespaceMetaData",
			hostClaim: &metal3api.HostClaim{
				TypeMeta:   tm,
				ObjectMeta: om,
				Spec: metal3api.HostClaimSpec{
					MetaData: &corev1.SecretReference{
						Name:      "test-secret",
						Namespace: "different-namespace", // Different from host's namespace
					},
				},
			},
			wantedErr: "hostclaims.metal3.io \"test\" is forbidden: metaData: cross-namespace Secret references are not allowed",
		},
		{
			name: "multipleSecretsCrossNamespace",
			hostClaim: &metal3api.HostClaim{
				TypeMeta:   tm,
				ObjectMeta: om,
				Spec: metal3api.HostClaimSpec{
					UserData: &corev1.SecretReference{
						Name:      "test-secret1",
						Namespace: "different-namespace1",
					},
					NetworkData: &corev1.SecretReference{
						Name:      "test-secret2",
						Namespace: "different-namespace2",
					},
					MetaData: &corev1.SecretReference{
						Name:      "test-secret3",
						Namespace: "different-namespace3",
					},
				},
			},
			wantedErr: "hostclaims.metal3.io \"test\" is forbidden: userData: cross-namespace Secret references are not allowed", // Should catch at least one error
		},
		{
			name: "sameNamespaceSecrets",
			hostClaim: &metal3api.HostClaim{
				TypeMeta:   tm,
				ObjectMeta: om, // namespace is "test-namespace"
				Spec: metal3api.HostClaimSpec{
					UserData: &corev1.SecretReference{
						Name:      "test-secret1",
						Namespace: "test-namespace", // Same as host's namespace
					},
					NetworkData: &corev1.SecretReference{
						Name:      "test-secret2",
						Namespace: "test-namespace", // Same as host's namespace
					},
					MetaData: &corev1.SecretReference{
						Name:      "test-secret3",
						Namespace: "test-namespace", // Same as host's namespace
					},
				},
			},
			wantedErr: "", // Should be valid
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			webhook := &HostClaimWebhook{}
			if err := webhook.validateHostClaim(tt.hostClaim); !errorArrContains(err, tt.wantedErr) {
				t.Errorf("metal3api.BareMetalHost.Validatemetal3api.BareMetalHost() error = %v, wantErr %v", err, tt.wantedErr)
			}
		})
	}
}
