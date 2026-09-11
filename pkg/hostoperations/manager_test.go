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

package hostoperations

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	"github.com/metal3-io/baremetal-operator/pkg/hardwareutils/bmc"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner/fixture"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// testScheme builds a minimal scheme for the manager tests.
func testScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	if err := metal3api.AddToScheme(s); err != nil {
		panic(err)
	}
	return s
}

// makeTestHost builds a minimal BareMetalHost for use in tests.
func makeTestHost() *metal3api.BareMetalHost {
	return &metal3api.BareMetalHost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-host",
			Namespace: "test-ns",
		},
	}
}

// makeTestHFS builds a HostFirmwareSettings with the given conditions.
func makeTestHFS(host *metal3api.BareMetalHost, conditions []metav1.Condition) *metal3api.HostFirmwareSettings {
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

// makeTestHFC builds a HostFirmwareComponents with the given conditions.
func makeTestHFC(host *metal3api.BareMetalHost, conditions []metav1.Condition, specUpdates, statusUpdates []metal3api.FirmwareUpdate) *metal3api.HostFirmwareComponents {
	return &metal3api.HostFirmwareComponents{
		ObjectMeta: metav1.ObjectMeta{Name: host.Name, Namespace: host.Namespace},
		Spec:       metal3api.HostFirmwareComponentsSpec{Updates: specUpdates},
		Status:     metal3api.HostFirmwareComponentsStatus{Updates: statusUpdates, Conditions: conditions},
	}
}

// makeFixtureProvisioner creates a fixture.Fixture and an associated Provisioner
// for the given host. Callers configure the Fixture before calling, and can
// inspect captured arguments on it after each provisioner call.
func makeFixtureProvisioner(host *metal3api.BareMetalHost, fix *fixture.Fixture) provisioner.Provisioner {
	prov, _ := fix.NewProvisioner(context.TODO(), provisioner.BuildHostData(*host, bmc.Credentials{}), func(_, _ string) {})
	return prov
}

var (
	condChanged = []metav1.Condition{
		{Type: "ChangeDetected", Status: "True", Reason: "Success"},
		{Type: "Valid", Status: "True", Reason: "Success"},
	}
	condUnchanged = []metav1.Condition{
		{Type: "ChangeDetected", Status: "False", Reason: "Success"},
		{Type: "Valid", Status: "True", Reason: "Success"},
	}
)

// --- CollectServicingState ---

func TestCollectServicingState(t *testing.T) {
	specUpdates := []metal3api.FirmwareUpdate{{Component: "bmc", URL: "http://example.com/bmc.exe"}}

	hupBoth := &metal3api.HostUpdatePolicy{
		Spec: metal3api.HostUpdatePolicySpec{
			FirmwareSettings: metal3api.HostUpdatePolicyOnReboot,
			FirmwareUpdates:  metal3api.HostUpdatePolicyOnReboot,
		},
	}
	hupSettingsOnly := &metal3api.HostUpdatePolicy{
		Spec: metal3api.HostUpdatePolicySpec{FirmwareSettings: metal3api.HostUpdatePolicyOnReboot},
	}
	hupUpdatesOnly := &metal3api.HostUpdatePolicy{
		Spec: metal3api.HostUpdatePolicySpec{FirmwareUpdates: metal3api.HostUpdatePolicyOnReboot},
	}

	t.Run("nil HUP disables all servicing", func(t *testing.T) {
		host := makeTestHost()
		scheme := testScheme()
		c := fakeclient.NewClientBuilder().WithScheme(scheme).Build()
		require.NoError(t, c.Create(t.Context(), makeTestHFS(host, condChanged)))
		require.NoError(t, c.Create(t.Context(), makeTestHFC(host, condChanged, specUpdates, nil)))

		m := NewManager(c, scheme, host, nil, logr.Discard())
		state, err := m.CollectServicingState(t.Context(), nil)
		require.NoError(t, err)
		assert.False(t, state.HasChanges)
		assert.False(t, state.HFCDirty)
	})

	t.Run("HFS dirty with settings policy", func(t *testing.T) {
		host := makeTestHost()
		scheme := testScheme()
		c := fakeclient.NewClientBuilder().WithScheme(scheme).Build()
		require.NoError(t, c.Create(t.Context(), makeTestHFS(host, condChanged)))

		m := NewManager(c, scheme, host, nil, logr.Discard())
		state, err := m.CollectServicingState(t.Context(), hupSettingsOnly)
		require.NoError(t, err)
		assert.True(t, state.HasChanges)
		assert.False(t, state.HFCDirty)
		assert.Nil(t, state.HFC)
	})

	t.Run("HFC dirty with updates policy", func(t *testing.T) {
		host := makeTestHost()
		scheme := testScheme()
		c := fakeclient.NewClientBuilder().WithScheme(scheme).Build()
		require.NoError(t, c.Create(t.Context(), makeTestHFC(host, condChanged, specUpdates, nil)))

		m := NewManager(c, scheme, host, nil, logr.Discard())
		state, err := m.CollectServicingState(t.Context(), hupUpdatesOnly)
		require.NoError(t, err)
		assert.True(t, state.HasChanges)
		assert.True(t, state.HFCDirty)
		assert.NotNil(t, state.HFC)
	})

	t.Run("nothing dirty", func(t *testing.T) {
		host := makeTestHost()
		scheme := testScheme()
		c := fakeclient.NewClientBuilder().WithScheme(scheme).Build()
		require.NoError(t, c.Create(t.Context(), makeTestHFS(host, condUnchanged)))
		require.NoError(t, c.Create(t.Context(), makeTestHFC(host, condUnchanged, specUpdates, specUpdates)))

		m := NewManager(c, scheme, host, nil, logr.Discard())
		state, err := m.CollectServicingState(t.Context(), hupBoth)
		require.NoError(t, err)
		assert.False(t, state.HasChanges)
		assert.False(t, state.HFCDirty)
	})

	t.Run("HFC diff excludes already-applied updates", func(t *testing.T) {
		host := makeTestHost()
		scheme := testScheme()
		applied := []metal3api.FirmwareUpdate{{Component: "bmc", URL: "http://example.com/bmc.exe"}}
		pending := []metal3api.FirmwareUpdate{
			{Component: "bmc", URL: "http://example.com/bmc.exe"},
			{Component: "bios", URL: "http://example.com/bios.exe"},
		}
		c := fakeclient.NewClientBuilder().WithScheme(scheme).Build()
		require.NoError(t, c.Create(t.Context(), makeTestHFC(host, condChanged, pending, applied)))

		m := NewManager(c, scheme, host, nil, logr.Discard())
		state, err := m.CollectServicingState(t.Context(), hupUpdatesOnly)
		require.NoError(t, err)
		assert.True(t, state.HFCDirty)
		assert.Equal(t, []metal3api.FirmwareUpdate{{Component: "bios", URL: "http://example.com/bios.exe"}},
			state.data.TargetFirmwareComponents)
	})
}

// --- PerformPowerOff ---

func TestPerformPowerOff(t *testing.T) {
	t.Run("soft mode passed through when no errors", func(t *testing.T) {
		host := makeTestHost()
		fix := &fixture.Fixture{PoweredOn: true}
		prov := makeFixtureProvisioner(host, fix)
		m := NewManager(nil, nil, host, prov, logr.Discard())
		_, err := m.PerformPowerOff(t.Context(), metal3api.RebootModeSoft)
		require.NoError(t, err)
		assert.Equal(t, metal3api.RebootModeSoft, fix.PowerOff.CapturedMode)
	})

	t.Run("escalates to hard when ErrorCount > 0", func(t *testing.T) {
		host := makeTestHost()
		host.Status.ErrorCount = 3
		fix := &fixture.Fixture{PoweredOn: true}
		prov := makeFixtureProvisioner(host, fix)
		m := NewManager(nil, nil, host, prov, logr.Discard())
		_, err := m.PerformPowerOff(t.Context(), metal3api.RebootModeSoft)
		require.NoError(t, err)
		assert.Equal(t, metal3api.RebootModeHard, fix.PowerOff.CapturedMode)
	})

	t.Run("sets force when PowerManagementError is active", func(t *testing.T) {
		host := makeTestHost()
		host.Status.ErrorType = metal3api.PowerManagementError
		fix := &fixture.Fixture{PoweredOn: true}
		prov := makeFixtureProvisioner(host, fix)
		m := NewManager(nil, nil, host, prov, logr.Discard())
		_, err := m.PerformPowerOff(t.Context(), metal3api.RebootModeSoft)
		require.NoError(t, err)
		assert.True(t, fix.PowerOff.CapturedForce)
	})

	t.Run("propagates provisioner error", func(t *testing.T) {
		host := makeTestHost()
		fix := &fixture.Fixture{PowerOff: fixture.PowerOffMock{Error: errTest}}
		prov := makeFixtureProvisioner(host, fix)
		m := NewManager(nil, nil, host, prov, logr.Discard())
		_, err := m.PerformPowerOff(t.Context(), metal3api.RebootModeSoft)
		assert.ErrorIs(t, err, errTest)
	})
}

// --- PerformPowerOn ---

func TestPerformPowerOn(t *testing.T) {
	t.Run("no force when no power management error", func(t *testing.T) {
		host := makeTestHost()
		fix := &fixture.Fixture{}
		prov := makeFixtureProvisioner(host, fix)
		m := NewManager(nil, nil, host, prov, logr.Discard())
		_, err := m.PerformPowerOn(t.Context())
		require.NoError(t, err)
		assert.False(t, fix.PowerOn.CapturedForce)
	})

	t.Run("sets force when PowerManagementError is active", func(t *testing.T) {
		host := makeTestHost()
		host.Status.ErrorType = metal3api.PowerManagementError
		fix := &fixture.Fixture{}
		prov := makeFixtureProvisioner(host, fix)
		m := NewManager(nil, nil, host, prov, logr.Discard())
		_, err := m.PerformPowerOn(t.Context())
		require.NoError(t, err)
		assert.True(t, fix.PowerOn.CapturedForce)
	})

	t.Run("propagates provisioner error", func(t *testing.T) {
		host := makeTestHost()
		fix := &fixture.Fixture{PowerOn: fixture.PowerOnMock{Error: errTest}}
		prov := makeFixtureProvisioner(host, fix)
		m := NewManager(nil, nil, host, prov, logr.Discard())
		_, err := m.PerformPowerOn(t.Context())
		assert.ErrorIs(t, err, errTest)
	})
}

// --- PerformService ---

func TestPerformService(t *testing.T) {
	t.Run("passes state data to provisioner and returns result", func(t *testing.T) {
		host := makeTestHost()
		fix := &fixture.Fixture{Service: fixture.ServiceMock{Started: true, Result: provisioner.Result{Dirty: true}}}
		prov := makeFixtureProvisioner(host, fix)
		m := NewManager(nil, nil, host, prov, logr.Discard())
		state := ServicingState{HasChanges: true}
		result, started, err := m.PerformService(t.Context(), state)
		require.NoError(t, err)
		assert.True(t, started)
		assert.True(t, result.Dirty)
	})

	t.Run("passes force when ServicingError is active", func(t *testing.T) {
		host := makeTestHost()
		host.Status.ErrorType = metal3api.ServicingError
		fix := &fixture.Fixture{}
		prov := makeFixtureProvisioner(host, fix)
		m := NewManager(nil, nil, host, prov, logr.Discard())
		_, _, err := m.PerformService(t.Context(), ServicingState{})
		require.NoError(t, err)
		assert.True(t, fix.Service.CapturedForce)
	})
}

var errTest = &testError{"test error"}

type testError struct{ msg string }

func (e *testError) Error() string { return e.msg }
