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

package controllers

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// performPowerOff issues a power-off to the provisioner.
// It escalates to a hard reboot if there are existing errors.
// The caller is responsible for interpreting the result and updating host status.
func performPowerOff(ctx context.Context, prov provisioner.Provisioner, host *metal3api.BareMetalHost, mode metal3api.RebootMode) (provisioner.Result, error) {
	if host.Status.ErrorCount > 0 {
		mode = metal3api.RebootModeHard
	}
	return prov.PowerOff(ctx, mode,
		host.Status.ErrorType == metal3api.PowerManagementError,
		host.Spec.AutomatedCleaningMode)
}

// performPowerOn issues a power-on to the provisioner.
// The caller is responsible for interpreting the result and updating host status.
func performPowerOn(ctx context.Context, prov provisioner.Provisioner, host *metal3api.BareMetalHost) (provisioner.Result, error) {
	return prov.PowerOn(ctx, host.Status.ErrorType == metal3api.PowerManagementError)
}

// servicingState holds the data collected for a pending servicing operation.
type servicingState struct {
	data       provisioner.ServicingData
	hasChanges bool
	hfsDirty   bool
	hfcDirty   bool
	hfc        *metal3api.HostFirmwareComponents
}

// collectServicingState reads HostFirmwareSettings and HostFirmwareComponents to
// determine what firmware/BIOS changes are pending and builds the ServicingData.
// The caller is responsible for any status updates based on the returned state.
//
// (NOTE)janders: Servicing is an opt-in feature requiring a HostUpdatePolicy with
// FirmwareSettings or FirmwareUpdates set to onReboot.
func collectServicingState(ctx context.Context, reader client.Reader, host *metal3api.BareMetalHost, hup *metal3api.HostUpdatePolicy) (servicingState, error) {
	var state servicingState
	hostKey := client.ObjectKey{Name: host.Name, Namespace: host.Namespace}

	var liveFirmwareSettingsAllowed, liveFirmwareUpdatesAllowed bool
	if hup != nil {
		liveFirmwareSettingsAllowed = hup.Spec.FirmwareSettings == metal3api.HostUpdatePolicyOnReboot
		liveFirmwareUpdatesAllowed = hup.Spec.FirmwareUpdates == metal3api.HostUpdatePolicyOnReboot
	}

	if liveFirmwareSettingsAllowed {
		// handling HFS-based FirmwareSettings here
		hfs := &metal3api.HostFirmwareSettings{}
		if err := reader.Get(ctx, hostKey, hfs); err != nil {
			if !k8serrors.IsNotFound(err) {
				return state, fmt.Errorf("could not load host firmware settings: %w", err)
			}
		} else {
			changed, valid, err := hostObjectHasChanges(hfs.Status.Conditions, hfs.GetGeneration())
			if err != nil {
				return state, fmt.Errorf("hostFirmwareSettings not ready yet: %w", err)
			}
			if valid && changed {
				if len(hfs.Status.Settings) == 0 {
					return state, errors.New("host firmware status settings not available")
				}
				state.hfsDirty = true
				state.data.ActualFirmwareSettings = hfs.Status.Settings
				state.data.TargetFirmwareSettings = hfs.Spec.Settings
			}
			state.data.HasFirmwareSpec = state.data.HasFirmwareSpec || len(hfs.Spec.Settings) > 0
		}
	}

	if liveFirmwareUpdatesAllowed {
		hfc := &metal3api.HostFirmwareComponents{}
		if err := reader.Get(ctx, hostKey, hfc); err != nil {
			if !k8serrors.IsNotFound(err) {
				return state, fmt.Errorf("could not load host firmware components: %w", err)
			}
		} else {
			changed, valid, err := hostObjectHasChanges(hfc.Status.Conditions, hfc.GetGeneration())
			if err != nil {
				return state, fmt.Errorf("hostFirmwareComponents not ready yet: %w", err)
			}
			if valid && changed {
				state.hfcDirty = true
				state.hfc = hfc
				// Handle only components in hfc.Spec.Updates but not yet in hfc.Status.Updates.
				if hfc.Status.Updates != nil {
					state.data.TargetFirmwareComponents = getUpdatesDifference(hfc.Spec.Updates, hfc.Status.Updates)
				} else {
					state.data.TargetFirmwareComponents = hfc.Spec.Updates
				}
			}
			state.data.HasFirmwareSpec = state.data.HasFirmwareSpec || len(hfc.Spec.Updates) > 0
		}
	}

	state.hasChanges = state.hfsDirty || state.hfcDirty
	return state, nil
}

// performService calls the provisioner's Service function.
// The caller is responsible for interpreting the result and updating host status.
func performService(ctx context.Context, prov provisioner.Provisioner, host *metal3api.BareMetalHost, state servicingState) (provisioner.Result, bool, error) {
	return prov.Service(ctx, state.data, state.hasChanges, host.Status.ErrorType == metal3api.ServicingError)
}

// applyHFCServicingError clears pending updates from HFC status after a Prepare or Service
// error. No-op if dirty is false or if there are no pending updates to clear.
// Writes to Kubernetes as needed.
func applyHFCServicingError(ctx context.Context, c client.Client, hfc *metal3api.HostFirmwareComponents, dirty bool) error {
	if !dirty || hfc.Status.Updates == nil {
		return nil
	}
	hfc.Status.Updates = nil
	return c.Status().Update(ctx, hfc)
}

// applyHFCServicingResult syncs HostFirmwareComponents status after a successful Prepare or
// Service provisioner call. No-op if dirty or started is false.
// Copies spec updates to status and refreshes component versions from the provisioner.
// Writes to Kubernetes as needed.
func applyHFCServicingResult(ctx context.Context, c client.Client, prov provisioner.Provisioner, hfc *metal3api.HostFirmwareComponents, dirty, started bool) error {
	if !dirty || !started {
		return nil
	}
	if reflect.DeepEqual(hfc.Status.Updates, hfc.Spec.Updates) {
		return nil
	}
	hfc.Status.Updates = hfc.Spec.Updates
	components, err := prov.GetFirmwareComponents(ctx)
	if err != nil {
		return fmt.Errorf("failed to get firmware components: %w", err)
	}
	hfc.Status.Components = components
	return c.Status().Update(ctx, hfc)
}
