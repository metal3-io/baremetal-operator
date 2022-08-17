package devicehints

import (
	"fmt"

	metal3v1alpha1 "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
)

// MakeHintMap converts a RootDeviceHints instance into a string map
// suitable to pass to ironic.
func MakeHintMap(source *metal3v1alpha1.RootDeviceHints) map[string]string {
	hints := map[string]string{}

	if source == nil {
		return hints
	}

	if source.DeviceName != "" {
		hints["name"] = source.DeviceName
	}
	if source.HCTL != "" {
		hints["hctl"] = source.HCTL
	}
	if source.Model != "" {
		hints["model"] = fmt.Sprintf("<in> %s", source.Model)
	}
	if source.Vendor != "" {
		hints["vendor"] = fmt.Sprintf("<in> %s", source.Vendor)
	}
	if source.SerialNumber != "" {
		hints["serial"] = source.SerialNumber
	}
	if source.MinSizeGigabytes != 0 {
		hints["size"] = fmt.Sprintf(">= %d", source.MinSizeGigabytes)
	}
	if source.WWN != "" {
		hints["wwn"] = source.WWN
	}
	if source.WWNWithExtension != "" {
		hints["wwn_with_extension"] = source.WWNWithExtension
	}
	if source.WWNVendorExtension != "" {
		hints["wwn_vendor_extension"] = source.WWNVendorExtension
	}
	switch {
	case source.Rotational == nil:
	case *source.Rotational == true:
		hints["rotational"] = "true"
	case *source.Rotational == false:
		hints["rotational"] = "false"
	}

	return hints
}
