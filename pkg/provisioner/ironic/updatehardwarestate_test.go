package ironic

import (
	"net/http"
	"testing"

	"github.com/gophercloud/gophercloud/openstack/baremetal/v1/nodes"
	"github.com/stretchr/testify/assert"

	"github.com/metal3-io/baremetal-operator/pkg/bmc"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner/ironic/clients"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner/ironic/testserver"
)

func TestUpdateHardwareState(t *testing.T) {

	nodeUUID := "33ce8659-7400-4c68-9535-d10766f07a58"

	cases := []struct {
		name                 string
		ironic               *testserver.IronicMock
		inspector            *testserver.InspectorMock
		hostCurrentlyPowered bool
		hostName             string

		expectUnreadablePower bool

		expectedPublish string
		expectedError   string
	}{
		{
			name: "unkown-power-state",
			ironic: testserver.NewIronic(t).Ready().Node(nodes.Node{
				UUID: nodeUUID,
			}),
			expectUnreadablePower: true,
		},
		{
			name: "updated-power-on-state",
			ironic: testserver.NewIronic(t).Ready().Node(nodes.Node{
				UUID:       nodeUUID,
				PowerState: "power on",
			}),
			hostCurrentlyPowered: true,
		},
		{
			name: "not-updated-power-on-state",
			ironic: testserver.NewIronic(t).Ready().Node(nodes.Node{
				UUID:       nodeUUID,
				PowerState: "power on",
			}),
			hostCurrentlyPowered: false,
		},
		{
			name: "updated-power-off-state",
			ironic: testserver.NewIronic(t).Ready().Node(nodes.Node{
				UUID:       nodeUUID,
				PowerState: "power off",
			}),
			hostCurrentlyPowered: false,
		},
		{
			name: "not-updated-power-off-state",
			ironic: testserver.NewIronic(t).Ready().Node(nodes.Node{
				UUID:       nodeUUID,
				PowerState: "power off",
			}),
			hostCurrentlyPowered: true,
		},
		{
			name: "no-power",
			ironic: testserver.NewIronic(t).Ready().Node(nodes.Node{
				UUID:       nodeUUID,
				PowerState: "None",
			}),
			hostCurrentlyPowered:  true,
			expectUnreadablePower: true,
		},
		{
			name: "node-not-found",

			hostName: "worker-0",
			ironic:   testserver.NewIronic(t).Ready().NodeError(nodeUUID, http.StatusGatewayTimeout),

			expectedError: "failed to find existing host: failed to find node by ID 33ce8659-7400-4c68-9535-d10766f07a58: Expected HTTP response code \\[200\\].*",

			expectUnreadablePower: true,
		},
		{
			name: "node-not-found-by-name",

			hostName: "worker-0",
			ironic:   testserver.NewIronic(t).Ready().NoNode(nodeUUID).NodeError("myhost", http.StatusGatewayTimeout),

			expectedError: "failed to find existing host: failed to find node by name worker-0: EOF",

			expectUnreadablePower: true,
		},
		{
			name:   "not-ironic-node",
			ironic: testserver.NewIronic(t).Ready().NoNode(nodeUUID).NoNode("myhost"),

			expectedError: "Host not registered",

			expectUnreadablePower: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.ironic != nil {
				tc.ironic.Start()
				defer tc.ironic.Stop()
			}

			if tc.inspector != nil {
				tc.inspector.Start()
				defer tc.inspector.Stop()
			}

			host := makeHost()
			host.Status.Provisioning.ID = nodeUUID
			host.Status.PoweredOn = tc.hostCurrentlyPowered
			if tc.hostName != "" {
				host.Name = tc.hostName
			}

			publishedMsg := ""
			publisher := func(reason, message string) {
				publishedMsg = reason + " " + message
			}
			auth := clients.AuthConfig{Type: clients.NoAuth}
			prov, err := newProvisionerWithSettings(host, bmc.Credentials{}, publisher,
				tc.ironic.Endpoint(), auth, tc.inspector.Endpoint(), auth,
			)
			if err != nil {
				t.Fatalf("could not create provisioner: %s", err)
			}

			hwStatus, err := prov.UpdateHardwareState()

			assert.Equal(t, tc.expectUnreadablePower, hwStatus.PoweredOn == nil)

			assert.Equal(t, tc.expectedPublish, publishedMsg)
			if tc.expectedError == "" {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
				assert.Regexp(t, tc.expectedError, err.Error())
			}

		})
	}
}
