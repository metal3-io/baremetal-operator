//go:build vbmctl
// +build vbmctl

// Provides the implementation of the "vbmctl status" command.
package main

import (
	"fmt"

	"github.com/metal3-io/baremetal-operator/test/vbmctl/pkg/config"
	containers "github.com/metal3-io/baremetal-operator/test/vbmctl/pkg/containers"
	"github.com/metal3-io/baremetal-operator/test/vbmctl/pkg/libvirt"
	"github.com/spf13/cobra"
	libvirtgo "libvirt.org/go/libvirt"
)

func newStatusCmd() *cobra.Command {
	var containerName string
	var emulatorType string

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show status of the environment",
		Long:  "Display the current status of VMs.",
		RunE: func(_ *cobra.Command, _ []string) error {
			ctx, cancel := contextWithSignal()
			defer cancel()

			cfg, err := loadConfig()
			if err != nil {
				return err
			}

			conn, err := libvirtgo.NewConnect(cfg.Spec.Libvirt.URI)
			if err != nil {
				return fmt.Errorf("failed to connect to libvirt: %w", err)
			}
			defer func() { _, _ = conn.Close() }()

			vmManager, err := libvirt.NewVMManager(conn, libvirt.VMManagerOptions{
				PoolName: cfg.Spec.Pool.Name,
				PoolPath: cfg.Spec.Pool.Path,
			})
			if err != nil {
				return fmt.Errorf("failed to create VM manager: %w", err)
			}

			vms, err := vmManager.List(ctx)
			if err != nil {
				return fmt.Errorf("failed to list VMs: %w", err)
			}

			// command-line flag takes precedence over config file value
			if containerName == "" {
				if cfg.Spec.ImageServer != nil {
					containerName = cfg.Spec.ImageServer.ContainerName
				} else {
					containerName = config.DefaultImageServerContainerName
				}
			}
			// check if the image server is present
			containerInfo, err := containers.GetImageServerInfo(ctx, containerName)
			if err != nil {
				return err
			}

			// command-line flag takes precedence over config file value
			if emulatorType == "" {
				if cfg.Spec.BMCEmulator != nil {
					emulatorType = cfg.Spec.BMCEmulator.Type
				} else {
					emulatorType = config.DefaultBMCEmulatorType
				}
			}
			// check if the bmc emulator is present
			emulatorInfo, err := containers.GetBMCEmulatorInfo(ctx, emulatorType)
			if err != nil {
				return err
			}

			//nolint:forbidigo // CLI output is intentional
			fmt.Printf("Image Server container: %s\n", containerInfo)
			//nolint:forbidigo // CLI output is intentional
			fmt.Printf("BMC Emulator container: %s\n", emulatorInfo)
			//nolint:forbidigo // CLI output is intentional
			fmt.Println("Virtual Machines:")
			//nolint:forbidigo // CLI output is intentional
			fmt.Println("  NAME\t\tSTATE\t\tMEMORY\tVCPUs")
			for _, vm := range vms {
				//nolint:forbidigo // CLI output is intentional
				fmt.Printf("  %s\t\t%s\t\t%dMB\t%d\n",
					vm.Config.Name, vm.State, vm.Config.Memory, vm.Config.VCPUs)
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&containerName, "name", "", "name of the image server container (default is "+config.DefaultImageServerContainerName+" if not set in config file)")
	cmd.Flags().StringVar(&emulatorType, "emulator-type", "", "type of the BMC emulator container to check for (default is "+config.DefaultBMCEmulatorType+" if not set in config file)")

	return cmd
}
