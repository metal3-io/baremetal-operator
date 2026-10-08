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

package mgrutils

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func TestObjectHasChanges(t *testing.T) {
	tests := []struct {
		scenario    string
		conditions  []metav1.Condition
		generation  int64
		wantChanged bool
		wantValid   bool
		wantErr     bool
	}{
		{
			scenario: "no condition present",
		},
		{
			scenario: "generation mismatch returns error",
			conditions: []metav1.Condition{
				{Type: "ChangeDetected", Status: "True", Reason: "Success", ObservedGeneration: 1},
				{Type: "Valid", Status: "True", Reason: "Success", ObservedGeneration: 1},
			},
			wantErr: true,
		},
		{
			scenario: "Valid=False",
			conditions: []metav1.Condition{
				{Type: "ChangeDetected", Status: "True", Reason: "Success"},
				{Type: "Valid", Status: "False", Reason: "Failure"},
			},
		},
		{
			scenario: "ChangeDetected=True Valid=True",
			conditions: []metav1.Condition{
				{Type: "ChangeDetected", Status: "True", Reason: "Success"},
				{Type: "Valid", Status: "True", Reason: "Success"},
			},
			wantChanged: true,
			wantValid:   true,
		},
		{
			scenario: "ChangeDetected=False Valid=True",
			conditions: []metav1.Condition{
				{Type: "ChangeDetected", Status: "False", Reason: "Success"},
				{Type: "Valid", Status: "True", Reason: "Success"},
			},
			wantValid: true,
		},
		{
			// changedCond is current (gen 1) but validCond is stale (gen 0).
			scenario:   "stale Valid condition from earlier generation",
			generation: 1,
			conditions: []metav1.Condition{
				{Type: "ChangeDetected", Status: "True", Reason: "Success", ObservedGeneration: 1},
				{Type: "Valid", Status: "True", Reason: "Success", ObservedGeneration: 0},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.scenario, func(t *testing.T) {
			changed, valid, err := ObjectHasChanges(tc.conditions, "ChangeDetected", "Valid", tc.generation)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantChanged, changed)
			assert.Equal(t, tc.wantValid, valid)
		})
	}
}

func TestOwnerReferenceExists(t *testing.T) {
	owner := &metav1.ObjectMeta{UID: types.UID("test-uid")}

	t.Run("object with matching UID returns true", func(t *testing.T) {
		resource := &metav1.ObjectMeta{
			OwnerReferences: []metav1.OwnerReference{
				{UID: types.UID("test-uid")},
			},
		}
		assert.True(t, OwnerReferenceExists(owner, resource))
	})

	t.Run("object with no owner references returns false", func(t *testing.T) {
		resource := &metav1.ObjectMeta{}
		assert.False(t, OwnerReferenceExists(owner, resource))
	})
}
