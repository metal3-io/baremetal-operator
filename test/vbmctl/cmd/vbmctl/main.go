//go:build vbmctl
// +build vbmctl

// Package main provides the vbmctl CLI entrypoint.
package main

import (
	"fmt"
	"os"

	"github.com/metal3-io/baremetal-operator/test/vbmctl/pkg/config"
	"github.com/spf13/cobra"
)

func main() {
	if err := newRootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "vbmctl",
		Short: "Virtual Bare Metal Controller - manage virtual bare metal environments",
		Long: `vbmctl is a tool for creating and managing virtual bare metal environments
for testing and development purposes. It currently provides functionality for:

  - Creating and managing virtual machines using libvirt
  - Creating and managing libvirt networks
  - Image server for provisioning
  - Reserving IP addresses for VMs via DHCP on existing libvirt networks
  - BMC emulator support (sushy-tools, vbmc)

Planned features (not yet implemented):
  - Support multiple named environments

vbmctl is designed to be as simple to use as 'kind' for creating
test environments for the Bare Metal Operator (BMO) and CAPM3.`,
		SilenceUsage: true,
		PersistentPreRunE: func(_ *cobra.Command, _ []string) error {
			if showVersion {
				//nolint:forbidigo // CLI output is intentional
				fmt.Printf("vbmctl version %s\n", config.Version)
				os.Exit(0)
			}
			return nil
		},
	}

	// Global flags
	rootCmd.PersistentFlags().StringVarP(&cfgFile, "config", "c", "", "config file (default is ./vbmctl.yaml)")
	rootCmd.PersistentFlags().StringVar(&libvirtURI, "libvirt-uri", "", "libvirt connection URI (default is qemu:///system)")
	rootCmd.PersistentFlags().BoolVar(&showVersion, "version", false, "show version and exit")

	// Add subcommands
	rootCmd.AddCommand(newCreateCmd())
	rootCmd.AddCommand(newDeleteCmd())
	rootCmd.AddCommand(newStatusCmd())
	rootCmd.AddCommand(newConfigCmd())
	rootCmd.AddCommand(newVersionCmd())

	return rootCmd
}
