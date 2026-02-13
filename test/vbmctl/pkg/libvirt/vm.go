//go:build vbmctl
// +build vbmctl

package libvirt

import (
	"context"
	"errors"
	"fmt"
	"log"

	"github.com/metal3-io/baremetal-operator/test/vbmctl/pkg/api"
	"libvirt.org/go/libvirt"
)

const (
	// kiBPerMB is the number of KiB in a MB.
	kiBPerMB = 1024
)

// VMManager manages virtual machines through libvirt.
type VMManager struct {
	conn     *libvirt.Connect
	renderer *TemplateRenderer
	pool     *PoolManager
	opts     VMManagerOptions
}

// VMManagerOptions contains options for VMManager.
type VMManagerOptions struct {
	// PoolName is the name of the storage pool to use.
	PoolName string

	// PoolPath is the filesystem path for the storage pool.
	PoolPath string
}

// DefaultVMManagerOptions returns VMManagerOptions with sensible defaults.
func DefaultVMManagerOptions() VMManagerOptions {
	return VMManagerOptions{
		PoolName: "baremetal-e2e",
		PoolPath: "/tmp/pool_oo",
	}
}

// NewVMManager creates a new VM manager.
func NewVMManager(conn *libvirt.Connect, opts VMManagerOptions) (*VMManager, error) {
	renderer, err := NewTemplateRenderer()
	if err != nil {
		return nil, fmt.Errorf("failed to create template renderer: %w", err)
	}

	poolManager := NewPoolManager(conn)

	return &VMManager{
		conn:     conn,
		renderer: renderer,
		pool:     poolManager,
		opts:     opts,
	}, nil
}

// Create creates a new virtual machine from the given configuration.
// The VM is defined but not started.
func (m *VMManager) Create(ctx context.Context, cfg api.VMConfig) (*api.VM, error) {
	// Apply defaults
	cfg = cfg.Defaults()

	// Ensure storage pool exists
	if _, err := m.pool.EnsurePool(ctx, api.PoolConfig{
		Name: m.opts.PoolName,
		Path: m.opts.PoolPath,
	}); err != nil {
		return nil, fmt.Errorf("failed to ensure storage pool: %w", err)
	}

	// Create volumes for the VM
	for _, volCfg := range cfg.Volumes {
		volCfg = volCfg.Defaults()

		// Create volume with VM-specific name
		volumeName := fmt.Sprintf("%s-%s", cfg.Name, volCfg.Name)
		if err := m.pool.CreateVolume(ctx, m.opts.PoolName, volumeName, volCfg.Size); err != nil {
			return nil, fmt.Errorf("failed to create volume %s: %w", volumeName, err)
		}
		log.Printf("Created volume %s\n", volumeName)
	}

	// Reserve IP addresses if specified
	if err := m.reserveIPAddresses(cfg); err != nil {
		return nil, fmt.Errorf("failed to reserve IP addresses: %w", err)
	}

	// Render VM XML
	templateData := VMConfigToTemplateData(cfg, m.opts.PoolPath)
	vmXML, err := m.renderer.RenderVM(templateData)
	if err != nil {
		return nil, fmt.Errorf("failed to render VM template: %w", err)
	}

	// Define the domain
	domain, err := m.conn.DomainDefineXML(vmXML)
	if err != nil {
		return nil, fmt.Errorf("failed to define domain: %w", err)
	}
	defer func() { _ = domain.Free() }()

	uuid, err := domain.GetUUIDString()
	if err != nil {
		return nil, fmt.Errorf("failed to get domain UUID: %w", err)
	}

	log.Printf("Created VM %s with UUID %s\n", cfg.Name, uuid)

	return &api.VM{
		Config: cfg,
		UUID:   uuid,
		State:  api.VMStateStopped,
	}, nil
}

// Delete removes a virtual machine and its associated resources.
// If deleteVolumes is true, it also deletes the VM's volumes from the storage pool.
func (m *VMManager) Delete(ctx context.Context, name string, deleteVolumes bool) error {
	domain, err := m.conn.LookupDomainByName(name)
	if err != nil {
		// Check if domain doesn't exist
		var libvirtErr libvirt.Error
		if errors.As(err, &libvirtErr) {
			if libvirtErr.Code == libvirt.ERR_NO_DOMAIN {
				log.Printf("Domain %s does not exist, skipping\n", name)
				// Still try to delete volumes if requested
				if deleteVolumes {
					if volErr := m.deleteVMVolumes(ctx, name); volErr != nil {
						log.Printf("Warning: failed to delete volumes for %s: %v\n", name, volErr)
					}
				}
				return nil
			}
		}
		return fmt.Errorf("failed to lookup domain %s: %w", name, err)
	}
	defer func() { _ = domain.Free() }()

	// Check if running and destroy if so
	state, _, err := domain.GetState()
	if err != nil {
		return fmt.Errorf("failed to get domain state: %w", err)
	}

	if state == libvirt.DOMAIN_RUNNING || state == libvirt.DOMAIN_PAUSED {
		if err := domain.Destroy(); err != nil {
			return fmt.Errorf("failed to destroy domain: %w", err)
		}
		log.Printf("Destroyed running domain %s\n", name)
	}

	// Undefine the domain
	if err := domain.UndefineFlags(libvirt.DOMAIN_UNDEFINE_NVRAM); err != nil {
		// Try without flags if NVRAM flag fails
		if err := domain.Undefine(); err != nil {
			return fmt.Errorf("failed to undefine domain: %w", err)
		}
	}

	log.Printf("Undefined domain %s\n", name)

	// Delete associated volumes if requested
	if deleteVolumes {
		if err := m.deleteVMVolumes(ctx, name); err != nil {
			return fmt.Errorf("failed to delete volumes for %s: %w", name, err)
		}
	}

	return nil
}

// deleteVMVolumes deletes all volumes associated with a VM.
// It looks for volumes with names matching the pattern "{vmName}-*".
func (m *VMManager) deleteVMVolumes(ctx context.Context, vmName string) error {
	volumes, err := m.pool.ListVolumes(ctx, m.opts.PoolName)
	if err != nil {
		log.Printf("Warning: failed to list volumes in pool %s: %v\n", m.opts.PoolName, err)
		return err
	}

	prefix := vmName + "-"
	for _, vol := range volumes {
		if len(vol.Config.Name) > len(prefix) && vol.Config.Name[:len(prefix)] == prefix {
			if err := m.pool.DeleteVolume(ctx, m.opts.PoolName, vol.Config.Name); err != nil {
				log.Printf("Warning: failed to delete volume %s: %v\n", vol.Config.Name, err)
			} else {
				log.Printf("Deleted volume %s\n", vol.Config.Name)
			}
		}
	}

	return nil
}

// CreateAll creates multiple VMs from a list of configurations.
func (m *VMManager) CreateAll(ctx context.Context, configs []api.VMConfig) ([]*api.VM, error) {
	vms := make([]*api.VM, 0, len(configs))

	for _, cfg := range configs {
		vm, err := m.Create(ctx, cfg)
		if err != nil {
			return vms, fmt.Errorf("failed to create VM %s: %w", cfg.Name, err)
		}
		vms = append(vms, vm)
	}

	return vms, nil
}

// DeleteAll deletes multiple VMs by name.
func (m *VMManager) DeleteAll(ctx context.Context, names []string, deleteVolumes bool) error {
	var lastErr error
	for _, name := range names {
		if err := m.Delete(ctx, name, deleteVolumes); err != nil {
			log.Printf("Error deleting VM %s: %v\n", name, err)
			lastErr = err
		}
	}
	return lastErr
}

// List returns all virtual machines.
func (m *VMManager) List(_ context.Context) ([]*api.VM, error) {
	domains, err := m.conn.ListAllDomains(0)
	if err != nil {
		return nil, fmt.Errorf("failed to list domains: %w", err)
	}

	vms := make([]*api.VM, 0, len(domains))
	for _, domain := range domains {
		name, err := domain.GetName()
		if err != nil {
			_ = domain.Free()
			continue
		}

		uuid, _ := domain.GetUUIDString()
		state, _ := m.getDomainState(&domain)
		info, _ := domain.GetInfo()

		// Safe conversion: Memory and NrVirtCpu are always reasonable values
		// from libvirt that fit well within int range
		memoryMB := int(info.Memory / kiBPerMB) //nolint:gosec // memory in KiB from libvirt is always reasonable
		vcpus := int(info.NrVirtCpu)            //nolint:gosec // vCPU count from libvirt is always reasonable

		vms = append(vms, &api.VM{
			Config: api.VMConfig{
				Name:   name,
				Memory: memoryMB,
				VCPUs:  vcpus,
			},
			UUID:  uuid,
			State: state,
		})

		_ = domain.Free()
	}

	return vms, nil
}

// getDomainState converts libvirt domain state to api.VMState.
func (m *VMManager) getDomainState(domain *libvirt.Domain) (api.VMState, error) {
	state, _, err := domain.GetState()
	if err != nil {
		return api.VMStateUnknown, fmt.Errorf("failed to get domain state: %w", err)
	}

	switch state {
	case libvirt.DOMAIN_RUNNING:
		return api.VMStateRunning, nil
	case libvirt.DOMAIN_PAUSED:
		return api.VMStatePaused, nil
	case libvirt.DOMAIN_SHUTDOWN, libvirt.DOMAIN_SHUTOFF:
		return api.VMStateStopped, nil
	default:
		return api.VMStateUnknown, nil
	}
}

// reserveIPAddresses reserves IP addresses in DHCP for the VM's network interfaces.
func (m *VMManager) reserveIPAddresses(cfg api.VMConfig) error {
	for i, net := range cfg.Networks {
		if net.IPAddress == "" {
			continue
		}

		if err := m.reserveIPAddress(cfg.Name, i, net); err != nil {
			return err
		}
	}

	return nil
}

// reserveIPAddress reserves a single IP address in DHCP for a network interface.
func (m *VMManager) reserveIPAddress(vmName string, index int, net api.NetworkAttachment) error {
	network, err := m.conn.LookupNetworkByName(net.Network)
	if err != nil {
		return fmt.Errorf("failed to lookup network %s: %w", net.Network, err)
	}
	defer func() { _ = network.Free() }()

	// Generate host name for DHCP entry
	hostName := fmt.Sprintf("%s-%d", vmName, index)

	dhcpXML, err := m.renderer.RenderDHCPHost(DHCPHostData{
		MACAddress: net.MACAddress,
		Name:       hostName,
		IPAddress:  net.IPAddress,
	})
	if err != nil {
		return fmt.Errorf("failed to render DHCP host entry: %w", err)
	}

	if err := network.Update(
		libvirt.NETWORK_UPDATE_COMMAND_ADD_LAST,
		libvirt.NETWORK_SECTION_IP_DHCP_HOST,
		-1,
		dhcpXML,
		libvirt.NETWORK_UPDATE_AFFECT_LIVE|libvirt.NETWORK_UPDATE_AFFECT_CONFIG,
	); err != nil {
		return fmt.Errorf("failed to add DHCP host entry: %w", err)
	}

	log.Printf("Reserved IP %s for %s on network %s\n", net.IPAddress, vmName, net.Network)
	return nil
}
