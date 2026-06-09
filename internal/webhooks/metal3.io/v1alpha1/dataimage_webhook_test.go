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
	"testing"

	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestDataImageUpdate(t *testing.T) {
	tests := []struct {
		name      string
		bes       *metal3api.DataImage
		old       *metal3api.DataImage
		wantedErr string
	}{
		{
			name: "valid",
			bes: &metal3api.DataImage{
				TypeMeta: metav1.TypeMeta{
					Kind:       "metal3api.DataImage",
					APIVersion: "metal3.io/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "test-namespace",
				},
				Spec: metal3api.DataImageSpec{URL: "http://example.com"},
			},
			old: &metal3api.DataImage{
				TypeMeta: metav1.TypeMeta{
					Kind:       "metal3api.DataImage",
					APIVersion: "metal3.io/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "test-namespace",
				},
				Spec: metal3api.DataImageSpec{},
			},
			wantedErr: "",
		},
		{
			name: "invalidURL",
			bes: &metal3api.DataImage{
				TypeMeta: metav1.TypeMeta{
					Kind:       "metal3api.DataImage",
					APIVersion: "metal3.io/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "test-namespace",
				},
				Spec: metal3api.DataImageSpec{URL: "abc"},
			},
			old: &metal3api.DataImage{
				TypeMeta: metav1.TypeMeta{
					Kind:       "metal3api.DataImage",
					APIVersion: "metal3.io/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "test-namespace",
				},
				Spec: metal3api.DataImageSpec{},
			},
			wantedErr: "URL \"abc\" is invalid: parse \"abc\": invalid URI for request",
		},
		{
			name: "emptyURL",
			bes: &metal3api.DataImage{
				TypeMeta: metav1.TypeMeta{
					Kind:       "metal3api.DataImage",
					APIVersion: "metal3.io/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "test-namespace",
				},
				Spec: metal3api.DataImageSpec{},
			},
			old: &metal3api.DataImage{
				TypeMeta: metav1.TypeMeta{
					Kind:       "metal3api.DataImage",
					APIVersion: "metal3.io/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "test-namespace",
				},
				Spec: metal3api.DataImageSpec{URL: ""},
			},
			wantedErr: "URL \"\" is invalid: parse \"\": empty url",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			webhook := &DataImage{}
			context := t.Context()
			if _, err := webhook.ValidateUpdate(context, tt.old, tt.bes); !errorContains(err, tt.wantedErr) {
				t.Errorf("metal3api.DataImage.ValidateUpdate() error = %v, wantErr %v", err, tt.wantedErr)
			}
		})
	}
}
