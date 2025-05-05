package ironic

import (
	"strings"

	"github.com/gophercloud/gophercloud/v2/openstack/baremetal/v1/nodes"
	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
)

var bootModeCapabilities = map[metal3api.BootMode]string{
	metal3api.UEFI:           "boot_mode:uefi",
	metal3api.UEFISecureBoot: "boot_mode:uefi,secure_boot:true",
	metal3api.Legacy:         "boot_mode:bios",
}

// We can't just replace the capabilities because we need to keep the
// values provided by inspection. We can't replace only the boot_mode
// because the API isn't fine-grained enough for that. So we have to
// look at the existing value and modify it. This function
// encapsulates the logic for building the value and knowing which
// update operation to use with the results.
func buildCapabilitiesValue(ironicNode *nodes.Node, bootMode metal3api.BootMode) string {
	if ironicNode == nil {
		// Creating a new node, no existing properties
		return bootModeCapabilities[bootMode]
	}

	capabilities, ok := ironicNode.Properties["capabilities"]
	if !ok {
		// There is no existing capabilities value
		return bootModeCapabilities[bootMode]
	}
	existingCapabilities, ok := capabilities.(string)
	if !ok {
		// The existing value is not a string, so we can replace
		// the whole thing.
		return bootModeCapabilities[bootMode]
	}

	if existingCapabilities == "" {
		// The existing value is empty so we can replace the whole
		// thing.
		return bootModeCapabilities[bootMode]
	}

	var filteredCapabilities []string
	for _, item := range strings.Split(existingCapabilities, ",") {
		if !strings.HasPrefix(item, "boot_mode:") && !strings.HasPrefix(item, "secure_boot:") {
			filteredCapabilities = append(filteredCapabilities, item)
		}
	}
	filteredCapabilities = append(filteredCapabilities, bootModeCapabilities[bootMode])

	return strings.Join(filteredCapabilities, ",")
}

// Secure boot is a normal capability that goes into instance_info (we
// also put it to properties for consistency, although it's not
// strictly required in our case).

// Instance info capabilities were invented later and
// use a normal JSON mapping instead of a custom
// string value.
func buildInstanceInfoCapabilities(bootMode metal3api.BootMode) map[string]string {
	capabilities := map[string]string{}
	if bootMode == metal3api.UEFISecureBoot {
		capabilities["secure_boot"] = "true"
	}
	return capabilities
}
