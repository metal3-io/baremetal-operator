// Copyright (c) 2016-2018 Hewlett Packard Enterprise Development LP

package bmc

import (
	"net/url"

	"github.com/gophercloud/gophercloud/openstack/baremetal/v1/nodes"

	metal3v1alpha1 "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
)

var (
	iloSettingNeedCustomWorkload = []string{
		"MinProcIdlePkgState", "MinProcIdlePower", "PowerRegulator",
		"ChannelInterleaving", "EnergyPerfBias", "IntelUpiFreq",
		"IntelUpiPowerManagement", "MaxMemBusFreqMHz", "MaxPcieSpeed"}
)

func init() {
	registerFactory("ilo5", newILO5AccessDetails, []string{"https"})
}

func newILO5AccessDetails(parsedURL *url.URL, disableCertificateVerification bool) (AccessDetails, error) {
	return &iLO5AccessDetails{
		bmcType:                        parsedURL.Scheme,
		portNum:                        parsedURL.Port(),
		hostname:                       parsedURL.Hostname(),
		disableCertificateVerification: disableCertificateVerification,
	}, nil
}

type iLO5AccessDetails struct {
	bmcType                        string
	portNum                        string
	hostname                       string
	disableCertificateVerification bool
}

func (a *iLO5AccessDetails) Type() string {
	return a.bmcType
}

// NeedsMAC returns true when the host is going to need a separate
// port created rather than having it discovered.
func (a *iLO5AccessDetails) NeedsMAC() bool {
	// For the inspection to work, we need a MAC address
	// https://github.com/metal3-io/baremetal-operator/pull/284#discussion_r317579040
	return true
}

func (a *iLO5AccessDetails) Driver() string {
	return "ilo5"
}

func (a *iLO5AccessDetails) DisableCertificateVerification() bool {
	return a.disableCertificateVerification
}

// DriverInfo returns a data structure to pass as the DriverInfo
// parameter when creating a node in Ironic. The structure is
// pre-populated with the access information, and the caller is
// expected to add any other information that might be needed (such as
// the kernel and ramdisk locations).
func (a *iLO5AccessDetails) DriverInfo(bmcCreds Credentials) map[string]interface{} {

	result := map[string]interface{}{
		"ilo_username": bmcCreds.Username,
		"ilo_password": bmcCreds.Password,
		"ilo_address":  a.hostname,
	}

	if a.disableCertificateVerification {
		result["ilo_verify_ca"] = false
	}

	if a.portNum != "" {
		result["client_port"] = a.portNum
	}

	return result
}

func (a *iLO5AccessDetails) BootInterface() string {
	return "ilo-ipxe"
}

func (a *iLO5AccessDetails) ManagementInterface() string {
	return ""
}

func (a *iLO5AccessDetails) PowerInterface() string {
	return ""
}

func (a *iLO5AccessDetails) RAIDInterface() string {
	return "ilo5"
}

func (a *iLO5AccessDetails) VendorInterface() string {
	return ""
}

// Add the custom workloadprofile if one of the setting requires it
// see https://github.com/denysvitali/ilo-rest-api-docs/blob/master/source/includes/_ilo5_resourcedefns.md
func iloGetWorkloadProfile(settings []map[string]string) map[string]string {
	for _, value := range settings {
		if contains(value["name"], iloSettingNeedCustomWorkload) {
			return map[string]string{
				"name":  "WorkloadProfile",
				"value": "custom",
			}
		}
	}
	return nil
}

// A private method to build the clean steps for ILO configuration from BaremetalHost spec
func iloBIOSCleanSteps(firmware *metal3v1alpha1.FirmwareConfig) []nodes.CleanStep {
	// This cleaning step resets all BIOS settings to factory default for a given node
	cleanSteps := []nodes.CleanStep{
		nodes.CleanStep{
			Interface: "bios",
			Step:      "factory_reset",
		},
	}

	// If not configure ILO, only need to clear old configuration
	if firmware == nil {
		return cleanSteps
	}
	settings := buildBIOSSettings(*firmware,
		map[string]string{
			"SimultaneousMultithreadingEnabled": "ProcHyperthreading",
			"VirtualizationEnabled":             "ProcVirtualization",
			"SriovEnabled":                      "Sriov",
			"BootOrderPolicy":                   "BootOrderPolicy",
			"LLCPrefetchEnabled":                "LlcPrefetch",
		},
		trueToEnabled,
	)
	if firmware.NUMAEnabled != "" {
		settings = append(settings, []map[string]string{
			{
				"name":  "SubNumaClustering",
				"value": trueToEnabled[firmware.NUMAEnabled],
			},
			{
				"name":  "EnergyEfficientTurbo",
				"value": trueToEnabled[firmware.NUMAEnabled],
			},
		}...)
	}
	if firmware.CStateEnabled != "" {
		trueToCState := map[string]string{
			"true":  "C6", // C1E is also valid
			"false": "NoCStates",
		}
		settings = append(settings, map[string]string{
			"name":  "MinProcIdlePower",
			"value": trueToCState[firmware.CStateEnabled],
		})
	}

	// some settings require the workload profile to be custom
	settings = append(settings, iloGetWorkloadProfile(settings))

	return append(cleanSteps, nodes.CleanStep{
		Interface: "bios",
		Step:      "apply_configuration",
		Args: map[string]interface{}{
			"settings": settings,
		},
	})
}

// Build the clean steps for ILO configuration from BaremetalHost spec
func (a *iLO5AccessDetails) BIOSCleanSteps(firmware *metal3v1alpha1.FirmwareConfig) []nodes.CleanStep {
	return iloBIOSCleanSteps(firmware)
}
