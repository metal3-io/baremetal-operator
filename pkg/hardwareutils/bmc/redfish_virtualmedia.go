package bmc

import (
	"net/url"
)

func init() {
	schemes := []string{"http", "https"}
	RegisterFactory("redfish-virtualmedia", newRedfishVirtualMediaAccessDetails, schemes)
	RegisterFactory("ilo5-virtualmedia", newRedfishVirtualMediaAccessDetails, schemes)
}

func newRedfishVirtualMediaAccessDetails(parsedURL *url.URL, disableCertificateVerification bool) (AccessDetails, error) {
	return &redfishVirtualMediaAccessDetails{
		redfishAccessDetails{
			bmcType:                        parsedURL.Scheme,
			host:                           parsedURL.Host,
			path:                           parsedURL.Path,
			disableCertificateVerification: disableCertificateVerification,
		},
	}, nil
}

type redfishVirtualMediaAccessDetails struct {
	redfishAccessDetails
}

func (a *redfishVirtualMediaAccessDetails) Type() string {
	return a.bmcType
}

// NeedsMAC returns true when the host is going to need a separate
// port created rather than having it discovered.
func (a *redfishVirtualMediaAccessDetails) NeedsMAC() bool {
	// For the inspection to work, we need a MAC address
	// https://github.com/metal3-io/baremetal-operator/pull/284#discussion_r317579040
	return true
}

func (a *redfishVirtualMediaAccessDetails) Driver() string {
	return redfish
}

func (a *redfishVirtualMediaAccessDetails) DisableCertificateVerification() bool {
	return a.disableCertificateVerification
}

// DriverInfo returns a data structure to pass as the DriverInfo
// parameter when creating a node in Ironic. The structure is
// pre-populated with the access information, and the caller is
// expected to add any other information that might be needed (such as
// the kernel and ramdisk locations).
func (a *redfishVirtualMediaAccessDetails) DriverInfo(bmcCreds Credentials) map[string]interface{} {
	return a.redfishAccessDetails.DriverInfo(bmcCreds)
}

func (a *redfishVirtualMediaAccessDetails) BIOSInterface() string {
	return ""
}

func (a *redfishVirtualMediaAccessDetails) BootInterface() string {
	return "redfish-virtual-media"
}

func (a *redfishVirtualMediaAccessDetails) FirmwareInterface() string {
	return redfish
}

func (a *redfishVirtualMediaAccessDetails) ManagementInterface() string {
	return ""
}

func (a *redfishVirtualMediaAccessDetails) PowerInterface() string {
	return ""
}

func (a *redfishVirtualMediaAccessDetails) RAIDInterface() string {
	return redfish
}

func (a *redfishVirtualMediaAccessDetails) VendorInterface() string {
	return ""
}

func (a *redfishVirtualMediaAccessDetails) SupportsSecureBoot() bool {
	return true
}

func (a *redfishVirtualMediaAccessDetails) SupportsISOPreprovisioningImage() bool {
	return true
}

func (a *redfishVirtualMediaAccessDetails) RequiresProvisioningNetwork() bool {
	return false
}

func (a *redfishVirtualMediaAccessDetails) BuildBIOSSettings(firmwareConfig *FirmwareConfig) (settings []map[string]string, err error) {
	return a.redfishAccessDetails.BuildBIOSSettings(firmwareConfig)
}
