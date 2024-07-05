package ironic

import (
	"fmt"
	"net/url"
	"testing"
	"time"

	"github.com/gophercloud/gophercloud/v2/openstack/baremetal/v1/nodes"
	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	"github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1/profile"
	"github.com/metal3-io/baremetal-operator/pkg/hardwareutils/bmc"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner/fixture"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner/ironic/clients"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner/ironic/testbmc"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner/ironic/testserver"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
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

func TestGetUpdateOptsForNodeWithRootHints(t *testing.T) {
	eventPublisher := func(reason, message string) {}
	auth := clients.AuthConfig{Type: clients.NoAuth}

	host := makeHost()
	prov, err := newProvisionerWithSettings(host, bmc.Credentials{}, eventPublisher, "https://ironic.test", auth)
	if err != nil {
		t.Fatal(err)
	}
	ironicNode := &nodes.Node{}

	provData := provisioner.ProvisionData{
		Image:           *host.Spec.Image,
		BootMode:        metal3api.DefaultBootMode,
		RootDeviceHints: host.Status.Provisioning.RootDeviceHints,
	}
	patches := prov.getUpdateOptsForNode(ironicNode, provData).Updates

	t.Logf("patches: %v", patches)

	expected := []struct {
		Path  string            // the node property path
		Map   map[string]string // Expected roothdevicehint map
		Value interface{}       // the value being passed to ironic (or value associated with the key)
	}{
		{
			Path:  "/properties/root_device",
			Value: "userdefined_devicename",
			Map: map[string]string{
				"name":                 "s== userd_devicename",
				"hctl":                 "s== 1:2:3:4",
				"model":                "<in> userd_model",
				"vendor":               "<in> userd_vendor",
				"serial":               "s== userd_serial",
				"size":                 ">= 40",
				"wwn":                  "s== userd_wwn",
				"wwn_with_extension":   "s== userd_with_extension",
				"wwn_vendor_extension": "s== userd_vendor_extension",
				"rotational":           "true",
			},
		},
	}

	for _, e := range expected {
		t.Run(e.Path, func(t *testing.T) {
			t.Logf("expected: %v", e)
			var update nodes.UpdateOperation
			for _, patch := range patches {
				update = patch.(nodes.UpdateOperation)
				if update.Path == e.Path {
					break
				}
			}
			if update.Path != e.Path {
				t.Errorf("did not find %q in updates", e.Path)
				return
			}
			t.Logf("update: %v", update)
			if e.Map != nil {
				assert.Equal(t, e.Map, update.Value, fmt.Sprintf("%s does not match", e.Path))
			} else {
				assert.Equal(t, e.Value, update.Value, fmt.Sprintf("%s does not match", e.Path))
			}
		})
	}
}

func TestGetUpdateOptsForNodeVirtual(t *testing.T) {
	host := metal3api.BareMetalHost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "myhost",
			Namespace: "myns",
			UID:       "27720611-e5d1-45d3-ba3a-222dcfaa4ca2",
		},
		Spec: metal3api.BareMetalHostSpec{
			BMC: metal3api.BMCDetails{
				Address: "test://test.bmc/",
			},
			Image: &metal3api.Image{
				URL:          "not-empty",
				Checksum:     "checksum",
				ChecksumType: metal3api.MD5,
				DiskFormat:   ptr.To("raw"),
			},
			Online:          true,
			HardwareProfile: "unknown",
		},
		Status: metal3api.BareMetalHostStatus{
			HardwareProfile: "libvirt",
			Provisioning: metal3api.ProvisionStatus{
				ID: "provisioning-id",
			},
		},
	}

	eventPublisher := func(reason, message string) {}
	auth := clients.AuthConfig{Type: clients.NoAuth}

	prov, err := newProvisionerWithSettings(host, bmc.Credentials{}, eventPublisher, "https://ironic.test", auth)
	if err != nil {
		t.Fatal(fmt.Errorf("could not create provisioner: %w", err))
	}
	ironicNode := &nodes.Node{}

	hwProf, _ := profile.GetProfile("libvirt")
	provData := provisioner.ProvisionData{
		Image:           *host.Spec.Image,
		BootMode:        metal3api.DefaultBootMode,
		HardwareProfile: hwProf,
	}
	patches := prov.getUpdateOptsForNode(ironicNode, provData).Updates

	t.Logf("patches: %v", patches)

	expected := []struct {
		Path  string      // the node property path
		Key   string      // if value is a map, the key we care about
		Value interface{} // the value being passed to ironic (or value associated with the key)
	}{
		{
			Path:  "/instance_info/image_source",
			Value: "not-empty",
		},
		{
			Path:  "/instance_info/image_os_hash_algo",
			Value: "md5",
		},
		{
			Path:  "/instance_info/image_os_hash_value",
			Value: "checksum",
		},
		{
			Path:  "/instance_info/image_disk_format",
			Value: "raw",
		},
		{
			Path:  "/instance_info/capabilities",
			Value: map[string]string{},
		},
		{
			Path:  "/instance_uuid",
			Value: "27720611-e5d1-45d3-ba3a-222dcfaa4ca2",
		},
	}

	for _, e := range expected {
		t.Run(e.Path, func(t *testing.T) {
			t.Logf("expected: %v", e)
			var update nodes.UpdateOperation
			for _, patch := range patches {
				update = patch.(nodes.UpdateOperation)
				if update.Path == e.Path {
					break
				}
			}
			if update.Path != e.Path {
				t.Errorf("did not find %q in updates", e.Path)
				return
			}
			t.Logf("update: %v", update)
			assert.Equal(t, e.Value, update.Value, fmt.Sprintf("%s does not match", e.Path))
		})
	}
}

func TestGetUpdateOptsForNodeDell(t *testing.T) {
	host := metal3api.BareMetalHost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "myhost",
			Namespace: "myns",
			UID:       "27720611-e5d1-45d3-ba3a-222dcfaa4ca2",
		},
		Spec: metal3api.BareMetalHostSpec{
			BMC: metal3api.BMCDetails{
				Address: "test://test.bmc/",
			},
			Image: &metal3api.Image{
				URL:          "not-empty",
				Checksum:     "checksum",
				ChecksumType: metal3api.MD5,
				// DiskFormat not given to verify it is not added in instance_info
			},
			Online: true,
		},
		Status: metal3api.BareMetalHostStatus{
			HardwareProfile: "dell",
			Provisioning: metal3api.ProvisionStatus{
				ID: "provisioning-id",
			},
		},
	}

	eventPublisher := func(reason, message string) {}
	auth := clients.AuthConfig{Type: clients.NoAuth}

	prov, err := newProvisionerWithSettings(host, bmc.Credentials{}, eventPublisher, "https://ironic.test", auth)
	if err != nil {
		t.Fatal(err)
	}
	ironicNode := &nodes.Node{}

	hwProf, _ := profile.GetProfile("dell")
	provData := provisioner.ProvisionData{
		Image:           *host.Spec.Image,
		BootMode:        metal3api.DefaultBootMode,
		HardwareProfile: hwProf,
		CPUArchitecture: "x86_64",
	}
	patches := prov.getUpdateOptsForNode(ironicNode, provData).Updates

	t.Logf("patches: %v", patches)

	expected := []struct {
		Path  string      // the node property path
		Key   string      // if value is a map, the key we care about
		Value interface{} // the value being passed to ironic (or value associated with the key)
	}{
		{
			Path:  "/instance_info/image_source",
			Value: "not-empty",
		},
		{
			Path:  "/instance_info/image_os_hash_algo",
			Value: "md5",
		},
		{
			Path:  "/instance_info/image_os_hash_value",
			Value: "checksum",
		},
		{
			Path:  "/instance_uuid",
			Value: "27720611-e5d1-45d3-ba3a-222dcfaa4ca2",
		},
		{
			Path:  "/properties/cpu_arch",
			Value: "x86_64",
		},
	}

	for _, e := range expected {
		t.Run(e.Path, func(t *testing.T) {
			t.Logf("expected: %v", e)
			var update nodes.UpdateOperation
			for _, patch := range patches {
				update = patch.(nodes.UpdateOperation)
				if update.Path == e.Path {
					break
				}
			}
			if update.Path != e.Path {
				t.Errorf("did not find %q in updates", e.Path)
				return
			}
			t.Logf("update: %v", update)
			assert.Equal(t, e.Value, update.Value, fmt.Sprintf("%s does not match", e.Path))
		})
	}
}

func TestGetUpdateOptsForNodeLiveIso(t *testing.T) {
	eventPublisher := func(reason, message string) {}
	auth := clients.AuthConfig{Type: clients.NoAuth}

	host := makeHostLiveIso()
	prov, err := newProvisionerWithSettings(host, bmc.Credentials{}, eventPublisher, "https://ironic.test", auth)
	if err != nil {
		t.Fatal(err)
	}
	ironicNode := &nodes.Node{}

	provData := provisioner.ProvisionData{
		Image:    *host.Spec.Image,
		BootMode: metal3api.DefaultBootMode,
	}
	patches := prov.getUpdateOptsForNode(ironicNode, provData).Updates

	t.Logf("patches: %v", patches)

	expected := []struct {
		Path  string         // the node property path
		Key   string         // if value is a map, the key we care about
		Value interface{}    // the value being passed to ironic (or value associated with the key)
		Op    nodes.UpdateOp // The operation add/replace/remove
	}{
		{
			Path:  "/instance_info/boot_iso",
			Value: "not-empty",
			Op:    nodes.AddOp,
		},
		{
			Path:  "/instance_info/capabilities",
			Value: map[string]string{},
		},
		{
			Path:  "/deploy_interface",
			Value: "ramdisk",
			Op:    nodes.AddOp,
		},
	}

	for _, e := range expected {
		t.Run(e.Path, func(t *testing.T) {
			t.Logf("expected: %v", e)
			var update nodes.UpdateOperation
			for _, patch := range patches {
				update = patch.(nodes.UpdateOperation)
				if update.Path == e.Path {
					break
				}
			}
			if update.Path != e.Path {
				t.Errorf("did not find %q in updates", e.Path)
				return
			}
			t.Logf("update: %v", update)
			assert.Equal(t, e.Value, update.Value, fmt.Sprintf("%s does not match", e.Path))
		})
	}
}

func TestGetUpdateOptsForNodeImageToLiveIso(t *testing.T) {
	eventPublisher := func(reason, message string) {}
	auth := clients.AuthConfig{Type: clients.NoAuth}

	host := makeHostLiveIso()
	prov, err := newProvisionerWithSettings(host, bmc.Credentials{}, eventPublisher, "https://ironic.test", auth)
	if err != nil {
		t.Fatal(err)
	}
	ironicNode := &nodes.Node{
		InstanceInfo: map[string]interface{}{
			"image_source":        "oldimage",
			"image_os_hash_value": "thechecksum",
			"image_os_hash_algo":  "md5",
		},
	}

	provData := provisioner.ProvisionData{
		Image:    *host.Spec.Image,
		BootMode: metal3api.DefaultBootMode,
	}
	patches := prov.getUpdateOptsForNode(ironicNode, provData).Updates

	t.Logf("patches: %v", patches)

	expected := []struct {
		Path  string         // the node property path
		Key   string         // if value is a map, the key we care about
		Value interface{}    // the value being passed to ironic (or value associated with the key)
		Op    nodes.UpdateOp // The operation add/replace/remove
	}{
		{
			Path:  "/instance_info/boot_iso",
			Value: "not-empty",
			Op:    nodes.AddOp,
		},
		{
			Path:  "/deploy_interface",
			Value: "ramdisk",
			Op:    nodes.AddOp,
		},
		{
			Path: "/instance_info/image_source",
			Op:   nodes.RemoveOp,
		},
		{
			Path: "/instance_info/image_os_hash_algo",
			Op:   nodes.RemoveOp,
		},
		{
			Path: "/instance_info/image_os_hash_value",
			Op:   nodes.RemoveOp,
		},
	}

	for _, e := range expected {
		t.Run(e.Path, func(t *testing.T) {
			t.Logf("expected: %v", e)
			var update nodes.UpdateOperation
			for _, patch := range patches {
				update = patch.(nodes.UpdateOperation)
				if update.Path == e.Path {
					break
				}
			}
			if update.Path != e.Path {
				t.Errorf("did not find %q in updates", e.Path)
				return
			}
			t.Logf("update: %v", update)
			assert.Equal(t, e.Value, update.Value, fmt.Sprintf("%s value does not match", e.Path))
			assert.Equal(t, e.Op, update.Op, fmt.Sprintf("%s operation does not match", e.Path))
		})
	}
}

func TestGetUpdateOptsForNodeLiveIsoToImage(t *testing.T) {
	eventPublisher := func(reason, message string) {}
	auth := clients.AuthConfig{Type: clients.NoAuth}

	host := makeHost()
	host.Spec.Image.URL = "newimage"
	host.Spec.Image.Checksum = "thechecksum"
	host.Spec.Image.ChecksumType = metal3api.MD5
	prov, err := newProvisionerWithSettings(host, bmc.Credentials{}, eventPublisher, "https://ironic.test", auth)
	if err != nil {
		t.Fatal(err)
	}
	ironicNode := &nodes.Node{
		InstanceInfo: map[string]interface{}{
			"boot_iso": "oldimage",
		},
		DeployInterface: "ramdisk",
	}

	provData := provisioner.ProvisionData{
		Image:    *host.Spec.Image,
		BootMode: metal3api.DefaultBootMode,
	}
	patches := prov.getUpdateOptsForNode(ironicNode, provData).Updates

	t.Logf("patches: %v", patches)

	expected := []struct {
		Path  string         // the node property path
		Key   string         // if value is a map, the key we care about
		Value interface{}    // the value being passed to ironic (or value associated with the key)
		Op    nodes.UpdateOp // The operation add/replace/remove
	}{
		{
			Path: "/instance_info/boot_iso",
			Op:   nodes.RemoveOp,
		},
		{
			Path: "/deploy_interface",
			Op:   nodes.RemoveOp,
		},
		{
			Path:  "/instance_info/image_source",
			Value: "newimage",
			Op:    nodes.AddOp,
		},
		{
			Path:  "/instance_info/image_os_hash_algo",
			Value: "md5",
			Op:    nodes.AddOp,
		},
		{
			Path:  "/instance_info/image_os_hash_value",
			Value: "thechecksum",
			Op:    nodes.AddOp,
		},
	}

	for _, e := range expected {
		t.Run(e.Path, func(t *testing.T) {
			t.Logf("expected: %v", e)
			var update nodes.UpdateOperation
			for _, patch := range patches {
				update = patch.(nodes.UpdateOperation)
				if update.Path == e.Path {
					break
				}
			}
			if update.Path != e.Path {
				t.Errorf("did not find %q in updates", e.Path)
				return
			}
			t.Logf("update: %v", update)
			assert.Equal(t, e.Value, update.Value, fmt.Sprintf("%s value does not match", e.Path))
			assert.Equal(t, e.Op, update.Op, fmt.Sprintf("%s operation does not match", e.Path))
		})
	}
}

func TestGetUpdateOptsForNodeCustomDeploy(t *testing.T) {
	eventPublisher := func(reason, message string) {}
	auth := clients.AuthConfig{Type: clients.NoAuth}

	host := makeHostCustomDeploy(true)
	prov, err := newProvisionerWithSettings(host, bmc.Credentials{}, eventPublisher, "https://ironic.test", auth)
	if err != nil {
		t.Fatal(err)
	}
	ironicNode := &nodes.Node{}

	provData := provisioner.ProvisionData{
		Image:        metal3api.Image{},
		BootMode:     metal3api.DefaultBootMode,
		CustomDeploy: host.Spec.CustomDeploy,
	}
	patches := prov.getUpdateOptsForNode(ironicNode, provData).Updates

	t.Logf("patches: %v", patches)

	expected := []struct {
		Path  string         // the node property path
		Key   string         // if value is a map, the key we care about
		Value interface{}    // the value being passed to ironic (or value associated with the key)
		Op    nodes.UpdateOp // The operation add/replace/remove
	}{
		{
			Path:  "/instance_uuid",
			Value: "27720611-e5d1-45d3-ba3a-222dcfaa4ca2",
		},
		{
			Path:  "/deploy_interface",
			Value: "custom-agent",
			Op:    nodes.AddOp,
		},
	}

	for _, e := range expected {
		t.Run(e.Path, func(t *testing.T) {
			t.Logf("expected: %v", e)
			var update nodes.UpdateOperation
			for _, patch := range patches {
				update = patch.(nodes.UpdateOperation)
				if update.Path == e.Path {
					break
				}
			}
			if update.Path != e.Path {
				t.Errorf("did not find %q in updates", e.Path)
				return
			}
			t.Logf("update: %v", update)
			assert.Equal(t, e.Value, update.Value, fmt.Sprintf("%s does not match", e.Path))
		})
	}
}

func TestGetUpdateOptsForNodeCustomDeployWithImage(t *testing.T) {
	eventPublisher := func(reason, message string) {}
	auth := clients.AuthConfig{Type: clients.NoAuth}

	host := makeHostCustomDeploy(false)
	prov, err := newProvisionerWithSettings(host, bmc.Credentials{}, eventPublisher, "https://ironic.test", auth)
	if err != nil {
		t.Fatal(err)
	}
	ironicNode := &nodes.Node{}

	provData := provisioner.ProvisionData{
		Image:        *host.Spec.Image,
		BootMode:     metal3api.DefaultBootMode,
		CustomDeploy: host.Spec.CustomDeploy,
	}
	patches := prov.getUpdateOptsForNode(ironicNode, provData).Updates

	t.Logf("patches: %v", patches)

	expected := []struct {
		Path  string         // the node property path
		Key   string         // if value is a map, the key we care about
		Value interface{}    // the value being passed to ironic (or value associated with the key)
		Op    nodes.UpdateOp // The operation add/replace/remove
	}{
		{
			Path:  "/instance_info/image_source",
			Value: "not-empty",
		},
		{
			Path:  "/instance_uuid",
			Value: "27720611-e5d1-45d3-ba3a-222dcfaa4ca2",
		},
		{
			Path:  "/deploy_interface",
			Value: "custom-agent",
			Op:    nodes.AddOp,
		},
	}

	for _, e := range expected {
		t.Run(e.Path, func(t *testing.T) {
			t.Logf("expected: %v", e)
			var update nodes.UpdateOperation
			for _, patch := range patches {
				update = patch.(nodes.UpdateOperation)
				if update.Path == e.Path {
					break
				}
			}
			if update.Path != e.Path {
				t.Errorf("did not find %q in updates", e.Path)
				return
			}
			t.Logf("update: %v", update)
			assert.Equal(t, e.Value, update.Value, fmt.Sprintf("%s does not match", e.Path))
		})
	}
}

func TestGetUpdateOptsForNodeImageToCustomDeploy(t *testing.T) {
	eventPublisher := func(reason, message string) {}
	auth := clients.AuthConfig{Type: clients.NoAuth}

	host := makeHostCustomDeploy(false)
	prov, err := newProvisionerWithSettings(host, bmc.Credentials{}, eventPublisher, "https://ironic.test", auth)
	if err != nil {
		t.Fatal(err)
	}
	ironicNode := &nodes.Node{
		InstanceInfo: map[string]interface{}{
			"image_source":        "oldimage",
			"image_os_hash_value": "thechecksum",
			"image_os_hash_algo":  "md5",
		},
	}

	provData := provisioner.ProvisionData{
		Image:        metal3api.Image{},
		BootMode:     metal3api.DefaultBootMode,
		CustomDeploy: host.Spec.CustomDeploy,
	}
	patches := prov.getUpdateOptsForNode(ironicNode, provData).Updates

	t.Logf("patches: %v", patches)

	expected := []struct {
		Path  string         // the node property path
		Key   string         // if value is a map, the key we care about
		Value interface{}    // the value being passed to ironic (or value associated with the key)
		Op    nodes.UpdateOp // The operation add/replace/remove
	}{
		{
			Path:  "/deploy_interface",
			Value: "custom-agent",
			Op:    nodes.AddOp,
		},
		{
			Path: "/instance_info/image_source",
			Op:   nodes.RemoveOp,
		},
		{
			Path: "/instance_info/image_os_hash_algo",
			Op:   nodes.RemoveOp,
		},
		{
			Path: "/instance_info/image_os_hash_value",
			Op:   nodes.RemoveOp,
		},
	}

	for _, e := range expected {
		t.Run(e.Path, func(t *testing.T) {
			t.Logf("expected: %v", e)
			var update nodes.UpdateOperation
			for _, patch := range patches {
				update = patch.(nodes.UpdateOperation)
				if update.Path == e.Path {
					break
				}
			}
			if update.Path != e.Path {
				t.Errorf("did not find %q in updates", e.Path)
				return
			}
			t.Logf("update: %v", update)
			assert.Equal(t, e.Value, update.Value, fmt.Sprintf("%s value does not match", e.Path))
			assert.Equal(t, e.Op, update.Op, fmt.Sprintf("%s operation does not match", e.Path))
		})
	}
}

func TestGetUpdateOptsForNodeSecureBoot(t *testing.T) {
	host := metal3api.BareMetalHost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "myhost",
			Namespace: "myns",
			UID:       "27720611-e5d1-45d3-ba3a-222dcfaa4ca2",
		},
		Spec: metal3api.BareMetalHostSpec{
			BMC: metal3api.BMCDetails{
				Address: "test://test.bmc/",
			},
			Image: &metal3api.Image{
				URL:          "not-empty",
				Checksum:     "checksum",
				ChecksumType: metal3api.MD5,
				DiskFormat:   ptr.To("raw"),
			},
			Online:          true,
			HardwareProfile: "unknown",
		},
		Status: metal3api.BareMetalHostStatus{
			HardwareProfile: "libvirt",
			Provisioning: metal3api.ProvisionStatus{
				ID: "provisioning-id",
			},
		},
	}

	eventPublisher := func(reason, message string) {}
	auth := clients.AuthConfig{Type: clients.NoAuth}

	prov, err := newProvisionerWithSettings(host, bmc.Credentials{}, eventPublisher, "https://ironic.test", auth)
	if err != nil {
		t.Fatal(fmt.Errorf("could not create provisioner: %w", err))
	}
	ironicNode := &nodes.Node{}

	hwProf, _ := profile.GetProfile("libvirt")
	provData := provisioner.ProvisionData{
		Image:           *host.Spec.Image,
		BootMode:        metal3api.UEFISecureBoot,
		HardwareProfile: hwProf,
	}
	patches := prov.getUpdateOptsForNode(ironicNode, provData).Updates

	t.Logf("patches: %v", patches)

	expected := []struct {
		Path  string      // the node property path
		Key   string      // if value is a map, the key we care about
		Value interface{} // the value being passed to ironic (or value associated with the key)
	}{
		{
			Path:  "/instance_info/image_source",
			Value: "not-empty",
		},
		{
			Path: "/instance_info/capabilities",
			Value: map[string]string{
				"secure_boot": "true",
			},
		},
	}

	for _, e := range expected {
		t.Run(e.Path, func(t *testing.T) {
			t.Logf("expected: %v", e)
			var update nodes.UpdateOperation
			for _, patch := range patches {
				update = patch.(nodes.UpdateOperation)
				if update.Path == e.Path {
					break
				}
			}
			if update.Path != e.Path {
				t.Errorf("did not find %q in updates", e.Path)
				return
			}
			t.Logf("update: %v", update)
			assert.Equal(t, e.Value, update.Value, fmt.Sprintf("%s does not match", e.Path))
		})
	}
}

func TestBuildCleanStepsForUpdateFirmware(t *testing.T) {
	nodeUUID := "eec38659-4c68-7431-9535-d10766f07a58"
	cases := []struct {
		name                     string
		ironic                   *testserver.IronicMock
		targetFirmwareComponents []metal3api.FirmwareUpdate
		expectedFirmwareUpdates  []map[string]string
	}{
		{
			name: "no updates",
			ironic: testserver.NewIronic(t).WithDefaultResponses().Node(nodes.Node{
				ProvisionState: string(nodes.DeployFail),
				UUID:           nodeUUID,
			}),
			targetFirmwareComponents: nil,
			expectedFirmwareUpdates:  nil,
		},
		{
			name: "bmc update",
			ironic: testserver.NewIronic(t).WithDefaultResponses().Node(nodes.Node{
				ProvisionState: string(nodes.DeployFail),
				UUID:           nodeUUID,
			}),
			targetFirmwareComponents: []metal3api.FirmwareUpdate{
				{
					Component: "bmc",
					URL:       "https://mybmc.newfirmware",
				},
			},
			expectedFirmwareUpdates: []map[string]string{
				{
					"component": "bmc",
					"url":       "https://mybmc.newfirmware",
				},
			},
		},
		{
			name: "bios update",
			ironic: testserver.NewIronic(t).WithDefaultResponses().Node(nodes.Node{
				ProvisionState: string(nodes.DeployFail),
				UUID:           nodeUUID,
			}),
			targetFirmwareComponents: []metal3api.FirmwareUpdate{
				{
					Component: "bios",
					URL:       "https://mybios.newfirmware",
				},
			},
			expectedFirmwareUpdates: []map[string]string{
				{
					"component": "bios",
					"url":       "https://mybios.newfirmware",
				},
			},
		},
		{
			name: "bmc and bios update",
			ironic: testserver.NewIronic(t).WithDefaultResponses().Node(nodes.Node{
				ProvisionState: string(nodes.DeployFail),
				UUID:           nodeUUID,
			}),
			targetFirmwareComponents: []metal3api.FirmwareUpdate{
				{
					Component: "bmc",
					URL:       "https://mybmc.newfirmware",
				},
				{
					Component: "bios",
					URL:       "https://mybios.newfirmware",
				},
			},
			expectedFirmwareUpdates: []map[string]string{
				{
					"component": "bmc",
					"url":       "https://mybmc.newfirmware",
				},
				{
					"component": "bios",
					"url":       "https://mybios.newfirmware",
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
				TargetFirmwareComponents: tc.targetFirmwareComponents,
			})

			assert.Equal(t, nil, err)
			if tc.targetFirmwareComponents == nil {
				assert.Equal(t, tc.expectedFirmwareUpdates, []map[string]string(nil))
			} else {
				settings := cleanSteps[0].Args["settings"]
				assert.ElementsMatch(t, tc.expectedFirmwareUpdates, settings)
			}
		})
	}
}
