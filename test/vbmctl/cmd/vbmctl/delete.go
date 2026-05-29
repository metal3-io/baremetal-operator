//go:build vbmctl
// +build vbmctl

// Provides the implementation of the "vbmctl delete" command.
package main

import (
	"errors"
	"fmt"

	"github.com/metal3-io/baremetal-operator/test/vbmctl/pkg/config"
	containers "github.com/metal3-io/baremetal-operator/test/vbmctl/pkg/containers"
	"github.com/metal3-io/baremetal-operator/test/vbmctl/pkg/libvirt"
	"github.com/metal3-io/baremetal-operator/test/vbmctl/pkg/network"
	"github.com/spf13/cobra"
	libvirtgo "libvirt.org/go/libvirt"
)

func newDeleteCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete resources",
		Long:  "Delete virtual bare metal resources such as VMs.",
	}

	cmd.AddCommand(newDeleteVMCmd())
	cmd.AddCommand(newDeleteBMLCmd())
	cmd.AddCommand(newDeleteNetworkCmd())
	cmd.AddCommand(newDeleteImageServerCmd())
	cmd.AddCommand(newDeleteBMCEmulatorCmd())
	return cmd
}

func newDeleteVMCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "vm [name]",
		Short: "Delete a virtual machine",
		Long:  "Delete a virtual machine and its associated volumes.",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			ctx, cancel := contextWithSignal()
			defer cancel()

			name := args[0]

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

			if err := vmManager.Delete(ctx, name, true); err != nil {
				return fmt.Errorf("failed to delete VM: %w", err)
			}

			//nolint:forbidigo // CLI output is intentional
			fmt.Printf("Deleted VM %s (and its volumes)\n", name)
			return nil
		},
	}

	return cmd
}

func newDeleteBMLCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "bml",
		Short: "Delete the bare metal lab",
		Long:  "Delete the bare metal lab (all VMs and their volumes defined in the spec.vms section of the configuration file).",
		RunE: func(_ *cobra.Command, _ []string) error {
			ctx, cancel := contextWithSignal()
			defer cancel()

			cfg, err := loadConfig()
			if err != nil {
				return err
			}

			if len(cfg.Spec.VMs) == 0 {
				return errors.New("no VMs defined in configuration (spec.vms is empty)")
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

			names := make([]string, len(cfg.Spec.VMs))
			for i, vm := range cfg.Spec.VMs {
				names[i] = vm.Name
			}

			//nolint:forbidigo // CLI output is intentional
			fmt.Printf("Deleting bare metal lab (%d VMs)...\n", len(names))
			err = vmManager.DeleteAll(ctx, names, true)
			if err != nil {
				return err
			}

			//nolint:forbidigo // CLI output is intentional
			fmt.Println("Deleted VMs (and their volumes):")
			for _, name := range names {
				//nolint:forbidigo // CLI output is intentional
				fmt.Printf("  - %s\n", name)
			}

			// Delete veth pairs
			err = network.DeleteAllVeth(ctx, cfg.Spec.VethPairs)
			if err != nil {
				// Don't fail whole command if veth deletion fails
				//nolint:forbidigo // CLI output is intentional
				fmt.Printf("failed to delete veth pairs: %v\n", err)
			} else {
				//nolint:forbidigo // CLI output is intentional
				fmt.Println("Deleted veth pairs:")
				for _, pair := range cfg.Spec.VethPairs {
					//nolint:forbidigo // CLI output is intentional
					fmt.Printf("  - between %s and %s\n", pair.Link1, pair.Link2)
				}
			}

			networkManager, err := libvirt.NewNetworkManager(conn)
			if err != nil {
				return fmt.Errorf("failed to create libvirt network manager: %w", err)
			}

			networks := make([]string, len(cfg.Spec.Networks))
			for i, network := range cfg.Spec.Networks {
				networks[i] = network.Name
			}

			//nolint:forbidigo // CLI output is intentional
			fmt.Printf("Deleting libvirt networks (%d networks)...\n", len(networks))

			if err = networkManager.DeleteNetworks(ctx, networks); err != nil {
				// Don't fail whole command if network deletion fails
				//nolint:forbidigo // CLI output is intentional
				fmt.Printf("Warning: failed to delete network: %v\n", err)
			}

			//nolint:forbidigo // CLI output is intentional
			fmt.Println("Deleted networks:")
			for _, name := range networks {
				//nolint:forbidigo // CLI output is intentional
				fmt.Printf("  - %s\n", name)
			}

			err = containers.DeleteBridgeNetworks(ctx, cfg.Spec.DockerNetworks)
			if err != nil {
				// Don't fail whole command if network deletion fails
				//nolint:forbidigo // CLI output is intentional
				fmt.Printf("Warning: failed to delete Docker network: %v\n", err)
			}
			//nolint:forbidigo // CLI output is intentional
			fmt.Println("Deleted Docker networks:")
			for _, net := range cfg.Spec.DockerNetworks {
				//nolint:forbidigo // CLI output is intentional
				fmt.Printf("  - name: %s\n", net.Name)
			}

			if cfg.Spec.ImageServer != nil {
				err := containers.DeleteImageServerInstance(ctx, cfg.Spec.ImageServer.ContainerName)
				// don't fail the whole command if image server deletion fails, just log the error
				if err != nil {
					//nolint:forbidigo // CLI output is intentional
					fmt.Printf("%v\n", err)
				}
			} else {
				//nolint:forbidigo // CLI output is intentional
				fmt.Println("No image server configuration found in the config file.")
			}

			if cfg.Spec.BMCEmulator != nil {
				err := containers.DeleteBMCEmulatorInstance(ctx, cfg.Spec.BMCEmulator.Type)
				// don't fail the whole command if BMC emulator deletion fails, just log the error
				if err != nil {
					//nolint:forbidigo // CLI output is intentional
					fmt.Printf("%v\n", err)
				}
			} else {
				//nolint:forbidigo // CLI output is intentional
				fmt.Println("No BMC emulator configuration found in the config file.")
			}

			return nil
		},
	}

	return cmd
}

func newDeleteNetworkCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "network [name]",
		Short: "Delete a network",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			ctx, cancel := contextWithSignal()
			defer cancel()

			name := args[0]

			cfg, err := loadConfig()
			if err != nil {
				return err
			}

			conn, err := libvirtgo.NewConnect(cfg.Spec.Libvirt.URI)
			if err != nil {
				return fmt.Errorf("failed to connect to libvirt: %w", err)
			}
			defer func() { _, _ = conn.Close() }()

			networkManager, err := libvirt.NewNetworkManager(conn)
			if err != nil {
				return fmt.Errorf("failed to create Network manager: %w", err)
			}

			if err := networkManager.DeleteNetwork(ctx, name); err != nil {
				return fmt.Errorf("failed to delete network: %w", err)
			}

			//nolint:forbidigo // CLI output is intentional
			fmt.Printf("Deleted network %s\n", name)
			return nil
		},
	}

	return cmd
}

func newDeleteImageServerCmd() *cobra.Command {
	var containerName string

	cmd := &cobra.Command{
		Use:   "image-server",
		Short: "Delete the image server instance",
		Long:  "Delete the image server instance used for provisioning.",
		RunE: func(_ *cobra.Command, _ []string) error {
			ctx, cancel := contextWithSignal()
			defer cancel()

			cfg, err := loadConfig()
			if err != nil {
				return err
			}

			// command-line flag takes precedence over config file value
			if containerName == "" {
				if cfg.Spec.ImageServer != nil {
					containerName = cfg.Spec.ImageServer.ContainerName
				} else {
					containerName = config.DefaultImageServerContainerName
				}
			}

			return containers.DeleteImageServerInstance(ctx, containerName)
		},
	}

	cmd.Flags().StringVar(&containerName, "name", "", "name of the image server container to delete (default is "+config.DefaultImageServerContainerName+" if not set in config file)")

	return cmd
}

func newDeleteBMCEmulatorCmd() *cobra.Command {
	var emulatorType string

	cmd := &cobra.Command{
		Use:   "bmc-emulator",
		Short: "Delete the BMC emulator instance",
		Long:  "Delete the BMC emulator instance used for provisioning.",
		RunE: func(_ *cobra.Command, _ []string) error {
			ctx, cancel := contextWithSignal()
			defer cancel()

			cfg, err := loadConfig()
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

			return containers.DeleteBMCEmulatorInstance(ctx, emulatorType)
		},
	}

	cmd.Flags().StringVar(&emulatorType, "emulator-type", "", "type of the BMC emulator container to delete (default is "+config.DefaultBMCEmulatorType+" if not set in config file)")

	return cmd
}
