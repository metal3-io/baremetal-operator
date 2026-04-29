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
			MACAddress:  "00:11:22:33:44:55",
			PCIAddress:  "0000:00:00.0"},
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
		Name:       "eth0",
		MAC:        "00:11:22:33:44:55",
		IP:         "192.0.2.1",
		PCIAddress: "0000:00:00.0",
		PXE:        true,
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

func TestGetFirmwareDetails(t *testing.T) {
	// Test full (known) firmware payload
	firmware := getFirmwareDetails(inventory.SystemFirmwareType{
		Vendor:    "foobar",
		Version:   "1.2.3",
		BuildDate: "2019-07-10",
	})

	assert.Equal(t, "foobar", firmware.BIOS.Vendor)
}

func TestGetDiskType(t *testing.T) {
	assert.Equal(t, metal3api.HDD, getDiskType(inventory.RootDiskType{Rotational: true}))
	assert.Equal(t, metal3api.NVME, getDiskType(inventory.RootDiskType{Name: "/dev/nvme0n1"}))
	assert.Equal(t, metal3api.NVME, getDiskType(inventory.RootDiskType{Name: "/dev/nvme0n1p1"}))
	assert.Equal(t, metal3api.SSD, getDiskType(inventory.RootDiskType{Name: "/dev/sda"}))
	// Rotational takes precedence over NVMe name
	assert.Equal(t, metal3api.HDD, getDiskType(inventory.RootDiskType{Rotational: true, Name: "/dev/nvme0n1"}))
	// Empty name, non-rotational
	assert.Equal(t, metal3api.SSD, getDiskType(inventory.RootDiskType{}))
}

func TestGetStorageDetails(t *testing.T) {
	disks := []inventory.RootDiskType{
		{
			Name:               "/dev/sda",
			Rotational:         true,
			Size:               500000000000,
			Vendor:             "ATA",
			Model:              "VBOX HARDDISK",
			Serial:             "VB12345678",
			Wwn:                "0x5000c123",
			WwnVendorExtension: "0x1234",
			WwnWithExtension:   "0x5000c1231234",
			Hctl:               "0:0:0:0",
		},
		{
			Name:   "/dev/nvme0n1",
			ByPath: "/dev/disk/by-path/pci-0000:00:1f.0-nvme-1",
			Size:   256000000000,
			Vendor: "Samsung",
			Model:  "SSD 970 EVO",
		},
	}

	storage := getStorageDetails(disks)
	assert.Len(t, storage, 2)

	// First disk: rotational, no ByPath — Name stays as device name
	assert.Equal(t, "/dev/sda", storage[0].Name)
	assert.Equal(t, []string{"/dev/sda"}, storage[0].AlternateNames)
	assert.True(t, storage[0].Rotational)
	assert.Equal(t, metal3api.HDD, storage[0].Type)
	assert.Equal(t, metal3api.Capacity(500000000000), storage[0].SizeBytes)
	assert.Equal(t, "ATA", storage[0].Vendor)
	assert.Equal(t, "VBOX HARDDISK", storage[0].Model)
	assert.Equal(t, "VB12345678", storage[0].SerialNumber)
	assert.Equal(t, "0x5000c123", storage[0].WWN)
	assert.Equal(t, "0x1234", storage[0].WWNVendorExtension)
	assert.Equal(t, "0x5000c1231234", storage[0].WWNWithExtension)
	assert.Equal(t, "0:0:0:0", storage[0].HCTL)

	// Second disk: NVMe with ByPath — Name becomes ByPath
	assert.Equal(t, "/dev/disk/by-path/pci-0000:00:1f.0-nvme-1", storage[1].Name)
	assert.Equal(t, []string{"/dev/nvme0n1", "/dev/disk/by-path/pci-0000:00:1f.0-nvme-1"}, storage[1].AlternateNames)
	assert.False(t, storage[1].Rotational)
	assert.Equal(t, metal3api.NVME, storage[1].Type)
	assert.Equal(t, metal3api.Capacity(256000000000), storage[1].SizeBytes)
}

func TestGetStorageDetailsEmpty(t *testing.T) {
	storage := getStorageDetails([]inventory.RootDiskType{})
	assert.Empty(t, storage)
}

func TestGetCPUDetails(t *testing.T) {
	cpu := getCPUDetails(&inventory.CPUType{
		Architecture: "x86_64",
		ModelName:    "Intel(R) Core(TM) i7-8650U",
		Frequency:    "1900.0000",
		Count:        8,
		Flags:        []string{"vmx", "avx", "aes"},
	})
	assert.Equal(t, "x86_64", cpu.Arch)
	assert.Equal(t, "Intel(R) Core(TM) i7-8650U", cpu.Model)
	assert.Equal(t, metal3api.ClockSpeed(1900)*metal3api.MegaHertz, cpu.ClockMegahertz)
	assert.Equal(t, 8, cpu.Count)
	// Flags should be sorted
	assert.Equal(t, []string{"aes", "avx", "vmx"}, cpu.Flags)
}

func TestGetCPUDetailsRounding(t *testing.T) {
	cpu := getCPUDetails(&inventory.CPUType{
		Frequency: "2499.998",
	})
	assert.Equal(t, metal3api.ClockSpeed(2500)*metal3api.MegaHertz, cpu.ClockMegahertz)
}

func TestGetSystemVendorDetails(t *testing.T) {
	vendor := getSystemVendorDetails(inventory.SystemVendorType{
		Manufacturer: "Dell Inc.",
		ProductName:  "PowerEdge R640",
		SerialNumber: "ABC1234",
	})
	assert.Equal(t, "Dell Inc.", vendor.Manufacturer)
	assert.Equal(t, "PowerEdge R640", vendor.ProductName)
	assert.Equal(t, "ABC1234", vendor.SerialNumber)
}

func TestGetSystemVendorDetailsEmpty(t *testing.T) {
	vendor := getSystemVendorDetails(inventory.SystemVendorType{})
	assert.Empty(t, vendor.Manufacturer)
	assert.Empty(t, vendor.ProductName)
	assert.Empty(t, vendor.SerialNumber)
}
