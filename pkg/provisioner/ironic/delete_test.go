package ironic

import (
	"net/http"
	"testing"
	"time"

	"github.com/gophercloud/gophercloud/v2/openstack/baremetal/v1/nodes"
	"github.com/stretchr/testify/assert"

	"github.com/metal3-io/baremetal-operator/pkg/hardwareutils/bmc"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner/ironic/clients"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner/ironic/testserver"
)

type TestFunc func(string)

func TestDelete(t *testing.T) {
	deleteTest(t, false)
}

func TestDetach(t *testing.T) {
	deleteTest(t, true)
}

func deleteTest(t *testing.T, detach bool) {
	t.Helper()
	nodeUUID := "33ce8659-7400-4c68-9535-d10766f07a58"

	cases := []struct {
		name     string
		ironic   *testserver.IronicMock
		hostName string

		expectedDirty        bool
		expectedRequestAfter time.Duration
		expectedUpdate       *nodes.UpdateOperation

		expectedError string
	}{
		{
			name: "delete-host-fail",
			ironic: testserver.NewIronic(t).Node(
				nodes.Node{
					UUID:           nodeUUID,
					ProvisionState: "active",
					Maintenance:    true,
				},
			).DeleteError(nodeUUID, http.StatusInternalServerError),
			expectedError: "failed to remove host",
		},
		{
			name: "delete-host-busy",
			ironic: testserver.NewIronic(t).Node(
				nodes.Node{
					UUID:           nodeUUID,
					ProvisionState: "active",
					Maintenance:    true,
				},
			).DeleteError(nodeUUID, http.StatusConflict),
			expectedDirty:        true,
			expectedRequestAfter: provisionRequeueDelay,
		},
		{
			name: "delete-host-not-found",
			ironic: testserver.NewIronic(t).Node(
				nodes.Node{
					UUID:           nodeUUID,
					ProvisionState: "active",
					Maintenance:    true,
				},
			).DeleteError(nodeUUID, http.StatusNotFound),
			expectedDirty:        true,
			expectedRequestAfter: 0,
		},
		{
			name: "delete-ok",
			ironic: testserver.NewIronic(t).Node(
				nodes.Node{
					UUID:           nodeUUID,
					ProvisionState: "active",
					Maintenance:    true,
				},
			).Delete(nodeUUID),
			expectedDirty:        true,
			expectedRequestAfter: 0,
		},
		{
			name: "host-not-found",

			hostName: "worker-0",
			ironic:   testserver.NewIronic(t).NodeError(nodeUUID, http.StatusGatewayTimeout),

			expectedError: "failed to find node by ID 33ce8659-7400-4c68-9535-d10766f07a58: Gateway Timeout.*",
		},
		{
			name:   "not-ironic-node",
			ironic: testserver.NewIronic(t).NoNode(nodeUUID).NoNode("myns" + nameSeparator + "myhost").NoNode("myhost"),
		},
		{
			name: "available-node",
			ironic: testserver.NewIronic(t).Node(
				nodes.Node{
					UUID:           nodeUUID,
					ProvisionState: "available",
					Maintenance:    false,
				},
			).Delete(nodeUUID),
			expectedDirty:        true,
			expectedRequestAfter: 0,
		},
		{
			name: "stale-instance-uuid-update",
			ironic: testserver.NewIronic(t).Node(
				nodes.Node{
					UUID:           nodeUUID,
					ProvisionState: "available",
					InstanceUUID:   nodeUUID,
				},
			).NodeUpdate(nodes.Node{
				UUID: nodeUUID,
			}).Delete(nodeUUID),
			expectedDirty:        true,
			expectedRequestAfter: 0,
			expectedUpdate: &nodes.UpdateOperation{
				Op:   nodes.RemoveOp,
				Path: "/instance_uuid",
			},
		},
		{
			name: "not-in-maintenance-update-fail",
			ironic: testserver.NewIronic(t).WithDefaultResponses().Node(
				nodes.Node{
					UUID:           nodeUUID,
					ProvisionState: "active",
					Maintenance:    false,
				},
			).NodeMaintenanceError(nodeUUID, http.StatusInternalServerError),

			expectedError: "failed to set host maintenance flag",
		},
		{
			name: "not-in-maintenance-update-busy",
			ironic: testserver.NewIronic(t).WithDefaultResponses().Node(
				nodes.Node{
					UUID:           nodeUUID,
					ProvisionState: "active",
					Maintenance:    false,
				},
			).NodeMaintenanceError(nodeUUID, http.StatusConflict),

			expectedDirty:        true,
			expectedRequestAfter: provisionRequeueDelay,
		},
		{
			name: "not-in-maintenance-update",
			ironic: testserver.NewIronic(t).Node(
				nodes.Node{
					UUID:           nodeUUID,
					ProvisionState: "active",
					Maintenance:    false,
				},
			).NodeMaintenance(nodes.Node{
				UUID: nodeUUID,
			}, true),
			expectedDirty:        true,
			expectedRequestAfter: 0,
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

			if tc.hostName != "" {
				host.Name = tc.hostName
			}

			auth := clients.AuthConfig{Type: clients.NoAuth}
			prov, err := newProvisionerWithSettings(host, bmc.Credentials{}, nullEventPublisher, tc.ironic.Endpoint(), auth)
			if err != nil {
				t.Fatalf("could not create provisioner: %s", err)
			}

			var result provisioner.Result
			if detach {
				result, err = prov.Detach()
			} else {
				result, err = prov.Delete()
			}

			assert.Equal(t, tc.expectedDirty, result.Dirty)
			assert.Equal(t, tc.expectedRequestAfter, result.RequeueAfter)

			if tc.expectedUpdate != nil {
				assert.Equal(t, *tc.expectedUpdate, tc.ironic.GetLastNodeUpdateRequestFor(nodeUUID)[0])
			}

			if tc.expectedError == "" {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
				assert.Regexp(t, tc.expectedError, err.Error())
			}
		})
	}
}
