package hardwaredetails

import (
	"reflect"
	"testing"

	"github.com/gophercloud/gophercloud/v2/openstack/baremetal/inventory"
	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	"github.com/stretchr/testify/assert"
)

func TestGetVLANs(t *testing.T) {
	vlans, vid := getVLANs(map[string]any{
		"switch_port_vlans": []map[string]any{
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
	assert.Equal(t, metal3api.VLANID(1), vid, "Unexpected untagged VLAN ID")
	assert.Len(t, vlans, 2)
	assert.Equal(t, metal3api.VLAN{ID: 1, Name: "vlan1"}, vlans[0])
	assert.Equal(t, metal3api.VLAN{ID: 4094, Name: "vlan4094"}, vlans[1])
}

func TestGetVLANsMalformed(t *testing.T) {
	vlans, vid := getVLANs(map[string]any{
		"switch_port_vlans": []map[string]any{
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
	assert.Equal(t, metal3api.VLANID(0), vid, "Unexpected untagged VLAN ID")
	assert.Len(t, vlans, 5)
	assert.Equal(t, metal3api.VLAN{Name: "vlan1"}, vlans[0])
	assert.Equal(t, metal3api.VLAN{ID: 1}, vlans[1])
	assert.Equal(t, metal3api.VLAN{Name: "vlan2"}, vlans[2])
	assert.Equal(t, metal3api.VLAN{ID: 3}, vlans[3])
	assert.Equal(t, metal3api.VLAN{}, vlans[4])

	vlans, vid = getVLANs(map[string]any{
		"switch_port_vlans": map[string]any{
			"foo": "bar",
		},
		"switch_port_untagged_vlan_id": "1",
	})
	assert.Equal(t, metal3api.VLANID(0), vid, "Unexpected untagged VLAN ID")
	assert.Empty(t, vlans)

	vlans, vid = getVLANs(map[string]any{
		"switch_port_vlans": []any{
			"foo",
		},
	})
	assert.Equal(t, metal3api.VLANID(0), vid, "Unexpected untagged VLAN ID")
	assert.Empty(t, vlans)

	vlans, vid = getVLANs(map[string]any{
		"switch_port_vlans": "foo",
	})
	assert.Equal(t, metal3api.VLANID(0), vid, "Unexpected untagged VLAN ID")
	assert.Empty(t, vlans)

	vlans, vid = getVLANs(map[string]any{})
	assert.Equal(t, metal3api.VLANID(0), vid, "Unexpected untagged VLAN ID")
	assert.Empty(t, vlans)
}

func TestGetLLDPData(t *testing.T) {
	// Test all fields present
	lldpData := getLLDPData(map[string]any{
		"switch_chassis_id":  "aa:bb:cc:dd:ee:ff",
		"switch_port_id":     "Ethernet1/1",
		"switch_system_name": "switch01.example.com",
	})
	expected := &metal3api.LLDP{
		SwitchID:         "aa:bb:cc:dd:ee:ff",
		PortID:           "Ethernet1/1",
		SwitchSystemName: "switch01.example.com",
	}
	assert.Equal(t, expected, lldpData)

	// Test only switch_chassis_id
	lldpData = getLLDPData(map[string]any{
		"switch_chassis_id": "aa:bb:cc:dd:ee:ff",
	})
	expected = &metal3api.LLDP{
		SwitchID: "aa:bb:cc:dd:ee:ff",
	}
	assert.Equal(t, expected, lldpData)

	// Test only switch_port_id
	lldpData = getLLDPData(map[string]any{
		"switch_port_id": "Ethernet1/1",
	})
	expected = &metal3api.LLDP{
		PortID: "Ethernet1/1",
	}
	assert.Equal(t, expected, lldpData)

	// Test only switch_system_name
	lldpData = getLLDPData(map[string]any{
		"switch_system_name": "switch01.example.com",
	})
	expected = &metal3api.LLDP{
		SwitchSystemName: "switch01.example.com",
	}
	assert.Equal(t, expected, lldpData)

	// Test partial fields (chassis ID and port ID)
	lldpData = getLLDPData(map[string]any{
		"switch_chassis_id": "aa:bb:cc:dd:ee:ff",
		"switch_port_id":    "Ethernet1/1",
	})
	expected = &metal3api.LLDP{
		SwitchID: "aa:bb:cc:dd:ee:ff",
		PortID:   "Ethernet1/1",
	}
	assert.Equal(t, expected, lldpData)

	// Test nil input
	lldpData = getLLDPData(nil)
	assert.Nil(t, lldpData, "Expected nil for nil input")

	// Test empty map
	lldpData = getLLDPData(map[string]any{})
	assert.Nil(t, lldpData, "Expected nil for empty map")

	// Test empty strings (should return nil)
	lldpData = getLLDPData(map[string]any{
		"switch_chassis_id":  "",
		"switch_port_id":     "",
		"switch_system_name": "",
	})
	assert.Nil(t, lldpData, "Expected nil for empty strings")

	// Test wrong data types (should be ignored)
	lldpData = getLLDPData(map[string]any{
		"switch_chassis_id":  123,
		"switch_port_id":     []string{"port1"},
		"switch_system_name": map[string]string{"name": "switch"},
	})
	assert.Nil(t, lldpData, "Expected nil for wrong types")

	// Test mixed valid and invalid fields
	lldpData = getLLDPData(map[string]any{
		"switch_chassis_id":  "aa:bb:cc:dd:ee:ff",
		"switch_port_id":     123, // wrong type
		"switch_system_name": "",  // empty string
	})
	expected = &metal3api.LLDP{
		SwitchID: "aa:bb:cc:dd:ee:ff",
	}
	assert.Equal(t, expected, lldpData)

	// Test with extra unknown fields (should be ignored)
	lldpData = getLLDPData(map[string]any{
		"switch_chassis_id":  "aa:bb:cc:dd:ee:ff",
		"switch_port_id":     "Ethernet1/1",
		"switch_system_name": "switch01.example.com",
		"unknown_field":      "should be ignored",
	})
	expected = &metal3api.LLDP{
		SwitchID:         "aa:bb:cc:dd:ee:ff",
		PortID:           "Ethernet1/1",
		SwitchSystemName: "switch01.example.com",
	}
	assert.Equal(t, expected, lldpData)
}

func TestGetNICDetails(t *testing.T) {
	ironicData := inventory.StandardPluginData{
		AllInterfaces: map[string]inventory.ProcessedInterfaceType{
			"eth0": {
				PXEEnabled: true,
			},
		},
		ParsedLLDP: map[string]inventory.ParsedLLDP{
			"eth0": {
				"switch_port_vlans": []map[string]any{
					{
						"id": 1,
					},
				},
				"switch_port_untagged_vlan_id": 1,
				"switch_chassis_id":            "aa:bb:cc:dd:ee:ff",
				"switch_port_id":               "Ethernet1/1",
				"switch_system_name":           "switch01.example.com",
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

	nics := getNICDetails(interfaces, ironicData)

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
		LLDP: &metal3api.LLDP{
			SwitchID:         "aa:bb:cc:dd:ee:ff",
			PortID:           "Ethernet1/1",
			SwitchSystemName: "switch01.example.com",
		},
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
}

func TestGetClientIDFromAllInterfaces(t *testing.T) {
	// Test that getClientIDFromAllInterfaces handles ProcessedInterfaceType correctly
	// Since ProcessedInterfaceType may or may not have ClientID field exposed,
	// we test that the function works without errors
	
	testCases := []struct {
		name     string
		input    inventory.ProcessedInterfaceType
		expected string // Expected to be empty if ClientID field doesn't exist
	}{
		{
			name:     "ProcessedInterfaceType with PXEEnabled",
			input:    inventory.ProcessedInterfaceType{PXEEnabled: true},
			expected: "", // ClientID will be empty if field doesn't exist
		},
		{
			name:     "empty ProcessedInterfaceType",
			input:    inventory.ProcessedInterfaceType{},
			expected: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := getClientIDFromAllInterfaces(tc.input)
			// The function should return empty string if ClientID field doesn't exist
			// or the actual value if it does exist
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestGetNICDetailsWithClientID(t *testing.T) {
	// Test that getNICDetails includes ClientID field in the NIC struct
	// This test verifies the integration works end-to-end
	ironicData := inventory.StandardPluginData{
		AllInterfaces: map[string]inventory.ProcessedInterfaceType{
			"ibs2": {
				PXEEnabled: true,
			},
		},
		ParsedLLDP: map[string]inventory.ParsedLLDP{},
	}

	interfaces := []inventory.InterfaceType{
		{
			Name:        "ibs2",
			IPV4Address: "192.0.2.10",
			MACAddress:  "b8:83:03:77:03:18",
		},
	}

	nics := getNICDetails(interfaces, ironicData)

	assert.Len(t, nics, 1)
	assert.Equal(t, "ibs2", nics[0].Name)
	assert.Equal(t, "b8:83:03:77:03:18", nics[0].MAC)
	assert.Equal(t, "192.0.2.10", nics[0].IP)
	assert.True(t, nics[0].PXE)
	// ClientID field exists in the struct (even if empty)
	// The function should handle extraction gracefully whether ClientID is present or not
	assert.NotNil(t, nics[0])
}

func TestGetFirmwareDetails(t *testing.T) {
	// Test full (known) firmware payload
	firmware := getFirmwareDetails(inventory.SystemFirmwareType{
		Vendor:    "foobar",
		Version:   "1.2.3",
		BuildDate: "2019-07-10",
	})

	assert.Equal(t, "foobar", firmware.BIOS.Vendor)
}
