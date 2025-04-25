package bmc

import (
	"net/url"
)

const (
	idrac        = "idrac"
	idracRedfish = "idrac-redfish"
	redfish      = "redfish"
)

func init() {
	schemes := []string{"http", "https"}
	RegisterFactory("idrac-virtualmedia", newRedfishiDracVirtualMediaAccessDetails, schemes)
}

func newRedfishiDracVirtualMediaAccessDetails(parsedURL *url.URL, disableCertificateVerification bool) (AccessDetails, error) {
	return &redfishiDracVirtualMediaAccessDetails{
		redfishAccessDetails{
			bmcType:                        parsedURL.Scheme,
			host:                           parsedURL.Host,
			path:                           parsedURL.Path,
			disableCertificateVerification: disableCertificateVerification,
		},
	}, nil
}

type redfishiDracVirtualMediaAccessDetails struct {
	redfishAccessDetails
}

func (a *redfishiDracVirtualMediaAccessDetails) Type() string {
	return a.bmcType
}

// NeedsMAC returns true when the host is going to need a separate
// port created rather than having it discovered.
func (a *redfishiDracVirtualMediaAccessDetails) NeedsMAC() bool {
	// For the inspection to work, we need a MAC address
	// https://github.com/metal3-io/baremetal-operator/pull/284#discussion_r317579040
	return true
}

func (a *redfishiDracVirtualMediaAccessDetails) DisableCertificateVerification() bool {
	return a.disableCertificateVerification
}

// DriverInfo returns a data structure to pass as the DriverInfo
// parameter when creating a node in Ironic. The structure is
// pre-populated with the access information, and the caller is
// expected to add any other information that might be needed (such as
// the kernel and ramdisk locations).
func (a *redfishiDracVirtualMediaAccessDetails) DriverInfo(bmcCreds Credentials) map[string]interface{} {
	return a.redfishAccessDetails.DriverInfo(bmcCreds)
}

// iDrac Virtual Media Overrides

func (a *redfishiDracVirtualMediaAccessDetails) Driver() string {
	return idrac
}

func (a *redfishiDracVirtualMediaAccessDetails) BIOSInterface() string {
	return idracRedfish
}

func (a *redfishiDracVirtualMediaAccessDetails) BootInterface() string {
	return "idrac-redfish-virtual-media"
}

func (a *redfishiDracVirtualMediaAccessDetails) FirmwareInterface() string {
	return redfish
}

func (a *redfishiDracVirtualMediaAccessDetails) ManagementInterface() string {
	return idracRedfish
}

func (a *redfishiDracVirtualMediaAccessDetails) PowerInterface() string {
	return idracRedfish
}

func (a *redfishiDracVirtualMediaAccessDetails) RAIDInterface() string {
	return idracRedfish
}

func (a *redfishiDracVirtualMediaAccessDetails) VendorInterface() string {
	// NOTE(dtantsur): the idrac hardware type defaults to WSMAN vendor, we need to use the Redfish implementation.
	return idracRedfish
}

func (a *redfishiDracVirtualMediaAccessDetails) SupportsSecureBoot() bool {
	return true
}

func (a *redfishiDracVirtualMediaAccessDetails) SupportsISOPreprovisioningImage() bool {
	return true
}

func (a *redfishiDracVirtualMediaAccessDetails) RequiresProvisioningNetwork() bool {
	return false
}

func (a *redfishiDracVirtualMediaAccessDetails) BuildBIOSSettings(firmwareConfig *FirmwareConfig) (settings []map[string]string, err error) {
	return a.redfishAccessDetails.BuildBIOSSettings(firmwareConfig)
}
