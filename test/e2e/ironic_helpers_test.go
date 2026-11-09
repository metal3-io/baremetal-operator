//go:build e2e
// +build e2e

package e2e

import (
	"context"
	"fmt"
	"strings"

	"github.com/gophercloud/gophercloud/v2/openstack/baremetal/v1/ports"
)

// fetchIronicPorts queries the Ironic API using ListDetail and returns ports
// for the given node.  ListDetail is needed (vs List) to get Extra and
// LocalLinkConnection fields.
func fetchIronicPorts(e2eConfig *Config, namespace, bmhName string) ([]ports.Port, error) {
	client := CreateIronicClient(e2eConfig)
	nodeName := fmt.Sprintf("%s~%s", namespace, bmhName)

	opts := ports.ListOpts{
		Node: nodeName,
	}

	allPages, err := ports.ListDetail(client, opts).AllPages(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to list detail ports for node %s: %w", nodeName, err)
	}

	return ports.ExtractPorts(allPages)
}

// findPortByMAC returns the first Ironic port matching the given MAC address (case-insensitive).
func findPortByMAC(portList []ports.Port, mac string) *ports.Port {
	for i := range portList {
		if strings.EqualFold(portList[i].Address, mac) {
			return &portList[i]
		}
	}
	return nil
}
