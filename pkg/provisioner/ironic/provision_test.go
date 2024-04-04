package ironic

import (
	"net/url"
	"testing"
	"time"

	"github.com/gophercloud/gophercloud/v2/openstack/baremetal/v1/nodes"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/util/intstr"

	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	"github.com/metal3-io/baremetal-operator/pkg/hardwareutils/bmc"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner/fixture"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner/ironic/clients"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner/ironic/testbmc"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner/ironic/testserver"
)

func TestProvision(t *testing.T) {
	nodeUUID := "33ce8659-7400-4c68-9535-d10766f07a58"
	testImage := metal3api.Image{
		URL:          "http://test-image",
		Checksum:     "abcd",
		ChecksumType: metal3api.SHA256,
	}

	cases := []struct {
		name                   string
		ironic                 *testserver.IronicMock
		forceReboot            bool
		expectedDirty          bool
		expectedError          bool
		expectedErrorMessage   string
		expectedRequestAfter   int
		expectedProvisionState nodes.TargetProvisionState
	}{
		{
			name: "deployFail state",
			ironic: testserver.NewIronic(t).WithDefaultResponses().Node(nodes.Node{
				ProvisionState: string(nodes.DeployFail),
				UUID:           nodeUUID,
			}),
			expectedRequestAfter:   10,
			expectedDirty:          true,
			expectedProvisionState: nodes.TargetActive,
		},
		{
			name: "deployFail state - same image",
			ironic: testserver.NewIronic(t).WithDefaultResponses().Node(nodes.Node{
				ProvisionState: string(nodes.DeployFail),
				UUID:           nodeUUID,
				InstanceInfo: map[string]interface{}{
					"image_source":        testImage.URL,
					"image_os_hash_algo":  string(testImage.ChecksumType),
					"image_os_hash_value": testImage.Checksum,
				},
				LastError: "no work today",
			}),
			expectedRequestAfter: 0,
			expectedDirty:        false,
			expectedErrorMessage: "Image provisioning failed: no work today",
		},
		{
			name: "cleanFail state",
			ironic: testserver.NewIronic(t).WithDefaultResponses().Node(nodes.Node{
				ProvisionState: string(nodes.CleanFail),
				UUID:           nodeUUID,
			}),
			expectedRequestAfter:   10,
			expectedDirty:          true,
			expectedProvisionState: nodes.TargetManage,
		},
		{
			name: "manageable state",
			ironic: testserver.NewIronic(t).WithDefaultResponses().Node(nodes.Node{
				ProvisionState: string(nodes.Manageable),
				UUID:           nodeUUID,
			}),
			expectedRequestAfter:   10,
			expectedDirty:          true,
			expectedProvisionState: nodes.TargetProvide,
		},
		{
			name: "available state",
			ironic: testserver.NewIronic(t).WithDefaultResponses().Node(nodes.Node{
				ProvisionState: string(nodes.Available),
				UUID:           nodeUUID,
			}),
			expectedRequestAfter:   10,
			expectedDirty:          true,
			expectedProvisionState: nodes.TargetActive,
		},
		{
			name: "active state",
			ironic: testserver.NewIronic(t).Node(nodes.Node{
				ProvisionState: string(nodes.Active),
				UUID:           nodeUUID,
			}),
			expectedRequestAfter: 0,
			expectedDirty:        false,
		},
		{
			name: "other state: Cleaning",
			ironic: testserver.NewIronic(t).Node(nodes.Node{
				ProvisionState: string(nodes.Cleaning),
				UUID:           nodeUUID,
			}),
			expectedRequestAfter: 10,
			expectedDirty:        true,
		},
		{
			name: "other state: Deploy Wait",
			ironic: testserver.NewIronic(t).Node(nodes.Node{
				ProvisionState: string(nodes.DeployWait),
				UUID:           nodeUUID,
			}),
			expectedRequestAfter: 10,
			expectedDirty:        true,
		},
		{
			name: "forceReboot: Deploy Wait",
			ironic: testserver.NewIronic(t).WithDefaultResponses().Node(nodes.Node{
				ProvisionState: string(nodes.DeployWait),
				UUID:           nodeUUID,
			}),
			forceReboot:            true,
			expectedRequestAfter:   10,
			expectedDirty:          true,
			expectedProvisionState: nodes.TargetDeleted,
		},
		{
			name: "fault state",
			ironic: testserver.NewIronic(t).WithDefaultResponses().Node(nodes.Node{
				ProvisionState: string(nodes.Manageable),
				UUID:           nodeUUID,
				Fault:          "power fault",
				Maintenance:    true,
			}),
			expectedRequestAfter: 10,
			expectedDirty:        true,
		},
		{
			name: "maintenance mode",
			ironic: testserver.NewIronic(t).WithDefaultResponses().Node(nodes.Node{
				ProvisionState: string(nodes.Manageable),
				UUID:           nodeUUID,
				Maintenance:    true,
			}).NodeMaintenance(nodes.Node{
				UUID: nodeUUID,
			}, false),
			expectedRequestAfter: 0,
			expectedDirty:        true,
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
			publisher := func(reason, message string) {}
			auth := clients.AuthConfig{Type: clients.NoAuth}
			prov, err := newProvisionerWithSettings(host, bmc.Credentials{}, publisher, tc.ironic.Endpoint(), auth)
			if err != nil {
				t.Fatalf("could not create provisioner: %s", err)
			}

			result, err := prov.Provision(provisioner.ProvisionData{
				Image:      testImage,
				HostConfig: fixture.NewHostConfigData("testUserData", "test: NetworkData", "test: Meta"),
				BootMode:   metal3api.DefaultBootMode,
			}, tc.forceReboot)

			assert.Equal(t, tc.expectedDirty, result.Dirty)
			assert.Equal(t, time.Second*time.Duration(tc.expectedRequestAfter), result.RequeueAfter)
			assert.Equal(t, tc.expectedErrorMessage, result.ErrorMessage)
			if !tc.expectedError {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}

			lastProvOp := tc.ironic.GetLastNodeStatesProvisionUpdateRequestFor(nodeUUID)
			assert.Equal(t, tc.expectedProvisionState, lastProvOp.Target)
		})
	}
}

func TestDeprovision(t *testing.T) {
	nodeUUID := "33ce8659-7400-4c68-9535-d10766f07a58"
	cases := []struct {
		name                 string
		ironic               *testserver.IronicMock
		expectedDirty        bool
		expectedError        bool
		expectedErrorMessage bool
		expectedRequestAfter int
	}{
		{
			name: "active state",
			ironic: testserver.NewIronic(t).WithDefaultResponses().Node(nodes.Node{
				ProvisionState: string(nodes.Active),
				UUID:           nodeUUID,
			}),
			expectedRequestAfter: 10,
			expectedDirty:        true,
		},
		{
			name: "deploy failed state",
			ironic: testserver.NewIronic(t).WithDefaultResponses().Node(nodes.Node{
				ProvisionState: string(nodes.DeployFail),
				UUID:           nodeUUID,
			}),
			expectedRequestAfter: 10,
			expectedDirty:        true,
		},
		{
			name: "error state",
			ironic: testserver.NewIronic(t).WithDefaultResponses().Node(nodes.Node{
				ProvisionState: string(nodes.Error),
				UUID:           nodeUUID,
			}),
			expectedErrorMessage: true,
		},
		{
			name: "available state",
			ironic: testserver.NewIronic(t).WithDefaultResponses().Node(nodes.Node{
				ProvisionState: string(nodes.Available),
				UUID:           nodeUUID,
			}),
			expectedRequestAfter: 0,
			expectedDirty:        false,
		},
		{
			name: "deleting state",
			ironic: testserver.NewIronic(t).Node(nodes.Node{
				ProvisionState: string(nodes.Deleting),
				UUID:           nodeUUID,
			}),
			expectedRequestAfter: 10,
			expectedDirty:        true,
		},
		{
			name: "cleaning state",
			ironic: testserver.NewIronic(t).Node(nodes.Node{
				ProvisionState: string(nodes.Cleaning),
				UUID:           nodeUUID,
			}),
			expectedRequestAfter: 10,
			expectedDirty:        true,
		},
		{
			name: "cleanWait state",
			ironic: testserver.NewIronic(t).Node(nodes.Node{
				ProvisionState: string(nodes.CleanWait),
				UUID:           nodeUUID,
			}),
			expectedRequestAfter: 10,
			expectedDirty:        true,
		},
		{
			name: "clean fail state",
			ironic: testserver.NewIronic(t).WithDefaultResponses().Node(nodes.Node{
				ProvisionState: string(nodes.CleanFail),
				UUID:           nodeUUID,
			}),
			expectedErrorMessage: true,
		},
		{
			name: "manageable state",
			ironic: testserver.NewIronic(t).WithDefaultResponses().Node(nodes.Node{
				ProvisionState: string(nodes.Manageable),
				UUID:           nodeUUID,
			}),
			expectedRequestAfter: 10,
			expectedDirty:        true,
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
			publisher := func(reason, message string) {}
			auth := clients.AuthConfig{Type: clients.NoAuth}
			prov, err := newProvisionerWithSettings(host, bmc.Credentials{}, publisher, tc.ironic.Endpoint(), auth)
			if err != nil {
				t.Fatalf("could not create provisioner: %s", err)
			}

			result, err := prov.Deprovision(false)

			assert.Equal(t, tc.expectedDirty, result.Dirty)
			assert.Equal(t, tc.expectedErrorMessage, result.ErrorMessage != "")
			assert.Equal(t, time.Second*time.Duration(tc.expectedRequestAfter), result.RequeueAfter)
			if !tc.expectedError {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		})
	}
}

func TestIronicHasSameImage(t *testing.T) {
	nodeUUID := "33ce8659-7400-4c68-9535-d10766f07a58"
	cases := []struct {
		name             string
		expected         bool
		node             nodes.Node
		liveImage        bool
		hostImage        string
		hostChecksum     string
		hostChecksumType metal3api.ChecksumType
	}{
		{
			name:      "image same",
			expected:  true,
			liveImage: false,
			node: nodes.Node{
				InstanceInfo: map[string]interface{}{
					"image_source":        "theimage",
					"image_os_hash_value": "thechecksum",
					"image_os_hash_algo":  "md5",
				},
			},
			hostImage:        "theimage",
			hostChecksum:     "thechecksum",
			hostChecksumType: metal3api.MD5,
		},
		{
			name:      "image same - auto checksum",
			expected:  true,
			liveImage: false,
			node: nodes.Node{
				InstanceInfo: map[string]interface{}{
					"image_source":   "theimage",
					"image_checksum": "thechecksum",
				},
			},
			hostImage:        "theimage",
			hostChecksum:     "thechecksum",
			hostChecksumType: "auto",
		},
		{
			name:      "image different",
			expected:  false,
			liveImage: false,
			node: nodes.Node{
				InstanceInfo: map[string]interface{}{
					"image_source":        "theimage",
					"image_os_hash_value": "thechecksum",
					"image_os_hash_algo":  "md5",
				},
			},
			hostImage:        "different",
			hostChecksum:     "thechecksum",
			hostChecksumType: metal3api.MD5,
		},
		{
			name:      "image checksum different",
			expected:  false,
			liveImage: false,
			node: nodes.Node{
				InstanceInfo: map[string]interface{}{
					"image_source":        "theimage",
					"image_os_hash_value": "thechecksum",
					"image_os_hash_algo":  "md5",
				},
			},
			hostImage:        "theimage",
			hostChecksum:     "different",
			hostChecksumType: metal3api.MD5,
		},
		{
			name:      "image checksum changed to auto",
			expected:  false,
			liveImage: false,
			node: nodes.Node{
				InstanceInfo: map[string]interface{}{
					"image_source":        "theimage",
					"image_os_hash_value": "thechecksum",
					"image_os_hash_algo":  "md5",
				},
			},
			hostImage:        "theimage",
			hostChecksum:     "thechecksum",
			hostChecksumType: "auto",
		},
		{
			name:      "image checksum type different",
			expected:  false,
			liveImage: false,
			node: nodes.Node{
				InstanceInfo: map[string]interface{}{
					"image_source":        "theimage",
					"image_os_hash_value": "thechecksum",
					"image_os_hash_algo":  "md5",
				},
			},
			hostImage:        "theimage",
			hostChecksum:     "thechecksum",
			hostChecksumType: metal3api.SHA512,
		},
		{
			name:      "live image same",
			liveImage: true,
			expected:  true,
			node: nodes.Node{
				InstanceInfo: map[string]interface{}{
					"boot_iso": "theimage",
				},
			},
			hostImage: "theimage",
		},
		{
			name:      "live image different",
			liveImage: true,
			expected:  false,
			node: nodes.Node{
				InstanceInfo: map[string]interface{}{
					"boot_iso": "theimage",
				},
			},
			hostImage: "different",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ironic := testserver.NewIronic(t).WithDefaultResponses().Node(tc.node)
			ironic.Start()
			defer ironic.Stop()

			var host metal3api.BareMetalHost
			if tc.liveImage {
				host = makeHostLiveIso()
				host.Spec.Image.URL = tc.hostImage
			} else {
				host = makeHost()
				host.Spec.Image.URL = tc.hostImage
				host.Spec.Image.Checksum = tc.hostChecksum
				host.Spec.Image.ChecksumType = tc.hostChecksumType
			}
			host.Status.Provisioning.ID = nodeUUID
			publisher := func(reason, message string) {}
			auth := clients.AuthConfig{Type: clients.NoAuth}
			prov, err := newProvisionerWithSettings(host, bmc.Credentials{}, publisher, ironic.Endpoint(), auth)
			if err != nil {
				t.Fatalf("could not create provisioner: %s", err)
			}
			node := tc.node
			sameImage := prov.ironicHasSameImage(&node, *host.Spec.Image)
			assert.Equal(t, tc.expected, sameImage)
		})
	}
}

func TestBuildCleanSteps(t *testing.T) {
	var True = true
	var False = false

	nodeUUID := "33ce8659-7400-4c68-9535-d10766f07a58"
	cases := []struct {
		name             string
		ironic           *testserver.IronicMock
		currentSettings  metal3api.SettingsMap
		desiredSettings  metal3api.DesiredSettingsMap
		firmwareConfig   *metal3api.FirmwareConfig
		expectedSettings []map[string]interface{}
	}{
		{
			name: "no current settings",
			ironic: testserver.NewIronic(t).WithDefaultResponses().Node(nodes.Node{
				ProvisionState: string(nodes.DeployFail),
				UUID:           nodeUUID,
			}),
			currentSettings: nil,
			desiredSettings: nil,
			firmwareConfig: &metal3api.FirmwareConfig{
				VirtualizationEnabled:             &True,
				SimultaneousMultithreadingEnabled: &False,
			},
			expectedSettings: []map[string]interface{}{
				{
					"name":  "ProcVirtualization",
					"value": "Enabled",
				},
				{
					"name":  "ProcHyperthreading",
					"value": "Disabled",
				},
			},
		},
		{
			name: "current settings same as bmc",
			ironic: testserver.NewIronic(t).WithDefaultResponses().Node(nodes.Node{
				ProvisionState: string(nodes.DeployFail),
				UUID:           nodeUUID,
			}),
			currentSettings: map[string]string{
				"L2Cache":            "10x256 KB",
				"NumCores":           "10",
				"ProcVirtualization": "Enabled",
				"ProcHyperthreading": "Disabled",
			},
			desiredSettings: nil,
			firmwareConfig: &metal3api.FirmwareConfig{
				VirtualizationEnabled:             &True,
				SimultaneousMultithreadingEnabled: &False,
			},
			expectedSettings: nil,
		},
		{
			name: "current settings different than bmc",
			ironic: testserver.NewIronic(t).WithDefaultResponses().Node(nodes.Node{
				ProvisionState: string(nodes.DeployFail),
				UUID:           nodeUUID,
			}),
			currentSettings: metal3api.SettingsMap{
				"L2Cache":            "10x256 KB",
				"NumCores":           "10",
				"ProcVirtualization": "Disabled",
				"ProcHyperthreading": "Enabled",
			},
			desiredSettings: nil,
			firmwareConfig: &metal3api.FirmwareConfig{
				VirtualizationEnabled:             &True,
				SimultaneousMultithreadingEnabled: &False,
			},
			expectedSettings: []map[string]interface{}{
				{
					"name":  "ProcVirtualization",
					"value": "Enabled",
				},
				{
					"name":  "ProcHyperthreading",
					"value": "Disabled",
				},
			},
		},
		{
			name: "current settings same as bmc different than desired",
			ironic: testserver.NewIronic(t).WithDefaultResponses().Node(nodes.Node{
				ProvisionState: string(nodes.DeployFail),
				UUID:           nodeUUID,
			}),
			currentSettings: metal3api.SettingsMap{
				"L2Cache":               "10x256 KB",
				"NumCores":              "10",
				"NetworkBootRetryCount": "20",
				"ProcVirtualization":    "Enabled",
				"ProcHyperthreading":    "Disabled",
			},
			desiredSettings: metal3api.DesiredSettingsMap{
				"NetworkBootRetryCount": intstr.FromInt(10),
				"ProcVirtualization":    intstr.FromString("Disabled"),
				"ProcHyperthreading":    intstr.FromString("Enabled"),
			},
			firmwareConfig: &metal3api.FirmwareConfig{
				VirtualizationEnabled:             &True,
				SimultaneousMultithreadingEnabled: &False,
			},
			expectedSettings: []map[string]interface{}{
				{
					"name":  "NetworkBootRetryCount",
					"value": 10,
				},
				{
					"name":  "ProcVirtualization",
					"value": "Disabled",
				},
				{
					"name":  "ProcHyperthreading",
					"value": "Enabled",
				},
			},
		},
		{
			name: "current settings different than bmc and desired",
			ironic: testserver.NewIronic(t).WithDefaultResponses().Node(nodes.Node{
				ProvisionState: string(nodes.DeployFail),
				UUID:           nodeUUID,
			}),
			currentSettings: metal3api.SettingsMap{
				"L2Cache":               "10x256 KB",
				"NumCores":              "10",
				"NetworkBootRetryCount": "20",
				"ProcVirtualization":    "Enabled",
				"ProcHyperthreading":    "Disabled",
			},
			desiredSettings: metal3api.DesiredSettingsMap{
				"NetworkBootRetryCount": intstr.FromString("5"),
				"ProcVirtualization":    intstr.FromString("Enabled"),
				"ProcHyperthreading":    intstr.FromString("Disabled"),
			},
			firmwareConfig: &metal3api.FirmwareConfig{
				VirtualizationEnabled:             &False,
				SimultaneousMultithreadingEnabled: &True,
			},
			expectedSettings: []map[string]interface{}{
				{
					"name":  "ProcVirtualization",
					"value": "Disabled",
				},
				{
					"name":  "ProcHyperthreading",
					"value": "Enabled",
				},
				{
					"name":  "NetworkBootRetryCount",
					"value": "5",
				},
			},
		},
		{
			name: "bmc and desired duplicate settings",
			ironic: testserver.NewIronic(t).WithDefaultResponses().Node(nodes.Node{
				ProvisionState: string(nodes.DeployFail),
				UUID:           nodeUUID,
			}),
			currentSettings: metal3api.SettingsMap{
				"L2Cache":            "10x256 KB",
				"NumCores":           "10",
				"ProcVirtualization": "Enabled",
				"ProcHyperthreading": "Disabled",
			},
			desiredSettings: metal3api.DesiredSettingsMap{
				"ProcVirtualization": intstr.FromString("Disabled"),
				"ProcHyperthreading": intstr.FromString("Enabled"),
			},
			firmwareConfig: &metal3api.FirmwareConfig{
				VirtualizationEnabled:             &False,
				SimultaneousMultithreadingEnabled: &True,
			},
			expectedSettings: []map[string]interface{}{
				{
					"name":  "ProcHyperthreading",
					"value": "Enabled",
				},
				{
					"name":  "ProcVirtualization",
					"value": "Disabled",
				},
			},
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
			publisher := func(reason, message string) {}
			auth := clients.AuthConfig{Type: clients.NoAuth}
			prov, err := newProvisionerWithSettings(host, bmc.Credentials{}, publisher, tc.ironic.Endpoint(), auth)
			if err != nil {
				t.Fatalf("could not create provisioner: %s", err)
			}

			parsedURL := &url.URL{Scheme: "redfish", Host: "10.1.1.1"}

			testBMC, _ := testbmc.NewTestBMCAccessDetails(parsedURL, false)

			cleanSteps, err := prov.buildManualCleaningSteps(testBMC, provisioner.PrepareData{
				FirmwareConfig:         tc.firmwareConfig,
				ActualFirmwareSettings: tc.currentSettings,
				TargetFirmwareSettings: tc.desiredSettings,
			})

			assert.Equal(t, nil, err)
			if cleanSteps == nil {
				assert.Equal(t, tc.expectedSettings, []map[string]interface{}(nil))
			} else {
				settings := cleanSteps[0].Args["settings"]
				assert.ElementsMatch(t, tc.expectedSettings, settings)
			}
		})
	}
}
