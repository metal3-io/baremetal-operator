package bmc

import (
	"net/url"
	"strings"
)

func init() {
	schemes := []string{"http", "https"}
	RegisterFactory("redfish-noop", newRedfishNoopAccessDetails, schemes)
	RegisterFactory("ilo5-redfish-noop", newRedfishNoopAccessDetails, schemes)
}

func redfishNoopDetails(parsedURL *url.URL, disableCertificateVerification bool) *redfishAccessDetails {
	return &redfishAccessDetails{
		bmcType:                        parsedURL.Scheme,
		host:                           parsedURL.Host,
		path:                           parsedURL.Path,
		disableCertificateVerification: disableCertificateVerification,
	}
}

func newRedfishNoopAccessDetails(parsedURL *url.URL, disableCertificateVerification bool) (AccessDetails, error) {
	return redfishDetails(parsedURL, disableCertificateVerification), nil
}

type redfishNoopAccessDetails struct {
	bmcType                        string
	host                           string
	path                           string
	disableCertificateVerification bool
}

const redfishNoopDefaultScheme = "https"

func (a *redfishNoopAccessDetails) Type() string {
	return a.bmcType
}

// NeedsMAC returns true when the host is going to need a separate
// port created rather than having it discovered.
func (a *redfishNoopAccessDetails) NeedsMAC() bool {
	// For the inspection to work, we need a MAC address
	// https://github.com/metal3-io/baremetal-operator/pull/284#discussion_r317579040
	return true
}

func (a *redfishNoopAccessDetails) Driver() string {
	return "redfish"
}

func (a *redfishNoopAccessDetails) DisableCertificateVerification() bool {
	return a.disableCertificateVerification
}

func getRedfishNoopAddress(bmcType, host string) string {
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
func (a *redfishNoopAccessDetails) DriverInfo(bmcCreds Credentials) map[string]interface{} {
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
func (a *redfishNoopAccessDetails) BootInterface() string {
	return "ipxe"
}

func (a *redfishNoopAccessDetails) ManagementInterface() string {
	return "noop"
}

func (a *redfishNoopAccessDetails) PowerInterface() string {
	return ""
}

func (a *redfishNoopAccessDetails) RAIDInterface() string {
	return ""
}

func (a *redfishNoopAccessDetails) VendorInterface() string {
	return ""
}

func (a *redfishNoopAccessDetails) SupportsSecureBoot() bool {
	return true
}
