package ironic

import (
	"github.com/metal3-io/baremetal-operator/pkg/provisioner"
)

// validateProvisionData provides early validation for provisioning data.
// The goal is to report errors as early as possible, before the node goes into
// cleaning and then deployment.
func validateProvisionData(data provisioner.ProvisionData) error {
	if data.Image.URL != "" {
		if _, _, err := data.Image.GetChecksum(); err != nil {
			return err
		}
	}

	return nil
}
