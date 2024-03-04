package hardwaredetails

import (
	"reflect"
	"testing"

	"github.com/gophercloud/gophercloud/v2/openstack/baremetal/inventory"
	"github.com/gophercloud/gophercloud/v2/openstack/baremetalintrospection/v1/introspection"
	"github.com/stretchr/testify/assert"

	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
)

func TestGetVLANs(t *testing.T) {
	vlans, vid := getVLANs(map[string]interface{}{
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
	})
	if vid != 1 {
		t.Errorf("Unexpected untagged VLAN ID %d", vid)
	}
	if len(vlans) != 2 {
		t.Errorf("Expected 2 VLANs, got %d", len(vlans))
	}
	if (vlans[0] != metal3api.VLAN{ID: 1, Name: "vlan1"}) {
		t.Errorf("Unexpected VLAN %d %s", vlans[0].ID, vlans[0].Name)
	}
	if (vlans[1] != metal3api.VLAN{ID: 4094, Name: "vlan4094"}) {
		t.Errorf("Unexpected VLAN %d %s", vlans[1].ID, vlans[1].Name)
	}
}

func TestGetVLANsMalformed(t *testing.T) {
	vlans, vid := getVLANs(map[string]interface{}{
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
	})
	if vid != 0 {
		t.Errorf("Unexpected untagged VLAN ID %d", vid)
	}
	if len(vlans) != 5 {
		t.Errorf("Expected 5 VLANs, got %d", len(vlans))
	}
	if (vlans[0] != metal3api.VLAN{Name: "vlan1"}) {
		t.Errorf("Unexpected VLAN %d %s", vlans[0].ID, vlans[0].Name)
	}
	if (vlans[1] != metal3api.VLAN{ID: 1}) {
		t.Errorf("Unexpected VLAN %d %s", vlans[0].ID, vlans[0].Name)
	}
	if (vlans[2] != metal3api.VLAN{Name: "vlan2"}) {
		t.Errorf("Unexpected VLAN %d %s", vlans[0].ID, vlans[0].Name)
	}
	if (vlans[3] != metal3api.VLAN{ID: 3}) {
		t.Errorf("Unexpected VLAN %d %s", vlans[0].ID, vlans[0].Name)
	}
	if (vlans[4] != metal3api.VLAN{}) {
		t.Errorf("Unexpected VLAN %d %s", vlans[0].ID, vlans[0].Name)
	}

	vlans, vid = getVLANs(map[string]interface{}{
		"switch_port_vlans": map[string]interface{}{
			"foo": "bar",
		},
		"switch_port_untagged_vlan_id": "1",
	})
	if vid != 0 {
		t.Errorf("Unexpected untagged VLAN ID %d", vid)
	}
	if len(vlans) != 0 {
		t.Errorf("Expected 0 VLANs, got %d", len(vlans))
	}

	vlans, vid = getVLANs(map[string]interface{}{
		"switch_port_vlans": []interface{}{
			"foo",
		},
	})
	if vid != 0 {
		t.Errorf("Unexpected untagged VLAN ID %d", vid)
	}
	if len(vlans) != 0 {
		t.Errorf("Expected 0 VLANs, got %d", len(vlans))
	}

	vlans, vid = getVLANs(map[string]interface{}{
		"switch_port_vlans": "foo",
	})
	if vid != 0 {
		t.Errorf("Unexpected untagged VLAN ID %d", vid)
	}
	if len(vlans) != 0 {
		t.Errorf("Expected 0 VLANs, got %d", len(vlans))
	}

	vlans, vid = getVLANs(map[string]interface{}{})
	if vid != 0 {
		t.Errorf("Unexpected untagged VLAN ID %d", vid)
	}
	if len(vlans) != 0 {
		t.Errorf("Expected 0 VLANs, got %d", len(vlans))
	}
}

func TestGetNICDetailsInspector(t *testing.T) {
	ironicData := inventory.StandardPluginData{
		AllInterfaces: map[string]inventory.ProcessedInterfaceType{
			"eth0": {
				PXEEnabled: true,
			},
		},
		ParsedLLDP: map[string]inventory.ParsedLLDP{
			"eth0": {
				"switch_port_vlans": []map[string]interface{}{
					{
						"id": 1,
					},
				},
				"switch_port_untagged_vlan_id": 1,
			},
		},
	}
	inspectorData := introspection.Data{
		AllInterfaces: map[string]introspection.BaseInterfaceType{
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
	}
	interfaces := []inventory.InterfaceType{
		{
			Name:        "eth0",
			IPV4Address: "192.0.2.1",
			MACAddress:  "00:11:22:33:44:55"},
		{
			Name:        "eth1",
			IPV6Address: "2001:db8::1",
			MACAddress:  "66:77:88:99:aa:bb",
			SpeedMbps:   1000},
		{
			Name:        "eth46",
			IPV6Address: "2001:db8::2",
			IPV4Address: "192.0.2.2",
			MACAddress:  "00:11:22:33:44:66"},
		{
			Name:       "ethNoIp",
			MACAddress: "00:11:22:33:44:77"},
	}

	cases := []struct {
		name          string
		ironicData    *inventory.StandardPluginData
		inspectorData *introspection.Data
	}{
		{
			name:          "with-ironic",
			ironicData:    &ironicData,
			inspectorData: nil,
		},
		{
			name:          "with-inspector",
			ironicData:    nil,
			inspectorData: &inspectorData,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			nics := getNICDetails(interfaces, tc.ironicData, tc.inspectorData)

			// 5 expected because eth46 results in two items
			assert.Len(t, nics, 5)
			if (!reflect.DeepEqual(nics[0], metal3api.NIC{
				Name: "eth0",
				MAC:  "00:11:22:33:44:55",
				IP:   "192.0.2.1",
				PXE:  true,
				VLANs: []metal3api.VLAN{
					{ID: 1},
				},
				VLANID: 1,
			})) {
				t.Errorf("Unexpected NIC data")
			}
			if (!reflect.DeepEqual(nics[1], metal3api.NIC{
				Name:      "eth1",
				MAC:       "66:77:88:99:aa:bb",
				IP:        "2001:db8::1",
				SpeedGbps: 1,
			})) {
				t.Errorf("Unexpected NIC data")
			}
			if (!reflect.DeepEqual(nics[2], metal3api.NIC{
				Name: "eth46",
				MAC:  "00:11:22:33:44:66",
				IP:   "192.0.2.2",
			})) {
				t.Errorf("Unexpected NIC data")
			}
			if (!reflect.DeepEqual(nics[3], metal3api.NIC{
				Name: "eth46",
				MAC:  "00:11:22:33:44:66",
				IP:   "2001:db8::2",
			})) {
				t.Errorf("Unexpected NIC data")
			}
			if (!reflect.DeepEqual(nics[4], metal3api.NIC{
				Name: "ethNoIp",
				MAC:  "00:11:22:33:44:77",
			})) {
				t.Errorf("Unexpected NIC data")
			}
		})
	}
}

func TestGetFirmwareDetails(t *testing.T) {
	// Test full (known) firmware payload
	firmware := getFirmwareDetails(inventory.SystemFirmwareType{
		Vendor:    "foobar",
		Version:   "1.2.3",
		BuildDate: "2019-07-10",
	})

	if firmware.BIOS.Vendor != "foobar" {
		t.Errorf("Expected firmware BIOS vendor to be foobar, but got: %s", firmware)
	}
}
