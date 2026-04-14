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
	"strconv"
	"syscall"

	vbmctlapi "github.com/metal3-io/baremetal-operator/test/vbmctl/pkg/api"
	"github.com/metal3-io/baremetal-operator/test/vbmctl/pkg/config"
	containers "github.com/metal3-io/baremetal-operator/test/vbmctl/pkg/containers"
	"github.com/metal3-io/baremetal-operator/test/vbmctl/pkg/libvirt"
	"github.com/spf13/cobra"
	libvirtgo "libvirt.org/go/libvirt"
)

var (
	// Global flags.
	cfgFile     string
	libvirtURI  string
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
for testing and development purposes. It currently provides functionality for:

  - Creating and managing virtual machines using libvirt
  - Creating and managing libvirt networks
  - Image server for provisioning
  - Reserving IP addresses for VMs via DHCP on existing libvirt networks

Planned features (not yet implemented):
  - BMC emulator support (sushy-tools, vbmc)

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

func newCreateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create resources",
		Long:  "Create virtual bare metal resources such as VMs.",
	}

	cmd.AddCommand(newCreateVMCmd())
	cmd.AddCommand(newCreateBMLCmd())
	cmd.AddCommand(newCreateNetworkCmd())
	cmd.AddCommand(newCreateImageServerCmd())
	return cmd
}

func newCreateVMCmd() *cobra.Command {
	var (
		name        string
		memory      int
		vcpus       int
		network     string
		macAddress  string
		ipAddress   string
		volumeSizes []int
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

			if len(volumeSizes) == 0 {
				volumeSizes = []int{config.DefaultVolumeSize}
			}
			volumes := make([]vbmctlapi.VolumeConfig, len(volumeSizes))
			for i, sz := range volumeSizes {
				volumes[i] = vbmctlapi.VolumeConfig{
					Name: "p" + strconv.Itoa(i+1),
					Size: sz,
				}
			}

			vmCfg := vbmctlapi.VMConfig{
				Name:    name,
				Memory:  memory,
				VCPUs:   vcpus,
				Volumes: volumes,
			}

			if network != "" {
				vmCfg.Networks = []vbmctlapi.NetworkAttachment{
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
	cmd.Flags().IntSliceVar(&volumeSizes, "volume-size", nil,
		"volume size in GB (default a single "+strconv.Itoa(config.DefaultVolumeSize)+
			" GB volume, repeat flag to add multiple volumes or use comma-separated values)")

	return cmd
}

func newCreateBMLCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "bml",
		Short: "Create a bare metal lab from configuration file",
		Long: `Create a bare metal lab (bml) with all VMs and networks defined in the spec.vms
and spec.networks sections of the configuration file. Note that network block can
be omitted and VMs can be connected to existing networks as well.

Example configuration:
  spec:
    vms:
      - name: "bmo-e2e-0"
        memory: 4096
        vcpus: 2
        volumes:
          - name: "root"
            size: 20
        networkAttachments:
          - network: "baremetal-e2e"
            macAddress: "00:60:2f:31:81:01"
	networks:
      - name: "baremetal-e2e"
	  - bridge: "metal3"
    imageServer:
      image: "nginxinc/nginx-unprivileged"
      port: 8080
      containerPort: 8080
      dataDir: "/var/lib/vbmctl/images"
      containerDataDir: "/usr/share/nginx/html",
      containerName: "vbmctl-image-server"`,
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

			if cfg.Spec.ImageServer != nil {
				err = containers.CreateImageServerInstance(ctx, cfg.Spec.ImageServer)
				if err != nil {
					return err
				}
			} else {
				//nolint:forbidigo // CLI output is intentional
				fmt.Println("No image server configuration found in the config file.")
			}

			conn, err := libvirtgo.NewConnect(cfg.Spec.Libvirt.URI)
			if err != nil {
				return fmt.Errorf("failed to connect to libvirt: %w", err)
			}
			defer func() { _, _ = conn.Close() }()

			// Create networks before VMs
			networkManager, err := libvirt.NewNetworkManager(conn)
			if err != nil {
				return fmt.Errorf("failed to create Network manager: %w", err)
			}
			networks, err := networkManager.CreateNetworks(ctx, cfg.Spec.Networks)
			if err != nil {
				return err
			}
			//nolint:forbidigo // CLI output is intentional
			fmt.Println("\nCreated networks:")
			for _, network := range networks {
				//nolint:forbidigo // CLI output is intentional
				fmt.Printf("  - %s (UUID: %s)\n", network.Name, network.UUID)
			}

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

func newCreateNetworkCmd() *cobra.Command {
	var (
		name    string
		bridge  string
		address string
		netmask string
	)

	cmd := &cobra.Command{
		Use:   "network",
		Short: "Create a network",
		Long:  "Create a new network with the specified configuration.",
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

			networkManager, err := libvirt.NewNetworkManager(conn)
			if err != nil {
				return fmt.Errorf("failed to create Network manager: %w", err)
			}

			networkCfg := vbmctlapi.NetworkConfig{
				Name:    name,
				Bridge:  bridge,
				Address: address,
				Netmask: netmask,
			}

			network, err := networkManager.CreateNetwork(ctx, networkCfg)
			if err != nil {
				return err
			}
			//nolint:forbidigo // CLI output is intentional
			fmt.Printf("Created network: %s (UUID: %s)\n", network.Name, network.UUID)
			return nil
		},
	}

	cmd.Flags().StringVar(&name, "name", config.DefaultNetworkName, "name of the network")
	cmd.Flags().StringVar(&bridge, "bridge", config.DefaultNetworkBridge, "name of the bridge interface")
	cmd.Flags().StringVar(&address, "address", config.DefaultNetworkAddress, "address of bridge")
	cmd.Flags().StringVar(&netmask, "netmask", config.DefaultNetworkNetmask, "netmask for network")

	return cmd
}

func newCreateImageServerCmd() *cobra.Command {
	var (
		containerName               string
		image                       string
		imageServerPort             uint16
		imageServerContainerPort    uint16
		imageServerDataDir          string
		imageServerContainerDataDir string
	)

	cmd := &cobra.Command{
		Use:   "image-server",
		Short: "Create an image server instance",
		Long:  "Create an image server instance to be used for provisioning.",
		RunE: func(_ *cobra.Command, _ []string) error {
			ctx, cancel := contextWithSignal()
			defer cancel()

			cfg, err := loadConfig()
			if err != nil {
				return err
			}

			// Resolve effective image server config: config file values, falling back to defaults.
			// Command-line flags take precedence over both.
			effective := &vbmctlapi.ImageServerConfig{
				Image:            config.DefaultImageServerImage,
				Port:             config.DefaultImageServerPort,
				ContainerPort:    config.DefaultImageServerContainerPort,
				DataDir:          config.DefaultImageServerDataDir,
				ContainerDataDir: config.DefaultContainerDataDir,
				ContainerName:    config.DefaultImageServerContainerName,
			}
			if cfg.Spec.ImageServer != nil {
				effective = cfg.Spec.ImageServer
			}

			if image == "" {
				image = effective.Image
			}
			if imageServerPort == 0 {
				imageServerPort = effective.Port
			}
			if imageServerContainerPort == 0 {
				imageServerContainerPort = effective.ContainerPort
			}
			if imageServerDataDir == "" {
				imageServerDataDir = effective.DataDir
			}
			if imageServerContainerDataDir == "" {
				imageServerContainerDataDir = effective.ContainerDataDir
			}
			if containerName == "" {
				containerName = effective.ContainerName
			}

			return containers.CreateImageServerInstance(ctx, &vbmctlapi.ImageServerConfig{
				Image:            image,
				Port:             imageServerPort,
				ContainerPort:    imageServerContainerPort,
				DataDir:          imageServerDataDir,
				ContainerDataDir: imageServerContainerDataDir,
				ContainerName:    containerName,
			})
		},
	}

	cmd.Flags().StringVar(&containerName, "name", "", "name of the image server container (default is "+config.DefaultImageServerContainerName+" if not set in config file)")
	cmd.Flags().StringVar(&image, "image", "", "container image to use for the image server (default is "+config.DefaultImageServerImage+" if not set in config file)")
	cmd.Flags().Uint16Var(&imageServerPort, "host-port", 0, "host port to bind the image server to (default is "+strconv.Itoa(int(config.DefaultImageServerPort))+" if not set in config file)")
	cmd.Flags().Uint16Var(&imageServerContainerPort, "container-port", 0, "container port that the image server listens on (default is "+strconv.Itoa(int(config.DefaultImageServerContainerPort))+" if not set in config file)")
	cmd.Flags().StringVar(&imageServerDataDir, "image-dir", "", "host directory to mount as a volume for the image server (default is "+config.DefaultImageServerDataDir+" if not set in config file)")
	cmd.Flags().StringVar(&imageServerContainerDataDir, "container-dir", "", "directory inside the container to mount the data volume to (default is "+config.DefaultContainerDataDir+" if not set in config file)")

	return cmd
}

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

			networkManager, err := libvirt.NewNetworkManager(conn)
			if err != nil {
				return fmt.Errorf("failed to create Network manager: %w", err)
			}

			networks := make([]string, len(cfg.Spec.Networks))
			for i, network := range cfg.Spec.Networks {
				networks[i] = network.Name
			}

			//nolint:forbidigo // CLI output is intentional
			fmt.Printf("Deleting networks (%d networks)...\n", len(networks))

			if err := networkManager.DeleteNetworks(ctx, networks); err != nil {
				return err
			}

			//nolint:forbidigo // CLI output is intentional
			fmt.Println("Deleted networks:")
			for _, name := range networks {
				//nolint:forbidigo // CLI output is intentional
				fmt.Printf("  - %s\n", name)
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

func newStatusCmd() *cobra.Command {
	var containerName string

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

			//nolint:forbidigo // CLI output is intentional
			fmt.Printf("Image Server container: %s\n", containerInfo)
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

			if cfg.Spec.ImageServer != nil {
				//nolint:forbidigo // CLI output is intentional
				fmt.Printf("Image Server:\n  Image: %s\n  Host Port: %d\n  Container Port: %d\n  Data Dir: %s\n  Container Name: %s\n",
					cfg.Spec.ImageServer.Image,
					cfg.Spec.ImageServer.Port,
					cfg.Spec.ImageServer.ContainerPort,
					cfg.Spec.ImageServer.DataDir,
					cfg.Spec.ImageServer.ContainerName)
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
			fmt.Printf("vbmctl version %s\n", config.Version)
		},
	}
}

// loadConfig loads the configuration from file or returns defaults.
func loadConfig() (*config.Config, error) {
	var (
		cfg *config.Config
		err error
	)

	// Use explicit config file if provided
	if cfgFile != "" {
		cfg, err = config.Load(cfgFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load config from %s: %w", cfgFile, err)
		}
	} else if found := config.FindConfigFile(); found != "" {
		// Try to find config file in standard locations
		cfg, err = config.Load(found)
		if err != nil {
			return nil, fmt.Errorf("failed to load config from %s: %w", found, err)
		}
	} else {
		// Use defaults
		cfg = config.Default()
	}

	// Apply command-line overrides, then defaults, then validate.
	applyFlagOverrides(cfg)
	cfg.ApplyDefaults()

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

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
		defer signal.Stop(sigChan)
		select {
		case <-sigChan:
			cancel()
		case <-ctx.Done():
		}
	}()

	return ctx, cancel
}
