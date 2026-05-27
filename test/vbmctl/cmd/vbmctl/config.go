//go:build vbmctl
// +build vbmctl

// Provides the implementation of the "vbmctl config" command.
package main

import (
	"fmt"

	"github.com/metal3-io/baremetal-operator/test/vbmctl/pkg/config"
	"github.com/spf13/cobra"
)

func newConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage vbmctl configuration",
		Long:  "Commands for managing vbmctl configuration files.",
	}

	cmd.AddCommand(newConfigInitCmd())
	cmd.AddCommand(newConfigViewCmd())

	return cmd
}

func newConfigInitCmd() *cobra.Command {
	var output string

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize a new configuration file",
		Long:  "Create a new vbmctl configuration file with default values.",
		RunE: func(_ *cobra.Command, _ []string) error {
			cfg := config.Default()

			if output == "" {
				output = config.DefaultConfigFileName
			}

			if err := cfg.Save(output); err != nil {
				return fmt.Errorf("failed to save config: %w", err)
			}

			//nolint:forbidigo // CLI output is intentional
			fmt.Printf("Created config file: %s\n", output)
			return nil
		},
	}

	cmd.Flags().StringVarP(&output, "output", "o", "", "output file path (default is vbmctl.yaml)")

	return cmd
}

func newConfigViewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "view",
		Short: "View the current configuration",
		Long:  "Display the current vbmctl configuration.",
		RunE: func(_ *cobra.Command, _ []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}

			//nolint:forbidigo // CLI output is intentional
			fmt.Printf("Libvirt URI: %s\n", cfg.Spec.Libvirt.URI)
			//nolint:forbidigo // CLI output is intentional
			fmt.Printf("Storage Pool: %s at %s\n", cfg.Spec.Pool.Name, cfg.Spec.Pool.Path)

			if len(cfg.Spec.VMs) > 0 {
				//nolint:forbidigo // CLI output is intentional
				fmt.Println("Configured VMs:")
				for _, vm := range cfg.Spec.VMs {
					//nolint:forbidigo // CLI output is intentional
					fmt.Printf("  - %s (memory: %dMB, vcpus: %d)\n", vm.Name, vm.Memory, vm.VCPUs)
				}
			}

			if cfg.Spec.ImageServer != nil {
				//nolint:forbidigo // CLI output is intentional
				fmt.Printf("Image Server:\n  Image: %s\n  Host Port: %d\n  Container Port: %d\n  Data Dir: %s\n  Container Name: %s\n",
					cfg.Spec.ImageServer.Image,
					cfg.Spec.ImageServer.Port,
					cfg.Spec.ImageServer.ContainerPort,
					cfg.Spec.ImageServer.DataDir,
					cfg.Spec.ImageServer.ContainerName)
			}

			if cfg.Spec.BMCEmulator != nil {
				//nolint:forbidigo // CLI output is intentional
				fmt.Printf("BMC Emulator:\n  Emulator Type: %s\n  Config File: %s\n  Image: %s\n  Listen Address: %s\n  Listen Port: %d\n",
					cfg.Spec.BMCEmulator.Type,
					cfg.Spec.BMCEmulator.ConfigFile,
					cfg.Spec.BMCEmulator.Image,
					cfg.Spec.BMCEmulator.ListenAddress,
					cfg.Spec.BMCEmulator.ListenPort)
			}

			return nil
		},
	}

	return cmd
}
