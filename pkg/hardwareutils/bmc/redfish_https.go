package bmc

import (
	"fmt"
	"net/url"
)

func init() {
	schemes := []string{"http", "https"}
	RegisterFactory("redfish-uefihttp", newRedfishHTTPBootMediaAccessDetails, schemes)
}

func newRedfishHTTPBootMediaAccessDetails(parsedURL *url.URL, disableCertificateVerification bool) (AccessDetails, error) {
	return &redfishHTTPBootMediaAccessDetails{
		bmcType:                        parsedURL.Scheme,
		host:                           parsedURL.Host,
		path:                           parsedURL.Path,
		disableCertificateVerification: disableCertificateVerification,
	}, nil
}

type redfishHTTPBootMediaAccessDetails struct {
	bmcType                        string
	host                           string
	path                           string
	disableCertificateVerification bool
}

func (a *redfishHTTPBootMediaAccessDetails) Type() string {
	return a.bmcType
}

// NeedsMAC returns false because the Redfish driver in Ironic can
// pre-populate MAC addresses during inspection, so a BootMACAddress is not
// required up front. When inspection is disabled a MAC is still required,
// but that requirement is enforced by the callers via
// host.InspectionDisabled(), not by this driver-level flag.
func (a *redfishHTTPBootMediaAccessDetails) NeedsMAC() bool {
	return false
}

func (a *redfishHTTPBootMediaAccessDetails) Driver() string {
	return redfish
}

func (a *redfishHTTPBootMediaAccessDetails) DisableCertificateVerification() bool {
	return a.disableCertificateVerification
}

// DriverInfo returns a data structure to pass as the DriverInfo
// parameter when creating a node in Ironic. The structure is
// pre-populated with the access information, and the caller is
// expected to add any other information that might be needed (such as
// the kernel and ramdisk locations).
func (a *redfishHTTPBootMediaAccessDetails) DriverInfo(bmcCreds Credentials) map[string]interface{} {
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

func (a *redfishHTTPBootMediaAccessDetails) BIOSInterface() string {
	return ""
}

func (a *redfishHTTPBootMediaAccessDetails) BootInterface() string {
	return "redfish-https"
}

func (a *redfishHTTPBootMediaAccessDetails) FirmwareInterface() string {
	return redfish
}

func (a *redfishHTTPBootMediaAccessDetails) ManagementInterface() string {
	return ""
}

func (a *redfishHTTPBootMediaAccessDetails) PowerInterface() string {
	return ""
}

func (a *redfishHTTPBootMediaAccessDetails) RAIDInterface() string {
	return redfish
}

func (a *redfishHTTPBootMediaAccessDetails) VendorInterface() string {
	return ""
}

func (a *redfishHTTPBootMediaAccessDetails) SupportsSecureBoot() bool {
	return true
}

func (a *redfishHTTPBootMediaAccessDetails) SupportsISOPreprovisioningImage() bool {
	return true
}

func (a *redfishHTTPBootMediaAccessDetails) RequiresProvisioningNetwork() bool {
	return false
}

func (a *redfishHTTPBootMediaAccessDetails) BuildBIOSSettings(firmwareConfig *FirmwareConfig) (settings []map[string]string, err error) {
	if firmwareConfig != nil {
		return nil, fmt.Errorf("firmware settings for %s are not supported", a.Driver())
	}
	return nil, nil
}
