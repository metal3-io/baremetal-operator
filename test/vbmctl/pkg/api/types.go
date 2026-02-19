// Package api defines the core types used by vbmctl for managing virtual
// bare metal environments.
package api

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
	Networks []NetworkAttachment `json:"networks,omitempty" yaml:"networks,omitempty"`
}

// VolumeConfig represents the configuration for a storage volume.
type VolumeConfig struct {
	// Name is the name of the volume.
	Name string `json:"name" yaml:"name"`

	// Size is the size of the volume in GB.
	Size int `json:"size" yaml:"size"`
}

// NetworkAttachment represents a network interface attached to a VM.
type NetworkAttachment struct {
	// Network is the name of the libvirt network to attach to.
	Network string `json:"network" yaml:"network"`

	// MACAddress is the MAC address for this interface.
	// If not specified, one will be generated.
	MACAddress string `json:"macAddress,omitempty" yaml:"macAddress,omitempty"`

	// IPAddress is an optional static IP address to reserve via DHCP.
	IPAddress string `json:"ipAddress,omitempty" yaml:"ipAddress,omitempty"`
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
