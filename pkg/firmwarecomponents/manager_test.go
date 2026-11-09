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

package firmwarecomponents

import (
	"context"
	"errors"
	"testing"

	"github.com/go-logr/logr"
	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	"github.com/metal3-io/baremetal-operator/internal/testutil"
	"github.com/metal3-io/baremetal-operator/pkg/hardwareutils/bmc"
	"github.com/metal3-io/baremetal-operator/pkg/mgrutils"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner/fixture"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func makeTestHost() *metal3api.BareMetalHost {
	return testutil.NewBaremetalhost("test-host", "test-ns", "").SetUID("test-uid").Build()
}

func hostKey(host *metal3api.BareMetalHost) client.ObjectKey {
	return client.ObjectKey{Name: host.Name, Namespace: host.Namespace}
}

// makeFixtureProvisioner builds a fixture-based provisioner pre-loaded with the
// given firmware components and error for GetFirmwareComponents.
func makeFixtureProvisioner(host *metal3api.BareMetalHost, components []metal3api.FirmwareComponentStatus, err error) provisioner.Provisioner {
	fix := fixture.Fixture{
		HostFirmwareComponents: fixture.HostFirmwareComponentsMock{
			Components: components,
			Error:      err,
		},
	}
	prov, _ := fix.NewProvisioner(context.TODO(), provisioner.BuildHostData(*host, bmc.Credentials{}), func(_, _ string) {})
	return prov
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

func makeHFC(host *metal3api.BareMetalHost, conditions []metav1.Condition, spec, status []metal3api.FirmwareUpdate) *metal3api.HostFirmwareComponents {
	return &metal3api.HostFirmwareComponents{
		ObjectMeta: metav1.ObjectMeta{Name: host.Name, Namespace: host.Namespace},
		Spec:       metal3api.HostFirmwareComponentsSpec{Updates: spec},
		Status:     metal3api.HostFirmwareComponentsStatus{Updates: status, Conditions: conditions},
	}
}

// --- EnsureComponents ---

func TestEnsureComponents(t *testing.T) {
	t.Run("creates HFC when absent", func(t *testing.T) {
		host := makeTestHost()
		scheme := testutil.TestScheme()
		c := fakeclient.NewClientBuilder().WithScheme(scheme).WithObjects(host).Build()
		m := NewManager(c, scheme, nil, logr.Discard())
		require.NoError(t, m.EnsureComponents(t.Context(), host))

		hfc := &metal3api.HostFirmwareComponents{}
		require.NoError(t, c.Get(t.Context(), hostKey(host), hfc))
		assert.True(t, mgrutils.OwnerReferenceExists(host, hfc), "owner reference should be set")
	})

	t.Run("adds owner ref to manually-created HFC", func(t *testing.T) {
		host := makeTestHost()
		scheme := testutil.TestScheme()
		existing := makeHFC(host, nil, nil, nil) // no owner ref
		c := fakeclient.NewClientBuilder().WithScheme(scheme).WithObjects(host, existing).Build()

		m := NewManager(c, scheme, nil, logr.Discard())
		require.NoError(t, m.EnsureComponents(t.Context(), host))

		hfc := &metal3api.HostFirmwareComponents{}
		require.NoError(t, c.Get(t.Context(), hostKey(host), hfc))
		assert.True(t, mgrutils.OwnerReferenceExists(host, hfc), "owner reference should have been added")
	})

	t.Run("idempotent when HFC already exists with owner ref", func(t *testing.T) {
		host := makeTestHost()
		scheme := testutil.TestScheme()
		existing := makeHFC(host, nil, nil, nil)
		require.NoError(t, controllerutil.SetOwnerReference(host, existing, scheme))
		c := fakeclient.NewClientBuilder().WithScheme(scheme).WithObjects(host, existing).Build()

		m := NewManager(c, scheme, nil, logr.Discard())
		require.NoError(t, m.EnsureComponents(t.Context(), host))
	})
}

func TestNewManager(t *testing.T) {
	scheme := testutil.TestScheme()
	c := fakeclient.NewClientBuilder().WithScheme(scheme).Build()

	t.Run("panics on nil client", func(t *testing.T) {
		assert.Panics(t, func() { NewManager(nil, scheme, nil, logr.Discard()) })
	})
	t.Run("panics on nil scheme", func(t *testing.T) {
		assert.Panics(t, func() { NewManager(c, nil, nil, logr.Discard()) })
	})
}

// --- GetComponentsChanges ---

func TestGetComponentsChanges(t *testing.T) {
	updates := []metal3api.FirmwareUpdate{{Component: "bmc", URL: "http://example.com/bmc.bin"}}

	tests := []struct {
		scenario   string
		conditions []metav1.Condition
		wantDirty  bool
		wantErr    bool
	}{
		{scenario: "no HFC", conditions: nil, wantDirty: false},
		{scenario: "change detected and valid", conditions: condChanged, wantDirty: true},
		{scenario: "no change detected", conditions: condUnchanged, wantDirty: false},
		{scenario: "invalid", conditions: condInvalid, wantDirty: false},
		{
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
			c := fakeclient.NewClientBuilder().WithScheme(testutil.TestScheme()).Build()
			if tc.conditions != nil {
				require.NoError(t, c.Create(t.Context(), makeHFC(host, tc.conditions, updates, nil)))
			}

			m := NewManager(c, testutil.TestScheme(), nil, logr.Discard())
			dirty, _, err := m.GetComponentsChanges(t.Context(), host)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantDirty, dirty)
		})
	}
}

// --- ApplyError ---

func TestApplyError(t *testing.T) {
	updates := []metal3api.FirmwareUpdate{{Component: "bmc", URL: "http://example.com/bmc.bin"}}

	t.Run("no-op when dirty=false", func(t *testing.T) {
		host := makeTestHost()
		hfc := makeHFC(host, nil, updates, updates)
		scheme := testutil.TestScheme()
		c := fakeclient.NewClientBuilder().WithScheme(scheme).WithObjects(hfc).WithStatusSubresource(hfc).Build()
		m := NewManager(c, scheme, nil, logr.Discard())
		require.NoError(t, m.ApplyError(t.Context(), hfc, false))
		// Status not changed
		got := &metal3api.HostFirmwareComponents{}
		require.NoError(t, c.Get(t.Context(), hostKey(host), got))
		assert.Equal(t, updates, got.Status.Updates)
	})

	t.Run("no-op when hfc is nil", func(t *testing.T) {
		scheme := testutil.TestScheme()
		c := fakeclient.NewClientBuilder().WithScheme(scheme).Build()
		m := NewManager(c, scheme, nil, logr.Discard())
		require.NoError(t, m.ApplyError(t.Context(), nil, true))
	})

	t.Run("no-op when Updates already nil", func(t *testing.T) {
		host := makeTestHost()
		hfc := makeHFC(host, nil, updates, nil)
		scheme := testutil.TestScheme()
		c := fakeclient.NewClientBuilder().WithScheme(scheme).Build()
		m := NewManager(c, scheme, nil, logr.Discard())
		require.NoError(t, m.ApplyError(t.Context(), hfc, true))
	})

	t.Run("clears Updates and writes to Kubernetes", func(t *testing.T) {
		host := makeTestHost()
		hfc := makeHFC(host, nil, updates, updates)
		scheme := testutil.TestScheme()
		c := fakeclient.NewClientBuilder().WithScheme(scheme).
			WithObjects(hfc).WithStatusSubresource(hfc).Build()
		m := NewManager(c, scheme, nil, logr.Discard())
		require.NoError(t, m.ApplyError(t.Context(), hfc, true))

		got := &metal3api.HostFirmwareComponents{}
		require.NoError(t, c.Get(t.Context(), hostKey(host), got))
		assert.Nil(t, got.Status.Updates)
	})
}

// --- ApplyResult ---

func TestApplyResult(t *testing.T) {
	specUpdates := []metal3api.FirmwareUpdate{{Component: "bmc", URL: "http://example.com/bmc.bin"}}
	returnedComps := []metal3api.FirmwareComponentStatus{{Component: "bmc", CurrentVersion: "1.0"}}

	t.Run("no-op when dirty=false", func(t *testing.T) {
		host := makeTestHost()
		hfc := makeHFC(host, nil, specUpdates, nil)
		scheme := testutil.TestScheme()
		c := fakeclient.NewClientBuilder().WithScheme(scheme).Build()
		m := NewManager(c, scheme, nil, logr.Discard())
		require.NoError(t, m.ApplyResult(t.Context(), hfc, false, true))
		assert.Nil(t, hfc.Status.Updates)
	})

	t.Run("no-op when started=false", func(t *testing.T) {
		host := makeTestHost()
		hfc := makeHFC(host, nil, specUpdates, nil)
		scheme := testutil.TestScheme()
		c := fakeclient.NewClientBuilder().WithScheme(scheme).Build()
		m := NewManager(c, scheme, nil, logr.Discard())
		require.NoError(t, m.ApplyResult(t.Context(), hfc, true, false))
		assert.Nil(t, hfc.Status.Updates)
	})

	t.Run("no-op when Status.Updates already equals Spec.Updates", func(t *testing.T) {
		host := makeTestHost()
		hfc := makeHFC(host, nil, specUpdates, specUpdates)
		scheme := testutil.TestScheme()
		c := fakeclient.NewClientBuilder().WithScheme(scheme).Build()
		// Provisioner has components configured; if GetFirmwareComponents were called,
		// hfc.Status.Components would be non-empty.
		prov := makeFixtureProvisioner(host, returnedComps, nil)
		m := NewManager(c, scheme, prov, logr.Discard())
		require.NoError(t, m.ApplyResult(t.Context(), hfc, true, true))
		assert.Empty(t, hfc.Status.Components, "GetFirmwareComponents should not be called when updates are already equal")
	})

	t.Run("syncs updates and components to Kubernetes", func(t *testing.T) {
		host := makeTestHost()
		hfc := makeHFC(host, nil, specUpdates, nil)
		scheme := testutil.TestScheme()
		c := fakeclient.NewClientBuilder().WithScheme(scheme).
			WithObjects(hfc).WithStatusSubresource(hfc).Build()
		prov := makeFixtureProvisioner(host, returnedComps, nil)
		m := NewManager(c, scheme, prov, logr.Discard())
		require.NoError(t, m.ApplyResult(t.Context(), hfc, true, true))

		got := &metal3api.HostFirmwareComponents{}
		require.NoError(t, c.Get(t.Context(), hostKey(host), got))
		assert.Equal(t, specUpdates, got.Status.Updates)
		assert.Equal(t, returnedComps, got.Status.Components)
	})

	t.Run("error from provisioner is propagated", func(t *testing.T) {
		host := makeTestHost()
		hfc := makeHFC(host, nil, specUpdates, nil)
		scheme := testutil.TestScheme()
		c := fakeclient.NewClientBuilder().WithScheme(scheme).
			WithObjects(hfc).WithStatusSubresource(hfc).Build()
		provError := errors.New("I failed")
		prov := makeFixtureProvisioner(host, nil, provError)
		m := NewManager(c, scheme, prov, logr.Discard())
		err := m.ApplyResult(t.Context(), hfc, true, true)
		assert.ErrorIs(t, err, provError)
	})

	t.Run("nil provisioner returns error", func(t *testing.T) {
		host := makeTestHost()
		hfc := makeHFC(host, nil, specUpdates, nil)
		scheme := testutil.TestScheme()
		c := fakeclient.NewClientBuilder().WithScheme(scheme).Build()
		m := NewManager(c, scheme, nil, logr.Discard())
		err := m.ApplyResult(t.Context(), hfc, true, true)
		require.Error(t, err)
	})
}

// --- GetUpdatesDifference ---

func TestGetUpdatesDifference(t *testing.T) {
	spec := []metal3api.FirmwareUpdate{
		{Component: "bmc", URL: "http://example.com/bmc.bin"},
		{Component: "bios", URL: "http://example.com/bios-v2.bin"},
	}
	status := []metal3api.FirmwareUpdate{
		{Component: "bmc", URL: "http://example.com/bmc.bin"},
		{Component: "bios", URL: "http://example.com/bios-v1.bin"},
	}
	diff := GetUpdatesDifference(spec, status)
	assert.Equal(t, []metal3api.FirmwareUpdate{{Component: "bios", URL: "http://example.com/bios-v2.bin"}}, diff)

	assert.Empty(t, GetUpdatesDifference(spec, spec), "no diff when spec == status")
	assert.Equal(t, spec, GetUpdatesDifference(spec, nil), "all updates pending when status is empty")
}
