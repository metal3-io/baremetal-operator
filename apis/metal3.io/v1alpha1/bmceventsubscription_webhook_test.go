/*
Copyright 2019 The Kubernetes Authors.

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

package v1alpha1

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestBMCEventSubscriptionUpdate(t *testing.T) {
	tests := []struct {
		name      string
		bes       *BMCEventSubscription
		old       *BMCEventSubscription
		wantedErr string
	}{
		{
			// This is valid because the spec wasn't updated.
			name: "valid",
			bes: &BMCEventSubscription{
				TypeMeta: metav1.TypeMeta{
					Kind:       "BMCEventSubscription",
					APIVersion: "metal3.io/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "test-namespace",
				},
				Spec: BMCEventSubscriptionSpec{},
			},
			old: &BMCEventSubscription{
				TypeMeta: metav1.TypeMeta{
					Kind:       "BMCEventSubscription",
					APIVersion: "metal3.io/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "test-namespace",
				},
				Spec: BMCEventSubscriptionSpec{},
			},
			wantedErr: "",
		},
		{
			name: "invalid",
			bes: &BMCEventSubscription{
				TypeMeta: metav1.TypeMeta{
					Kind:       "BMCEventSubscription",
					APIVersion: "metal3.io/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "test-namespace",
				},
				Spec: BMCEventSubscriptionSpec{},
			},
			old: &BMCEventSubscription{
				TypeMeta: metav1.TypeMeta{
					Kind:       "BMCEventSubscription",
					APIVersion: "metal3.io/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "test-namespace",
				},
				Spec: BMCEventSubscriptionSpec{Context: "abc"},
			},
			wantedErr: "subscriptions cannot be updated, please recreate it",
		},
		{
			// Status updates are valid
			name: "status updated",
			bes: &BMCEventSubscription{
				TypeMeta: metav1.TypeMeta{
					Kind:       "BMCEventSubscription",
					APIVersion: "metal3.io/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "test-namespace",
				},
				Spec: BMCEventSubscriptionSpec{},
				Status: BMCEventSubscriptionStatus{
					SubscriptionID: "some-id",
				},
			},
			old: &BMCEventSubscription{
				TypeMeta: metav1.TypeMeta{
					Kind:       "BMCEventSubscription",
					APIVersion: "metal3.io/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "test-namespace",
				},
				Spec:   BMCEventSubscriptionSpec{},
				Status: BMCEventSubscriptionStatus{},
			},
			wantedErr: "",
		},
		{
			// Finalizer updates are valid
			name: "finalizers updated",
			bes: &BMCEventSubscription{
				TypeMeta: metav1.TypeMeta{
					Kind:       "BMCEventSubscription",
					APIVersion: "metal3.io/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test",
					Namespace:  "test-namespace",
					Finalizers: []string{BMCEventSubscriptionFinalizer},
				},
				Spec:   BMCEventSubscriptionSpec{},
				Status: BMCEventSubscriptionStatus{},
			},
			old: &BMCEventSubscription{
				TypeMeta: metav1.TypeMeta{
					Kind:       "BMCEventSubscription",
					APIVersion: "metal3.io/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test",
					Namespace:  "test-namespace",
					Finalizers: []string{},
				},
				Spec:   BMCEventSubscriptionSpec{},
				Status: BMCEventSubscriptionStatus{},
			},
			wantedErr: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := tt.bes.ValidateUpdate(tt.old); !errorContains(err, tt.wantedErr) {
				t.Errorf("BMCEventSubscription.ValidateUpdate() error = %v, wantErr %v", err, tt.wantedErr)
			}
		})
	}
}
