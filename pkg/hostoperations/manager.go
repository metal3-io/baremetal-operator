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
	"errors"
	"fmt"
	"reflect"

	"github.com/go-logr/logr"
	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	"github.com/metal3-io/baremetal-operator/pkg/logging"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	Host        *metal3api.BareMetalHost
	Provisioner provisioner.Provisioner
	Log         logr.Logger
}

// ManagerInterface is the interface for hardware operations on a BareMetalHost.
type ManagerInterface interface {
	PerformPowerOff(ctx context.Context, mode metal3api.RebootMode) (provisioner.Result, error)
	PerformPowerOn(ctx context.Context) (provisioner.Result, error)
	GetFirmwareSettingsChanges(ctx context.Context) (bool, *metal3api.HostFirmwareSettings, error)
	GetFirmwareComponentsChanges(ctx context.Context) (bool, *metal3api.HostFirmwareComponents, error)
	CollectServicingState(ctx context.Context, hup *metal3api.HostUpdatePolicy) (ServicingState, error)
	PerformService(ctx context.Context, state ServicingState) (provisioner.Result, bool, error)
	ApplyHFCServicingError(ctx context.Context, hfc *metal3api.HostFirmwareComponents, dirty bool) error
	ApplyHFCServicingResult(ctx context.Context, hfc *metal3api.HostFirmwareComponents, dirty, started bool) error
}

// NewManager returns a new Manager for hardware operations on a BareMetalHost.
func NewManager(c client.Client, host *metal3api.BareMetalHost, prov provisioner.Provisioner, log logr.Logger) ManagerInterface {
	return &Manager{client: c, Host: host, Provisioner: prov, Log: log}
}

// GetUpdatesDifference returns the firmware updates in spec that are not yet reflected in status.
func GetUpdatesDifference(specUpdates, statusUpdates []metal3api.FirmwareUpdate) []metal3api.FirmwareUpdate {
	diff := []metal3api.FirmwareUpdate{}
	updated := make(map[string]string, len(statusUpdates))
	for _, s := range statusUpdates {
		updated[s.Component] = s.URL
	}
	for _, firmware := range specUpdates {
		if _, ok := updated[firmware.Component]; !ok || firmware.URL != updated[firmware.Component] {
			diff = append(diff, firmware)
		}
	}
	return diff
}

// hostObjectHasChanges checks whether an HFS/HFC object has valid pending changes.
// HFS and HFC both use "ChangeDetected" and "Valid" as their condition type strings
// (metal3api.FirmwareSettingsChangeDetected == metal3api.HostFirmwareComponentsChangeDetected).
func hostObjectHasChanges(conditions []metav1.Condition, expectedGeneration int64) (changed, valid bool, err error) {
	readyCond := meta.FindStatusCondition(conditions, string(metal3api.FirmwareSettingsChangeDetected))
	if readyCond == nil {
		return false, false, nil
	}
	if readyCond.ObservedGeneration != expectedGeneration {
		return false, false, fmt.Errorf("generation %d != observed generation %d", expectedGeneration, readyCond.ObservedGeneration)
	}
	if !meta.IsStatusConditionTrue(conditions, string(metal3api.FirmwareSettingsValid)) {
		return false, false, nil
	}
	if readyCond.Status == metav1.ConditionTrue {
		return true, true, nil
	}
	return false, true, nil
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

// GetFirmwareSettingsChanges returns dirty=true if there are valid pending firmware settings
// changes on the HostFirmwareSettings for this host. The hfs object is returned when available
// regardless of validity, so callers can inspect spec contents.
func (m *Manager) GetFirmwareSettingsChanges(ctx context.Context) (dirty bool, hfs *metal3api.HostFirmwareSettings, err error) {
	hostKey := client.ObjectKey{Name: m.Host.Name, Namespace: m.Host.Namespace}
	hfs = &metal3api.HostFirmwareSettings{}
	if err = m.client.Get(ctx, hostKey, hfs); err != nil {
		if !k8serrors.IsNotFound(err) {
			return false, nil, fmt.Errorf("could not load host firmware settings: %w", err)
		}
		m.Log.V(logging.VerbosityLevelDebug).Info("could not get hostFirmwareSettings",
			logging.LogFieldNamespace, hostKey)
		return false, nil, nil
	}

	changed, valid, err := hostObjectHasChanges(hfs.Status.Conditions, hfs.GetGeneration())
	if err != nil {
		return false, nil, fmt.Errorf("hostFirmwareSettings not ready yet: %w", err)
	}
	if !valid {
		m.Log.Info("hostFirmwareSettings not valid", logging.LogFieldNamespace, hostKey)
		return false, hfs, nil
	}

	if changed {
		if len(hfs.Status.Settings) == 0 {
			return false, nil, errors.New("host firmware status settings not available")
		}
		m.Log.Info("hostFirmwareSettings indicating ChangeDetected", logging.LogFieldNamespace, hostKey)
		return true, hfs, nil
	}

	m.Log.V(logging.VerbosityLevelTrace).Info("hostFirmwareSettings no updates", logging.LogFieldNamespace, hostKey)
	return false, hfs, nil
}

// GetFirmwareComponentsChanges returns dirty=true if there are valid pending firmware component
// changes on the HostFirmwareComponents for this host. The hfc object is returned when available
// regardless of validity, so callers can inspect spec contents.
func (m *Manager) GetFirmwareComponentsChanges(ctx context.Context) (dirty bool, hfc *metal3api.HostFirmwareComponents, err error) {
	hostKey := client.ObjectKey{Name: m.Host.Name, Namespace: m.Host.Namespace}
	hfc = &metal3api.HostFirmwareComponents{}
	if err = m.client.Get(ctx, hostKey, hfc); err != nil {
		if !k8serrors.IsNotFound(err) {
			return false, nil, fmt.Errorf("could not load host firmware components: %w", err)
		}
		m.Log.V(logging.VerbosityLevelDebug).Info("could not get hostFirmwareComponents",
			logging.LogFieldNamespace, hostKey)
		return false, nil, nil
	}

	changed, valid, err := hostObjectHasChanges(hfc.Status.Conditions, hfc.GetGeneration())
	if err != nil {
		return false, nil, fmt.Errorf("hostFirmwareComponents not ready yet: %w", err)
	}
	if !valid {
		m.Log.Info("hostFirmwareComponents not valid", logging.LogFieldNamespace, hostKey)
		return false, hfc, nil
	}
	if changed {
		m.Log.Info("hostFirmwareComponents indicating ChangeDetected", logging.LogFieldNamespace, hostKey)
		return true, hfc, nil
	}

	m.Log.V(logging.VerbosityLevelTrace).Info("hostFirmwareComponents no updates", logging.LogFieldNamespace, hostKey)
	return false, hfc, nil
}

// CollectServicingState reads HostFirmwareSettings and HostFirmwareComponents to
// determine what firmware/BIOS changes are pending and builds the ServicingData.
// The caller is responsible for any status updates based on the returned state.
func (m *Manager) CollectServicingState(ctx context.Context, hup *metal3api.HostUpdatePolicy) (ServicingState, error) {
	var state ServicingState

	var liveFirmwareSettingsAllowed, liveFirmwareUpdatesAllowed bool
	if hup != nil {
		liveFirmwareSettingsAllowed = hup.Spec.FirmwareSettings == metal3api.HostUpdatePolicyOnReboot
		liveFirmwareUpdatesAllowed = hup.Spec.FirmwareUpdates == metal3api.HostUpdatePolicyOnReboot
	}

	var hfsDirty bool
	if liveFirmwareSettingsAllowed {
		dirty, hfs, err := m.GetFirmwareSettingsChanges(ctx)
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
		dirty, hfc, err := m.GetFirmwareComponentsChanges(ctx)
		if err != nil {
			return state, err
		}
		if dirty {
			state.HFCDirty = true
			state.HFC = hfc
			if hfc.Status.Updates != nil {
				state.data.TargetFirmwareComponents = GetUpdatesDifference(hfc.Spec.Updates, hfc.Status.Updates)
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

// ApplyHFCServicingError clears pending updates from HFC status after a Prepare or Service
// error. No-op if dirty is false or if there are no pending updates to clear.
// Writes to Kubernetes as needed.
func (m *Manager) ApplyHFCServicingError(ctx context.Context, hfc *metal3api.HostFirmwareComponents, dirty bool) error {
	if !dirty || hfc == nil || hfc.Status.Updates == nil {
		return nil
	}
	hfc.Status.Updates = nil
	return m.client.Status().Update(ctx, hfc)
}

// ApplyHFCServicingResult syncs HostFirmwareComponents status after a successful Prepare or
// Service provisioner call. No-op if dirty or started is false.
// Copies spec updates to status and refreshes component versions from the provisioner.
// Writes to Kubernetes as needed.
func (m *Manager) ApplyHFCServicingResult(ctx context.Context, hfc *metal3api.HostFirmwareComponents, dirty, started bool) error {
	if !dirty || !started {
		return nil
	}
	if reflect.DeepEqual(hfc.Status.Updates, hfc.Spec.Updates) {
		return nil
	}
	hfc.Status.Updates = append([]metal3api.FirmwareUpdate(nil), hfc.Spec.Updates...)
	components, err := m.Provisioner.GetFirmwareComponents(ctx)
	if err != nil {
		return fmt.Errorf("failed to get firmware components: %w", err)
	}
	hfc.Status.Components = components
	return m.client.Status().Update(ctx, hfc)
}
