package clients

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack/utils"
)

// AvailableFeatures represents features that Ironic API provides.
// See https://docs.openstack.org/ironic/latest/contributor/webapi-version-history.html
type AvailableFeatures struct {
	MaxVersion int
}

// The minimum microversion that BMO can work with. We need to be really
// conservative when updating this value since doing so unconditionally breaks
// operators of older Ironic, even if the feature we need is optional.
// Update docs/configuration.md when updating the version.
// Version 1.89 allows attaching and detaching virtual media.
const baseline = 89

var baselineVersionString = fmt.Sprintf("1.%d", baseline)

func GetAvailableFeatures(ctx context.Context, client *gophercloud.ServiceClient) (features AvailableFeatures, err error) {
	mvs, err := utils.GetSupportedMicroversions(ctx, client)
	if err != nil {
		return
	}

	if mvs.MaxMajor != 1 || mvs.MaxMinor < baseline {
		err = fmt.Errorf("ironic API 1.%d or newer is required, got %d.%d", baseline, mvs.MaxMajor, mvs.MaxMinor)
		return
	}

	features.MaxVersion = mvs.MaxMinor
	return
}

func (af AvailableFeatures) Log(logger logr.Logger) {
	// NOTE(dtantsur): update this every time we have more features of interest
	logger.Info("supported Ironic API features",
		"maxVersion", fmt.Sprintf("1.%d", af.MaxVersion),
		"chosenVersion", af.ChooseMicroversion(),
		"virtualMediaGET", af.HasVirtualMediaGetAPI(),
		"disablePowerOff", af.HasDisablePowerOff())
}

func (af AvailableFeatures) HasVirtualMediaGetAPI() bool {
	return af.MaxVersion >= 93 //nolint:mnd
}

func (af AvailableFeatures) HasDisablePowerOff() bool {
	return af.MaxVersion >= 95 //nolint:mnd
}

func (af AvailableFeatures) ChooseMicroversion() string {
	if af.HasDisablePowerOff() {
		return "1.95"
	}

	if af.HasVirtualMediaGetAPI() {
		return "1.93"
	}

	return baselineVersionString
}
