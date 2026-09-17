// SPDX-License-Identifier: Apache-2.0

// The Provisioner methods that do nothing. A Go interface cannot lose a method,
// so everything out of scope lives here rather than among real work.

package anaconda

import (
	"context"
	"errors"

	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner"
)

// Unsupported builds the error result for an out of scope request. An error
// rather than a silent success, so no host sits Provisioned on nothing.
func Unsupported(method, detail string) provisioner.Result {
	return provisioner.Result{
		ErrorMessage: method + ": " + detail + " is not supported by the anaconda provisioner",
	}
}

// Prepare reports nothing to prepare. RAID is refused rather than ignored, so
// no host boots believing a volume layout it asked for exists.
func (*Provisioner) Prepare(
	_ context.Context,
	data provisioner.PrepareData,
	_, _ bool,
) (provisioner.Result, bool, error) {
	for _, cfg := range []*metal3api.RAIDConfig{data.TargetRAIDConfig, data.ActualRAIDConfig} {
		if cfg == nil {
			continue
		}

		if len(cfg.HardwareRAIDVolumes) > 0 || len(cfg.SoftwareRAIDVolumes) > 0 {
			return Unsupported("prepare", "RAID configuration"), false, nil
		}
	}

	return provisioner.Result{}, false, nil
}

// HasCapacity reports capacity, out of band work contends for nothing.
func (*Provisioner) HasCapacity(_ context.Context) (bool, error) { return true, nil }

// PreprovisioningImageFormats reports none, nothing here boots a ramdisk.
func (*Provisioner) PreprovisioningImageFormats(_ context.Context) ([]metal3api.ImageFormat, error) {
	return nil, nil
}

// Adopt succeeds unconditionally, no external provisioning system holds a node
// record for this host that could disagree with the BareMetalHost.
func (*Provisioner) Adopt(_ context.Context, _ provisioner.AdoptData, _ bool) (provisioner.Result, error) {
	return provisioner.Result{}, nil
}

// Service refuses. Succeeding would report firmware or RAID changes applied
// when nothing touched the machine, which is what this whole file exists to stop.
func (*Provisioner) Service(
	_ context.Context,
	_ provisioner.ServicingData,
	_, _ bool,
) (provisioner.Result, bool, error) {
	return Unsupported("service", "servicing a provisioned host"), false, nil
}

func (*Provisioner) Delete(_ context.Context) (provisioner.Result, error) {
	return provisioner.Result{}, nil
}

func (*Provisioner) Detach(_ context.Context, _ bool) (provisioner.Result, error) {
	return provisioner.Result{}, nil
}

func (*Provisioner) GetFirmwareSettings(
	_ context.Context,
	_ bool,
) (metal3api.SettingsMap, map[string]metal3api.SettingSchema, error) {
	return nil, nil, nil
}

// GetFirmwareComponents reports none. Nothing here can flash firmware, and BMO
// runs this every reconcile, so answering costs a BMC round trip for nothing.
func (*Provisioner) GetFirmwareComponents(_ context.Context) ([]metal3api.FirmwareComponentStatus, error) {
	return nil, nil
}

func (*Provisioner) AddBMCEventSubscriptionForNode(
	_ context.Context,
	_ *metal3api.BMCEventSubscription,
	_ provisioner.HTTPHeaders,
) (provisioner.Result, error) {
	result := Unsupported("add_bmc_event_subscription", "BMC event subscriptions")

	// The subscription controller discards the Result, so a failure has to be an error.
	return result, errors.New(result.ErrorMessage)
}

// RemoveBMCEventSubscriptionForNode succeeds where Add fails, an error here
// would keep the subscription finalizer and block the CR's deletion forever.
func (*Provisioner) RemoveBMCEventSubscriptionForNode(
	_ context.Context,
	_ metal3api.BMCEventSubscription,
) (provisioner.Result, error) {
	return provisioner.Result{}, nil
}

// HasPowerFailure reports no power fault, the Redfish power state models none.
func (*Provisioner) HasPowerFailure(_ context.Context) bool { return false }
