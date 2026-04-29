package ironic

import (
	"context"
	"testing"

	"github.com/gophercloud/gophercloud/v2/openstack/baremetal/v1/nodes"
	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	"github.com/metal3-io/baremetal-operator/pkg/hardwareutils/bmc"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner/ironic/clients"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner/ironic/testserver"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestIsRedfishBMC(t *testing.T) {
	testCases := []struct {
		name       string
		bmcAddress string
		expected   bool
	}{
		{
			name:       "redfish BMC",
			bmcAddress: "redfish://192.168.1.1/redfish/v1/Systems/1",
			expected:   true,
		},
		{
			name:       "redfish-virtualmedia BMC",
			bmcAddress: "redfish-virtualmedia://192.168.1.1/redfish/v1/Systems/1",
			expected:   true,
		},
		{
			name:       "idrac-redfish BMC",
			bmcAddress: "idrac-redfish://192.168.1.1/redfish/v1/Systems/1",
			expected:   true,
		},
		{
			name:       "idrac-virtualmedia BMC",
			bmcAddress: "idrac-virtualmedia://192.168.1.1/redfish/v1/Systems/1",
			expected:   true,
		},
		{
			name:       "IPMI BMC",
			bmcAddress: "ipmi://192.168.1.1",
			expected:   false,
		},
		{
			name:       "libvirt BMC",
			bmcAddress: "libvirt://192.168.1.1",
			expected:   false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			bmcAccess, err := bmc.NewAccessDetails(tc.bmcAddress, false)
			assert.NoError(t, err)
			result := isRedfishBMC(bmcAccess)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestEnrollNodeRedfishInspectInterface(t *testing.T) {
	cases := []struct {
		name                     string
		bmcAddress               string
		expectedInspectInterface string
	}{
		{
			name:                     "redfish BMC uses redfish inspect",
			bmcAddress:               "redfish://192.168.1.1/redfish/v1/Systems/1",
			expectedInspectInterface: "redfish",
		},
		{
			name:                     "redfish-virtualmedia BMC uses redfish inspect",
			bmcAddress:               "redfish-virtualmedia://192.168.1.1/redfish/v1/Systems/1",
			expectedInspectInterface: "redfish",
		},
		{
			name:                     "idrac-redfish BMC uses redfish inspect",
			bmcAddress:               "idrac-redfish://192.168.1.1/redfish/v1/Systems/1",
			expectedInspectInterface: "redfish",
		},
		{
			name:                     "IPMI BMC uses agent inspect",
			bmcAddress:               "ipmi://192.168.1.1",
			expectedInspectInterface: "agent",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			host := makeHostForRedfish()
			host.Spec.BMC.Address = tc.bmcAddress
			host.Spec.BootMACAddress = "12:34:56:78:90:ab"
			host.Status.Provisioning.ID = ""

			var createdNode *nodes.Node
			createCallback := func(node nodes.Node) {
				createdNode = &node
			}

			ironic := testserver.NewIronic(t).
				WithDrivers().
				CreateNodes(createCallback).
				NoNode(host.Namespace + nameSeparator + host.Name).
				NoNode(host.Name)
			ironic.Start()
			defer ironic.Stop()

			publisher := func(reason, message string) {}
			auth := clients.AuthConfig{Type: clients.NoAuth}
			prov, err := newProvisionerWithSettings(host, bmc.Credentials{}, publisher,
				ironic.Endpoint(), auth,
			)
			if err != nil {
				t.Fatalf("could not create provisioner: %s", err)
			}

			result, provID, err := prov.Register(
				context.TODO(),
				provisioner.ManagementAccessData{
					BootMode: metal3api.DefaultBootMode,
				},
				false,
				false,
			)

			assert.NoError(t, err)
			assert.False(t, result.Dirty)
			assert.Equal(t, "", result.ErrorMessage)
			assert.NotEmpty(t, provID)

			// Verify the node was created with the correct inspect interface
			assert.NotNil(t, createdNode)
			assert.Equal(t, tc.expectedInspectInterface, createdNode.InspectInterface)
		})
	}
}

func TestInspectHardwareSwitchesToAgent(t *testing.T) {
	nodeUUID := "uuid-redfish-switch"

	t.Run("switches from redfish to agent after first inspection", func(t *testing.T) {
		host := makeHostForRedfish()
		host.Spec.BMC.Address = "redfish://192.168.1.1/redfish/v1/Systems/1"
		host.Status.Provisioning.ID = nodeUUID

		ironic := testserver.NewIronic(t).
			WithDefaultResponses().
			Node(nodes.Node{
				UUID:               nodeUUID,
				Name:               host.Namespace + nameSeparator + host.Name,
				ProvisionState:     string(nodes.Manageable),
				InspectInterface:   "redfish",
				DriverInternalInfo: map[string]interface{}{},
			})
		ironic.Start()
		defer ironic.Stop()

		publisher := func(reason, message string) {}
		auth := clients.AuthConfig{Type: clients.NoAuth}
		prov, err := newProvisionerWithSettings(host, bmc.Credentials{}, publisher,
			ironic.Endpoint(), auth,
		)
		if err != nil {
			t.Fatalf("could not create provisioner: %s", err)
		}

		// First call should complete redfish inspection and start agent inspection
		result, started, details, err := prov.InspectHardware(
			context.TODO(),
			provisioner.InspectData{
				BootMode: metal3api.DefaultBootMode,
			},
			false,
			false,
			false,
		)

		assert.NoError(t, err)
		assert.True(t, started, "should have started agent inspection")
		assert.NotNil(t, details)
	})

	t.Run("does not switch if already switched", func(t *testing.T) {
		host := makeHostForRedfish()
		host.Spec.BMC.Address = "redfish://192.168.1.1/redfish/v1/Systems/1"
		host.Status.Provisioning.ID = nodeUUID

		ironic := testserver.NewIronic(t).
			WithDefaultResponses().
			Node(nodes.Node{
				UUID:             nodeUUID,
				Name:             host.Namespace + nameSeparator + host.Name,
				ProvisionState:   string(nodes.Manageable),
				InspectInterface: "redfish",
				DriverInternalInfo: map[string]interface{}{
					initialRedfishInspectionDoneKey: true,
				},
			})
		ironic.Start()
		defer ironic.Stop()

		publisher := func(reason, message string) {}
		auth := clients.AuthConfig{Type: clients.NoAuth}
		prov, err := newProvisionerWithSettings(host, bmc.Credentials{}, publisher,
			ironic.Endpoint(), auth,
		)
		if err != nil {
			t.Fatalf("could not create provisioner: %s", err)
		}

		// Call should complete without switching
		result, started, details, err := prov.InspectHardware(
			context.TODO(),
			provisioner.InspectData{
				BootMode: metal3api.DefaultBootMode,
			},
			false,
			false,
			false,
		)

		assert.NoError(t, err)
		assert.False(t, started, "should not have started another inspection")
		assert.NotNil(t, details)
		assert.False(t, result.Dirty)
	})

	t.Run("agent inspection does not trigger switch", func(t *testing.T) {
		host := makeHostForRedfish()
		host.Spec.BMC.Address = "redfish://192.168.1.1/redfish/v1/Systems/1"
		host.Status.Provisioning.ID = nodeUUID

		ironic := testserver.NewIronic(t).
			WithDefaultResponses().
			Node(nodes.Node{
				UUID:               nodeUUID,
				Name:               host.Namespace + nameSeparator + host.Name,
				ProvisionState:     string(nodes.Manageable),
				InspectInterface:   "agent",
				DriverInternalInfo: map[string]interface{}{},
			})
		ironic.Start()
		defer ironic.Stop()

		publisher := func(reason, message string) {}
		auth := clients.AuthConfig{Type: clients.NoAuth}
		prov, err := newProvisionerWithSettings(host, bmc.Credentials{}, publisher,
			ironic.Endpoint(), auth,
		)
		if err != nil {
			t.Fatalf("could not create provisioner: %s", err)
		}

		// Call should complete without switching
		result, started, details, err := prov.InspectHardware(
			context.TODO(),
			provisioner.InspectData{
				BootMode: metal3api.DefaultBootMode,
			},
			false,
			false,
			false,
		)

		assert.NoError(t, err)
		assert.False(t, started)
		assert.NotNil(t, details)
		assert.False(t, result.Dirty)
	})
}

func makeHostForRedfish() *metal3api.BareMetalHost {
	return &metal3api.BareMetalHost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "myhost",
			Namespace: "myns",
			UID:       "27720611-e5d1-45d3-ba3a-222dcfaa4ca2",
		},
		Spec: metal3api.BareMetalHostSpec{
			BMC: metal3api.BMCDetails{
				Address:         "ipmi://192.168.1.1",
				CredentialsName: "bmc-creds",
			},
		},
		Status: metal3api.BareMetalHostStatus{
			Provisioning: metal3api.ProvisionStatus{
				ID: "provisioning-id",
			},
		},
	}
}
