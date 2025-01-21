//go:build e2e
// +build e2e

package e2e

import (
	"os"

	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	"gopkg.in/yaml.v2"
)

type Network struct {
	// Name of the libvirt network.
	Name string `yaml:"name,omitempty"`
	// MacAddress of the interface connected to the network.
	MacAddress string `yaml:"macAddress,omitempty"`
	// IPAddress to reserve for the MAC address in the network.
	IPAddress string `yaml:"ipAddress,omitempty"`
}

// BMC defines connection details for a baseboard management controller
// and other details needed for creating a virtual machine related to it.
type BMC struct {
	// User is the username for accessing the BMC.
	User string `yaml:"user,omitempty"`
	// Password is the password for accessing the BMC.
	Password string `yaml:"password,omitempty"`
	// Address of the BMC, e.g. "redfish-virtualmedia+http://192.168.222.1:8000/redfish/v1/Systems/bmo-e2e-1".
	Address string `yaml:"address,omitempty"`
	// DisableCertificateVerification indicates whether to disable certificate verification for the BMC connection.
	DisableCertificateVerification bool `yaml:"disableCertificateVerification,omitempty"`
	// BootMacAddress is the MAC address of the BMHs network interface.
	BootMacAddress string `yaml:"bootMacAddress,omitempty"`
	// BootMode is the boot mode for the BareMetalHost, e.g. "UEFI" or "legacy".
	BootMode metal3api.BootMode `yaml:"bootMode,omitempty"`
	// Name of the machine associated with this BMC.
	Name string `yaml:"name,omitempty"`
	// IPAddress is a reserved IP address for the BMH managed through this BMC.
	// This is used in tests that make ssh connections to the BMH.
	// Example: 192.168.222.122
	IPAddress string `yaml:"ipAddress,omitempty"`
	// RootDeviceHints provides guidance for where to write the disk image.
	RootDeviceHints metal3api.RootDeviceHints `yaml:"rootDeviceHints,omitempty"`
	// Networks describes the network interfaces that should be added to the VM representing this BMH.
	Networks []Network `yaml:"networks,omitempty"`
}

func LoadBMCConfig(configPath string) ([]BMC, error) {
	configData, err := os.ReadFile(configPath) //#nosec
	var bmcs []BMC
	if err != nil {
		return nil, err
	}
	if err := yaml.Unmarshal(configData, &bmcs); err != nil {
		return nil, err
	}
	return bmcs, nil
}
