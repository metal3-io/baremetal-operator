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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestBMCEventSubscriptionValidateCreate(t *testing.T) {
	tm := metav1.TypeMeta{
		Kind:       "BMCEventSubscription",
		APIVersion: "metal3.io/v1alpha1",
	}

	om := metav1.ObjectMeta{
		Name:      "test",
		Namespace: "test-namespace",
	}

	tests := []struct {
		name      string
		newS      *metal3api.BMCEventSubscription
		oldS      *metal3api.BMCEventSubscription
		wantedErr string
	}{
		{
			name:      "valid",
			newS:      &metal3api.BMCEventSubscription{TypeMeta: tm, ObjectMeta: om, Spec: metal3api.BMCEventSubscriptionSpec{HostName: "worker-01", Destination: "http://localhost/abc/abc.php"}},
			oldS:      nil,
			wantedErr: "",
		},
		{
			name: "missingHostName",
			newS: &metal3api.BMCEventSubscription{
				TypeMeta:   tm,
				ObjectMeta: om,
				Spec:       metal3api.BMCEventSubscriptionSpec{Destination: "http://localhost/abc/abc"},
			},
			oldS:      nil,
			wantedErr: "hostName cannot be empty",
		},
		{
			name: "missingDestination",
			newS: &metal3api.BMCEventSubscription{
				TypeMeta:   tm,
				ObjectMeta: om,
				Spec:       metal3api.BMCEventSubscriptionSpec{HostName: "worker-01"},
			},
			oldS:      nil,
			wantedErr: "destination cannot be empty",
		},
		{
			name: "destinationNotUrl",
			newS: &metal3api.BMCEventSubscription{
				TypeMeta:   tm,
				ObjectMeta: om,
				Spec:       metal3api.BMCEventSubscriptionSpec{HostName: "worker-01", Destination: "abc"},
			},
			oldS:      nil,
			wantedErr: "destination is invalid: parse \"abc\": invalid URI for request",
		},
		{
			name: "destinationMissingTrailingSlash",
			newS: &metal3api.BMCEventSubscription{
				TypeMeta:   tm,
				ObjectMeta: om,
				Spec:       metal3api.BMCEventSubscriptionSpec{HostName: "worker-01", Destination: "http://localhost"},
			},
			oldS:      nil,
			wantedErr: "hostname-only destination must have a trailing slash",
		},
		{
			name: "httpHeadersRef valid",
			newS: &metal3api.BMCEventSubscription{
				TypeMeta:   tm,
				ObjectMeta: om,
				Spec: metal3api.BMCEventSubscriptionSpec{HostName: "worker-01", Destination: "http://localhost/abc/abc.php",
					HTTPHeadersRef: &corev1.SecretReference{Namespace: om.Namespace, Name: "headers"}},
			},
			oldS:      nil,
			wantedErr: "",
		},
		{
			name: "httpHeadersRef in different namespace",
			newS: &metal3api.BMCEventSubscription{
				TypeMeta:   tm,
				ObjectMeta: om,
				Spec: metal3api.BMCEventSubscriptionSpec{HostName: "worker-01", Destination: "http://localhost/abc/abc.php",
					HTTPHeadersRef: &corev1.SecretReference{Namespace: "different", Name: "headers"}},
			},
			oldS:      nil,
			wantedErr: "httpHeadersRef secret must be in the same namespace as the BMCEventSubscription",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			webhook := &BMCEventSubscription{}
			if err := webhook.validateSubscription(tt.newS); !errorArrContains(err, tt.wantedErr) {
				t.Errorf("BareMetalHost.validateSubscription() error = %v, wantErr %v", err, tt.wantedErr)
			}
		})
	}
}
