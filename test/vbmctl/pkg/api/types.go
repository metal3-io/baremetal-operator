// Package api defines the core types used by vbmctl for managing virtual
// bare metal environments.
package vbmctlapi

import "k8s.io/utils/ptr"

// VMConfig represents the configuration for a virtual machine.
type VMConfig struct {
	// Name is the name of the virtual machine.
	Name string `json:"name" yaml:"name"`

	// Memory is the amount of memory in MB.
	Memory int `json:"memory" yaml:"memory"`

	// VCPUs is the number of virtual CPUs.
	VCPUs int `json:"vcpus" yaml:"vcpus"`

	// Volumes is a list of volumes to attach to the VM.
	Volumes []VolumeConfig `json:"volumes,omitempty" yaml:"volumes,omitempty"`

	// Networks is a list of networks to attach to the VM.
	Networks []NetworkAttachment `json:"networkAttachments,omitempty" yaml:"networkAttachments,omitempty"`
}

// VolumeConfig represents the configuration for a storage volume.
type VolumeConfig struct {
	// Name is the name of the volume.
	Name string `json:"name" yaml:"name"`

	// Size is the size of the volume in GB.
	Size int `json:"size" yaml:"size"`
}

// ImageServerConfig represents the configuration for the image server.
type ImageServerConfig struct {
	// Port is the host port to bind the image server to.
	Port uint16 `json:"port" yaml:"port"`

	// ContainerPort is the container port that the image server listens on.
	ContainerPort uint16 `json:"containerPort" yaml:"containerPort"`

	// DataDir is the host directory to mount as a volume for the image server.
	DataDir string `json:"dataDir" yaml:"dataDir"`

	// ContainerDataDir is the directory inside the container to mount the data volume to.
	ContainerDataDir string `json:"containerDataDir" yaml:"containerDataDir"`

	// Image is the container image to use for the image server.
	Image string `json:"image" yaml:"image"`

	// ContainerName is the name of the container to create for the image server.
	ContainerName string `json:"containerName" yaml:"containerName"`
}

// VolumeMount represents a single host-to-container volume binding.
type VolumeMount struct {
	// HostPath is the path on the host to mount.
	HostPath string

	// BindSpec is the container-side bind specification, e.g.
	// "/container/path" or "/container/path:Z".
	BindSpec string
}

// BMCEmulatorConfig represents the configuration for the BMC emulator.
type BMCEmulatorConfig struct {
	// BMC Emulator type
	Type string `json:"type" yaml:"type"`

	// ConfigFile is the path to the sushy-tools config file and is only
	// applicable when Type is "sushy-tools".
	ConfigFile string `json:"configFile" yaml:"configFile"`

	// ListenAddress is the address that the BMC emulator listens on to
	// use for sushy-tools when not using a config file.
	ListenAddress string `json:"listenAddress" yaml:"listenAddress"`

	// ListenPort is the port that the BMC emulator listens on to
	// use for sushy-tools when not using a config file.
	ListenPort uint16 `json:"listenPort" yaml:"listenPort"`

	// StoragePool is the name of the libvirt storage pool to use for
	// sushy-tools.
	StoragePool string `json:"storagePool" yaml:"storagePool"`

	// LibvirtURI is the libvirt URI that sushy-tools should connect to.
	LibvirtURI string `json:"libvirtURI" yaml:"libvirtUri"`

	// Image is the container image to use for the BMC emulator.
	Image string `json:"image" yaml:"image"`

	// Cmd is an internal runtime command for the BMC emulator container.
	Cmd []string `json:"-" yaml:"-"`

	// Env contains internal runtime environment variables for the emulator container.
	Env map[string]string `json:"-" yaml:"-"`

	// ContainerName is an internal runtime container name for the BMC emulator.
	ContainerName string `json:"-" yaml:"-"`

	// VolumeMounts is an internal runtime list of host-to-container volume bindings.
	VolumeMounts []VolumeMount `json:"-" yaml:"-"`
}

// NetworkAttachment represents a network interface attached to a VM.
type NetworkAttachment struct {
	// Network is the name of the libvirt network to attach to.
	Network string `json:"network" yaml:"network"`

	// MACAddress is the MAC address for this interface.
	// This field is required when a network is specified.
	MACAddress string `json:"macAddress,omitempty" yaml:"macAddress,omitempty"`

	// IPAddress is an optional static IP address to reserve via DHCP.
	IPAddress string `json:"ipAddress,omitempty" yaml:"ipAddress,omitempty"`
}

// Network represents libvirt network.
type NetworkConfig struct {
	// Name is the name of the libvirt network.
	Name string `json:"name" yaml:"name"`

	// Bridge is the name of the bridge interface for the network.
	Bridge string `json:"bridge,omitempty" yaml:"bridge,omitempty"`

	// Address is the address of the bridge interface. Address is expected to be IPv4.
	Address string `json:"address,omitempty" yaml:"address,omitempty"`

	// Netmask is the netmask for the network.
	Netmask string `json:"netmask,omitempty" yaml:"netmask,omitempty"`
}

type VethPair struct {
	// Link 1 is the name of the first network interface to be connected
	Link1 string `json:"link1" yaml:"link1"`

	// Link 2 is the name of the second network interface to be connected
	Link2 string `json:"link2" yaml:"link2"`

	// Veth1 is the name of the veth interface to be pushed under Link1
	Veth1 string `json:"veth1" yaml:"veth1"`

	// Veth2 is the name of the veth interface to be pushed under Link2
	Veth2 string `json:"veth2" yaml:"veth2"`
}

// PoolConfig represents the configuration for a storage pool.
type PoolConfig struct {
	// Name is the name of the storage pool.
	Name string `json:"name" yaml:"name"`

	// Path is the filesystem path for the pool.
	Path string `json:"path" yaml:"path"`
}

// VM represents a virtual machine with its current state.
type VM struct {
	// Config is the configuration used to create the VM.
	Config VMConfig `json:"config" yaml:"config"`

	// UUID is the unique identifier assigned by libvirt.
	UUID string `json:"uuid,omitempty" yaml:"uuid,omitempty"`

	// State is the current state of the VM.
	State VMState `json:"state,omitempty" yaml:"state,omitempty"`
}

// VMState represents the state of a virtual machine.
// We are using our own type here instead of libvirt-go's constants
// to avoid a direct dependency on libvirt-go in this package.
// Otherwise, we would need libvirt-dev installed to build the package.
type VMState string

const (
	// VMStateRunning indicates the VM is running.
	VMStateRunning VMState = "running"
	// VMStateStopped indicates the VM is stopped.
	VMStateStopped VMState = "stopped"
	// VMStatePaused indicates the VM is paused.
	VMStatePaused VMState = "paused"
	// VMStateUnknown indicates the VM state is unknown.
	VMStateUnknown VMState = "unknown"
)

// Volume represents a storage volume with its current state.
type Volume struct {
	// Config is the configuration used to create the volume.
	Config VolumeConfig `json:"config" yaml:"config"`

	// Path is the filesystem path to the volume.
	Path string `json:"path,omitempty" yaml:"path,omitempty"`

	// Capacity is the actual capacity in bytes.
	Capacity uint64 `json:"capacity,omitempty" yaml:"capacity,omitempty"`

	// Allocation is the current allocation in bytes.
	Allocation uint64 `json:"allocation,omitempty" yaml:"allocation,omitempty"`
}

// Pool represents a storage pool with its current state.
type Pool struct {
	// Config is the configuration used to create the pool.
	Config PoolConfig `json:"config" yaml:"config"`

	// UUID is the unique identifier assigned by libvirt.
	UUID string `json:"uuid,omitempty" yaml:"uuid,omitempty"`

	// Active indicates whether the pool is currently active.
	Active bool `json:"active,omitempty" yaml:"active,omitempty"`

	// Capacity is the total capacity in bytes.
	Capacity uint64 `json:"capacity,omitempty" yaml:"capacity,omitempty"`

	// Available is the available space in bytes.
	Available uint64 `json:"available,omitempty" yaml:"available,omitempty"`
}

type DockerBridgeNetwork struct {
	// Name is the name of the docker network
	Name string `json:"name" yaml:"name"`

	// BridgeName is the name for the network bridge that is to be created
	BridgeName string `json:"bridgeName" yaml:"bridgeName"`

	// IPv4 is a boolean for enabling IPv4, true by default
	IPv4 *bool `json:"ipv4,omitempty" yaml:"ipv4,omitempty"`

	// IPv6 is a boolean for enabling IPv6, false by default
	IPv6 *bool `json:"ipv6,omitempty" yaml:"ipv6,omitempty"`

	// Subnet is the subnet to be used
	Subnet string `json:"subnet" yaml:"subnet"`

	// DriverMtu is the maximum transmission unit for the network
	DriverMtu uint64 `json:"driverMtu,omitempty" yaml:"driverMtu,omitempty"`
}

// Defaults returns a copy of VMConfig with default values applied.
func (c VMConfig) Defaults() VMConfig {
	cfg := c
	if cfg.Memory == 0 {
		cfg.Memory = 4096 // 4GB default
	}
	if cfg.VCPUs == 0 {
		cfg.VCPUs = 2 // 2 vCPUs default
	}
	return cfg
}

// Defaults returns a copy of VolumeConfig with default values applied.
func (c VolumeConfig) Defaults() VolumeConfig {
	cfg := c
	if cfg.Size == 0 {
		cfg.Size = 20 // 20GB default
	}
	return cfg
}

// Defaults returns a copy of NetworkConfig with default values applied.
func (c NetworkConfig) Defaults() NetworkConfig {
	cfg := c
	if cfg.Bridge == "" {
		cfg.Bridge = "metal3"
	}
	if cfg.Address == "" {
		cfg.Address = "192.168.222.1"
	}
	if cfg.Netmask == "" {
		cfg.Netmask = "255.255.255.0"
	}
	return cfg
}

func (c DockerBridgeNetwork) Defaults() DockerBridgeNetwork {
	cfg := c
	if cfg.IPv4 == nil {
		cfg.IPv4 = ptr.To(true)
	}
	if cfg.IPv6 == nil {
		cfg.IPv6 = ptr.To(false)
	}
	return cfg
}
