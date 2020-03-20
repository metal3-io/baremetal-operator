package ironic

import (
	"reflect"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"

	"github.com/gophercloud/gophercloud/openstack/baremetal/v1/nodes"
	"github.com/gophercloud/gophercloud/openstack/baremetalintrospection/v1/introspection"

	metal3v1alpha1 "github.com/metal3-io/baremetal-operator/pkg/apis/metal3/v1alpha1"
	"github.com/metal3-io/baremetal-operator/pkg/bmc"
)

func init() {
	logf.SetLogger(logf.ZapLogger(true))
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
				URL: "not-empty",
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

	prov, err := newProvisioner(host, bmc.Credentials{}, eventPublisher)
	ironicNode := &nodes.Node{}

	patches, err := prov.getUpdateOptsForNode(ironicNode, "checksum")
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("patches: %v", patches)

	var update nodes.UpdateOperation

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
			Path:  "/instance_info/image_checksum",
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
			Path:  "/instance_info/root_device",
			Value: "/dev/vda",
			Key:   "name",
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

	for i, e := range expected {
		update = patches[i].(nodes.UpdateOperation)
		if e.Key != "" {
			m := update.Value.(map[string]string)
			if m[e.Key] != e.Value {
				t.Errorf("expected %s=%q got %s=%q", e.Path, e.Value, update.Path, update.Value)
			}
		} else {
			if update.Value != e.Value {
				t.Errorf("expected %s=%q got %s=%q", e.Path, e.Value, update.Path, update.Value)
			}
		}
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
				URL: "not-empty",
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

	prov, err := newProvisioner(host, bmc.Credentials{}, eventPublisher)
	ironicNode := &nodes.Node{}

	patches, err := prov.getUpdateOptsForNode(ironicNode, "checksum")
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("patches: %v", patches)

	var update nodes.UpdateOperation

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
			Path:  "/instance_info/image_checksum",
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
			Path:  "/instance_info/root_device",
			Value: "0:0:0:0",
			Key:   "hctl",
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

	for i, e := range expected {
		update = patches[i].(nodes.UpdateOperation)
		if e.Key != "" {
			m := update.Value.(map[string]string)
			if m[e.Key] != e.Value {
				t.Errorf("expected %s=%q got %s=%q", e.Path, e.Value, update.Path, update.Value)
			}
		} else {
			if update.Value != e.Value {
				t.Errorf("expected %s=%q got %s=%q", e.Path, e.Value, update.Path, update.Value)
			}
		}
	}
}

func TestGetVLANs(t *testing.T) {
	vlans, vid := getVLANs(introspection.BaseInterfaceType{
		LLDPProcessed: map[string]interface{}{
			"switch_port_vlans": []map[string]interface{}{
				map[string]interface{}{
					"id":   1,
					"name": "vlan1",
				},
				map[string]interface{}{
					"id":   4094,
					"name": "vlan4094",
				},
			},
			"switch_port_untagged_vlan_id": 1,
		},
	})
	if vid != 1 {
		t.Errorf("Unexpected untagged VLAN ID %d", vid)
	}
	if len(vlans) != 2 {
		t.Errorf("Expected 2 VLANs, got %d", len(vlans))
	}
	if (vlans[0] != metal3v1alpha1.VLAN{ID: 1, Name: "vlan1"}) {
		t.Errorf("Unexpected VLAN %d %s", vlans[0].ID, vlans[0].Name)
	}
	if (vlans[1] != metal3v1alpha1.VLAN{ID: 4094, Name: "vlan4094"}) {
		t.Errorf("Unexpected VLAN %d %s", vlans[1].ID, vlans[1].Name)
	}
}

func TestGetVLANsMalformed(t *testing.T) {
	vlans, vid := getVLANs(introspection.BaseInterfaceType{
		LLDPProcessed: map[string]interface{}{
			"switch_port_vlans": []map[string]interface{}{
				map[string]interface{}{
					"foo":  "bar",
					"name": "vlan1",
				},
				map[string]interface{}{
					"foo": "bar",
					"id":  1,
				},
				map[string]interface{}{
					"name": "vlan2",
					"id":   "2",
				},
				map[string]interface{}{
					"name": 3,
					"id":   3,
				},
				map[string]interface{}{
					"foo": "bar",
				},
			},
			"switch_port_untagged_vlan_id": "1",
		},
	})
	if vid != 0 {
		t.Errorf("Unexpected untagged VLAN ID %d", vid)
	}
	if len(vlans) != 5 {
		t.Errorf("Expected 5 VLANs, got %d", len(vlans))
	}
	if (vlans[0] != metal3v1alpha1.VLAN{Name: "vlan1"}) {
		t.Errorf("Unexpected VLAN %d %s", vlans[0].ID, vlans[0].Name)
	}
	if (vlans[1] != metal3v1alpha1.VLAN{ID: 1}) {
		t.Errorf("Unexpected VLAN %d %s", vlans[0].ID, vlans[0].Name)
	}
	if (vlans[2] != metal3v1alpha1.VLAN{Name: "vlan2"}) {
		t.Errorf("Unexpected VLAN %d %s", vlans[0].ID, vlans[0].Name)
	}
	if (vlans[3] != metal3v1alpha1.VLAN{ID: 3}) {
		t.Errorf("Unexpected VLAN %d %s", vlans[0].ID, vlans[0].Name)
	}
	if (vlans[4] != metal3v1alpha1.VLAN{}) {
		t.Errorf("Unexpected VLAN %d %s", vlans[0].ID, vlans[0].Name)
	}

	vlans, vid = getVLANs(introspection.BaseInterfaceType{
		LLDPProcessed: map[string]interface{}{
			"switch_port_vlans": map[string]interface{}{
				"foo": "bar",
			},
			"switch_port_untagged_vlan_id": "1",
		},
	})
	if vid != 0 {
		t.Errorf("Unexpected untagged VLAN ID %d", vid)
	}
	if len(vlans) != 0 {
		t.Errorf("Expected 0 VLANs, got %d", len(vlans))
	}

	vlans, vid = getVLANs(introspection.BaseInterfaceType{
		LLDPProcessed: map[string]interface{}{
			"switch_port_vlans": []interface{}{
				"foo",
			},
		},
	})
	if vid != 0 {
		t.Errorf("Unexpected untagged VLAN ID %d", vid)
	}
	if len(vlans) != 0 {
		t.Errorf("Expected 0 VLANs, got %d", len(vlans))
	}

	vlans, vid = getVLANs(introspection.BaseInterfaceType{
		LLDPProcessed: map[string]interface{}{
			"switch_port_vlans": "foo",
		},
	})
	if vid != 0 {
		t.Errorf("Unexpected untagged VLAN ID %d", vid)
	}
	if len(vlans) != 0 {
		t.Errorf("Expected 0 VLANs, got %d", len(vlans))
	}

	vlans, vid = getVLANs(introspection.BaseInterfaceType{
		LLDPProcessed: map[string]interface{}{},
	})
	if vid != 0 {
		t.Errorf("Unexpected untagged VLAN ID %d", vid)
	}
	if len(vlans) != 0 {
		t.Errorf("Expected 0 VLANs, got %d", len(vlans))
	}

	vlans, vid = getVLANs(introspection.BaseInterfaceType{})
	if vid != 0 {
		t.Errorf("Unexpected untagged VLAN ID %d", vid)
	}
	if len(vlans) != 0 {
		t.Errorf("Expected 0 VLANs, got %d", len(vlans))
	}
}

func TestGetNICDetails(t *testing.T) {
	nics := getNICDetails(
		[]introspection.InterfaceType{
			introspection.InterfaceType{
				Name: "eth0",
				IPV4Address: "192.0.2.1",
				MACAddress: "00:11:22:33:44:55"},
			introspection.InterfaceType{
				Name: "eth1",
				IPV6Address: "2001:db8::1",
				MACAddress: "66:77:88:99:aa:bb"},
		},
		map[string]introspection.BaseInterfaceType{
			"eth0": introspection.BaseInterfaceType{
				PXE: true,
				LLDPProcessed: map[string]interface{}{
					"switch_port_vlans": []map[string]interface{}{
						map[string]interface{}{
							"id": 1,
						},
					},
					"switch_port_untagged_vlan_id": 1,
				},
			},
		},
		introspection.ExtraHardwareDataSection{
			"eth1": introspection.ExtraHardwareData{
				"speed": "1Gbps",
			},
		})

	if len(nics) != 2 {
		t.Errorf("Expected 2 NICs, got %d", len(nics))
	}
	if (!reflect.DeepEqual(nics[0], metal3v1alpha1.NIC{
		Name: "eth0",
		MAC:  "00:11:22:33:44:55",
		IP:   "192.0.2.1",
		PXE:  true,
		VLANs: []metal3v1alpha1.VLAN{
			metal3v1alpha1.VLAN{ID: 1},
		},
		VLANID: 1,
	})) {
		t.Errorf("Unexpected NIC data")
	}
	if (!reflect.DeepEqual(nics[1], metal3v1alpha1.NIC{
		Name:      "eth1",
		MAC:       "66:77:88:99:aa:bb",
		IP:        "2001:db8::1",
		SpeedGbps: 1,
	})) {
		t.Errorf("Unexpected NIC data")
	}
}

func TestGetNICSpeedGbps(t *testing.T) {
	s1 := getNICSpeedGbps(introspection.ExtraHardwareData{
		"speed": "25Gbps",
	})
	if s1 != 25 {
		t.Errorf("Expected speed 25, got %d", s1)
	}

	s2 := getNICSpeedGbps(introspection.ExtraHardwareData{
		"speed": "100Mbps",
	})
	if s2 != 0 {
		t.Errorf("Expected speed 0, got %d", s2)
	}

	s3 := getNICSpeedGbps(introspection.ExtraHardwareData{
		"speed": 10,
	})
	if s3 != 0 {
		t.Errorf("Expected speed 0, got %d", s3)
	}

	s4 := getNICSpeedGbps(introspection.ExtraHardwareData{})
	if s4 != 0 {
		t.Errorf("Expected speed 0, got %d", s4)
	}
}

func TestGetFirmwareDetails(t *testing.T) {

	// Test full (known) firmware payload
	firmware := getFirmwareDetails(introspection.ExtraHardwareDataSection{
		"bios": {
			"vendor":  "foobar",
			"version": "1.2.3",
			"date":    "2019-07-10",
		},
	})

	if firmware.BIOS.Vendor != "foobar" {
		t.Errorf("Expected firmware BIOS vendor to be foobar, but got: %s", firmware)
	}

	// Ensure we can handle partial firmware/bios data
	firmware = getFirmwareDetails(introspection.ExtraHardwareDataSection{
		"bios": {
			"vendor":  "foobar",
			"version": "1.2.3",
		},
	})

	if firmware.BIOS.Date != "" {
		t.Errorf("Expected firmware BIOS date to be empty but got: %s", firmware)
	}

	// Ensure we can handle unexpected types
	firmware = getFirmwareDetails(introspection.ExtraHardwareDataSection{
		"bios": {
			"vendor":  3,
			"version": []int{2, 1},
			"date":    map[string]string{"year": "2019", "month": "07", "day": "10"},
		},
	})

	// Finally, ensure we can handle completely empty firmware data
	firmware = getFirmwareDetails(introspection.ExtraHardwareDataSection{})

	if (firmware != metal3v1alpha1.Firmware{}) {
		t.Errorf("Expected firmware data to be empty but got: %s", firmware)
	}

}
