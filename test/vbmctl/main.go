//go:build vbmctl
// +build vbmctl

// Package main provides the legacy vbmctl entrypoint for backward compatibility.
// New users should use the CLI at cmd/vbmctl instead.
package main

import (
	"context"
	"flag"
	"log"
	"os"

	"github.com/metal3-io/baremetal-operator/test/vbmctl/pkg/api"
	"github.com/metal3-io/baremetal-operator/test/vbmctl/pkg/config"
	"github.com/metal3-io/baremetal-operator/test/vbmctl/pkg/libvirt"
	"gopkg.in/yaml.v2"
	libvirtgo "libvirt.org/go/libvirt"
)

// BMCConfig represents the legacy BMC configuration format for backward compatibility.
// This mirrors the structure from test/e2e/bmc.go.
type BMCConfig struct {
	User                           string    `yaml:"user,omitempty"`
	Password                       string    `yaml:"password,omitempty"`
	Address                        string    `yaml:"address,omitempty"`
	DisableCertificateVerification bool      `yaml:"disableCertificateVerification,omitempty"`
	BootMacAddress                 string    `yaml:"bootMacAddress,omitempty"`
	Name                           string    `yaml:"name,omitempty"`
	IPAddress                      string    `yaml:"ipAddress,omitempty"`
	Networks                       []Network `yaml:"networks,omitempty"`
}

// Network represents a network configuration from the legacy format.
type Network struct {
	Name       string `yaml:"name,omitempty"`
	MacAddress string `yaml:"macAddress,omitempty"`
	IPAddress  string `yaml:"ipAddress,omitempty"`
}

func main() {
	var name = flag.String(
		"name", "BMH-0", "The name of the VM to create")
	var networkName = flag.String(
		"network-name", "baremetal-e2e", "The name of the network that the new VM should be attached to")
	var macAddress = flag.String(
		"mac-address", "00:60:2f:31:81:01", "Mac address of the VM on the network")
	var ipAddress = flag.String(
		"ip-address", "", "IP address of the VM on the network")
	var configFile = flag.String(
		"yaml-source-file", "", "yaml file where BMCS are defined. If this is set, ignore all other options")
	var memory = flag.Int(
		"memory", config.DefaultVMMemory, "Memory in MB for the VM")
	var vcpus = flag.Int(
		"vcpus", config.DefaultVMVCPUs, "Number of vCPUs for the VM")
	var volumeSize = flag.Int(
		"volume-size", config.DefaultVolumeSize, "Volume size in GB")
	var libvirtURI = flag.String(
		"libvirt-uri", config.DefaultLibvirtURI, "Libvirt connection URI")
	var poolName = flag.String(
		"pool-name", config.DefaultPoolName, "Storage pool name")
	var poolPath = flag.String(
		"pool-path", config.DefaultPoolPath, "Storage pool path")

	flag.Parse()

	ctx := context.Background()

	// Build list of VMs to create
	vmConfigs := []api.VMConfig{}

	if *configFile == "" {
		// Single VM from command-line flags
		vmCfg := api.VMConfig{
			Name:   *name,
			Memory: *memory,
			VCPUs:  *vcpus,
			Volumes: []api.VolumeConfig{
				{Name: "1", Size: *volumeSize},
				{Name: "2", Size: *volumeSize},
			},
			Networks: []api.NetworkAttachment{
				{
					Network:    *networkName,
					MACAddress: *macAddress,
					IPAddress:  *ipAddress,
				},
			},
		}
		vmConfigs = append(vmConfigs, vmCfg)
	} else {
		// Load from YAML file (legacy format)
		bmcs, err := loadBMCConfig(*configFile)
		if err != nil {
			log.Fatalf("Error loading config: %v\n", err)
		}

		for _, bmc := range bmcs {
			vmCfg := convertBMCToVMConfig(bmc, *memory, *vcpus, *volumeSize)
			vmConfigs = append(vmConfigs, vmCfg)
		}
	}

	// Connect to libvirt
	conn, err := libvirtgo.NewConnect(*libvirtURI)
	if err != nil {
		log.Fatalf("Error connecting to libvirt: %v\n", err)
	}

	// Create VM manager
	vmManager, err := libvirt.NewVMManager(conn, libvirt.VMManagerOptions{
		PoolName: *poolName,
		PoolPath: *poolPath,
	})
	if err != nil {
		_, _ = conn.Close()
		log.Fatalf("Error creating VM manager: %v\n", err)
	}

	// Create each VM
	for _, vmCfg := range vmConfigs {
		vm, err := vmManager.Create(ctx, vmCfg)
		if err != nil {
			log.Printf("Error creating VM %s: %v\n", vmCfg.Name, err)
			break
		}
		log.Printf("Successfully created VM %s (UUID: %s)\n", vm.Config.Name, vm.UUID)
	}

	_, _ = conn.Close()
}

// loadBMCConfig loads BMC configuration from a YAML file.
func loadBMCConfig(configPath string) ([]BMCConfig, error) {
	configData, err := os.ReadFile(configPath) //#nosec G304
	if err != nil {
		return nil, err
	}

	var bmcs []BMCConfig
	if err := yaml.Unmarshal(configData, &bmcs); err != nil {
		return nil, err
	}

	return bmcs, nil
}

// convertBMCToVMConfig converts a legacy BMCConfig to the new api.VMConfig format.
func convertBMCToVMConfig(bmc BMCConfig, memory, vcpus, volumeSize int) api.VMConfig {
	networks := make([]api.NetworkAttachment, len(bmc.Networks))
	for i, net := range bmc.Networks {
		networks[i] = api.NetworkAttachment{
			Network:    net.Name,
			MACAddress: net.MacAddress,
			IPAddress:  net.IPAddress,
		}
	}

	return api.VMConfig{
		Name:   bmc.Name,
		Memory: memory,
		VCPUs:  vcpus,
		Volumes: []api.VolumeConfig{
			{Name: "1", Size: volumeSize},
			{Name: "2", Size: volumeSize},
		},
		Networks: networks,
	}
}
