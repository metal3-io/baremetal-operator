//go:build e2e
// +build e2e

package e2e

import (
	"os"

	"gopkg.in/yaml.v2"
)

type Network struct {
	// Name of the network
	NetworkName string `yaml:"name,omitempty"`
	// MacAddress of the network
	MacAddress string `yaml:"macAddress,omitempty"`
	// IPAddress of the network
	IPAddress string `yaml:"ipAddress,omitempty"`
}

type Networks []Network

// BMC defines a BMH to use in the tests.
type BMC struct {
	// BMC initial username
	User string `yaml:"user,omitempty"`
	// BMC initial password
	Password string `yaml:"password,omitempty"`
	// BMC initial address
	Address string `yaml:"address,omitempty"`
	// BMC Mac address
	BootMacAddress string `yaml:"bootMacAddress,omitempty"`
	// The Hostname of the node, which will be read into BMH object
	HostName string `yaml:"hostName,omitempty"`
	// The IP address of the node
	// Optional. Only needed if e2eConfig variable
	// SSH_CHECK_PROVISIONED is true
	IPAddress string `yaml:"ipAddress,omitempty"`
	// Optional. Only needed if e2eConfig variable
	// SSH_CHECK_PROVISIONED is true
	SSHPort string `yaml:"sshPort,omitempty"`
	// Optional. Not needed for E2E tests
	Networks Networks
}

func LoadBMCConfig(configPath string) (*[]BMC, error) {
	configData, err := os.ReadFile(configPath) //#nosec
	var bmcs []BMC
	if err != nil {
		return nil, err
	}
	if err := yaml.Unmarshal(configData, &bmcs); err != nil {
		return nil, err
	}
	return &bmcs, nil
}
