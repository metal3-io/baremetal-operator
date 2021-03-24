// Copyright (c) 2016-2018 Hewlett Packard Enterprise Development LP

package bmc

import (
	"fmt"
	"net/url"

	"github.com/gophercloud/gophercloud/openstack/baremetal/v1/nodes"
	metal3v1alpha1 "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
)

func init() {
	RegisterFactory("ilo4", newILOAccessDetails, []string{"https"})
}

func newILOAccessDetails(parsedURL *url.URL, disableCertificateVerification bool) (AccessDetails, error) {
	return &iLOAccessDetails{
		bmcType:                        parsedURL.Scheme,
		portNum:                        parsedURL.Port(),
		hostname:                       parsedURL.Hostname(),
		disableCertificateVerification: disableCertificateVerification,
	}, nil
}

type iLOAccessDetails struct {
	bmcType                        string
	portNum                        string
	hostname                       string
	disableCertificateVerification bool
}

func (a *iLOAccessDetails) Type() string {
	return a.bmcType
}

// NeedsMAC returns true when the host is going to need a separate
// port created rather than having it discovered.
func (a *iLOAccessDetails) NeedsMAC() bool {
	// For the inspection to work, we need a MAC address
	// https://github.com/metal3-io/baremetal-operator/pull/284#discussion_r317579040
	return true
}

func (a *iLOAccessDetails) Driver() string {
	return "ilo"
}

func (a *iLOAccessDetails) DisableCertificateVerification() bool {
	return a.disableCertificateVerification
}

// DriverInfo returns a data structure to pass as the DriverInfo
// parameter when creating a node in Ironic. The structure is
// pre-populated with the access information, and the caller is
// expected to add any other information that might be needed (such as
// the kernel and ramdisk locations).
func (a *iLOAccessDetails) DriverInfo(bmcCreds Credentials) map[string]interface{} {

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

func (a *iLOAccessDetails) BootInterface() string {
	return "ilo-ipxe"
}

func (a *iLOAccessDetails) ManagementInterface() string {
	return ""
}

func (a *iLOAccessDetails) PowerInterface() string {
	return ""
}

func (a *iLOAccessDetails) RAIDInterface() string {
	return "no-raid"
}

func (a *iLOAccessDetails) VendorInterface() string {
	return ""
}

func (a *iLOAccessDetails) SupportsSecureBoot() bool {
	return true
}

func (a *iLOAccessDetails) BuildBIOSCleanSteps(firmwareConfig *metal3v1alpha1.FirmwareConfig) ([]nodes.CleanStep, error) {
	// If not configure ILO, only need to clear old configuration
	if firmwareConfig == nil {
		return nil, nil
	}

	var cleanSteps []nodes.CleanStep
	if firmwareConfig.ResetSettings {
		// This cleaning step resets all BIOS settings to factory default for a given node
		cleanSteps = append(
			cleanSteps,
			nodes.CleanStep{
				Interface: "bios",
				Step:      "factory_reset",
			},
		)
	}

	// Build public bios settings
	settings, err := buildBIOSSettings(*firmwareConfig,
		[]string{
			"ResetSettings",
		},
		map[string]string{
			"SimultaneousMultithreadingEnabled": "ProcHyperthreading",
			"VirtualizationEnabled":             "ProcVirtualization",
			"SriovEnabled":                      "Sriov",
		},
		map[string]string{
			"true":  "Enabled",
			"false": "Disabled",
		},
	)
	if err != nil {
		return nil, fmt.Errorf("build ilo4 public bios settings failed: %v", err)
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
