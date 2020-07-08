package hardwaredetails

import (
	"reflect"
	"testing"

	"github.com/gophercloud/gophercloud/openstack/baremetalintrospection/v1/introspection"

	metal3v1alpha1 "github.com/metal3-io/baremetal-operator/pkg/apis/metal3/v1alpha1"
)

func TestGetVLANs(t *testing.T) {
	vlans, vid := getVLANs(introspection.BaseInterfaceType{
		LLDPProcessed: map[string]interface{}{
			"switch_port_vlans": []map[string]interface{}{
				{
					"id":   1,
					"name": "vlan1",
				},
				{
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
				{
					"foo":  "bar",
					"name": "vlan1",
				},
				{
					"foo": "bar",
					"id":  1,
				},
				{
					"name": "vlan2",
					"id":   "2",
				},
				{
					"name": 3,
					"id":   3,
				},
				{
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
			{
				Name:        "eth0",
				IPV4Address: "192.0.2.1",
				MACAddress:  "00:11:22:33:44:55"},
			{
				Name:        "eth1",
				IPV6Address: "2001:db8::1",
				MACAddress:  "66:77:88:99:aa:bb"},
		},
		map[string]introspection.BaseInterfaceType{
			"eth0": {
				PXE: true,
				LLDPProcessed: map[string]interface{}{
					"switch_port_vlans": []map[string]interface{}{
						{
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
			{ID: 1},
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
