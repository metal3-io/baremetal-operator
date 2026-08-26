package ironic

import (
	"testing"

	"github.com/gophercloud/gophercloud/v2/openstack/baremetal/v1/nodes"
	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	"github.com/metal3-io/baremetal-operator/pkg/hardwareutils/bmc"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner/ironic/clients"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner/ironic/testserver"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetFirmwareComponents(t *testing.T) {
	nodeUUID := "158c5d59-9ace-9631-ed51-d842a45f1c52"

	vendor := "HPE"
	model := "ProLiant DL380 Gen10"
	serial := "CZ12345678"

	cases := []struct {
		name               string
		nodeUUID           string
		expectedComponents []metal3api.FirmwareComponentStatus
		ironic             *testserver.IronicMock
		wantErr            error
	}{
		{
			name:     "with-identity-fields",
			nodeUUID: nodeUUID,
			expectedComponents: []metal3api.FirmwareComponentStatus{
				{
					Component:      "bios",
					InitialVersion: "U30 v2.36 (07/16/2020)",
					CurrentVersion: "U30 v2.36 (07/16/2020)",
					Vendor:         "HPE",
					Model:          "ProLiant DL380 Gen10",
					SerialNumber:   "CZ12345678",
				},
			},
			ironic: testserver.NewIronic(t).FirmwareComponents(nodeUUID, []nodes.FirmwareComponent{
				{
					Component:      "bios",
					InitialVersion: "U30 v2.36 (07/16/2020)",
					CurrentVersion: "U30 v2.36 (07/16/2020)",
					Vendor:         &vendor,
					Model:          &model,
					SerialNumber:   &serial,
				},
			}),
		},
		{
			name:     "without-identity-fields",
			nodeUUID: nodeUUID,
			expectedComponents: []metal3api.FirmwareComponentStatus{
				{
					Component:      "bmc",
					InitialVersion: "iLO 5 v2.78",
					CurrentVersion: "iLO 5 v2.81",
				},
			},
			ironic: testserver.NewIronic(t).FirmwareComponents(nodeUUID, []nodes.FirmwareComponent{
				{
					Component:      "bmc",
					InitialVersion: "iLO 5 v2.78",
					CurrentVersion: "iLO 5 v2.81",
				},
			}),
		},
		{
			name:               "not-registered",
			nodeUUID:           "",
			expectedComponents: nil,
			ironic:             testserver.NewIronic(t).NoFirmwareComponents(nodeUUID),
			wantErr:            provisioner.ErrNeedsRegistration,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.ironic.Start()
			defer tc.ironic.Stop()

			host := makeHost()
			host.Name = "node-1"
			host.Status.Provisioning.ID = tc.nodeUUID

			auth := clients.AuthConfig{Type: clients.NoAuth}

			prov, err := newProvisionerWithSettings(host, bmc.Credentials{}, nullEventPublisher, tc.ironic.Endpoint(), auth)
			if err != nil {
				t.Fatalf("could not create provisioner: %s", err)
			}

			components, err := prov.GetFirmwareComponents(t.Context())

			require.ErrorIs(t, err, tc.wantErr)
			assert.Equal(t, tc.expectedComponents, components)
		})
	}
}
