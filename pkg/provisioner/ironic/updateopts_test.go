package ironic

import (
	"fmt"
	"testing"

	"github.com/gophercloud/gophercloud/openstack/baremetal/v1/nodes"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"

	metal3v1alpha1 "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	"github.com/metal3-io/baremetal-operator/pkg/bmc"
	"github.com/metal3-io/baremetal-operator/pkg/hardware"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner/ironic/clients"
)

func TestGetUpdateOptsForNodeWithRootHints(t *testing.T) {

	eventPublisher := func(reason, message string) {}
	auth := clients.AuthConfig{Type: clients.NoAuth}

	host := makeHost()
	prov, err := newProvisionerWithSettings(host, bmc.Credentials{}, eventPublisher,
		"https://ironic.test", auth, "https://ironic.test", auth,
	)
	if err != nil {
		t.Fatal(err)
	}
	ironicNode := &nodes.Node{}

	provData := provisioner.ProvisionData{
		BootMode:        metal3v1alpha1.DefaultBootMode,
		RootDeviceHints: host.Status.Provisioning.RootDeviceHints,
	}
	patches, err := prov.getUpdateOptsForNode(ironicNode, provData)
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
	host := metal3v1alpha1.BareMetalHost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "myhost",
			Namespace: "myns",
			UID:       "27720611-e5d1-45d3-ba3a-222dcfaa4ca2",
		},
		Spec: metal3v1alpha1.BareMetalHostSpec{
			BMC: metal3v1alpha1.BMCDetails{
				Address: "test://test.bmc/",
			},
			Image: &metal3v1alpha1.Image{
				URL:          "not-empty",
				Checksum:     "checksum",
				ChecksumType: metal3v1alpha1.MD5,
				DiskFormat:   pointer.StringPtr("raw"),
			},
			Online:          true,
			HardwareProfile: "unknown",
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
	if err != nil {
		t.Fatal(errors.Wrap(err, "could not create provisioner"))
	}
	ironicNode := &nodes.Node{}

	hwProf, _ := hardware.GetProfile("libvirt")
	provData := provisioner.ProvisionData{
		BootMode:        metal3v1alpha1.DefaultBootMode,
		HardwareProfile: hwProf,
	}
	patches, err := prov.getUpdateOptsForNode(ironicNode, provData)
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
			Path:  "/instance_info/capabilities",
			Value: map[string]string{},
		},
		{
			Path:  "/instance_uuid",
			Value: "27720611-e5d1-45d3-ba3a-222dcfaa4ca2",
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
	host := metal3v1alpha1.BareMetalHost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "myhost",
			Namespace: "myns",
			UID:       "27720611-e5d1-45d3-ba3a-222dcfaa4ca2",
		},
		Spec: metal3v1alpha1.BareMetalHostSpec{
			BMC: metal3v1alpha1.BMCDetails{
				Address: "test://test.bmc/",
			},
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
	if err != nil {
		t.Fatal(err)
	}
	ironicNode := &nodes.Node{}

	hwProf, _ := hardware.GetProfile("dell")
	provData := provisioner.ProvisionData{
		BootMode:        metal3v1alpha1.DefaultBootMode,
		HardwareProfile: hwProf,
	}
	patches, err := prov.getUpdateOptsForNode(ironicNode, provData)
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

func TestGetUpdateOptsForNodeLiveIso(t *testing.T) {
	eventPublisher := func(reason, message string) {}
	auth := clients.AuthConfig{Type: clients.NoAuth}

	prov, err := newProvisionerWithSettings(makeHostLiveIso(), bmc.Credentials{}, eventPublisher,
		"https://ironic.test", auth, "https://ironic.test", auth,
	)
	if err != nil {
		t.Fatal(err)
	}
	ironicNode := &nodes.Node{}

	provData := provisioner.ProvisionData{
		BootMode: metal3v1alpha1.DefaultBootMode,
	}
	patches, err := prov.getUpdateOptsForNode(ironicNode, provData)
	if err != nil {
		t.Fatal(err)
	}

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
			Op:    nodes.ReplaceOp,
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

	prov, err := newProvisionerWithSettings(makeHostLiveIso(), bmc.Credentials{}, eventPublisher,
		"https://ironic.test", auth, "https://ironic.test", auth,
	)
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
		BootMode: metal3v1alpha1.DefaultBootMode,
	}
	patches, err := prov.getUpdateOptsForNode(ironicNode, provData)
	if err != nil {
		t.Fatal(err)
	}

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
			Op:    nodes.ReplaceOp,
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
	host.Spec.Image.ChecksumType = metal3v1alpha1.MD5
	prov, err := newProvisionerWithSettings(host, bmc.Credentials{}, eventPublisher,
		"https://ironic.test", auth, "https://ironic.test", auth,
	)
	if err != nil {
		t.Fatal(err)
	}
	ironicNode := &nodes.Node{
		InstanceInfo: map[string]interface{}{
			"boot_iso": "oldimage",
		},
	}

	provData := provisioner.ProvisionData{
		BootMode: metal3v1alpha1.DefaultBootMode,
	}
	patches, err := prov.getUpdateOptsForNode(ironicNode, provData)
	if err != nil {
		t.Fatal(err)
	}

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
			Path:  "/deploy_interface",
			Value: "direct",
			Op:    nodes.ReplaceOp,
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

func TestGetUpdateOptsForNodeSecureBoot(t *testing.T) {
	host := metal3v1alpha1.BareMetalHost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "myhost",
			Namespace: "myns",
			UID:       "27720611-e5d1-45d3-ba3a-222dcfaa4ca2",
		},
		Spec: metal3v1alpha1.BareMetalHostSpec{
			BMC: metal3v1alpha1.BMCDetails{
				Address: "test://test.bmc/",
			},
			Image: &metal3v1alpha1.Image{
				URL:          "not-empty",
				Checksum:     "checksum",
				ChecksumType: metal3v1alpha1.MD5,
				DiskFormat:   pointer.StringPtr("raw"),
			},
			Online:          true,
			HardwareProfile: "unknown",
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
	if err != nil {
		t.Fatal(errors.Wrap(err, "could not create provisioner"))
	}
	ironicNode := &nodes.Node{}

	hwProf, _ := hardware.GetProfile("libvirt")
	provData := provisioner.ProvisionData{
		BootMode:        metal3v1alpha1.UEFISecureBoot,
		HardwareProfile: hwProf,
	}
	patches, err := prov.getUpdateOptsForNode(ironicNode, provData)
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
