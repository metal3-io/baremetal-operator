package ironic

import (
	"testing"

	"github.com/gophercloud/gophercloud/v2/openstack/baremetal/v1/nodes"
	"github.com/metal3-io/baremetal-operator/pkg/hardwareutils/bmc"
	"github.com/metal3-io/baremetal-operator/pkg/hostclaim"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner/ironic/clients"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner/ironic/testserver"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnrollWithConductorGroup(t *testing.T) {
	cases := []struct {
		name                   string
		useFailureDomain       bool
		labels                 map[string]string
		expectedConductorGroup string
	}{
		{
			name:             "flag enabled, label present",
			useFailureDomain: true,
			labels: map[string]string{
				hostclaim.FailureDomainLabelName: "zone-1",
			},
			expectedConductorGroup: "zone-1",
		},
		{
			name:             "flag enabled, label missing",
			useFailureDomain: true,
			labels:           map[string]string{},
			expectedConductorGroup: "",
		},
		{
			name:             "flag disabled, label present",
			useFailureDomain: false,
			labels: map[string]string{
				hostclaim.FailureDomainLabelName: "zone-1",
			},
			expectedConductorGroup: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			host := makeHost()
			host.ObjectMeta.Labels = tc.labels
			host.Status.Provisioning.ID = ""

			var createdNode *nodes.Node
			createCallback := func(node nodes.Node) {
				createdNode = &node
			}

			ironic := testserver.NewIronic(t).WithDrivers().CreateNodes(createCallback).NoNode(host.Namespace + nameSeparator + host.Name).NoNode(host.Name)
			// Add response for PATCH which is called after creation
			ironic.NodeUpdate(nodes.Node{
				UUID:           "node-0",
				ProvisionState: string(nodes.Verifying),
			})
			ironic.Start()
			defer ironic.Stop()

			auth := clients.AuthConfig{Type: clients.NoAuth}
			prov, err := newProvisionerWithSettings(host, bmc.Credentials{}, nullEventPublisher, ironic.Endpoint(), auth)
			require.NoError(t, err)

			prov.config.useFailureDomainAsConductorGroup = tc.useFailureDomain

			_, _, err = prov.Register(t.Context(), provisioner.ManagementAccessData{}, false, false)
			require.NoError(t, err)

			assert.Equal(t, tc.expectedConductorGroup, createdNode.ConductorGroup)
		})
	}
}

func TestUpdateConductorGroup(t *testing.T) {
	host := makeHost()
	host.ObjectMeta.Labels = map[string]string{
		hostclaim.FailureDomainLabelName: "zone-2",
	}

	ironic := testserver.NewIronic(t).WithDrivers().Node(nodes.Node{
		Name:           host.Namespace + nameSeparator + host.Name,
		UUID:           host.Status.Provisioning.ID,
		ConductorGroup: "zone-1",
		ProvisionState: string(nodes.Verifying), // Avoid configureNode which makes extra PATCH
	}).NodeUpdate(nodes.Node{
		UUID:           host.Status.Provisioning.ID,
		ProvisionState: string(nodes.Verifying),
	})
	ironic.Start()
	defer ironic.Stop()

	auth := clients.AuthConfig{Type: clients.NoAuth}
	prov, err := newProvisionerWithSettings(host, bmc.Credentials{}, nullEventPublisher, ironic.Endpoint(), auth)
	require.NoError(t, err)

	prov.config.useFailureDomainAsConductorGroup = true

	_, _, err = prov.Register(t.Context(), provisioner.ManagementAccessData{}, false, false)
	require.NoError(t, err)

	updates := ironic.GetLastNodeUpdateRequestFor(host.Status.Provisioning.ID)
	found := false
	for _, up := range updates {
		if up.Path == "/conductor_group" {
			assert.Equal(t, "zone-2", up.Value)
			found = true
		}
	}
	assert.True(t, found, "conductor_group update not found in any PATCH request")
}
