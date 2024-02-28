package ironic

import (
	"net/url"
	"testing"
	"time"

	"github.com/gophercloud/gophercloud/v2/openstack/baremetal/v1/nodes"
	"github.com/stretchr/testify/assert"

	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	"github.com/metal3-io/baremetal-operator/pkg/hardwareutils/bmc"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner/ironic/clients"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner/ironic/testserver"
)

type RAIDTestBMC struct{}

func (r *RAIDTestBMC) Type() string                                          { return "raid-test" }
func (r *RAIDTestBMC) NeedsMAC() bool                                        { return false }
func (r *RAIDTestBMC) Driver() string                                        { return "raid-test" }
func (r *RAIDTestBMC) DisableCertificateVerification() bool                  { return false }
func (r *RAIDTestBMC) DriverInfo(bmc.Credentials) (i map[string]interface{}) { return }
func (r *RAIDTestBMC) SupportsISOPreprovisioningImage() bool                 { return false }
func (r *RAIDTestBMC) BIOSInterface() string                                 { return "" }
func (r *RAIDTestBMC) BootInterface() string                                 { return "" }
func (r *RAIDTestBMC) FirmwareInterface() string                             { return "" }
func (r *RAIDTestBMC) ManagementInterface() string                           { return "" }
func (r *RAIDTestBMC) PowerInterface() string                                { return "" }
func (r *RAIDTestBMC) RAIDInterface() string                                 { return "" }
func (r *RAIDTestBMC) VendorInterface() string                               { return "" }
func (r *RAIDTestBMC) SupportsSecureBoot() bool                              { return false }
func (r *RAIDTestBMC) RequiresProvisioningNetwork() bool                     { return true }
func (r *RAIDTestBMC) BuildBIOSSettings(_ *bmc.FirmwareConfig) ([]map[string]string, error) {
	return nil, nil
}

func TestPrepare(t *testing.T) {
	bmc.RegisterFactory("raid-test", func(u *url.URL, dcv bool) (bmc.AccessDetails, error) {
		return &RAIDTestBMC{}, nil
	}, []string{})

	nodeUUID := "33ce8659-7400-4c68-9535-d10766f07a58"
	cases := []struct {
		name                 string
		ironic               *testserver.IronicMock
		unprepared           bool
		existRaidConfig      bool
		expectedStarted      bool
		expectedDirty        bool
		expectedError        bool
		expectedRequestAfter int
	}{
		{
			name: "manageable state(haven't clean steps)",
			ironic: testserver.NewIronic(t).WithDefaultResponses().Node(nodes.Node{
				ProvisionState: string(nodes.Manageable),
				UUID:           nodeUUID,
			}),
			unprepared:           true,
			expectedStarted:      true,
			expectedRequestAfter: 0,
			expectedDirty:        false,
		},
		{
			name: "manageable state(have clean steps)",
			ironic: testserver.NewIronic(t).WithDefaultResponses().Node(nodes.Node{
				ProvisionState: string(nodes.Manageable),
				UUID:           nodeUUID,
			}),
			unprepared:           true,
			existRaidConfig:      true,
			expectedStarted:      true,
			expectedRequestAfter: 10,
			expectedDirty:        true,
		},
		{
			name: "available state(haven't clean steps)",
			ironic: testserver.NewIronic(t).WithDefaultResponses().Node(nodes.Node{
				ProvisionState: string(nodes.Available),
				UUID:           nodeUUID,
			}),
			unprepared:           true,
			expectedStarted:      true,
			expectedRequestAfter: 0,
			expectedDirty:        false,
		},
		{
			name: "available state(have clean steps)",
			ironic: testserver.NewIronic(t).WithDefaultResponses().Node(nodes.Node{
				ProvisionState: string(nodes.Available),
				UUID:           nodeUUID,
			}),
			unprepared:           true,
			existRaidConfig:      true,
			expectedStarted:      false,
			expectedRequestAfter: 10,
			expectedDirty:        true,
		},
		{
			name: "cleanFail state(cleaned provision settings)",
			ironic: testserver.NewIronic(t).WithDefaultResponses().Node(nodes.Node{
				ProvisionState: string(nodes.CleanFail),
				UUID:           nodeUUID,
			}),
			expectedStarted:      false,
			expectedRequestAfter: 0,
			expectedDirty:        false,
		},
		{
			name: "cleanFail state(set ironic host to manageable)",
			ironic: testserver.NewIronic(t).WithDefaultResponses().Node(nodes.Node{
				ProvisionState: string(nodes.CleanFail),
				UUID:           nodeUUID,
			}),
			unprepared:           true,
			existRaidConfig:      true,
			expectedStarted:      false,
			expectedRequestAfter: 10,
			expectedDirty:        true,
		},
		{
			name: "cleaning state",
			ironic: testserver.NewIronic(t).WithDefaultResponses().Node(nodes.Node{
				ProvisionState: string(nodes.Cleaning),
				UUID:           nodeUUID,
			}),
			existRaidConfig:      true,
			expectedStarted:      false,
			expectedRequestAfter: 10,
			expectedDirty:        true,
		},
		{
			name: "cleanWait state",
			ironic: testserver.NewIronic(t).WithDefaultResponses().Node(nodes.Node{
				ProvisionState: string(nodes.CleanWait),
				UUID:           nodeUUID,
			}),
			existRaidConfig:      true,
			expectedStarted:      false,
			expectedRequestAfter: 10,
			expectedDirty:        true,
		},
		{
			name: "manageable state(manual clean finished)",
			ironic: testserver.NewIronic(t).WithDefaultResponses().Node(nodes.Node{
				ProvisionState: string(nodes.Manageable),
				UUID:           nodeUUID,
			}),
			existRaidConfig:      true,
			expectedStarted:      false,
			expectedRequestAfter: 0,
			expectedDirty:        false,
		},
		{
			name: "available state(automated clean finished)",
			ironic: testserver.NewIronic(t).WithDefaultResponses().Node(nodes.Node{
				ProvisionState: string(nodes.Available),
				UUID:           nodeUUID,
			}),
			existRaidConfig:      true,
			expectedStarted:      false,
			expectedRequestAfter: 0,
			expectedDirty:        false,
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
			prepData := provisioner.PrepareData{}
			if tc.existRaidConfig {
				host.Spec.BMC.Address = "raid-test://test.bmc/"
				prepData.TargetRAIDConfig = &metal3api.RAIDConfig{
					HardwareRAIDVolumes: []metal3api.HardwareRAIDVolume{
						{
							Name:  "root",
							Level: "1",
						},
						{
							Name:  "v1",
							Level: "1",
						},
					},
				}
			}

			publisher := func(reason, message string) {}
			auth := clients.AuthConfig{Type: clients.NoAuth}
			prov, err := newProvisionerWithSettings(host, bmc.Credentials{}, publisher, tc.ironic.Endpoint(), auth)
			if err != nil {
				t.Fatalf("could not create provisioner: %s", err)
			}

			result, started, err := prov.Prepare(prepData, tc.unprepared, tc.unprepared)

			assert.Equal(t, tc.expectedStarted, started)
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
