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

package hostoperations

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
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

// fakeProvisioner is a test double for provisioner.Provisioner that records
// call arguments and returns configurable results.
type fakeProvisioner struct {
	powerOnForce  bool
	powerOnResult provisioner.Result
	powerOnErr    error

	powerOffMode   metal3api.RebootMode
	powerOffForce  bool
	powerOffResult provisioner.Result
	powerOffErr    error

	serviceData    provisioner.ServicingData
	serviceForce   bool
	serviceResult  provisioner.Result
	serviceStarted bool
	serviceErr     error

	firmwareComps       []metal3api.FirmwareComponentStatus
	firmwareErr         error
	firmwareCompsCalled bool
}

func (f *fakeProvisioner) PowerOn(_ context.Context, force bool) (provisioner.Result, error) {
	f.powerOnForce = force
	return f.powerOnResult, f.powerOnErr
}

func (f *fakeProvisioner) PowerOff(_ context.Context, mode metal3api.RebootMode, force bool, _ metal3api.AutomatedCleaningMode) (provisioner.Result, error) {
	f.powerOffMode = mode
	f.powerOffForce = force
	return f.powerOffResult, f.powerOffErr
}

func (f *fakeProvisioner) Service(_ context.Context, data provisioner.ServicingData, _ bool, force bool) (provisioner.Result, bool, error) {
	f.serviceData = data
	f.serviceForce = force
	return f.serviceResult, f.serviceStarted, f.serviceErr
}

func (f *fakeProvisioner) GetFirmwareComponents(_ context.Context) ([]metal3api.FirmwareComponentStatus, error) {
	f.firmwareCompsCalled = true
	return f.firmwareComps, f.firmwareErr
}

// No-op stubs to satisfy the full provisioner.Provisioner interface.
func (*fakeProvisioner) Register(_ context.Context, _ provisioner.ManagementAccessData, _, _ bool) (provisioner.Result, string, error) {
	return provisioner.Result{}, "", nil
}
func (*fakeProvisioner) PreprovisioningImageFormats(_ context.Context) ([]metal3api.ImageFormat, error) {
	return nil, nil
}
func (*fakeProvisioner) InspectHardware(_ context.Context, _ provisioner.InspectData, _, _, _ bool) (provisioner.Result, bool, *metal3api.HardwareDetails, error) {
	return provisioner.Result{}, false, nil, nil
}
func (*fakeProvisioner) UpdateHardwareState(_ context.Context) (provisioner.HardwareState, error) {
	return provisioner.HardwareState{}, nil
}
func (*fakeProvisioner) Adopt(_ context.Context, _ provisioner.AdoptData, _ bool) (provisioner.Result, error) {
	return provisioner.Result{}, nil
}
func (*fakeProvisioner) Prepare(_ context.Context, _ provisioner.PrepareData, _, _ bool) (provisioner.Result, bool, error) {
	return provisioner.Result{}, false, nil
}
func (*fakeProvisioner) Provision(_ context.Context, _ provisioner.ProvisionData, _ bool) (provisioner.Result, error) {
	return provisioner.Result{}, nil
}
func (*fakeProvisioner) Deprovision(_ context.Context, _ bool, _ metal3api.AutomatedCleaningMode) (provisioner.Result, error) {
	return provisioner.Result{}, nil
}
func (*fakeProvisioner) Delete(_ context.Context) (provisioner.Result, error) {
	return provisioner.Result{}, nil
}
func (*fakeProvisioner) Detach(_ context.Context, _ bool) (provisioner.Result, error) {
	return provisioner.Result{}, nil
}
func (*fakeProvisioner) HasCapacity(_ context.Context) (bool, error) { return true, nil }
func (*fakeProvisioner) GetFirmwareSettings(_ context.Context, _ bool) (metal3api.SettingsMap, map[string]metal3api.SettingSchema, error) {
	return nil, nil, nil
}
func (*fakeProvisioner) AddBMCEventSubscriptionForNode(_ context.Context, _ *metal3api.BMCEventSubscription, _ provisioner.HTTPHeaders) (provisioner.Result, error) {
	return provisioner.Result{}, nil
}
func (*fakeProvisioner) RemoveBMCEventSubscriptionForNode(_ context.Context, _ metal3api.BMCEventSubscription) (provisioner.Result, error) {
	return provisioner.Result{}, nil
}
func (*fakeProvisioner) GetDataImageStatus(_ context.Context) (bool, error) { return false, nil }
func (*fakeProvisioner) AttachDataImage(_ context.Context, _ string) error  { return nil }
func (*fakeProvisioner) DetachDataImage(_ context.Context) error            { return nil }
func (*fakeProvisioner) HasPowerFailure(_ context.Context) bool             { return false }
func (*fakeProvisioner) GetHealth(_ context.Context) string                 { return "" }
func (*fakeProvisioner) AbortServicing(_ context.Context) (provisioner.Result, error) {
	return provisioner.Result{}, nil
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
// Status.Settings is pre-populated so dirty=true cases don't error.
func makeTestHFS(host *metal3api.BareMetalHost, conditions []metav1.Condition) *metal3api.HostFirmwareSettings {
	hfs := &metal3api.HostFirmwareSettings{
		ObjectMeta: metav1.ObjectMeta{
			Name:      host.Name,
			Namespace: host.Namespace,
		},
		Spec: metal3api.HostFirmwareSettingsSpec{
			Settings: metal3api.DesiredSettingsMap{
				"ProcVirtualization": intstr.FromString("Disabled"),
			},
		},
		Status: metal3api.HostFirmwareSettingsStatus{
			Settings: metal3api.SettingsMap{
				"ProcVirtualization": "Disabled",
			},
			Conditions: conditions,
		},
	}
	return hfs
}

// makeTestHFC builds a HostFirmwareComponents with the given conditions.
func makeTestHFC(host *metal3api.BareMetalHost, conditions []metav1.Condition, specUpdates, statusUpdates []metal3api.FirmwareUpdate) *metal3api.HostFirmwareComponents {
	return &metal3api.HostFirmwareComponents{
		ObjectMeta: metav1.ObjectMeta{
			Name:      host.Name,
			Namespace: host.Namespace,
		},
		Spec: metal3api.HostFirmwareComponentsSpec{
			Updates: specUpdates,
		},
		Status: metal3api.HostFirmwareComponentsStatus{
			Updates:    statusUpdates,
			Conditions: conditions,
		},
	}
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
	condInvalid = []metav1.Condition{
		{Type: "ChangeDetected", Status: "True", Reason: "Success"},
		{Type: "Valid", Status: "False", Reason: "Failure"},
	}
)

func TestGetUpdatesDifference(t *testing.T) {
	specUpdates := []metal3api.FirmwareUpdate{
		{Component: "bmc", URL: "http://example.com/bmc.exe"},
		{Component: "bios", URL: "http://example.com/bios-v1.exe"},
		{Component: "bios", URL: "http://example.com/bios-v2.exe"},
	}
	statusUpdates := []metal3api.FirmwareUpdate{
		{Component: "bmc", URL: "http://example.com/bmc.exe"},
		{Component: "bios", URL: "http://example.com/bios-v1.exe"},
	}
	diff := GetUpdatesDifference(specUpdates, statusUpdates)
	assert.Equal(t, []metal3api.FirmwareUpdate{{Component: "bios", URL: "http://example.com/bios-v2.exe"}}, diff)
}

func TestGetUpdatesDifferenceEmpty(t *testing.T) {
	spec := []metal3api.FirmwareUpdate{{Component: "bmc", URL: "http://example.com/bmc.exe"}}
	// Same in both spec and status → no difference.
	assert.Empty(t, GetUpdatesDifference(spec, spec))
	// Empty status → all spec items are new.
	assert.Equal(t, spec, GetUpdatesDifference(spec, nil))
}

func TestGetFirmwareSettingsChanges(t *testing.T) {
	tests := []struct {
		scenario   string
		conditions []metav1.Condition
		wantDirty  bool
		wantErr    bool
	}{
		{
			scenario:   "no HFS resource",
			conditions: nil, // HFS won't be created
			wantDirty:  false,
		},
		{
			scenario:   "change detected and valid",
			conditions: condChanged,
			wantDirty:  true,
		},
		{
			scenario:   "no change detected",
			conditions: condUnchanged,
			wantDirty:  false,
		},
		{
			scenario:   "invalid settings",
			conditions: condInvalid,
			wantDirty:  false,
		},
		{
			// ObservedGeneration=1 won't match the object's Generation=0 after Create.
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
			c := fakeclient.NewClientBuilder().WithScheme(testScheme()).Build()
			if tc.conditions != nil {
				hfs := makeTestHFS(host, tc.conditions)
				require.NoError(t, c.Create(t.Context(), hfs))
			}

			m := NewManager(c, host, nil, logr.Discard())
			dirty, _, err := m.GetFirmwareSettingsChanges(t.Context())
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantDirty, dirty)
		})
	}
}

func TestGetFirmwareComponentsChanges(t *testing.T) {
	updates := []metal3api.FirmwareUpdate{{Component: "bmc", URL: "http://example.com/bmc.exe"}}

	tests := []struct {
		scenario   string
		conditions []metav1.Condition
		wantDirty  bool
		wantErr    bool
	}{
		{
			scenario:   "no HFC resource",
			conditions: nil,
			wantDirty:  false,
		},
		{
			scenario:   "change detected and valid",
			conditions: condChanged,
			wantDirty:  true,
		},
		{
			scenario:   "no change detected",
			conditions: condUnchanged,
			wantDirty:  false,
		},
		{
			scenario:   "invalid",
			conditions: condInvalid,
			wantDirty:  false,
		},
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
			c := fakeclient.NewClientBuilder().WithScheme(testScheme()).Build()
			if tc.conditions != nil {
				hfc := makeTestHFC(host, tc.conditions, updates, nil)
				require.NoError(t, c.Create(t.Context(), hfc))
			}

			m := NewManager(c, host, nil, logr.Discard())
			dirty, _, err := m.GetFirmwareComponentsChanges(t.Context())
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantDirty, dirty)
		})
	}
}

func TestCollectServicingState(t *testing.T) {
	specUpdates := []metal3api.FirmwareUpdate{{Component: "bmc", URL: "http://example.com/bmc.exe"}}

	hupBoth := &metal3api.HostUpdatePolicy{
		Spec: metal3api.HostUpdatePolicySpec{
			FirmwareSettings: metal3api.HostUpdatePolicyOnReboot,
			FirmwareUpdates:  metal3api.HostUpdatePolicyOnReboot,
		},
	}
	hupSettingsOnly := &metal3api.HostUpdatePolicy{
		Spec: metal3api.HostUpdatePolicySpec{
			FirmwareSettings: metal3api.HostUpdatePolicyOnReboot,
		},
	}
	hupUpdatesOnly := &metal3api.HostUpdatePolicy{
		Spec: metal3api.HostUpdatePolicySpec{
			FirmwareUpdates: metal3api.HostUpdatePolicyOnReboot,
		},
	}

	t.Run("nil HUP disables all servicing", func(t *testing.T) {
		host := makeTestHost()
		c := fakeclient.NewClientBuilder().WithScheme(testScheme()).Build()
		// Create dirty HFS and HFC — should be ignored because HUP is nil.
		require.NoError(t, c.Create(t.Context(), makeTestHFS(host, condChanged)))
		require.NoError(t, c.Create(t.Context(), makeTestHFC(host, condChanged, specUpdates, nil)))

		m := NewManager(c, host, nil, logr.Discard())
		state, err := m.CollectServicingState(t.Context(), nil)
		require.NoError(t, err)
		assert.False(t, state.HasChanges)
		assert.False(t, state.HFCDirty)
	})

	t.Run("HFS dirty with settings policy", func(t *testing.T) {
		host := makeTestHost()
		c := fakeclient.NewClientBuilder().WithScheme(testScheme()).Build()
		require.NoError(t, c.Create(t.Context(), makeTestHFS(host, condChanged)))

		m := NewManager(c, host, nil, logr.Discard())
		state, err := m.CollectServicingState(t.Context(), hupSettingsOnly)
		require.NoError(t, err)
		assert.True(t, state.HasChanges)
		assert.False(t, state.HFCDirty)
		assert.Nil(t, state.HFC)
	})

	t.Run("HFC dirty with updates policy", func(t *testing.T) {
		host := makeTestHost()
		c := fakeclient.NewClientBuilder().WithScheme(testScheme()).Build()
		require.NoError(t, c.Create(t.Context(), makeTestHFC(host, condChanged, specUpdates, nil)))

		m := NewManager(c, host, nil, logr.Discard())
		state, err := m.CollectServicingState(t.Context(), hupUpdatesOnly)
		require.NoError(t, err)
		assert.True(t, state.HasChanges)
		assert.True(t, state.HFCDirty)
		assert.NotNil(t, state.HFC)
	})

	t.Run("nothing dirty", func(t *testing.T) {
		host := makeTestHost()
		c := fakeclient.NewClientBuilder().WithScheme(testScheme()).Build()
		require.NoError(t, c.Create(t.Context(), makeTestHFS(host, condUnchanged)))
		require.NoError(t, c.Create(t.Context(), makeTestHFC(host, condUnchanged, specUpdates, specUpdates)))

		m := NewManager(c, host, nil, logr.Discard())
		state, err := m.CollectServicingState(t.Context(), hupBoth)
		require.NoError(t, err)
		assert.False(t, state.HasChanges)
		assert.False(t, state.HFCDirty)
	})

	t.Run("HFC diff excludes already-applied updates", func(t *testing.T) {
		host := makeTestHost()
		applied := []metal3api.FirmwareUpdate{{Component: "bmc", URL: "http://example.com/bmc.exe"}}
		pending := []metal3api.FirmwareUpdate{
			{Component: "bmc", URL: "http://example.com/bmc.exe"},
			{Component: "bios", URL: "http://example.com/bios.exe"},
		}
		c := fakeclient.NewClientBuilder().WithScheme(testScheme()).Build()
		require.NoError(t, c.Create(t.Context(), makeTestHFC(host, condChanged, pending, applied)))

		m := NewManager(c, host, nil, logr.Discard())
		state, err := m.CollectServicingState(t.Context(), hupUpdatesOnly)
		require.NoError(t, err)
		assert.True(t, state.HFCDirty)
		assert.Equal(t, []metal3api.FirmwareUpdate{{Component: "bios", URL: "http://example.com/bios.exe"}},
			state.data.TargetFirmwareComponents)
	})
}

func TestPerformPowerOff(t *testing.T) {
	t.Run("soft mode passed through when no errors", func(t *testing.T) {
		host := makeTestHost()
		prov := &fakeProvisioner{powerOffResult: provisioner.Result{}}
		m := NewManager(nil, host, prov, logr.Discard())
		_, err := m.PerformPowerOff(t.Context(), metal3api.RebootModeSoft)
		require.NoError(t, err)
		assert.Equal(t, metal3api.RebootModeSoft, prov.powerOffMode)
	})

	t.Run("escalates to hard when ErrorCount > 0", func(t *testing.T) {
		host := makeTestHost()
		host.Status.ErrorCount = 3
		prov := &fakeProvisioner{}
		m := NewManager(nil, host, prov, logr.Discard())
		_, err := m.PerformPowerOff(t.Context(), metal3api.RebootModeSoft)
		require.NoError(t, err)
		assert.Equal(t, metal3api.RebootModeHard, prov.powerOffMode)
	})

	t.Run("sets force when PowerManagementError is active", func(t *testing.T) {
		host := makeTestHost()
		host.Status.ErrorType = metal3api.PowerManagementError
		prov := &fakeProvisioner{}
		m := NewManager(nil, host, prov, logr.Discard())
		_, err := m.PerformPowerOff(t.Context(), metal3api.RebootModeSoft)
		require.NoError(t, err)
		assert.True(t, prov.powerOffForce)
	})

	t.Run("propagates provisioner error", func(t *testing.T) {
		host := makeTestHost()
		prov := &fakeProvisioner{powerOffErr: errTest}
		m := NewManager(nil, host, prov, logr.Discard())
		_, err := m.PerformPowerOff(t.Context(), metal3api.RebootModeSoft)
		assert.ErrorIs(t, err, errTest)
	})
}

func TestPerformPowerOn(t *testing.T) {
	t.Run("no force when no power management error", func(t *testing.T) {
		host := makeTestHost()
		prov := &fakeProvisioner{}
		m := NewManager(nil, host, prov, logr.Discard())
		_, err := m.PerformPowerOn(t.Context())
		require.NoError(t, err)
		assert.False(t, prov.powerOnForce)
	})

	t.Run("sets force when PowerManagementError is active", func(t *testing.T) {
		host := makeTestHost()
		host.Status.ErrorType = metal3api.PowerManagementError
		prov := &fakeProvisioner{}
		m := NewManager(nil, host, prov, logr.Discard())
		_, err := m.PerformPowerOn(t.Context())
		require.NoError(t, err)
		assert.True(t, prov.powerOnForce)
	})

	t.Run("propagates provisioner error", func(t *testing.T) {
		host := makeTestHost()
		prov := &fakeProvisioner{powerOnErr: errTest}
		m := NewManager(nil, host, prov, logr.Discard())
		_, err := m.PerformPowerOn(t.Context())
		assert.ErrorIs(t, err, errTest)
	})
}

func TestPerformService(t *testing.T) {
	t.Run("passes state data to provisioner and returns result", func(t *testing.T) {
		host := makeTestHost()
		prov := &fakeProvisioner{serviceStarted: true, serviceResult: provisioner.Result{Dirty: true}}
		m := NewManager(nil, host, prov, logr.Discard())
		state := ServicingState{HasChanges: true}
		result, started, err := m.PerformService(t.Context(), state)
		require.NoError(t, err)
		assert.True(t, started)
		assert.True(t, result.Dirty)
	})

	t.Run("passes force when ServicingError is active", func(t *testing.T) {
		host := makeTestHost()
		host.Status.ErrorType = metal3api.ServicingError
		prov := &fakeProvisioner{}
		m := NewManager(nil, host, prov, logr.Discard())
		_, _, err := m.PerformService(t.Context(), ServicingState{})
		require.NoError(t, err)
		assert.True(t, prov.serviceForce)
	})
}

func TestApplyHFCServicingError(t *testing.T) {
	updates := []metal3api.FirmwareUpdate{{Component: "bmc", URL: "http://example.com/bmc.exe"}}

	t.Run("no-op when dirty=false", func(t *testing.T) {
		host := makeTestHost()
		c := fakeclient.NewClientBuilder().WithScheme(testScheme()).Build()
		hfc := makeTestHFC(host, nil, updates, updates)
		require.NoError(t, c.Create(t.Context(), hfc))

		m := NewManager(c, host, nil, logr.Discard())
		require.NoError(t, m.ApplyHFCServicingError(t.Context(), hfc, false))

		got := &metal3api.HostFirmwareComponents{}
		require.NoError(t, c.Get(t.Context(), hostKey(host), got))
		assert.Equal(t, updates, got.Status.Updates, "updates should not be cleared")
	})

	t.Run("no-op when Updates already nil", func(t *testing.T) {
		host := makeTestHost()
		c := fakeclient.NewClientBuilder().WithScheme(testScheme()).Build()
		hfc := makeTestHFC(host, nil, updates, nil)
		require.NoError(t, c.Create(t.Context(), hfc))

		m := NewManager(c, host, nil, logr.Discard())
		require.NoError(t, m.ApplyHFCServicingError(t.Context(), hfc, true))
	})

	t.Run("clears Updates and writes to Kubernetes", func(t *testing.T) {
		host := makeTestHost()
		hfc := makeTestHFC(host, nil, updates, updates)
		c := fakeclient.NewClientBuilder().WithScheme(testScheme()).
			WithObjects(hfc).
			WithStatusSubresource(hfc).
			Build()

		m := NewManager(c, host, nil, logr.Discard())
		require.NoError(t, m.ApplyHFCServicingError(t.Context(), hfc, true))

		got := &metal3api.HostFirmwareComponents{}
		require.NoError(t, c.Get(t.Context(), hostKey(host), got))
		assert.Nil(t, got.Status.Updates, "Updates should have been cleared")
	})
}

func TestApplyHFCServicingResult(t *testing.T) {
	specUpdates := []metal3api.FirmwareUpdate{{Component: "bmc", URL: "http://example.com/bmc.exe"}}
	returnedComps := []metal3api.FirmwareComponentStatus{{Component: "bmc", CurrentVersion: "1.0.0"}}

	t.Run("no-op when dirty=false", func(t *testing.T) {
		host := makeTestHost()
		prov := &fakeProvisioner{firmwareComps: returnedComps}
		m := NewManager(nil, host, prov, logr.Discard())
		hfc := makeTestHFC(host, nil, specUpdates, nil)
		require.NoError(t, m.ApplyHFCServicingResult(t.Context(), hfc, false, true))
		assert.Nil(t, hfc.Status.Updates, "in-memory HFC should not be modified")
	})

	t.Run("no-op when started=false", func(t *testing.T) {
		host := makeTestHost()
		prov := &fakeProvisioner{firmwareComps: returnedComps}
		m := NewManager(nil, host, prov, logr.Discard())
		hfc := makeTestHFC(host, nil, specUpdates, nil)
		require.NoError(t, m.ApplyHFCServicingResult(t.Context(), hfc, true, false))
		assert.Nil(t, hfc.Status.Updates)
	})

	t.Run("no-op when Updates already equal to Spec", func(t *testing.T) {
		host := makeTestHost()
		// Status.Updates already matches Spec.Updates — no write should happen.
		hfc := makeTestHFC(host, nil, specUpdates, specUpdates)
		prov := &fakeProvisioner{firmwareComps: returnedComps}
		m := NewManager(nil, host, prov, logr.Discard())
		require.NoError(t, m.ApplyHFCServicingResult(t.Context(), hfc, true, true))
		assert.False(t, prov.firmwareCompsCalled, "GetFirmwareComponents should not be called when updates are already equal")
	})

	t.Run("syncs updates and components to Kubernetes", func(t *testing.T) {
		host := makeTestHost()
		hfc := makeTestHFC(host, nil, specUpdates, nil) // Status.Updates differs from Spec.Updates
		c := fakeclient.NewClientBuilder().WithScheme(testScheme()).
			WithObjects(hfc).
			WithStatusSubresource(hfc).
			Build()

		prov := &fakeProvisioner{firmwareComps: returnedComps}
		m := NewManager(c, host, prov, logr.Discard())
		require.NoError(t, m.ApplyHFCServicingResult(t.Context(), hfc, true, true))

		got := &metal3api.HostFirmwareComponents{}
		require.NoError(t, c.Get(t.Context(), hostKey(host), got))
		assert.Equal(t, specUpdates, got.Status.Updates, "Updates should be copied from Spec")
		assert.Equal(t, returnedComps, got.Status.Components, "Components should come from provisioner")
	})

	t.Run("propagates GetFirmwareComponents error", func(t *testing.T) {
		host := makeTestHost()
		c := fakeclient.NewClientBuilder().WithScheme(testScheme()).Build()
		hfc := makeTestHFC(host, nil, specUpdates, nil)
		require.NoError(t, c.Create(t.Context(), hfc))

		prov := &fakeProvisioner{firmwareErr: errTest}
		m := NewManager(c, host, prov, logr.Discard())
		err := m.ApplyHFCServicingResult(t.Context(), hfc, true, true)
		assert.ErrorIs(t, err, errTest)
	})
}

// errTest is a sentinel error used across tests.
var errTest = &testError{"test error"}

type testError struct{ msg string }

func (e *testError) Error() string { return e.msg }

// hostKey returns the ObjectKey for a host.
func hostKey(host *metal3api.BareMetalHost) client.ObjectKey {
	return client.ObjectKey{Namespace: host.Namespace, Name: host.Name}
}
