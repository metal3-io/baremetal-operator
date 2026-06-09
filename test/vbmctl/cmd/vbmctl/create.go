//go:build vbmctl
// +build vbmctl

// Provides the implementation of the "vbmctl create" command.
package main

import (
	"errors"
	"fmt"
	"strconv"

	vbmctlapi "github.com/metal3-io/baremetal-operator/test/vbmctl/pkg/api"
	"github.com/metal3-io/baremetal-operator/test/vbmctl/pkg/config"
	containers "github.com/metal3-io/baremetal-operator/test/vbmctl/pkg/containers"
	"github.com/metal3-io/baremetal-operator/test/vbmctl/pkg/libvirt"
	"github.com/metal3-io/baremetal-operator/test/vbmctl/pkg/network"
	"github.com/spf13/cobra"
	libvirtgo "libvirt.org/go/libvirt"
)

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
	cmd.AddCommand(newCreateBMCEmulatorCmd())

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
        bridge: "metal3"
    imageServer:
      image: "nginxinc/nginx-unprivileged"
      port: 8080
      containerPort: 8080
      dataDir: "/var/lib/vbmctl/images"
      containerDataDir: "/usr/share/nginx/html"
      containerName: "vbmctl-image-server"
    bmcEmulator:
      type: "sushy-tools"
      configFile: "vbmc-emulator-file"
      image: "bmc-emulator:latest"
    vethPairs:
      - link1: "metal3"
        link2: "kind-bridge"
        veth1: "metalend"
        veth2: "kindend"
    dockerNetworks:
      - name: "kind"
        bridgeName: "kind-bridge"
        subnet: "fc00:f853:ccd:e793::/64"
        driverMtu: 1500
        ipv6: true`,
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

			if cfg.Spec.BMCEmulator != nil {
				err = containers.CreateBMCEmulatorInstance(ctx, cfg.Spec.BMCEmulator)
				if err != nil {
					return err
				}
			} else {
				//nolint:forbidigo // CLI output is intentional
				fmt.Println("No BMC emulator configuration found in the config file.")
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
			fmt.Println("Created networks:")
			for _, network := range networks {
				//nolint:forbidigo // CLI output is intentional
				fmt.Printf("  - %s (UUID: %s)\n", network.Name, network.UUID)
			}

			networkIDs, err := containers.CreateBridgeNetworks(ctx, cfg.Spec.DockerNetworks)
			if err != nil {
				return err
			}
			//nolint:forbidigo // CLI output is intentional
			fmt.Println("Created Docker networks:")
			for _, id := range networkIDs {
				//nolint:forbidigo // CLI output is intentional
				fmt.Printf("  - ID: %s\n", id)
			}

			// Connect the specified networks
			err = network.ConnectAllWithVeth(ctx, cfg.Spec.VethPairs)
			if err != nil {
				return fmt.Errorf("failed to create veth pairs: %w", err)
			}
			//nolint:forbidigo // CLI output is intentional
			fmt.Println("Created veth pairs:")
			for _, pair := range cfg.Spec.VethPairs {
				//nolint:forbidigo // CLI output is intentional
				fmt.Printf("  - between %s and %s\n", pair.Link1, pair.Link2)
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

func newCreateBMCEmulatorCmd() *cobra.Command {
	var (
		emulatorType  string
		image         string
		configFile    string
		listenAddress string
		listenPort    uint16
	)

	cmd := &cobra.Command{
		Use:   "bmc-emulator",
		Short: "Create a BMC emulator instance",
		Long:  "Create a BMC emulator instance to be used for testing.",
		RunE: func(_ *cobra.Command, _ []string) error {
			ctx, cancel := contextWithSignal()
			defer cancel()

			cfg, err := loadConfig()
			if err != nil {
				return err
			}

			// Resolve effective BMC emulator config: config file values as base,
			// command-line flags take precedence over both.
			effective := &vbmctlapi.BMCEmulatorConfig{}
			if cfg.Spec.BMCEmulator != nil {
				effective = cfg.Spec.BMCEmulator
			}

			// Resolve type: flag > config > default.
			if emulatorType == "" {
				if effective.Type != "" {
					emulatorType = effective.Type
				} else {
					emulatorType = config.DefaultBMCEmulatorType
				}
			}

			// Resolve image: flag > config > correct default for the resolved type.
			if image == "" {
				if effective.Image != "" {
					image = effective.Image
				} else if emulatorType == config.BMCEmulatorTypeSushyTools {
					image = config.DefaultBMCEmulatorSushyToolsImage
				} else {
					image = config.DefaultBMCEmulatorVBMCImage
				}
			}

			if emulatorType == config.BMCEmulatorTypeSushyTools {
				if configFile == "" {
					// There is no default config file for sushy-tools, so no
					// need to check if the value from config file is empty
					// before using it.
					configFile = effective.ConfigFile
				}

				// If using command line arguments to configure sushy-tools, apply the
				// defaults for listen address and listen port if they are not
				// explicitly set. In case a sushy-tools config file is specified,
				// defaults are not applied.
				if listenAddress == "" {
					if effective.ListenAddress != "" {
						listenAddress = effective.ListenAddress
					} else if configFile == "" {
						listenAddress = config.DefaultNetworkAddress
					}
				}
				if listenPort == 0 {
					if effective.ListenPort != 0 {
						listenPort = effective.ListenPort
					} else if configFile == "" {
						listenPort = config.DefaultBMCEmulatorSushyToolsListenPort
					}
				}

				//nolint:forbidigo // CLI output is intentional
				fmt.Printf("Using storage pool '%s' and libvirt URI '%s' for sushy-tools BMC emulator\n", cfg.Spec.Pool.Name, cfg.Spec.Libvirt.URI)
			}

			// Note that storage pool and libvirt URI are only relevant for sushy-tools,
			// but we set them for vbmc as well since they don't cause any issues and
			// it simplifies the logic by not having to conditionally set them based
			// on the emulator type.
			return containers.CreateBMCEmulatorInstance(ctx, &vbmctlapi.BMCEmulatorConfig{
				Type:          emulatorType,
				Image:         image,
				ConfigFile:    configFile,
				StoragePool:   cfg.Spec.Pool.Name,
				ListenAddress: listenAddress,
				ListenPort:    listenPort,
				LibvirtURI:    cfg.Spec.Libvirt.URI,
			})
		},
	}

	cmd.Flags().StringVar(&emulatorType, "emulator-type", "", "type of the BMC emulator (vbmc or sushy-tools, default is "+config.DefaultBMCEmulatorType+" if not set in config file)")
	cmd.Flags().StringVar(&image, "image", "", "container image to use for the BMC emulator (default is "+config.DefaultBMCEmulatorVBMCImage+" or "+config.DefaultBMCEmulatorSushyToolsImage+" if not set in config file)")
	cmd.Flags().StringVar(&configFile, "config-file", "", "configuration file to use for the BMC emulator in case of sushy-tools (default is none)")
	cmd.Flags().StringVar(&listenAddress, "listen-address", "", "address for the BMC emulator to listen on for incoming connections (only applicable for sushy-tools, default is "+config.DefaultNetworkAddress+" if not set in config file and no config file is used)")
	cmd.Flags().Uint16Var(&listenPort, "listen-port", 0, "port for the BMC emulator to listen on for incoming connections (only applicable for sushy-tools, default is "+strconv.Itoa(int(config.DefaultBMCEmulatorSushyToolsListenPort))+" if not set in config file and no config file is used)")

	return cmd
}
