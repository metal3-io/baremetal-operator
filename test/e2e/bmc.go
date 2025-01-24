//go:build e2e
// +build e2e

package e2e

import (
	"os"

	"gopkg.in/yaml.v2"
)

// BMC defines connection details for a baseboard management controller
// and other details needed for creating a virtual machine related to it.
type BMC struct {
	// User is the username for accessing the BMC.
	User string `yaml:"user,omitempty"`
	// Password is the password for accessing the BMC.
	Password string `yaml:"password,omitempty"`
	// Address of the BMC, e.g. "redfish-virtualmedia+http://192.168.222.1:8000/redfish/v1/Systems/bmo-e2e-1".
	Address string `yaml:"address,omitempty"`
	// BootMacAddress is the MAC address of the VMs network interface.
	BootMacAddress string `yaml:"bootMacAddress,omitempty"`
	// Name of the machine associated with this BMC.
	Name string `yaml:"name,omitempty"`
	// NetworkName is the name of the network that the new VM should be attached to
	NetworkName string `yaml:"networkName,omitempty"`
	// IPAddress is a reserved IP address for the VM.
	// This will be paired with the MAC address in the DHCP configuration.
	// Example: 192.168.222.122
	IPAddress string `yaml:"ipAddress,omitempty"`
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
