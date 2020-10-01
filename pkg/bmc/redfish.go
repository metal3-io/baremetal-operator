package bmc

import (
	"net/url"
	"strings"

	"github.com/gophercloud/gophercloud/openstack/baremetal/v1/nodes"

	metal3v1alpha1 "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
)

func init() {
	schemes := []string{"http", "https"}
	registerFactory("redfish", newRedfishAccessDetails, schemes)
	registerFactory("ilo5-redfish", newRedfishAccessDetails, schemes)
}

func redfishDetails(parsedURL *url.URL, disableCertificateVerification bool) *redfishAccessDetails {
	return &redfishAccessDetails{
		bmcType:                        parsedURL.Scheme,
		host:                           parsedURL.Host,
		path:                           parsedURL.Path,
		disableCertificateVerification: disableCertificateVerification,
	}
}

func newRedfishAccessDetails(parsedURL *url.URL, disableCertificateVerification bool) (AccessDetails, error) {
	return redfishDetails(parsedURL, disableCertificateVerification), nil
}

type redfishAccessDetails struct {
	bmcType                        string
	host                           string
	path                           string
	disableCertificateVerification bool
}

const redfishDefaultScheme = "https"

func (a *redfishAccessDetails) Type() string {
	return a.bmcType
}

// NeedsMAC returns true when the host is going to need a separate
// port created rather than having it discovered.
func (a *redfishAccessDetails) NeedsMAC() bool {
	// For the inspection to work, we need a MAC address
	// https://github.com/metal3-io/baremetal-operator/pull/284#discussion_r317579040
	return true
}

func (a *redfishAccessDetails) Driver() string {
	return "redfish"
}

func (a *redfishAccessDetails) DisableCertificateVerification() bool {
	return a.disableCertificateVerification
}

func getRedfishAddress(bmcType, host string) string {
	redfishAddress := []string{}
	schemes := strings.Split(bmcType, "+")
	if len(schemes) > 1 {
		redfishAddress = append(redfishAddress, schemes[1])
	} else {
		redfishAddress = append(redfishAddress, redfishDefaultScheme)
	}
	redfishAddress = append(redfishAddress, "://")
	redfishAddress = append(redfishAddress, host)
	return strings.Join(redfishAddress, "")
}

// DriverInfo returns a data structure to pass as the DriverInfo
// parameter when creating a node in Ironic. The structure is
// pre-populated with the access information, and the caller is
// expected to add any other information that might be needed (such as
// the kernel and ramdisk locations).
func (a *redfishAccessDetails) DriverInfo(bmcCreds Credentials) map[string]interface{} {
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

// That can be either pxe or redfish-virtual-media
func (a *redfishAccessDetails) BootInterface() string {
	return "ipxe"
}

func (a *redfishAccessDetails) ManagementInterface() string {
	return ""
}

func (a *redfishAccessDetails) PowerInterface() string {
	return ""
}

func (a *redfishAccessDetails) RAIDInterface() string {
	return ""
}

func (a *redfishAccessDetails) VendorInterface() string {
	return ""
}

func (a *redfishAccessDetails) BIOSCleanSteps(firmware *metal3v1alpha1.FirmwareConfig) (cleanSteps []nodes.CleanStep) {
	return cleanSteps
}
