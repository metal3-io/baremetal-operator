package bmc

import (
	"fmt"
	"net/url"

	"github.com/gophercloud/gophercloud/openstack/baremetal/v1/nodes"
	metal3v1alpha1 "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
)

func init() {
	RegisterFactory("irmc", newIRMCAccessDetails, []string{})
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
		"ipmi_username": bmcCreds.Username,
		"ipmi_password": bmcCreds.Password,
		"ipmi_address":  a.hostname,
	}

	if a.disableCertificateVerification {
		result["irmc_verify_ca"] = false
	}

	if a.portNum != "" {
		result["irmc_port"] = a.portNum
	}

	return result
}

func (a *iRMCAccessDetails) BootInterface() string {
	return "pxe"
}

func (a *iRMCAccessDetails) ManagementInterface() string {
	return ""
}

func (a *iRMCAccessDetails) PowerInterface() string {
	return "ipmitool"
}

func (a *iRMCAccessDetails) RAIDInterface() string {
	return "irmc"
}

func (a *iRMCAccessDetails) VendorInterface() string {
	return ""
}

func (a *iRMCAccessDetails) SupportsSecureBoot() bool {
	return true
}

func (a *iRMCAccessDetails) BuildBIOSCleanSteps(firmwareConfig *metal3v1alpha1.FirmwareConfig) ([]nodes.CleanStep, error) {
	// If not configure irmc, only need to clear old configuration,
	// but irmc bios interface does not support factory_reset.
	if firmwareConfig == nil {
		return nil, nil
	}

	var cleanSteps []nodes.CleanStep
	// Build public bios settings
	settings, err := buildBIOSSettings(*firmwareConfig,
		[]string{
			"ResetSettings",
		},
		map[string]string{
			"SimultaneousMultithreadingEnabled": "hyper_threading_enabled",
			"VirtualizationEnabled":             "cpu_vt_enabled",
			"SriovEnabled":                      "single_root_io_virtualization_support_enabled",
		},
		map[string]string{
			"true":  "True",
			"false": "False",
		},
	)
	if err != nil {
		return nil, fmt.Errorf("build irmc public bios settings failed: %v", err)
	}

	if len(settings) != 0 {
		// This cleaning step applies a set of BIOS settings for a node
		cleanSteps = append(
			cleanSteps,
			nodes.CleanStep{
				Interface: "bios",
				Step:      "apply_configuration",
				Args: map[string]interface{}{
					"settings": settings,
				},
			},
		)
	}

	return cleanSteps, nil
}
