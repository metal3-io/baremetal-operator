package bmc

import (
	"net/url"

	metal3v1alpha1 "github.com/metal3-io/baremetal-operator/pkg/apis/metal3/v1alpha1"
)

func init() {
	registerFactory("irmc", newIRMCAccessDetails, []string{})
}

func newIRMCAccessDetails(parsedURL *url.URL, disableCertificateVerification bool) (AccessDetails, error) {
	return &iRMCAccessDetails{
		bmcType:                        parsedURL.Scheme,
		portNum:                        parsedURL.Port(),
		hostname:                       parsedURL.Hostname(),
		disableCertificateVerification: disableCertificateVerification,
	}, nil
}

type iRMCAccessDetails struct {
	bmcType                        string
	portNum                        string
	hostname                       string
	disableCertificateVerification bool
}

func (a *iRMCAccessDetails) Type() string {
	return a.bmcType
}

// NeedsMAC returns true when the host is going to need a separate
// port created rather than having it discovered.
func (a *iRMCAccessDetails) NeedsMAC() bool {
	return false
}

func (a *iRMCAccessDetails) Driver() string {
	return "irmc"
}

func (a *iRMCAccessDetails) DisableCertificateVerification() bool {
	return a.disableCertificateVerification
}

// DriverInfo returns a data structure to pass as the DriverInfo
// parameter when creating a node in Ironic. The structure is
// pre-populated with the access information, and the caller is
// expected to add any other information that might be needed (such as
// the kernel and ramdisk locations).
func (a *iRMCAccessDetails) DriverInfo(bmcCreds Credentials) map[string]interface{} {
	result := map[string]interface{}{
		"irmc_username": bmcCreds.Username,
		"irmc_password": bmcCreds.Password,
		"irmc_address":  a.hostname,
	}

	if a.disableCertificateVerification {
		result["irmc_verify_ca"] = false
	}

	if a.portNum != "" {
		result["irmc_port"] = a.portNum
	}

	return result
}

// DefaultBootMode returns the default boot mode for the host
func (a *iRMCAccessDetails) DefaultBootMode() metal3v1alpha1.BootMode {
	return metal3v1alpha1.UEFI
}

func (a *iRMCAccessDetails) BootInterface() string {
	return "pxe"
}

func (a *iRMCAccessDetails) ManagementInterface() string {
	return ""
}

func (a *iRMCAccessDetails) PowerInterface() string {
	return ""
}

func (a *iRMCAccessDetails) RAIDInterface() string {
	return "irmc"
}

func (a *iRMCAccessDetails) VendorInterface() string {
	return ""
}
