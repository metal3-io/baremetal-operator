package bmc

import (
	"net/url"

	"github.com/gophercloud/gophercloud/openstack/baremetal/v1/nodes"

	metal3v1alpha1 "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
)

func init() {
	schemes := []string{"http", "https"}
	registerFactory("idrac-virtualmedia", newRedfishiDracVirtualMediaAccessDetails, schemes)
}

func newRedfishiDracVirtualMediaAccessDetails(parsedURL *url.URL, disableCertificateVerification bool) (AccessDetails, error) {
	return &redfishiDracVirtualMediaAccessDetails{
		bmcType:                        parsedURL.Scheme,
		host:                           parsedURL.Host,
		path:                           parsedURL.Path,
		disableCertificateVerification: disableCertificateVerification,
	}, nil
}

type redfishiDracVirtualMediaAccessDetails struct {
	bmcType                        string
	host                           string
	path                           string
	disableCertificateVerification bool
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
	result := map[string]interface{}{
		"redfish_system_id": a.path,
		"redfish_username":  bmcCreds.Username,
		"redfish_password":  bmcCreds.Password,
		"redfish_address":   getRedfishAddress(a.bmcType, a.host),
	}

	if a.disableCertificateVerification {
		result["redfish_verify_ca"] = false
	}

	return result
}

// iDrac Virtual Media Overrides

func (a *redfishiDracVirtualMediaAccessDetails) Driver() string {
	return "idrac"
}

func (a *redfishiDracVirtualMediaAccessDetails) BootInterface() string {
	return "idrac-redfish-virtual-media"
}

func (a *redfishiDracVirtualMediaAccessDetails) ManagementInterface() string {
	return "idrac-redfish"
}

func (a *redfishiDracVirtualMediaAccessDetails) PowerInterface() string {
	return "idrac-redfish"
}

func (a *redfishiDracVirtualMediaAccessDetails) RAIDInterface() string {
	return "no-raid"
}

func (a *redfishiDracVirtualMediaAccessDetails) VendorInterface() string {
	return "no-vendor"
}

func (a *redfishiDracVirtualMediaAccessDetails) BIOSCleanSteps(firmware *metal3v1alpha1.FirmwareConfig) []nodes.CleanStep {
	return idracBIOSCleanSteps(firmware)
}
