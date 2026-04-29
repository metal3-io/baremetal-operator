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
	"testing"

	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestHostClaimCreate(t *testing.T) {
	tests := []struct {
		name      string
		hostclaim *metal3api.HostClaim
		wantedErr string
	}{
		{
			name: "valid",
			hostclaim: &metal3api.HostClaim{TypeMeta: metav1.TypeMeta{
				Kind:       "HostClaim",
				APIVersion: "metal3.io/v1alpha1",
			}, ObjectMeta: metav1.ObjectMeta{
				Name:      "test",
				Namespace: "test-namespace",
			}, Spec: metal3api.HostClaimSpec{}},
			wantedErr: "",
		},
		{
			name: "invalid-bad-label-selector",
			hostclaim: &metal3api.HostClaim{TypeMeta: metav1.TypeMeta{
				Kind:       "HostClaim",
				APIVersion: "metal3.io/v1alpha1",
			}, ObjectMeta: metav1.ObjectMeta{
				Name:      "test",
				Namespace: "test-namespace",
			}, Spec: metal3api.HostClaimSpec{
				HostSelector: metal3api.HostSelector{
					MatchLabels: map[string]string{"-bad-key-": "v"},
				},
			}},
			wantedErr: "-bad-key-=v: name part must consist",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			webhook := &HostClaimWebhook{}
			ctx := t.Context()
			if _, err := webhook.ValidateCreate(ctx, tt.hostclaim); !errorContains(err, tt.wantedErr) {
				t.Errorf("HostClaim.ValidateCreate() error = %v, wantErr %v", err, tt.wantedErr)
			}
		})
	}
}

func TestHostClaimUpdate(t *testing.T) {
	tests := []struct {
		name      string
		hostclaim *metal3api.HostClaim
		wantedErr string
	}{
		{
			name: "valid",
			hostclaim: &metal3api.HostClaim{TypeMeta: metav1.TypeMeta{
				Kind:       "HostClaim",
				APIVersion: "metal3.io/v1alpha1",
			}, ObjectMeta: metav1.ObjectMeta{
				Name:      "test",
				Namespace: "test-namespace",
			}, Spec: metal3api.HostClaimSpec{}},
			wantedErr: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			webhook := &HostClaimWebhook{}
			ctx := t.Context()
			// We do not really test on oldObj as it is ignored
			if _, err := webhook.ValidateUpdate(ctx, nil, tt.hostclaim); !errorContains(err, tt.wantedErr) {
				t.Errorf("HostClaim.ValidateUpdate() error = %v, wantErr %v", err, tt.wantedErr)
			}
		})
	}
}
