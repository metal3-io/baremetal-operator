package ironic

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"

	"github.com/gophercloud/gophercloud/openstack/baremetal/v1/nodes"

	metal3v1alpha1 "github.com/metal3-io/baremetal-operator/pkg/apis/metal3/v1alpha1"
	"github.com/metal3-io/baremetal-operator/pkg/bmc"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner/ironic/clients"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner/ironic/testserver"
	"k8s.io/utils/pointer"
)

func init() {
	logf.SetLogger(logf.ZapLogger(true))
}

func TestProvisionerIsReady(t *testing.T) {

	cases := []struct {
		name      string
		ironic    *testserver.MockServer
		inspector *testserver.MockServer

		expectedIronicCalls    string
		expectedInspectorCalls string
		expectedIsReady        bool
		expectedError          string
	}{
		{
			name:      "IsReady",
			ironic:    testserver.New(t, "ironic").AddDrivers(),
			inspector: testserver.New(t, "inspector"),

			expectedIronicCalls:    "/v1;/v1/drivers;",
			expectedInspectorCalls: "/v1;",
			expectedIsReady:        true,
		},
		{
			name:      "NoDriversLoaded",
			ironic:    testserver.New(t, "ironic"),
			inspector: testserver.New(t, "inspector"),

			expectedIronicCalls: "/v1;/v1/drivers;",
		},
		{
			name:      "IronicDown",
			inspector: testserver.New(t, "inspector"),

			expectedIsReady: false,
		},
		{
			name:   "InspectorDown",
			ironic: testserver.New(t, "ironic").AddDrivers(),

			expectedIronicCalls: "/v1;/v1/drivers;",

			expectedIsReady: false,
		},
		{
			name:      "IronicNotOk",
			ironic:    testserver.New(t, "ironic").SetErrorCode(http.StatusInternalServerError),
			inspector: testserver.New(t, "inspector"),

			expectedIsReady: false,

			expectedIronicCalls: "/v1;",
		},
		{
			name:      "IronicNotOkAndNotExpected",
			ironic:    testserver.New(t, "ironic").SetErrorCode(http.StatusBadGateway),
			inspector: testserver.New(t, "inspector"),

			expectedIsReady: false,

			expectedIronicCalls: "/v1;",
		},
		{
			name:      "InspectorNotOk",
			ironic:    testserver.New(t, "ironic").AddDrivers(),
			inspector: testserver.New(t, "inspector").SetErrorCode(http.StatusInternalServerError),

			expectedIsReady: false,

			expectedIronicCalls:    "/v1;/v1/drivers;",
			expectedInspectorCalls: "/v1;",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.ironic != nil {
				tc.ironic.Start()
				defer tc.ironic.Stop()
			}

			if tc.inspector != nil {
				tc.inspector.Start()
				defer tc.inspector.Stop()
			}

			auth := clients.AuthConfig{Type: clients.NoAuth}

			prov, err := newProvisionerWithSettings(makeHost(), bmc.Credentials{}, nil,
				tc.ironic.Endpoint(), auth, tc.inspector.Endpoint(), auth,
			)
			ready, err := prov.IsReady()

			if tc.ironic != nil {
				assert.Equal(t, tc.expectedIronicCalls, tc.ironic.Requests)
			}
			if tc.inspector != nil {
				assert.Equal(t, tc.expectedInspectorCalls, tc.inspector.Requests)
			}

			if tc.expectedError != "" {
				assert.Regexp(t, tc.expectedError, err)
			} else {
				assert.Nil(t, err)
				assert.Equal(t, tc.expectedIsReady, ready)
			}
		})
	}
}

func TestGetUpdateOptsForNodeWithRootHints(t *testing.T) {

	eventPublisher := func(reason, message string) {}
	auth := clients.AuthConfig{Type: clients.NoAuth}

	prov, err := newProvisionerWithSettings(makeHost(), bmc.Credentials{}, eventPublisher,
		"https://ironic.test", auth, "https://ironic.test", auth,
	)
	ironicNode := &nodes.Node{}

	patches, err := prov.getUpdateOptsForNode(ironicNode)
	if err != nil {
		t.Fatal(err)
	}

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
	host := &metal3v1alpha1.BareMetalHost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "myhost",
			Namespace: "myns",
			UID:       "27720611-e5d1-45d3-ba3a-222dcfaa4ca2",
		},
		Spec: metal3v1alpha1.BareMetalHostSpec{
			Image: &metal3v1alpha1.Image{
				URL:          "not-empty",
				Checksum:     "checksum",
				ChecksumType: metal3v1alpha1.MD5,
				DiskFormat:   pointer.StringPtr("raw"),
			},
			Online: true,
		},
		Status: metal3v1alpha1.BareMetalHostStatus{
			HardwareProfile: "libvirt",
			Provisioning: metal3v1alpha1.ProvisionStatus{
				ID: "provisioning-id",
			},
		},
	}

	eventPublisher := func(reason, message string) {}
	auth := clients.AuthConfig{Type: clients.NoAuth}

	prov, err := newProvisionerWithSettings(host, bmc.Credentials{}, eventPublisher,
		"https://ironic.test", auth, "https://ironic.test", auth,
	)
	ironicNode := &nodes.Node{}

	patches, err := prov.getUpdateOptsForNode(ironicNode)
	if err != nil {
		t.Fatal(err)
	}

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
			Path:  "/instance_uuid",
			Value: "27720611-e5d1-45d3-ba3a-222dcfaa4ca2",
		},
		{
			Path:  "/instance_info/root_gb",
			Value: 10,
		},
		{
			Path:  "/properties/cpu_arch",
			Value: "x86_64",
		},
		{
			Path:  "/properties/local_gb",
			Value: 50,
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
	host := &metal3v1alpha1.BareMetalHost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "myhost",
			Namespace: "myns",
			UID:       "27720611-e5d1-45d3-ba3a-222dcfaa4ca2",
		},
		Spec: metal3v1alpha1.BareMetalHostSpec{
			Image: &metal3v1alpha1.Image{
				URL:          "not-empty",
				Checksum:     "checksum",
				ChecksumType: metal3v1alpha1.MD5,
				//DiskFormat not given to verify it is not added in instance_info
			},
			Online: true,
		},
		Status: metal3v1alpha1.BareMetalHostStatus{
			HardwareProfile: "dell",
			Provisioning: metal3v1alpha1.ProvisionStatus{
				ID: "provisioning-id",
			},
		},
	}

	eventPublisher := func(reason, message string) {}
	auth := clients.AuthConfig{Type: clients.NoAuth}

	prov, err := newProvisionerWithSettings(host, bmc.Credentials{}, eventPublisher,
		"https://ironic.test", auth, "https://ironic.test", auth,
	)
	ironicNode := &nodes.Node{}

	patches, err := prov.getUpdateOptsForNode(ironicNode)
	if err != nil {
		t.Fatal(err)
	}

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
			Path:  "/instance_info/root_gb",
			Value: 10,
		},
		{
			Path:  "/properties/cpu_arch",
			Value: "x86_64",
		},
		{
			Path:  "/properties/local_gb",
			Value: 50,
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

func makeHost() *metal3v1alpha1.BareMetalHost {
	rotational := true

	return &metal3v1alpha1.BareMetalHost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "myhost",
			Namespace: "myns",
			UID:       "27720611-e5d1-45d3-ba3a-222dcfaa4ca2",
		},
		Spec: metal3v1alpha1.BareMetalHostSpec{
			Image: &metal3v1alpha1.Image{
				URL: "not-empty",
			},
			Online:          true,
			HardwareProfile: "libvirt",
			RootDeviceHints: &metal3v1alpha1.RootDeviceHints{
				DeviceName:         "userd_devicename",
				HCTL:               "1:2:3:4",
				Model:              "userd_model",
				Vendor:             "userd_vendor",
				SerialNumber:       "userd_serial",
				MinSizeGigabytes:   40,
				WWN:                "userd_wwn",
				WWNWithExtension:   "userd_with_extension",
				WWNVendorExtension: "userd_vendor_extension",
				Rotational:         &rotational,
			},
		},
		Status: metal3v1alpha1.BareMetalHostStatus{
			Provisioning: metal3v1alpha1.ProvisionStatus{
				ID: "provisioning-id",
				// Place the hints in the status field to pretend the
				// controller has already reconciled partially.
				RootDeviceHints: &metal3v1alpha1.RootDeviceHints{
					DeviceName:         "userd_devicename",
					HCTL:               "1:2:3:4",
					Model:              "userd_model",
					Vendor:             "userd_vendor",
					SerialNumber:       "userd_serial",
					MinSizeGigabytes:   40,
					WWN:                "userd_wwn",
					WWNWithExtension:   "userd_with_extension",
					WWNVendorExtension: "userd_vendor_extension",
					Rotational:         &rotational,
				},
			},
			HardwareProfile: "libvirt",
		},
	}
}

func TestBuildCapabilitiesValue(t *testing.T) {

	cases := []struct {
		Scenario      string
		Node          nodes.Node
		Mode          metal3v1alpha1.BootMode
		ExpectedValue string
		ExpectedOp    nodes.UpdateOp
	}{
		{
			Scenario:      "unset",
			Node:          nodes.Node{},
			Mode:          metal3v1alpha1.UEFI,
			ExpectedValue: "boot_mode:uefi",
			ExpectedOp:    nodes.AddOp,
		},
		{
			Scenario: "empty",
			Node: nodes.Node{
				Properties: map[string]interface{}{
					"capabilities": "",
				},
			},
			Mode:          metal3v1alpha1.UEFI,
			ExpectedValue: "boot_mode:uefi",
			ExpectedOp:    nodes.ReplaceOp,
		},
		{
			Scenario: "not-there",
			Node: nodes.Node{
				Properties: map[string]interface{}{
					"capabilities": "cpu_vt:true,cpu_aes:true,cpu_hugepages:true,cpu_hugepages_1g:true",
				},
			},
			Mode:          metal3v1alpha1.UEFI,
			ExpectedValue: "cpu_vt:true,cpu_aes:true,cpu_hugepages:true,cpu_hugepages_1g:true,boot_mode:uefi",
			ExpectedOp:    nodes.ReplaceOp,
		},
		{
			Scenario: "uefi-to-uefi",
			Node: nodes.Node{
				Properties: map[string]interface{}{
					"capabilities": "boot_mode:uefi,cpu_vt:true,cpu_aes:true,cpu_hugepages:true,cpu_hugepages_1g:true",
				},
			},
			Mode:          metal3v1alpha1.UEFI,
			ExpectedValue: "boot_mode:uefi,cpu_vt:true,cpu_aes:true,cpu_hugepages:true,cpu_hugepages_1g:true",
			ExpectedOp:    nodes.ReplaceOp,
		},
		{
			Scenario: "bios-to-bios",
			Node: nodes.Node{
				Properties: map[string]interface{}{
					"capabilities": "boot_mode:bios,cpu_vt:true,cpu_aes:true,cpu_hugepages:true,cpu_hugepages_1g:true",
				},
			},
			Mode:          metal3v1alpha1.Legacy,
			ExpectedValue: "boot_mode:bios,cpu_vt:true,cpu_aes:true,cpu_hugepages:true,cpu_hugepages_1g:true",
			ExpectedOp:    nodes.ReplaceOp,
		},
		{
			Scenario: "bios-to-uefi",
			Node: nodes.Node{
				Properties: map[string]interface{}{
					"capabilities": "boot_mode:bios,cpu_vt:true,cpu_aes:true,cpu_hugepages:true,cpu_hugepages_1g:true",
				},
			},
			Mode:          metal3v1alpha1.UEFI,
			ExpectedValue: "boot_mode:uefi,cpu_vt:true,cpu_aes:true,cpu_hugepages:true,cpu_hugepages_1g:true",
			ExpectedOp:    nodes.ReplaceOp,
		},
		{
			Scenario: "uefi-to-bios",
			Node: nodes.Node{
				Properties: map[string]interface{}{
					"capabilities": "boot_mode:uefi,cpu_vt:true,cpu_aes:true,cpu_hugepages:true,cpu_hugepages_1g:true",
				},
			},
			Mode:          metal3v1alpha1.Legacy,
			ExpectedValue: "boot_mode:bios,cpu_vt:true,cpu_aes:true,cpu_hugepages:true,cpu_hugepages_1g:true",
			ExpectedOp:    nodes.ReplaceOp,
		},
	}

	for _, tc := range cases {
		t.Run(tc.Scenario, func(t *testing.T) {
			actualOp, actualVal := buildCapabilitiesValue(&tc.Node, tc.Mode)
			assert.Equal(t, tc.ExpectedOp, actualOp)
			assert.Equal(t, tc.ExpectedValue, actualVal)
		})
	}
}
