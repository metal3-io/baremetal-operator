package ironic

import (
	"testing"
	"time"

	"github.com/gophercloud/gophercloud/openstack/baremetal/v1/nodes"
	"github.com/gophercloud/gophercloud/openstack/baremetalintrospection/v1/introspection"
	"github.com/stretchr/testify/assert"

	"github.com/metal3-io/baremetal-operator/pkg/bmc"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner/ironic/clients"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner/ironic/testserver"
)

func TestAdopt(t *testing.T) {

	nodeUUID := "33ce8659-7400-4c68-9535-d10766f07a58"
	cases := []struct {
		name   string
		ironic *testserver.IronicMock

		expectedDirty        bool
		expectedError        bool
		expectedRequestAfter int
		force                bool
	}{
		{
			name: "node-in-enroll",
			ironic: testserver.NewIronic(t).Ready().Node(nodes.Node{
				ProvisionState: string(nodes.Enroll),
				UUID:           nodeUUID,
			}),

			expectedDirty: false,
			expectedError: true,
		},
		{
			name: "node-in-manageable",
			ironic: testserver.NewIronic(t).WithDefaultResponses().Node(nodes.Node{
				ProvisionState: string(nodes.Manageable),
				UUID:           nodeUUID,
			}),

			expectedDirty:        true,
			expectedRequestAfter: 10,
		},
		{
			name: "node-in-adopting",
			ironic: testserver.NewIronic(t).Ready().Node(nodes.Node{
				ProvisionState: string(nodes.Adopting),
				UUID:           nodeUUID,
			}),

			expectedDirty:        true,
			expectedRequestAfter: 10,
		},
		{
			name: "node-in-verifying",
			ironic: testserver.NewIronic(t).Ready().Node(nodes.Node{
				ProvisionState: string(nodes.Verifying),
				UUID:           nodeUUID,
			}),

			expectedDirty: false,
			expectedError: true,
		},
		{
			name: "node-in-AdoptFail",
			ironic: testserver.NewIronic(t).Ready().Node(nodes.Node{
				ProvisionState: string(nodes.AdoptFail),
				UUID:           nodeUUID,
			}),

			expectedDirty:        false,
			expectedRequestAfter: 0,
		},
		{
			name: "node-in-AdoptFail force retry",
			ironic: testserver.NewIronic(t).WithDefaultResponses().Node(nodes.Node{
				ProvisionState: string(nodes.AdoptFail),
				UUID:           nodeUUID,
			}),

			expectedDirty:        true,
			expectedRequestAfter: 10,
			force:                true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.ironic != nil {
				tc.ironic.Start()
				defer tc.ironic.Stop()
			}

			inspector := testserver.NewInspector(t).Ready().WithIntrospection(nodeUUID, introspection.Introspection{
				Finished: false,
			})
			inspector.Start()
			defer inspector.Stop()

			host := makeHost()
			host.Status.Provisioning.ID = nodeUUID
			publisher := func(reason, message string) {}
			auth := clients.AuthConfig{Type: clients.NoAuth}
			prov, err := newProvisionerWithSettings(host, bmc.Credentials{}, publisher,
				tc.ironic.Endpoint(), auth, inspector.Endpoint(), auth,
			)
			if err != nil {
				t.Fatalf("could not create provisioner: %s", err)
			}

			result, err := prov.Adopt(tc.force)

			assert.Equal(t, tc.expectedDirty, result.Dirty)
			assert.Equal(t, time.Second*time.Duration(tc.expectedRequestAfter), result.RequeueAfter)
			if !tc.expectedError {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		})
	}
}
