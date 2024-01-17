package testbmc

import (
	"net/url"

	"github.com/metal3-io/baremetal-operator/pkg/hardwareutils/bmc"
)

func init() {
	bmc.RegisterFactory("test", NewTestBMCAccessDetails, []string{})
	bmc.RegisterFactory("test-needs-mac", NewTestBMCAccessDetails, []string{})
}

func NewTestBMCAccessDetails(parsedURL *url.URL, disableCertificateVerification bool) (bmc.AccessDetails, error) {
	return &testAccessDetails{
		bmcType:                        parsedURL.Scheme,
		hostname:                       parsedURL.Hostname(),
		disableCertificateVerification: disableCertificateVerification,
	}, nil
}

type testAccessDetails struct {
	bmcType                        string
	hostname                       string
	disableCertificateVerification bool
}

func (a *testAccessDetails) Type() string {
	return a.bmcType
}

// NeedsMAC returns true when the host is going to need a separate
// port created rather than having it discovered.
func (a *testAccessDetails) NeedsMAC() bool {
	return a.bmcType == "test-needs-mac"
}

func (a *testAccessDetails) Driver() string {
	return "test"
}

func (a *testAccessDetails) DisableCertificateVerification() bool {
	return a.disableCertificateVerification
}

// DriverInfo returns a data structure to pass as the DriverInfo
// parameter when creating a node in Ironic. The structure is
// pre-populated with the access information, and the caller is
// expected to add any other information that might be needed (such as
// the kernel and ramdisk locations).
func (a *testAccessDetails) DriverInfo(bmcCreds bmc.Credentials) map[string]interface{} {
	result := map[string]interface{}{
		"test_port":     "42",
		"test_username": bmcCreds.Username,
		"test_password": bmcCreds.Password,
		"test_address":  a.hostname,
	}

	if a.disableCertificateVerification {
		result["test_verify_ca"] = false
	}
	return result
}

func (a *testAccessDetails) BIOSInterface() string {
	return ""
}

func (a *testAccessDetails) BootInterface() string {
	return "ipxe"
}

func (a *testAccessDetails) FirmwareInterface() string {
	return ""
}

func (a *testAccessDetails) ManagementInterface() string {
	return ""
}

func (a *testAccessDetails) PowerInterface() string {
	return ""
}

func (a *testAccessDetails) RAIDInterface() string {
	return "no-raid"
}

func (a *testAccessDetails) VendorInterface() string {
	return ""
}

func (a *testAccessDetails) SupportsSecureBoot() bool {
	return false
}

func (a *testAccessDetails) SupportsISOPreprovisioningImage() bool {
	return false
}

func (a *testAccessDetails) RequiresProvisioningNetwork() bool {
	return true
}

func (a *testAccessDetails) BuildBIOSSettings(firmwareConfig *bmc.FirmwareConfig) (settings []map[string]string, err error) {
	// Return sample BMC data for test purposes
	const enabled = "Enabled"
	const disabled = "Disabled"

	if firmwareConfig == nil {
		return nil, nil
	}

	var value string

	if firmwareConfig.VirtualizationEnabled != nil {
		value = disabled
		if *firmwareConfig.VirtualizationEnabled {
			value = enabled
		}
		settings = append(settings,
			map[string]string{
				"name":  "ProcVirtualization",
				"value": value,
			},
		)
	}

	if firmwareConfig.SimultaneousMultithreadingEnabled != nil {
		value = disabled
		if *firmwareConfig.SimultaneousMultithreadingEnabled {
			value = enabled
		}
		settings = append(settings,
			map[string]string{
				"name":  "ProcHyperthreading",
				"value": value,
			},
		)
	}

	if firmwareConfig.SriovEnabled != nil {
		value = disabled
		if *firmwareConfig.SriovEnabled {
			value = enabled
		}
		settings = append(settings,
			map[string]string{
				"name":  "Sriov",
				"value": value,
			},
		)
	}

	return
}
