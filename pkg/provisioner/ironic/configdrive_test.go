package ironic

import (
	"testing"

	"github.com/gophercloud/gophercloud/v2/openstack/baremetal/v1/nodes"
	"github.com/stretchr/testify/assert"

	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	"github.com/metal3-io/baremetal-operator/pkg/hardwareutils/bmc"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner/fixture"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner/ironic/clients"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner/ironic/testserver"
)

func TestEmpty(t *testing.T) {
	nodeUUID := "33ce8659-7400-4c68-9535-d10766f07a58"

	cases := []struct {
		name       string
		hostData   provisioner.HostConfigData
		diskFormat string
		expected   nodes.ConfigDrive
	}{
		{
			name:     "default",
			hostData: fixture.NewHostConfigData("", "", ""),
			expected: nodes.ConfigDrive{
				MetaData: map[string]interface{}{
					"local-hostname":   "myhost",
					"local_hostname":   "myhost",
					"metal3-name":      "myhost",
					"metal3-namespace": "myns",
					"name":             "myhost",
				},
			},
		},
		{
			name:       "default with disk format",
			hostData:   fixture.NewHostConfigData("", "", ""),
			diskFormat: "qcow2",
			expected: nodes.ConfigDrive{
				MetaData: map[string]interface{}{
					"local-hostname":   "myhost",
					"local_hostname":   "myhost",
					"metal3-name":      "myhost",
					"metal3-namespace": "myns",
					"name":             "myhost",
				},
			},
		},
		{
			name:     "everything",
			hostData: fixture.NewHostConfigData("testUserData", "test: NetworkData", "test: Meta"),
			expected: nodes.ConfigDrive{
				MetaData: map[string]interface{}{
					"local-hostname":   "myhost",
					"local_hostname":   "myhost",
					"metal3-name":      "myhost",
					"metal3-namespace": "myns",
					"name":             "myhost",
					"test":             "Meta",
				},
				NetworkData: map[string]interface{}{
					"test": "NetworkData",
				},
				UserData: "testUserData",
			},
		},
		{
			name:     "only network data",
			hostData: fixture.NewHostConfigData("", "test: NetworkData", ""),
			expected: nodes.ConfigDrive{
				MetaData: map[string]interface{}{
					"local-hostname":   "myhost",
					"local_hostname":   "myhost",
					"metal3-name":      "myhost",
					"metal3-namespace": "myns",
					"name":             "myhost",
				},
				NetworkData: map[string]interface{}{
					"test": "NetworkData",
				},
			},
		},
		{
			name:     "only user data",
			hostData: fixture.NewHostConfigData("testUserData", "", ""),
			expected: nodes.ConfigDrive{
				MetaData: map[string]interface{}{
					"local-hostname":   "myhost",
					"local_hostname":   "myhost",
					"metal3-name":      "myhost",
					"metal3-namespace": "myns",
					"name":             "myhost",
				},
				UserData: "testUserData",
			},
		},
		{
			name:     "only meta data",
			hostData: fixture.NewHostConfigData("", "", "test: Meta"),
			expected: nodes.ConfigDrive{
				MetaData: map[string]interface{}{
					"local-hostname":   "myhost",
					"local_hostname":   "myhost",
					"metal3-name":      "myhost",
					"metal3-namespace": "myns",
					"name":             "myhost",
					"test":             "Meta",
				},
			},
		},
		{
			name:       "live ISO",
			hostData:   fixture.NewHostConfigData("", "", ""),
			diskFormat: "live-iso",
			expected:   nodes.ConfigDrive{},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ironic := testserver.NewIronic(t).Node(nodes.Node{
				ProvisionState: string(nodes.Active),
				UUID:           nodeUUID,
			})
			ironic.Start()
			defer ironic.Stop()

			host := makeHost()
			host.Status.Provisioning.ID = nodeUUID
			publisher := func(reason, message string) {}
			auth := clients.AuthConfig{Type: clients.NoAuth}
			prov, err := newProvisionerWithSettings(host, bmc.Credentials{}, publisher, ironic.Endpoint(), auth)
			if err != nil {
				t.Fatalf("could not create provisioner: %s", err)
			}

			var diskFormat *string
			if tc.diskFormat != "" {
				dFormat := tc.diskFormat
				diskFormat = &dFormat
			}

			result, err := prov.getConfigDrive(provisioner.ProvisionData{
				HostConfig: tc.hostData,
				BootMode:   metal3api.DefaultBootMode,
				Image: metal3api.Image{
					URL:        "http://image",
					DiskFormat: diskFormat,
				},
			})

			if len(tc.expected.MetaData) > 0 {
				tc.expected.MetaData["uuid"] = string(prov.objectMeta.UID)
			}

			assert.Equal(t, tc.expected, result)
			assert.NoError(t, err)
		})
	}
}
