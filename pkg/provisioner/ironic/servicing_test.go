package ironic

import (
	"net/url"
	"testing"
	"time"

	"github.com/gophercloud/gophercloud/v2/openstack/baremetal/v1/nodes"
	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	"github.com/metal3-io/baremetal-operator/pkg/hardwareutils/bmc"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner/ironic/clients"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner/ironic/testserver"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/util/intstr"
)

type BIOSTestBMC struct{}

func (r *BIOSTestBMC) Type() string                                          { return "bios-test" }
func (r *BIOSTestBMC) NeedsMAC() bool                                        { return false }
func (r *BIOSTestBMC) Driver() string                                        { return "bios-test" }
func (r *BIOSTestBMC) DisableCertificateVerification() bool                  { return false }
func (r *BIOSTestBMC) DriverInfo(bmc.Credentials) (i map[string]interface{}) { return }
func (r *BIOSTestBMC) SupportsISOPreprovisioningImage() bool                 { return false }
func (r *BIOSTestBMC) BIOSInterface() string                                 { return "" }
func (r *BIOSTestBMC) BootInterface() string                                 { return "" }
func (r *BIOSTestBMC) FirmwareInterface() string                             { return "" }
func (r *BIOSTestBMC) ManagementInterface() string                           { return "" }
func (r *BIOSTestBMC) PowerInterface() string                                { return "" }
func (r *BIOSTestBMC) RAIDInterface() string                                 { return "" }
func (r *BIOSTestBMC) VendorInterface() string                               { return "" }
func (r *BIOSTestBMC) SupportsSecureBoot() bool                              { return false }
func (r *BIOSTestBMC) RequiresProvisioningNetwork() bool                     { return true }
func (r *BIOSTestBMC) BuildBIOSSettings(_ *bmc.FirmwareConfig) ([]map[string]string, error) {
	return nil, nil
}

func TestService(t *testing.T) {
	bmc.RegisterFactory("bios-test", func(u *url.URL, dcv bool) (bmc.AccessDetails, error) {
		return &BIOSTestBMC{}, nil
	}, []string{})

	nodeUUID := "33ce8659-7400-4c68-9535-d10766f07a58"
	cases := []struct {
		name                 string
		ironic               *testserver.IronicMock
		unprepared           bool
		skipConfig           bool
		expectedStarted      bool
		expectedDirty        bool
		expectedError        bool
		expectedRequestAfter int
	}{
		{
			name: "active, no new steps",
			ironic: testserver.NewIronic(t).WithDefaultResponses().Node(nodes.Node{
				ProvisionState: string(nodes.Active),
				UUID:           nodeUUID,
			}),
			skipConfig:           true,
			unprepared:           true,
			expectedStarted:      true,
			expectedRequestAfter: 0,
			expectedDirty:        false,
		},
		{
			name: "active with steps",
			ironic: testserver.NewIronic(t).WithDefaultResponses().Node(nodes.Node{
				ProvisionState: string(nodes.Active),
				UUID:           nodeUUID,
			}),
			unprepared:           true,
			expectedStarted:      true,
			expectedRequestAfter: 10,
			expectedDirty:        true,
		},
		{
			name: "serviceFail state(cleaned provision settings)",
			ironic: testserver.NewIronic(t).WithDefaultResponses().Node(nodes.Node{
				ProvisionState: string(nodes.ServiceFail),
				UUID:           nodeUUID,
			}),
			expectedStarted:      false,
			expectedRequestAfter: 0,
			expectedDirty:        false,
		},
		{
			name: "serviceFail state(retry)",
			ironic: testserver.NewIronic(t).WithDefaultResponses().Node(nodes.Node{
				ProvisionState: string(nodes.ServiceFail),
				UUID:           nodeUUID,
			}),
			unprepared:           true,
			expectedStarted:      true,
			expectedRequestAfter: 10,
			expectedDirty:        true,
		},
		{
			name: "serviceFail state(retry with maintenance)",
			ironic: testserver.NewIronic(t).WithDefaultResponses().Node(nodes.Node{
				ProvisionState: string(nodes.ServiceFail),
				UUID:           nodeUUID,
				Maintenance:    true,
			}).NodeMaintenance(nodes.Node{
				UUID: nodeUUID,
			}, false),
			unprepared:      true,
			expectedStarted: false,
			expectedDirty:   true,
		},
		{
			name: "servicing state",
			ironic: testserver.NewIronic(t).WithDefaultResponses().Node(nodes.Node{
				ProvisionState: string(nodes.Servicing),
				UUID:           nodeUUID,
			}),
			expectedStarted:      false,
			expectedRequestAfter: 10,
			expectedDirty:        true,
		},
		{
			name: "serviceWait state",
			ironic: testserver.NewIronic(t).WithDefaultResponses().Node(nodes.Node{
				ProvisionState: string(nodes.ServiceWait),
				UUID:           nodeUUID,
			}),
			expectedStarted:      false,
			expectedRequestAfter: 10,
			expectedDirty:        true,
		},
		{
			name: "active state(servicing finished)",
			ironic: testserver.NewIronic(t).WithDefaultResponses().Node(nodes.Node{
				ProvisionState: string(nodes.Active),
				UUID:           nodeUUID,
			}),
			expectedStarted:      false,
			expectedRequestAfter: 0,
			expectedDirty:        false,
		},
		{
			name: "unexpected state",
			ironic: testserver.NewIronic(t).WithDefaultResponses().Node(nodes.Node{
				ProvisionState: string(nodes.Cleaning),
				UUID:           nodeUUID,
			}),
			expectedStarted:      false,
			expectedRequestAfter: 0,
			expectedDirty:        false,
			expectedError:        true,
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
			prepData := provisioner.ServicingData{}
			if !tc.skipConfig {
				host.Spec.BMC.Address = "raid-test://test.bmc/"
				prepData.ActualFirmwareSettings = metal3api.SettingsMap{
					"Answer": "unknown",
				}
				prepData.TargetFirmwareSettings = metal3api.DesiredSettingsMap{
					"Answer": intstr.FromInt(42),
				}
			}

			publisher := func(reason, message string) {}
			auth := clients.AuthConfig{Type: clients.NoAuth}
			prov, err := newProvisionerWithSettings(host, bmc.Credentials{}, publisher, tc.ironic.Endpoint(), auth)
			if err != nil {
				t.Fatalf("could not create provisioner: %s", err)
			}
			prov.availableFeatures = clients.AvailableFeatures{MaxVersion: 87}

			result, started, err := prov.Service(prepData, tc.unprepared, tc.unprepared)

			assert.Equal(t, tc.expectedStarted, started)
			assert.Equal(t, tc.expectedDirty, result.Dirty)
			assert.Equal(t, time.Second*time.Duration(tc.expectedRequestAfter), result.RequeueAfter)
			if !tc.expectedError {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}
