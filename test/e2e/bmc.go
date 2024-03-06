package e2e

import (
	"os"

	. "github.com/onsi/gomega"
	"gopkg.in/yaml.v2"
)

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
}

func LoadBMCConfig(configPath string) *[]BMC {
	configData, err := os.ReadFile(configPath) //#nosec
	Expect(err).ToNot(HaveOccurred(), "Failed to read the bmcs config file")
	var bmcs []BMC
	Expect(yaml.Unmarshal(configData, &bmcs)).To(Succeed())
	return &bmcs
}
