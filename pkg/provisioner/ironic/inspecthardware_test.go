package ironic

import (
	"net/http"
	"testing"
	"time"

	"github.com/gophercloud/gophercloud/v2/openstack/baremetal/inventory"
	"github.com/gophercloud/gophercloud/v2/openstack/baremetal/v1/nodes"
	"github.com/stretchr/testify/assert"

	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	"github.com/metal3-io/baremetal-operator/pkg/hardwareutils/bmc"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner/ironic/clients"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner/ironic/testserver"
)

func TestInspectHardware(t *testing.T) {
	nodeUUID := "33ce8659-7400-4c68-9535-d10766f07a58"

	cases := []struct {
		name   string
		ironic *testserver.IronicMock

		restartOnFailure bool
		refresh          bool
		forceReboot      bool

		expectedStarted      bool
		expectedDirty        bool
		expectedRequestAfter int
		expectedResultError  string
		expectedDetailsHost  string

		expectedPublish string
		expectedError   string
	}{
		{
			name: "introspection-status-move-from-available",
			ironic: testserver.NewIronic(t).WithDefaultResponses().Node(nodes.Node{
				UUID:           nodeUUID,
				ProvisionState: "available",
			}),

			expectedStarted:      false,
			expectedDirty:        true,
			expectedRequestAfter: 10,
		},
		{
			name: "introspection-status-start-new-hardware-inspection",
			ironic: testserver.NewIronic(t).WithDefaultResponses().Node(nodes.Node{
				UUID:           nodeUUID,
				ProvisionState: "manageable",
			}).WithInventoryFailed(nodeUUID, http.StatusNotFound),

			expectedStarted:      true,
			expectedDirty:        true,
			expectedRequestAfter: 10,
			expectedPublish:      "InspectionStarted Hardware inspection started",
		},
		{
			name: "introspection-status-refresh-hardware-inspection",
			ironic: testserver.NewIronic(t).WithDefaultResponses().Node(nodes.Node{
				UUID:           nodeUUID,
				ProvisionState: "manageable",
			}).WithInventory(nodeUUID, nodes.InventoryData{
				Inventory: inventory.InventoryType{
					Hostname: "node-0",
				},
			}),

			refresh: true,

			expectedStarted:      true,
			expectedDirty:        true,
			expectedRequestAfter: 10,
			expectedPublish:      "InspectionStarted Hardware inspection started",
		},
		{
			name: "introspection-data-failed",
			ironic: testserver.NewIronic(t).WithDefaultResponses().Node(nodes.Node{
				UUID:           nodeUUID,
				ProvisionState: "manageable",
			}).WithInventoryFailed(nodeUUID, http.StatusBadRequest),

			expectedError: "failed to retrieve hardware introspection data: Bad request with: \\[GET http://127.0.0.1:.*/v1/nodes/33ce8659-7400-4c68-9535-d10766f07a58/inventory\\], error message: An error\\\n",
		},
		{
			name: "introspection-status-retry-on-wait",
			ironic: testserver.NewIronic(t).Node(nodes.Node{
				UUID:           nodeUUID,
				ProvisionState: "inspect wait",
			}),

			expectedDirty:        true,
			expectedRequestAfter: 15,
		},
		{
			name: "introspection-status-retry-on-inspecting",
			ironic: testserver.NewIronic(t).Node(nodes.Node{
				UUID:           nodeUUID,
				ProvisionState: "inspecting",
			}),

			expectedDirty:        true,
			expectedRequestAfter: 15,
		},
		{
			name: "introspection-failed",
			ironic: testserver.NewIronic(t).Node(nodes.Node{
				UUID:           nodeUUID,
				ProvisionState: "inspect failed",
				LastError:      "Timeout",
			}),

			expectedResultError: "Timeout",
		},
		{
			name: "introspection-aborted",
			ironic: testserver.NewIronic(t).Node(nodes.Node{
				UUID:           nodeUUID,
				ProvisionState: "inspect failed",
				LastError:      "Inspection was aborted by request.",
			}).NodeUpdate(nodes.Node{
				UUID:           nodeUUID,
				ProvisionState: string(nodes.InspectFail),
			}).WithNodeStatesProvisionUpdate(nodeUUID),

			expectedStarted:      true,
			expectedDirty:        true,
			expectedRequestAfter: 10,
			expectedPublish:      "InspectionStarted Hardware inspection started",
		},
		{
			name: "inspection-in-progress - forceReboot",
			ironic: testserver.NewIronic(t).Node(nodes.Node{
				UUID:           nodeUUID,
				ProvisionState: string(nodes.InspectWait),
			}).WithNodeStatesProvisionUpdate(nodeUUID),
			forceReboot: true,

			expectedStarted:      true,
			expectedDirty:        true,
			expectedRequestAfter: 10,
		},
		{
			name: "inspection-failed",
			ironic: testserver.NewIronic(t).Node(nodes.Node{
				UUID:           nodeUUID,
				ProvisionState: string(nodes.InspectFail),
			}),

			expectedResultError: "Inspection failed",
		},
		{
			name: "inspection-failed force",
			ironic: testserver.NewIronic(t).Node(nodes.Node{
				UUID:           nodeUUID,
				ProvisionState: string(nodes.InspectFail),
			}).NodeUpdate(nodes.Node{
				UUID:           nodeUUID,
				ProvisionState: string(nodes.InspectFail),
			}).WithNodeStatesProvisionUpdate(nodeUUID),
			restartOnFailure: true,

			expectedStarted:      true,
			expectedDirty:        true,
			expectedRequestAfter: 10,
			expectedPublish:      "InspectionStarted Hardware inspection started",
		},
		{
			name: "inspection-forceReboot",
			ironic: testserver.NewIronic(t).Node(nodes.Node{
				UUID:           nodeUUID,
				ProvisionState: string(nodes.InspectWait),
			}).WithNodeStatesProvisionUpdate(nodeUUID),
			forceReboot: true,

			expectedStarted:      true,
			expectedDirty:        true,
			expectedRequestAfter: 10,
		},
		{
			name: "inspection-complete",
			ironic: testserver.NewIronic(t).Node(nodes.Node{
				UUID:           nodeUUID,
				ProvisionState: string(nodes.Manageable),
			}).WithInventory(nodeUUID, nodes.InventoryData{
				Inventory: inventory.InventoryType{
					Hostname: "node-0",
				},
			}),

			expectedDirty:       false,
			expectedDetailsHost: "node-0",
			expectedPublish:     "InspectionComplete Hardware inspection completed",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.ironic != nil {
				tc.ironic.Start()
				defer tc.ironic.Stop()
			}

			host := makeHost()
			host.Status.Provisioning.ID = nodeUUID
			publishedMsg := ""
			publisher := func(reason, message string) {
				publishedMsg = reason + " " + message
			}
			auth := clients.AuthConfig{Type: clients.NoAuth}
			prov, err := newProvisionerWithSettings(host, bmc.Credentials{}, publisher, tc.ironic.Endpoint(), auth)
			if err != nil {
				t.Fatalf("could not create provisioner: %s", err)
			}

			result, started, details, err := prov.InspectHardware(
				provisioner.InspectData{BootMode: metal3api.DefaultBootMode},
				tc.restartOnFailure, tc.refresh, tc.forceReboot)

			assert.Equal(t, tc.expectedStarted, started)
			assert.Equal(t, tc.expectedDirty, result.Dirty)
			assert.Equal(t, time.Second*time.Duration(tc.expectedRequestAfter), result.RequeueAfter)
			assert.Equal(t, tc.expectedResultError, result.ErrorMessage)

			if details != nil {
				assert.Equal(t, tc.expectedDetailsHost, details.Hostname)
			}
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
