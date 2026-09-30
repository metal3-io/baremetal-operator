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

	"github.com/go-logr/logr"
	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	"github.com/metal3-io/baremetal-operator/pkg/firmwarecomponents"
	"github.com/metal3-io/baremetal-operator/pkg/firmwaresettings"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ServicingState holds the data collected for a pending servicing operation.
type ServicingState struct {
	data       provisioner.ServicingData // unexported; only PerformService uses it
	HasChanges bool
	HFCDirty   bool
	HFC        *metal3api.HostFirmwareComponents
}

// Manager manages hardware operations on a BareMetalHost.
type Manager struct {
	client      client.Client
	scheme      *runtime.Scheme
	Host        *metal3api.BareMetalHost
	Provisioner provisioner.Provisioner
	Log         logr.Logger
}

// ManagerInterface is the interface for hardware operations on a BareMetalHost.
type ManagerInterface interface {
	PerformPowerOff(ctx context.Context, mode metal3api.RebootMode) (provisioner.Result, error)
	PerformPowerOn(ctx context.Context) (provisioner.Result, error)
	CollectServicingState(ctx context.Context, hup *metal3api.HostUpdatePolicy) (ServicingState, error)
	PerformService(ctx context.Context, state ServicingState) (provisioner.Result, bool, error)
}

// NewManager returns a new Manager for hardware operations on a BareMetalHost.
func NewManager(c client.Client, scheme *runtime.Scheme, host *metal3api.BareMetalHost, prov provisioner.Provisioner, log logr.Logger) ManagerInterface {
	return &Manager{client: c, scheme: scheme, Host: host, Provisioner: prov, Log: log}
}

// PerformPowerOff issues a power-off to the provisioner.
// It escalates to a hard reboot if there are existing errors.
// The caller is responsible for interpreting the result and updating host status.
func (m *Manager) PerformPowerOff(ctx context.Context, mode metal3api.RebootMode) (provisioner.Result, error) {
	if m.Host.Status.ErrorCount > 0 {
		mode = metal3api.RebootModeHard
	}
	return m.Provisioner.PowerOff(ctx, mode,
		m.Host.Status.ErrorType == metal3api.PowerManagementError,
		m.Host.Spec.AutomatedCleaningMode)
}

// PerformPowerOn issues a power-on to the provisioner.
// The caller is responsible for interpreting the result and updating host status.
func (m *Manager) PerformPowerOn(ctx context.Context) (provisioner.Result, error) {
	return m.Provisioner.PowerOn(ctx, m.Host.Status.ErrorType == metal3api.PowerManagementError)
}

// CollectServicingState reads HostFirmwareSettings and HostFirmwareComponents via the
// firmwaresettings and firmwarecomponents managers to determine what firmware/BIOS changes
// are pending and builds the ServicingData.
func (m *Manager) CollectServicingState(ctx context.Context, hup *metal3api.HostUpdatePolicy) (ServicingState, error) {
	var state ServicingState

	var liveFirmwareSettingsAllowed, liveFirmwareUpdatesAllowed bool
	if hup != nil {
		liveFirmwareSettingsAllowed = hup.Spec.FirmwareSettings == metal3api.HostUpdatePolicyOnReboot
		liveFirmwareUpdatesAllowed = hup.Spec.FirmwareUpdates == metal3api.HostUpdatePolicyOnReboot
	}

	var hfsDirty bool
	if liveFirmwareSettingsAllowed {
		fsm := firmwaresettings.NewManager(m.client, m.scheme, m.Log)
		dirty, hfs, err := fsm.GetSettingsChanges(ctx, m.Host)
		if err != nil {
			return state, err
		}
		if dirty {
			hfsDirty = true
			state.data.ActualFirmwareSettings = hfs.Status.Settings
			state.data.TargetFirmwareSettings = hfs.Spec.Settings
		}
		if hfs != nil {
			state.data.HasFirmwareSpec = state.data.HasFirmwareSpec || len(hfs.Spec.Settings) > 0
		}
	}

	if liveFirmwareUpdatesAllowed {
		fcm := firmwarecomponents.NewManager(m.client, m.scheme, m.Provisioner, m.Log)
		dirty, hfc, err := fcm.GetComponentsChanges(ctx, m.Host)
		if err != nil {
			return state, err
		}
		if dirty {
			state.HFCDirty = true
			state.HFC = hfc
			if hfc.Status.Updates != nil {
				state.data.TargetFirmwareComponents = firmwarecomponents.GetUpdatesDifference(hfc.Spec.Updates, hfc.Status.Updates)
			} else {
				state.data.TargetFirmwareComponents = hfc.Spec.Updates
			}
		}
		if hfc != nil {
			state.data.HasFirmwareSpec = state.data.HasFirmwareSpec || len(hfc.Spec.Updates) > 0
		}
	}

	state.HasChanges = hfsDirty || state.HFCDirty
	return state, nil
}

// PerformService calls the provisioner's Service function.
// The caller is responsible for interpreting the result and updating host status.
func (m *Manager) PerformService(ctx context.Context, state ServicingState) (provisioner.Result, bool, error) {
	return m.Provisioner.Service(ctx, state.data, state.HasChanges, m.Host.Status.ErrorType == metal3api.ServicingError)
}
