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

package firmwaresettings

import (
	"testing"

	"github.com/go-logr/logr"
	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	"github.com/metal3-io/baremetal-operator/internal/testutil"
	"github.com/metal3-io/baremetal-operator/pkg/mgrutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func makeTestHost() *metal3api.BareMetalHost {
	return testutil.NewBaremetalhost("test-host", "test-ns", "").SetUID("test-uid").Build()
}

// --- helpers ---

var (
	condChanged = []metav1.Condition{
		{Type: "ChangeDetected", Status: "True", Reason: "Success"},
		{Type: "Valid", Status: "True", Reason: "Success"},
	}
	condUnchanged = []metav1.Condition{
		{Type: "ChangeDetected", Status: "False", Reason: "Success"},
		{Type: "Valid", Status: "True", Reason: "Success"},
	}
	condInvalid = []metav1.Condition{
		{Type: "ChangeDetected", Status: "True", Reason: "Success"},
		{Type: "Valid", Status: "False", Reason: "Failure"},
	}
)

func makeHFS(host *metal3api.BareMetalHost, conditions []metav1.Condition) *metal3api.HostFirmwareSettings {
	return &metal3api.HostFirmwareSettings{
		ObjectMeta: metav1.ObjectMeta{Name: host.Name, Namespace: host.Namespace},
		Spec: metal3api.HostFirmwareSettingsSpec{
			Settings: metal3api.DesiredSettingsMap{
				"ProcVirtualization": intstr.FromString("Disabled"),
			},
		},
		Status: metal3api.HostFirmwareSettingsStatus{
			Settings:   metal3api.SettingsMap{"ProcVirtualization": "Disabled"},
			Conditions: conditions,
		},
	}
}

// --- EnsureSettings ---

func TestEnsureSettings(t *testing.T) {
	t.Run("creates HFS when absent", func(t *testing.T) {
		host := makeTestHost()
		scheme := testutil.TestScheme()
		c := fakeclient.NewClientBuilder().WithScheme(scheme).WithObjects(host).Build()
		m := NewManager(c, scheme, logr.Discard())
		require.NoError(t, m.EnsureSettings(t.Context(), host))

		hfs := &metal3api.HostFirmwareSettings{}
		hostKey := client.ObjectKey{Name: host.Name, Namespace: host.Namespace}
		require.NoError(t, c.Get(t.Context(), hostKey, hfs))
		assert.True(t, mgrutils.OwnerReferenceExists(host, hfs), "owner reference should be set")
	})

	t.Run("idempotent when HFS already exists with owner ref", func(t *testing.T) {
		host := makeTestHost()
		scheme := testutil.TestScheme()
		existing := makeHFS(host, nil)
		require.NoError(t, controllerutil.SetOwnerReference(host, existing, scheme))
		c := fakeclient.NewClientBuilder().WithScheme(scheme).WithObjects(host, existing).Build()

		m := NewManager(c, scheme, logr.Discard())
		require.NoError(t, m.EnsureSettings(t.Context(), host))
	})

	t.Run("adds owner ref to manually-created HFS", func(t *testing.T) {
		host := makeTestHost()
		scheme := testutil.TestScheme()
		existing := makeHFS(host, nil) // no owner ref
		c := fakeclient.NewClientBuilder().WithScheme(scheme).WithObjects(host, existing).Build()

		m := NewManager(c, scheme, logr.Discard())
		require.NoError(t, m.EnsureSettings(t.Context(), host))

		hfs := &metal3api.HostFirmwareSettings{}
		hostKey := client.ObjectKey{Name: host.Name, Namespace: host.Namespace}
		require.NoError(t, c.Get(t.Context(), hostKey, hfs))
		assert.True(t, mgrutils.OwnerReferenceExists(host, hfs), "owner reference should have been added")
	})
}

func TestNewManager(t *testing.T) {
	scheme := testutil.TestScheme()
	c := fakeclient.NewClientBuilder().WithScheme(scheme).Build()

	t.Run("panics on nil client", func(t *testing.T) {
		assert.Panics(t, func() { NewManager(nil, scheme, logr.Discard()) })
	})
	t.Run("panics on nil scheme", func(t *testing.T) {
		assert.Panics(t, func() { NewManager(c, nil, logr.Discard()) })
	})
}

// --- GetSettingsChanges ---

func TestGetSettingsChanges(t *testing.T) {
	tests := []struct {
		scenario   string
		conditions []metav1.Condition // nil = HFS not created
		wantDirty  bool
		wantErr    bool
	}{
		{scenario: "no HFS", conditions: nil, wantDirty: false},
		{scenario: "change detected and valid", conditions: condChanged, wantDirty: true},
		{scenario: "no change detected", conditions: condUnchanged, wantDirty: false},
		{scenario: "invalid settings", conditions: condInvalid, wantDirty: false},
		{
			// ObservedGeneration=1 won't match object Generation=0 after Create.
			scenario: "generation mismatch returns error",
			conditions: []metav1.Condition{
				{Type: "ChangeDetected", Status: "True", Reason: "Success", ObservedGeneration: 1},
				{Type: "Valid", Status: "True", Reason: "Success", ObservedGeneration: 1},
			},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.scenario, func(t *testing.T) {
			host := makeTestHost()
			scheme := testutil.TestScheme()
			c := fakeclient.NewClientBuilder().WithScheme(scheme).Build()
			if tc.conditions != nil {
				require.NoError(t, c.Create(t.Context(), makeHFS(host, tc.conditions)))
			}

			m := NewManager(c, scheme, logr.Discard())
			dirty, _, err := m.GetSettingsChanges(t.Context(), host)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantDirty, dirty)
		})
	}
}
