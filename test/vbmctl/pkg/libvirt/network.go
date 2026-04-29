//go:build vbmctl
// +build vbmctl

package libvirt

import (
	"context"
	"errors"
	"fmt"
	"log"

	vbmctlapi "github.com/metal3-io/baremetal-operator/test/vbmctl/pkg/api"
	"libvirt.org/go/libvirt"
)

type NetworkManager struct {
	conn     *libvirt.Connect
	renderer *TemplateRenderer
}

type Network struct {
	Name   string
	Bridge string
	UUID   string
}

func NewNetworkManager(conn *libvirt.Connect) (*NetworkManager, error) {
	renderer, err := NewTemplateRenderer()
	if err != nil {
		return nil, fmt.Errorf("failed to create template renderer: %w", err)
	}
	return &NetworkManager{
		conn:     conn,
		renderer: renderer,
	}, nil
}

func (m *NetworkManager) CreateNetwork(_ context.Context, cfg vbmctlapi.NetworkConfig) (*Network, error) {
	// Render network XML
	networkXML, err := m.renderer.RenderNetwork(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to render VM template: %w", err)
	}

	// Check if network exists, define a new if it does not
	network, err := m.conn.LookupNetworkByName(cfg.Name)
	if err == nil {
		log.Printf("network %s already exists, continuing with existing network", cfg.Name)
	} else {
		network, err = m.conn.NetworkDefineXML(networkXML)
		if err != nil {
			return nil, fmt.Errorf("failed to define network from XML: %w", err)
		}
		// Start the network
		err = network.Create()
		if err != nil {
			return nil, fmt.Errorf("failed to start defined network %s: %w", cfg.Name, err)
		}
		err = network.SetAutostart(true)
		if err != nil {
			return nil, fmt.Errorf("failed to set autostart for network %s: %w", cfg.Name, err)
		}
	}

	// Get network information
	name, err := network.GetName()
	if err != nil {
		return nil, fmt.Errorf("no name defined for network: %w", err)
	}
	bridge, err := network.GetBridgeName()
	if err != nil {
		return nil, fmt.Errorf("no bridge name defined for network: %w", err)
	}
	uuid, err := network.GetUUIDString()
	if err != nil {
		return nil, fmt.Errorf("could not get UUID for network: %w", err)
	}
	if err := network.Free(); err != nil {
		return nil, fmt.Errorf("failed to free network object %s after creation: %w", name, err)
	}

	return &Network{
		Name:   name,
		Bridge: bridge,
		UUID:   uuid,
	}, nil
}

func (m *NetworkManager) CreateNetworks(ctx context.Context, configs []vbmctlapi.NetworkConfig) ([]*Network, error) {
	networks := make([]*Network, 0, len(configs))
	for _, cfg := range configs {
		network, err := m.CreateNetwork(ctx, cfg)
		if err != nil {
			// Clean up previously created networks
			log.Printf("Failed to create network %s, cleaning up %d previously created network(s)\n", cfg.Name, len(networks))
			for _, created := range networks {
				if delErr := m.DeleteNetwork(ctx, created.Name); delErr != nil {
					log.Printf("Warning: failed to clean up network %s: %v\n", created.Name, delErr)
				}
			}
			return nil, fmt.Errorf("failed to create network %s: %w", network.Name, err)
		}
		networks = append(networks, network)
	}
	return networks, nil
}

func (m *NetworkManager) DeleteNetwork(_ context.Context, name string) error {
	network, err := m.conn.LookupNetworkByName(name)
	if errors.Is(err, libvirt.ERR_NO_NETWORK) {
		log.Printf("Cannot delete libvirt network %s, does not exist.\n", name)
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to lookup network %s: %w", name, err)
	}
	if err := network.Destroy(); err != nil {
		return fmt.Errorf("failed to destroy network %s: %w", name, err)
	}
	if err := network.Undefine(); err != nil {
		return fmt.Errorf("failed to undefine network %s (was it transient?): %w", name, err)
	}
	if err := network.Free(); err != nil {
		return fmt.Errorf("failed to free network object %s after deletion: %w", name, err)
	}
	return nil
}

func (m *NetworkManager) DeleteNetworks(ctx context.Context, names []string) error {
	var lastErr error
	for _, name := range names {
		if err := m.DeleteNetwork(ctx, name); err != nil {
			log.Printf("Error deleting network %s: %v\n", name, err)
			lastErr = err
		}
	}
	return lastErr
}
