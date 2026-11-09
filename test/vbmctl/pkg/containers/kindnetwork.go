//go:build vbmctl
// +build vbmctl

package containers

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/netip"
	"strconv"

	vbmctlapi "github.com/metal3-io/baremetal-operator/test/vbmctl/pkg/api"
	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/client"
)

// CreateBridgeNetworks takes the network definitions and creates Docker bridge
// networks with the given configuration. No error is returned if the networks
// exist. If an error is encountered, all the created networks are removed.
func CreateBridgeNetworks(ctx context.Context, networks []vbmctlapi.DockerBridgeNetwork) ([]string, error) {
	createdNetworks := make([]string, 0, len(networks))
	for _, cfg := range networks {
		net := cfg.Defaults()
		subnetPrefix, err := netip.ParsePrefix(net.Subnet)
		if err != nil {
			return nil, fmt.Errorf("failed to parse subnet for Docker network %s: %w", net.Name, err)
		}
		networkOpts := client.NetworkCreateOptions{
			Driver:     "bridge",
			EnableIPv4: net.IPv4,
			EnableIPv6: net.IPv6,
			Options: map[string]string{
				"com.docker.network.driver.mtu":  strconv.FormatUint(net.DriverMtu, 10),
				"com.docker.network.bridge.name": net.BridgeName,
			},
			IPAM: &network.IPAM{
				Config: []network.IPAMConfig{
					{
						Subnet: subnetPrefix,
					},
				},
			},
		}
		createdID, err := CreateNetwork(ctx, net.Name, &networkOpts)
		// Don't fail if the network exists
		if errors.Is(err, ErrNetworkExists) {
			log.Printf("Warning: Docker network %s already exists, continuing.", net.Name)
		} else if err != nil {
			// Clean up previously created networks
			log.Printf("Failed to create network %s, cleaning up %d previously created Docker networks", net.Name, len(createdNetworks))
			for _, netID := range createdNetworks {
				delErr := DeleteNetwork(ctx, netID, &client.NetworkRemoveOptions{})
				if delErr != nil {
					log.Printf("Warning: failed to clean up Docker network: %v", delErr)
				}
			}
			return nil, fmt.Errorf("failed to create network: %w", err)
		} else {
			// Add network to the created list only if it really was created.
			// In case of cleanup procedure, this avoids cleaning up existing
			// networks.
			createdNetworks = append(createdNetworks, createdID)
		}
	}
	return createdNetworks, nil
}

// DeleteBridgeNetworks deletes the networks with given configurations.
// The networks are deleted by their name, so other fields in the networks
// are ignored. It keeps going when a network cannot be deleted, prints a
// warning for it and returns all such errors at the end.
func DeleteBridgeNetworks(ctx context.Context, networks []vbmctlapi.DockerBridgeNetwork) error {
	log.Printf("Deleting Docker networks (%d networks)...", len(networks))
	var errs []error
	for _, net := range networks {
		if err := deleteBridgeNetwork(ctx, net.Name); err != nil {
			log.Printf("Warning: %v", err)
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}

// deleteBridgeNetwork deletes the Docker network with the given name. A
// network that does not exist is skipped without an error.
func deleteBridgeNetwork(ctx context.Context, name string) error {
	networkID, err := GetNetworkByName(ctx, name)
	if errors.Is(err, ErrNetworkNotFound) {
		log.Printf("Warning: Docker network %s not found, skipping", name)
		return nil
	} else if err != nil {
		return fmt.Errorf("failed to get docker network %s: %w", name, err)
	}

	err = DeleteNetwork(ctx, networkID, &client.NetworkRemoveOptions{})
	if err != nil {
		return fmt.Errorf("failed to remove docker network %s: %w", name, err)
	}
	log.Printf("Deleted Docker network %s", name)

	return nil
}
