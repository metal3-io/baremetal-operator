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

func TestBMCEventSubscriptionUpdate(t *testing.T) {
	tests := []struct {
		name      string
		bes       *metal3api.BMCEventSubscription
		old       *metal3api.BMCEventSubscription
		wantedErr string
	}{
		{
			// This is valid because the spec wasn't updated.
			name: "valid",
			bes: &metal3api.BMCEventSubscription{
				TypeMeta: metav1.TypeMeta{
					Kind:       "metal3api.BMCEventSubscription",
					APIVersion: "metal3.io/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "test-namespace",
				},
				Spec: metal3api.BMCEventSubscriptionSpec{},
			},
			old: &metal3api.BMCEventSubscription{
				TypeMeta: metav1.TypeMeta{
					Kind:       "metal3api.BMCEventSubscription",
					APIVersion: "metal3.io/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "test-namespace",
				},
				Spec: metal3api.BMCEventSubscriptionSpec{},
			},
			wantedErr: "",
		},
		{
			name: "invalid",
			bes: &metal3api.BMCEventSubscription{
				TypeMeta: metav1.TypeMeta{
					Kind:       "metal3api.BMCEventSubscription",
					APIVersion: "metal3.io/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "test-namespace",
				},
				Spec: metal3api.BMCEventSubscriptionSpec{},
			},
			old: &metal3api.BMCEventSubscription{
				TypeMeta: metav1.TypeMeta{
					Kind:       "metal3api.BMCEventSubscription",
					APIVersion: "metal3.io/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "test-namespace",
				},
				Spec: metal3api.BMCEventSubscriptionSpec{Context: "abc"},
			},
			wantedErr: "subscriptions cannot be updated, please recreate it",
		},
		{
			// Status updates are valid
			name: "status updated",
			bes: &metal3api.BMCEventSubscription{
				TypeMeta: metav1.TypeMeta{
					Kind:       "metal3api.BMCEventSubscription",
					APIVersion: "metal3.io/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "test-namespace",
				},
				Spec: metal3api.BMCEventSubscriptionSpec{},
				Status: metal3api.BMCEventSubscriptionStatus{
					SubscriptionID: "some-id",
				},
			},
			old: &metal3api.BMCEventSubscription{
				TypeMeta: metav1.TypeMeta{
					Kind:       "metal3api.BMCEventSubscription",
					APIVersion: "metal3.io/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "test-namespace",
				},
				Spec:   metal3api.BMCEventSubscriptionSpec{},
				Status: metal3api.BMCEventSubscriptionStatus{},
			},
			wantedErr: "",
		},
		{
			// Finalizer updates are valid
			name: "finalizers updated",
			bes: &metal3api.BMCEventSubscription{
				TypeMeta: metav1.TypeMeta{
					Kind:       "metal3api.BMCEventSubscription",
					APIVersion: "metal3.io/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test",
					Namespace:  "test-namespace",
					Finalizers: []string{metal3api.BMCEventSubscriptionFinalizer},
				},
				Spec:   metal3api.BMCEventSubscriptionSpec{},
				Status: metal3api.BMCEventSubscriptionStatus{},
			},
			old: &metal3api.BMCEventSubscription{
				TypeMeta: metav1.TypeMeta{
					Kind:       "metal3api.BMCEventSubscription",
					APIVersion: "metal3.io/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test",
					Namespace:  "test-namespace",
					Finalizers: []string{},
				},
				Spec:   metal3api.BMCEventSubscriptionSpec{},
				Status: metal3api.BMCEventSubscriptionStatus{},
			},
			wantedErr: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			webhook := &BMCEventSubscription{}
			context := t.Context()
			if _, err := webhook.ValidateUpdate(context, tt.old, tt.bes); !errorContains(err, tt.wantedErr) {
				t.Errorf("metal3api.BMCEventSubscription.ValidateUpdate() error = %v, wantErr %v", err, tt.wantedErr)
			}
		})
	}
}
