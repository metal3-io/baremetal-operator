package ironic

import (
	"strings"
	"testing"

	"github.com/gophercloud/gophercloud/v2/openstack/baremetal/v1/nodes"
	"github.com/metal3-io/baremetal-operator/pkg/hardwareutils/bmc"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner/ironic/clients"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner/ironic/testserver"
)

func TestFindExistingHost(t *testing.T) {
	cases := []struct {
		name   string
		ironic *testserver.IronicMock

		hostName       string
		provisioningID string
		nodeName       string
	}{
		{
			name:           "no-node",
			hostName:       "name",
			provisioningID: "uuid",
			ironic:         testserver.NewIronic(t).NoNode("myns" + nameSeparator + "name").NoNode("name").NoNode("uuid"),
		},
		{
			name:           "by-name",
			hostName:       "name",
			provisioningID: "uuid",
			ironic: testserver.NewIronic(t).NoNode("uuid").
				Node(nodes.Node{
					Name: "myns" + nameSeparator + "name",
					UUID: "different-uuid",
				}),
			nodeName: "myns" + nameSeparator + "name",
		},
		{
			name:           "by-uuid",
			hostName:       "name",
			provisioningID: "uuid",
			ironic: testserver.NewIronic(t).NoNode("myns" + nameSeparator + "name").NoNode("name").
				Node(nodes.Node{
					Name: "myns" + nameSeparator + "different-name",
					UUID: "uuid",
				}),
			nodeName: "myns" + nameSeparator + "different-name",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.ironic != nil {
				tc.ironic.Start()
				defer tc.ironic.Stop()
			}

			auth := clients.AuthConfig{Type: clients.NoAuth}

			// Update the default host to match the test settings
			host := makeHost()
			host.ObjectMeta.Name = tc.hostName
			host.Status.Provisioning.ID = tc.provisioningID

			prov, err := newProvisionerWithSettings(host, bmc.Credentials{}, nil, tc.ironic.Endpoint(), auth)
			if err != nil {
				t.Fatalf("could not create provisioner: %s", err)
			}

			node, err := prov.findExistingHost(t.Context(), "")
			t.Logf("requests: %s", tc.ironic.Requests)
			if err != nil {
				t.Fatalf("could not look up host: %s", err)
			}

			if tc.nodeName == "" && node != nil {
				t.Fatalf("found unexpected node %s (%s)", node.Name, node.UUID)
			}
		})
	}
}

func TestFindExistingHostEmptyMAC(t *testing.T) {
	// Test that when bootMACAddress is empty, MAC-based port lookup is skipped
	// and no false MAC conflicts are reported. This is important for pre-provisioned
	// hardware workflows where the boot MAC will be populated by external controllers.

	cases := []struct {
		name           string
		ironic         *testserver.IronicMock
		hostName       string
		provisioningID string
		bootMAC        string
		expectNode     bool
		expectError    bool
	}{
		{
			name:           "empty-mac-no-node",
			hostName:       "name",
			provisioningID: "uuid",
			bootMAC:        "",
			ironic:         testserver.NewIronic(t).NoNode("myns" + nameSeparator + "name").NoNode("name").NoNode("uuid"),
			expectNode:     false,
			expectError:    false,
		},
		{
			name:           "empty-mac-node-exists-by-name",
			hostName:       "name",
			provisioningID: "uuid",
			bootMAC:        "",
			ironic: testserver.NewIronic(t).NoNode("uuid").
				Node(nodes.Node{
					Name: "myns" + nameSeparator + "name",
					UUID: "different-uuid",
				}),
			expectNode:  true,
			expectError: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.ironic != nil {
				tc.ironic.Start()
				defer tc.ironic.Stop()
			}

			auth := clients.AuthConfig{Type: clients.NoAuth}

			// Create host with empty bootMACAddress
			host := makeHost()
			host.ObjectMeta.Name = tc.hostName
			host.Status.Provisioning.ID = tc.provisioningID
			host.Spec.BootMACAddress = tc.bootMAC

			prov, err := newProvisionerWithSettings(host, bmc.Credentials{}, nil, tc.ironic.Endpoint(), auth)
			if err != nil {
				t.Fatalf("could not create provisioner: %s", err)
			}

			node, err := prov.findExistingHost(t.Context(), tc.bootMAC)

			// Verify no port queries were made when MAC is empty
			if tc.bootMAC == "" {
				if strings.Contains(tc.ironic.Requests, "/v1/ports") {
					t.Errorf("unexpected port query when bootMACAddress is empty, requests: %s", tc.ironic.Requests)
				}
			}

			if tc.expectError && err == nil {
				t.Fatalf("expected error but got none")
			}
			if !tc.expectError && err != nil {
				t.Fatalf("unexpected error: %s", err)
			}

			if tc.expectNode && node == nil {
				t.Fatalf("expected node but got nil")
			}
			if !tc.expectNode && node != nil {
				t.Fatalf("expected no node but got %s (%s)", node.Name, node.UUID)
			}
		})
	}
}
