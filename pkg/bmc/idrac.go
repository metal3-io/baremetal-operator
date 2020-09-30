package bmc

import (
	"net/url"
	"strings"

	"github.com/gophercloud/gophercloud/openstack/baremetal/v1/nodes"

	metal3v1alpha1 "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
)

func init() {
	registerFactory("idrac", newIDRACAccessDetails, []string{"http", "https"})
}

func newIDRACAccessDetails(parsedURL *url.URL, disableCertificateVerification bool) (AccessDetails, error) {
	return &iDracAccessDetails{
		bmcType:                        parsedURL.Scheme,
		portNum:                        parsedURL.Port(),
		hostname:                       parsedURL.Hostname(),
		path:                           parsedURL.Path,
		disableCertificateVerification: disableCertificateVerification,
	}, nil
}

type iDracAccessDetails struct {
	bmcType                        string
	portNum                        string
	hostname                       string
	path                           string
	disableCertificateVerification bool
}

func (a *iDracAccessDetails) Type() string {
	return a.bmcType
}

// NeedsMAC returns true when the host is going to need a separate
// port created rather than having it discovered.
func (a *iDracAccessDetails) NeedsMAC() bool {
	return false
}

func (a *iDracAccessDetails) Driver() string {
	return "idrac"
}

func (a *iDracAccessDetails) DisableCertificateVerification() bool {
	return a.disableCertificateVerification
}

// DriverInfo returns a data structure to pass as the DriverInfo
// parameter when creating a node in Ironic. The structure is
// pre-populated with the access information, and the caller is
// expected to add any other information that might be needed (such as
// the kernel and ramdisk locations).
func (a *iDracAccessDetails) DriverInfo(bmcCreds Credentials) map[string]interface{} {
	result := map[string]interface{}{
		"drac_username": bmcCreds.Username,
		"drac_password": bmcCreds.Password,
		"drac_address":  a.hostname,
	}
	if a.disableCertificateVerification {
		result["drac_verify_ca"] = false
	}

	schemes := strings.Split(a.bmcType, "+")
	if len(schemes) > 1 {
		result["drac_protocol"] = schemes[1]
	}
	if a.portNum != "" {
		result["drac_port"] = a.portNum
	}
	if a.path != "" {
		result["drac_path"] = a.path
	}

	return result
}

func (a *iDracAccessDetails) BootInterface() string {
	return "ipxe"
}

func (a *iDracAccessDetails) ManagementInterface() string {
	return ""
}

func (a *iDracAccessDetails) PowerInterface() string {
	return ""
}

func (a *iDracAccessDetails) RAIDInterface() string {
	return ""
}

func (a *iDracAccessDetails) VendorInterface() string {
	return ""
}

func idracBIOSCleanSteps(firmware *metal3v1alpha1.FirmwareConfig) []nodes.CleanStep {

	// This cleaning step resets all BIOS settings to factory default for a given node
	cleanSteps := append([]nodes.CleanStep{}, nodes.CleanStep{
		Interface: "bios",
		Step:      "factory_reset",
	})
	if firmware == nil {
		return cleanSteps
	}

	settings := buildBIOSSettings(*firmware,
		map[string]string{
			"SimultaneousMultithreadingEnabled": "LogicalProc",
			"VirtualizationEnabled":             "ProcVirtualization",
			"SriovEnabled":                      "SriovGlobalEnable",
			"LLCPrefetchEnabled":                "ProcHwPrefetcher",
			"CStateEnabled":                     "ProcCStates",
		},
		trueToEnabled,
	)

	if firmware.BootOrderPolicy != "" {
		bootOrderPolicyToEnabled := map[string]string{
			"RetryIndefinitely": "Enabled",
			"AttemptOnce":       "Disabled",
			"ResetAfterFailed":  "Disabled",
		}
		settings = append(settings, map[string]string{
			"name":  "BootSeqRetry",
			"value": bootOrderPolicyToEnabled[firmware.BootOrderPolicy],
		})
	}

	if firmware.NUMAEnabled != "" {
		settings = append(settings, []map[string]string{
			{
				"name":  "SubNumaCluster",
				"value": trueToEnabled[firmware.NUMAEnabled],
			},
			{
				"name":  "ProcTurboMode",
				"value": trueToEnabled[firmware.NUMAEnabled],
			},
		}...)
	}

	return append(cleanSteps, nodes.CleanStep{
		Interface: "bios",
		Step:      "apply_configuration",
		Args: map[string]interface{}{
			"settings": settings,
		},
	})
}

// Build the clean steps for IDRAC configuration from BaremetalHost spec
func (a *iDracAccessDetails) BIOSCleanSteps(firmware *metal3v1alpha1.FirmwareConfig) []nodes.CleanStep {
	return idracBIOSCleanSteps(firmware)
}
