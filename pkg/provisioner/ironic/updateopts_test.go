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
	"github.com/metal3-io/baremetal-operator/pkg/provisioner/ironic/clients"
)

func TestGetUpdateOptsForNodeWithRootHints(t *testing.T) {

	eventPublisher := func(reason, message string) {}
	auth := clients.AuthConfig{Type: clients.NoAuth}

	prov, err := newProvisionerWithSettings(makeHost(), bmc.Credentials{}, eventPublisher,
		"https://ironic.test", auth, "https://ironic.test", auth,
	)
	if err != nil {
		t.Fatal(err)
	}
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
