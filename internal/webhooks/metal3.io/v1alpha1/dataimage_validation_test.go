/*
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

func TestDataImageValidateCreate(t *testing.T) {
	tm := metav1.TypeMeta{
		Kind:       "DataImage",
		APIVersion: "metal3.io/v1alpha1",
	}

	om := metav1.ObjectMeta{
		Name:      "test",
		Namespace: "test-namespace",
	}

	tests := []struct {
		name      string
		newS      *metal3api.DataImage
		oldS      *metal3api.DataImage
		wantedErr string
	}{
		{
			name:      "validURL",
			newS:      &metal3api.DataImage{TypeMeta: tm, ObjectMeta: om, Spec: metal3api.DataImageSpec{URL: "http://localhost/abc/abc.php"}},
			oldS:      nil,
			wantedErr: "",
		},
		{
			name:      "invalidURL",
			newS:      &metal3api.DataImage{TypeMeta: tm, ObjectMeta: om, Spec: metal3api.DataImageSpec{URL: "abc"}},
			oldS:      nil,
			wantedErr: "URL \"abc\" is invalid: parse \"abc\": invalid URI for request",
		},
		{
			name:      "invalidURLScheme",
			newS:      &metal3api.DataImage{TypeMeta: tm, ObjectMeta: om, Spec: metal3api.DataImageSpec{URL: "unix://localhost/abc/abc.php"}},
			oldS:      nil,
			wantedErr: "URL \"unix://localhost/abc/abc.php\" is invalid: invalid scheme in URL, \"unix\" not allowed",
		},
		{
			name:      "invalidURLempty",
			newS:      &metal3api.DataImage{TypeMeta: tm, ObjectMeta: om, Spec: metal3api.DataImageSpec{URL: ""}},
			oldS:      nil,
			wantedErr: "URL \"\" is invalid: parse \"\": empty url",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			webhook := &DataImage{}
			if err := webhook.validateDataimage(tt.newS); !errorArrContains(err, tt.wantedErr) {
				t.Errorf("DataImageWebhook error = %v, wantErr %v", err, tt.wantedErr)
			}
		})
	}
}
