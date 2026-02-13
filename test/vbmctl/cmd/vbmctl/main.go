//go:build vbmctl
// +build vbmctl

// Package main provides the vbmctl CLI entrypoint.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/metal3-io/baremetal-operator/test/vbmctl/pkg/api"
	"github.com/metal3-io/baremetal-operator/test/vbmctl/pkg/config"
	"github.com/metal3-io/baremetal-operator/test/vbmctl/pkg/libvirt"
	"github.com/spf13/cobra"
	libvirtgo "libvirt.org/go/libvirt"
)

var (
	// Version is set at build time.
	Version = "dev"

	// Global flags.
	cfgFile     string
	libvirtURI  string
	verbosity   int
	showVersion bool
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
for testing and development purposes. It provides functionality for:

  - Creating and managing virtual machines using libvirt
  - Setting up networks for bare metal simulation
  - Managing BMC emulators (sushy-tools, vbmc)
  - Running image servers for provisioning

vbmctl is designed to be as simple to use as 'kind' for creating
test environments for the Bare Metal Operator (BMO) and CAPM3.`,
		SilenceUsage: true,
		PersistentPreRunE: func(_ *cobra.Command, _ []string) error {
			if showVersion {
				//nolint:forbidigo // CLI output is intentional
				fmt.Printf("vbmctl version %s\n", Version)
				os.Exit(0)
			}
			return nil
		},
	}

	// Global flags
	rootCmd.PersistentFlags().StringVarP(&cfgFile, "config", "c", "", "config file (default is ./vbmctl.yaml)")
	rootCmd.PersistentFlags().StringVar(&libvirtURI, "libvirt-uri", "", "libvirt connection URI (default is qemu:///system)")
	rootCmd.PersistentFlags().IntVarP(&verbosity, "verbosity", "v", 0, "verbosity level (0=normal, 1=verbose, 2=debug)")
	rootCmd.PersistentFlags().BoolVar(&showVersion, "version", false, "show version and exit")

	// Add subcommands
	rootCmd.AddCommand(newCreateCmd())
	rootCmd.AddCommand(newDeleteCmd())
	rootCmd.AddCommand(newStatusCmd())
	rootCmd.AddCommand(newConfigCmd())
	rootCmd.AddCommand(newVersionCmd())

	return rootCmd
}

func newCreateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create resources",
		Long:  "Create virtual bare metal resources such as VMs, networks, and storage pools.",
	}

	cmd.AddCommand(newCreateVMCmd())
	cmd.AddCommand(newCreateBMLCmd())
	return cmd
}

func newCreateVMCmd() *cobra.Command {
	var (
		name       string
		memory     int
		vcpus      int
		network    string
		macAddress string
		ipAddress  string
		volumeSize int
	)

	cmd := &cobra.Command{
		Use:   "vm",
		Short: "Create a virtual machine",
		Long:  "Create a new virtual machine with the specified configuration.",
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

			vmCfg := api.VMConfig{
				Name:   name,
				Memory: memory,
				VCPUs:  vcpus,
				Volumes: []api.VolumeConfig{
					{Name: "1", Size: volumeSize},
					{Name: "2", Size: volumeSize},
				},
			}

			if network != "" {
				vmCfg.Networks = []api.NetworkAttachment{
					{
						Network:    network,
						MACAddress: macAddress,
						IPAddress:  ipAddress,
					},
				}
			}

			vm, err := vmManager.Create(ctx, vmCfg)
			if err != nil {
				return fmt.Errorf("failed to create VM: %w", err)
			}

			//nolint:forbidigo // CLI output is intentional
			fmt.Printf("Created VM %s (UUID: %s)\n", vm.Config.Name, vm.UUID)
			return nil
		},
	}

	cmd.Flags().StringVar(&name, "name", "bmo-e2e-0", "name of the VM")
	cmd.Flags().IntVar(&memory, "memory", config.DefaultVMMemory, "memory in MB")
	cmd.Flags().IntVar(&vcpus, "vcpus", config.DefaultVMVCPUs, "number of vCPUs")
	cmd.Flags().StringVar(&network, "network", config.DefaultNetworkName, "network to attach to")
	cmd.Flags().StringVar(&macAddress, "mac-address", "00:60:2f:31:81:01", "MAC address for the network interface")
	cmd.Flags().StringVar(&ipAddress, "ip-address", "", "IP address to reserve (optional)")
	cmd.Flags().IntVar(&volumeSize, "volume-size", config.DefaultVolumeSize, "volume size in GB")

	return cmd
}

func newCreateBMLCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "bml",
		Short: "Create a bare metal lab from configuration file",
		Long: `Create a bare metal lab (bml) with all VMs defined in the spec.vms section
of the configuration file.

Example configuration:
  spec:
    vms:
      - name: "bmo-e2e-0"
        memory: 4096
        vcpus: 2
        volumes:
          - name: "root"
            size: 20
        networks:
          - network: "baremetal-e2e"
            macAddress: "00:60:2f:31:81:01"`,
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

			//nolint:forbidigo // CLI output is intentional
			fmt.Printf("Creating bare metal lab with %d VM(s)...\n", len(cfg.Spec.VMs))

			vms, err := vmManager.CreateAll(ctx, cfg.Spec.VMs)
			if err != nil {
				return err
			}

			//nolint:forbidigo // CLI output is intentional
			fmt.Println("\nCreated VMs:")
			for _, vm := range vms {
				//nolint:forbidigo // CLI output is intentional
				fmt.Printf("  - %s (UUID: %s)\n", vm.Config.Name, vm.UUID)
			}

			return nil
		},
	}

	return cmd
}

func newDeleteCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete resources",
		Long:  "Delete virtual bare metal resources such as VMs, networks, and storage pools.",
	}

	cmd.AddCommand(newDeleteVMCmd())
	cmd.AddCommand(newDeleteBMLCmd())
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

			if err := vmManager.DeleteAll(ctx, names, true); err != nil {
				return err
			}

			//nolint:forbidigo // CLI output is intentional
			fmt.Println("Deleted VMs (and their volumes):")
			for _, name := range names {
				//nolint:forbidigo // CLI output is intentional
				fmt.Printf("  - %s\n", name)
			}

			return nil
		},
	}

	return cmd
}

func newStatusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show status of the environment",
		Long:  "Display the current status of VMs, networks, and other resources.",
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

	return cmd
}

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

			return nil
		},
	}

	return cmd
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the version",
		Long:  "Print the version of vbmctl.",
		Run: func(_ *cobra.Command, _ []string) {
			//nolint:forbidigo // CLI output is intentional
			fmt.Printf("vbmctl version %s\n", Version)
		},
	}
}

// loadConfig loads the configuration from file or returns defaults.
func loadConfig() (*config.Config, error) {
	// Use explicit config file if provided
	if cfgFile != "" {
		cfg, err := config.Load(cfgFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load config from %s: %w", cfgFile, err)
		}
		applyFlagOverrides(cfg)
		return cfg, nil
	}

	// Try to find config file in standard locations
	if found := config.FindConfigFile(); found != "" {
		cfg, err := config.Load(found)
		if err != nil {
			return nil, fmt.Errorf("failed to load config from %s: %w", found, err)
		}
		applyFlagOverrides(cfg)
		return cfg, nil
	}

	// Use defaults
	cfg := config.Default()
	applyFlagOverrides(cfg)
	return cfg, nil
}

// applyFlagOverrides applies command-line flag overrides to the config.
func applyFlagOverrides(cfg *config.Config) {
	if libvirtURI != "" {
		cfg.Spec.Libvirt.URI = libvirtURI
	}
}

// contextWithSignal creates a context that is canceled on SIGINT or SIGTERM.
func contextWithSignal() (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		select {
		case <-sigChan:
			cancel()
		case <-ctx.Done():
		}
	}()

	return ctx, cancel
}
