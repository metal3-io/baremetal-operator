/*
Copyright 2019 The Kubernetes Authors.
Copyright 2025 The Metal3 Authors.

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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func errorContains(out error, want string) bool {
	if out == nil {
		return want == ""
	}
	if want == "" {
		return false
	}
	return strings.Contains(out.Error(), want)
}

func TestBareMetalHostCreate(t *testing.T) {
	tests := []struct {
		name      string
		bmh       *metal3api.BareMetalHost
		wantedErr string
	}{
		{
			name: "valid",
			bmh: &metal3api.BareMetalHost{TypeMeta: metav1.TypeMeta{
				Kind:       "BareMetalHost",
				APIVersion: "metal3.io/v1alpha1",
			}, ObjectMeta: metav1.ObjectMeta{
				Name:      "test",
				Namespace: "test-namespace",
			}, Spec: metal3api.BareMetalHostSpec{}},
			wantedErr: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			webhook := &BareMetalHost{}
			ctx := t.Context()
			if _, err := webhook.ValidateCreate(ctx, tt.bmh); !errorContains(err, tt.wantedErr) {
				t.Errorf("BareMetalHost.ValidateCreate() error = %v, wantErr %v", err, tt.wantedErr)
			}
		})
	}
}

func TestBareMetalHostUpdate(t *testing.T) {
	tests := []struct {
		name      string
		bmh       *metal3api.BareMetalHost
		old       *metal3api.BareMetalHost
		wantedErr string
	}{
		{
			name: "valid",
			bmh: &metal3api.BareMetalHost{TypeMeta: metav1.TypeMeta{
				Kind:       "BareMetalHost",
				APIVersion: "metal3.io/v1alpha1",
			}, ObjectMeta: metav1.ObjectMeta{
				Name:      "test",
				Namespace: "test-namespace",
			}, Spec: metal3api.BareMetalHostSpec{}},
			old: &metal3api.BareMetalHost{TypeMeta: metav1.TypeMeta{
				Kind:       "BareMetalHost",
				APIVersion: "metal3.io/v1alpha1",
			}, ObjectMeta: metav1.ObjectMeta{
				Name:      "test",
				Namespace: "test-namespace",
			}, Spec: metal3api.BareMetalHostSpec{}},
			wantedErr: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			webhook := &BareMetalHost{}
			ctx := t.Context()
			if _, err := webhook.ValidateUpdate(ctx, tt.old, tt.bmh); !errorContains(err, tt.wantedErr) {
				t.Errorf("BareMetalHost.ValidateUpdate() error = %v, wantErr %v", err, tt.wantedErr)
			}
		})
	}
}
