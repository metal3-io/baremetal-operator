package ironic

import (
	"fmt"
	"testing"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"

	"github.com/gophercloud/gophercloud/v2/openstack/baremetal/v1/nodes"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	"github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1/profile"
	"github.com/metal3-io/baremetal-operator/pkg/hardwareutils/bmc"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner/ironic/clients"
)

func TestOptionValueEqual(t *testing.T) {
	cases := []struct {
		Name   string
		Before interface{}
		After  interface{}
		Equal  bool
	}{
		{
			Name:   "nil interface",
			Before: nil,
			After:  "foo",
			Equal:  false,
		},
		{
			Name:   "equal string",
			Before: "foo",
			After:  "foo",
			Equal:  true,
		},
		{
			Name:   "unequal string",
			Before: "foo",
			After:  "bar",
			Equal:  false,
		},
		{
			Name:   "equal true",
			Before: true,
			After:  true,
			Equal:  true,
		},
		{
			Name:   "equal false",
			Before: false,
			After:  false,
			Equal:  true,
		},
		{
			Name:   "unequal true",
			Before: true,
			After:  false,
			Equal:  false,
		},
		{
			Name:   "unequal false",
			Before: false,
			After:  true,
			Equal:  false,
		},
		{
			Name:   "equal int",
			Before: 42,
			After:  42,
			Equal:  true,
		},
		{
			Name:   "unequal int",
			Before: 27,
			After:  42,
			Equal:  false,
		},
		{
			Name:   "string int",
			Before: "42",
			After:  42,
			Equal:  false,
		},
		{
			Name:   "int string",
			Before: 42,
			After:  "42",
			Equal:  false,
		},
		{
			Name:   "bool int",
			Before: false,
			After:  0,
			Equal:  false,
		},
		{
			Name:   "int bool",
			Before: 1,
			After:  true,
			Equal:  false,
		},
		{
			Name:   "string map",
			Before: "foo",
			After:  map[string]string{"foo": "foo"},
			Equal:  false,
		},
		{
			Name:   "map string",
			Before: map[string]interface{}{"foo": "foo"},
			After:  "foo",
			Equal:  false,
		},
		{
			Name:   "string list",
			Before: "foo",
			After:  []string{"foo"},
			Equal:  false,
		},
		{
			Name:   "list string",
			Before: []string{"foo"},
			After:  "foo",
			Equal:  false,
		},
		{
			Name:   "map list",
			Before: map[string]interface{}{"foo": "foo"},
			After:  []string{"foo"},
			Equal:  false,
		},
		{
			Name:   "list map",
			Before: []string{"foo"},
			After:  map[string]string{"foo": "foo"},
			Equal:  false,
		},
		{
			Name:   "equal map string-typed",
			Before: map[string]interface{}{"foo": "bar"},
			After:  map[string]string{"foo": "bar"},
			Equal:  true,
		},
		{
			Name:   "unequal map string-typed",
			Before: map[string]interface{}{"foo": "bar"},
			After:  map[string]string{"foo": "baz"},
			Equal:  false,
		},
		{
			Name:   "equal map int-typed",
			Before: map[string]interface{}{"foo": 42},
			After:  map[string]int{"foo": 42},
			Equal:  true,
		},
		{
			Name:   "unequal map int-typed",
			Before: map[string]interface{}{"foo": "bar"},
			After:  map[string]int{"foo": 42},
			Equal:  false,
		},
		{
			Name:   "equal map",
			Before: map[string]interface{}{"foo": "bar", "42": 42},
			After:  map[string]interface{}{"foo": "bar", "42": 42},
			Equal:  true,
		},
		{
			Name:   "unequal map",
			Before: map[string]interface{}{"foo": "bar", "42": 42},
			After:  map[string]interface{}{"foo": "bar", "42": 27},
			Equal:  false,
		},
		{
			Name:   "equal map empty string",
			Before: map[string]interface{}{"foo": ""},
			After:  map[string]interface{}{"foo": ""},
			Equal:  true,
		},
		{
			Name:   "unequal map replace empty string",
			Before: map[string]interface{}{"foo": ""},
			After:  map[string]interface{}{"foo": "bar"},
			Equal:  false,
		},
		{
			Name:   "unequal map replace with empty string",
			Before: map[string]interface{}{"foo": "bar"},
			After:  map[string]interface{}{"foo": ""},
			Equal:  false,
		},
		{
			Name:   "shorter map",
			Before: map[string]interface{}{"foo": "bar", "42": 42},
			After:  map[string]interface{}{"foo": "bar"},
			Equal:  false,
		},
		{
			Name:   "longer map",
			Before: map[string]interface{}{"foo": "bar"},
			After:  map[string]interface{}{"foo": "bar", "42": 42},
			Equal:  false,
		},
		{
			Name:   "different map",
			Before: map[string]interface{}{"foo": "bar"},
			After:  map[string]interface{}{"baz": "bar"},
			Equal:  false,
		},
		{
			Name:   "equal list string-typed",
			Before: []interface{}{"foo", "bar"},
			After:  []string{"foo", "bar"},
			Equal:  true,
		},
		{
			Name:   "unequal list string-typed",
			Before: []interface{}{"foo", "bar"},
			After:  []string{"foo", "baz"},
			Equal:  false,
		},
		{
			Name:   "equal list",
			Before: []interface{}{"foo", 42},
			After:  []interface{}{"foo", 42},
			Equal:  true,
		},
		{
			Name:   "unequal list",
			Before: []interface{}{"foo", 42},
			After:  []interface{}{"foo", 27},
			Equal:  false,
		},
		{
			Name:   "shorter list",
			Before: []interface{}{"foo", 42},
			After:  []interface{}{"foo"},
			Equal:  false,
		},
		{
			Name:   "longer list",
			Before: []interface{}{"foo"},
			After:  []interface{}{"foo", 42},
			Equal:  false,
		},
	}

	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			var ne = "="
			if c.Equal {
				ne = "!"
			}
			assert.Equal(t, c.Equal, optionValueEqual(c.Before, c.After),
				fmt.Sprintf("%v %s= %v", c.Before, ne, c.After))
		})
	}
}

func TestGetUpdateOperation(t *testing.T) {
	var nilp *string
	barExist := "bar"
	bar := "bar"
	baz := "baz"
	existingData := map[string]interface{}{
		"foo":  "bar",
		"foop": &barExist,
		"nil":  nilp,
	}
	cases := []struct {
		Name          string
		Field         string
		NewValue      interface{}
		ExpectedOp    nodes.UpdateOp
		ExpectedValue string
	}{
		{
			Name:       "add value",
			Field:      "baz",
			NewValue:   "quux",
			ExpectedOp: nodes.AddOp,
		},
		{
			Name:          "add value pointer",
			Field:         "baz",
			NewValue:      &bar,
			ExpectedValue: bar,
			ExpectedOp:    nodes.AddOp,
		},
		{
			Name:          "add pointer value pointer",
			Field:         "nil",
			NewValue:      &bar,
			ExpectedValue: bar,
			ExpectedOp:    nodes.AddOp,
		},
		{
			Name:     "keep value",
			Field:    "foo",
			NewValue: "bar",
		},
		{
			Name:     "keep pointer value",
			Field:    "foop",
			NewValue: "bar",
		},
		{
			Name:     "keep value pointer",
			Field:    "foo",
			NewValue: &bar,
		},
		{
			Name:     "keep pointer value pointer",
			Field:    "foop",
			NewValue: &bar,
		},
		{
			Name:       "change value",
			Field:      "foo",
			NewValue:   "baz",
			ExpectedOp: nodes.AddOp,
		},
		{
			Name:       "change pointer value",
			Field:      "foop",
			NewValue:   "baz",
			ExpectedOp: nodes.AddOp,
		},
		{
			Name:          "change value pointer",
			Field:         "foo",
			NewValue:      &baz,
			ExpectedValue: baz,
			ExpectedOp:    nodes.AddOp,
		},
		{
			Name:          "change pointer value pointer",
			Field:         "foop",
			NewValue:      &baz,
			ExpectedValue: baz,
			ExpectedOp:    nodes.AddOp,
		},
		{
			Name:       "delete value",
			Field:      "foo",
			NewValue:   nil,
			ExpectedOp: nodes.RemoveOp,
		},
		{
			Name:       "delete value pointer",
			Field:      "foo",
			NewValue:   nilp,
			ExpectedOp: nodes.RemoveOp,
		},
		{
			Name:     "nonexistent value",
			Field:    "bar",
			NewValue: nil,
		},
	}

	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			path := fmt.Sprintf("test/%s", c.Field)
			updateOp := getUpdateOperation(
				c.Field, existingData,
				c.NewValue,
				path, logr.Logger{})

			switch c.ExpectedOp {
			case nodes.AddOp, nodes.ReplaceOp:
				assert.NotNil(t, updateOp)
				ev := c.ExpectedValue
				if ev == "" {
					ev = c.NewValue.(string)
				}
				assert.Equal(t, c.ExpectedOp, updateOp.Op)
				assert.Equal(t, ev, updateOp.Value)
				assert.Equal(t, path, updateOp.Path)
			case nodes.RemoveOp:
				assert.NotNil(t, updateOp)
				assert.Equal(t, c.ExpectedOp, updateOp.Op)
				assert.Equal(t, path, updateOp.Path)
			default:
				assert.Nil(t, updateOp)
			}
		})
	}
}

func TestTopLevelUpdateOpt(t *testing.T) {
	u := updateOptsBuilder(logf.Log)
	u.SetTopLevelOpt("foo", "baz", "bar")
	ops := u.Updates
	assert.Len(t, ops, 1)
	op := ops[0].(nodes.UpdateOperation)
	assert.Equal(t, nodes.AddOp, op.Op)
	assert.Equal(t, "baz", op.Value)
	assert.Equal(t, "/foo", op.Path)

	u = updateOptsBuilder(logf.Log)
	u.SetTopLevelOpt("foo", "bar", "bar")
	assert.Len(t, u.Updates, 0)
}

func TestPropertiesUpdateOpts(t *testing.T) {
	newValues := optionsData{
		"foo": "bar",
		"baz": "quux",
	}
	node := nodes.Node{
		Properties: map[string]interface{}{
			"foo": "bar",
		},
	}

	u := updateOptsBuilder(logf.Log)
	u.SetPropertiesOpts(newValues, &node)
	ops := u.Updates
	assert.Len(t, ops, 1)
	op := ops[0].(nodes.UpdateOperation)
	assert.Equal(t, nodes.AddOp, op.Op)
	assert.Equal(t, "quux", op.Value)
	assert.Equal(t, "/properties/baz", op.Path)
}

func TestInstanceInfoUpdateOpts(t *testing.T) {
	newValues := optionsData{
		"foo": "bar",
		"baz": "quux",
	}
	node := nodes.Node{
		InstanceInfo: map[string]interface{}{
			"foo": "bar",
		},
	}

	u := updateOptsBuilder(logf.Log)
	u.SetInstanceInfoOpts(newValues, &node)
	ops := u.Updates
	assert.Len(t, ops, 1)
	op := ops[0].(nodes.UpdateOperation)
	assert.Equal(t, nodes.AddOp, op.Op)
	assert.Equal(t, "quux", op.Value)
	assert.Equal(t, "/instance_info/baz", op.Path)
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
				DiskFormat:   pointer.StringPtr("raw"),
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
		t.Fatal(errors.Wrap(err, "could not create provisioner"))
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
				DiskFormat:   pointer.StringPtr("raw"),
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
		t.Fatal(errors.Wrap(err, "could not create provisioner"))
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

func TestSanitisedValue(t *testing.T) {
	unchanged := []interface{}{
		"foo",
		42,
		true,
		[]string{"bar", "baz"},
		map[string]string{"foo": "bar"},
		map[string][]string{"foo": {"bar", "baz"}},
		map[string]interface{}{"foo": []string{"bar", "baz"}, "bar": 42},
	}

	for _, u := range unchanged {
		assert.Exactly(t, u, sanitisedValue(u))
	}

	unsafe := map[string]interface{}{
		"foo":           "bar",
		"password":      "secret",
		"ipmi_password": "secret",
	}
	safe := map[string]interface{}{
		"foo":           "bar",
		"password":      "<redacted>",
		"ipmi_password": "<redacted>",
	}
	assert.Exactly(t, safe, sanitisedValue(unsafe))
}
