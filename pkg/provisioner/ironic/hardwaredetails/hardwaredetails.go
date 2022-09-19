package hardwaredetails

import (
	"fmt"
	"math"
	"reflect"
	"sort"
	"strings"

	"github.com/gophercloud/gophercloud/openstack/baremetalintrospection/v1/introspection"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	metal3v1alpha1 "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
)

var log = logf.Log.WithName("baremetalhost-inspection")

// GetHardwareDetails converts Ironic introspection data into BareMetalHost HardwareDetails.
func GetHardwareDetails(data *introspection.Data) *metal3v1alpha1.HardwareDetails {
	details := new(metal3v1alpha1.HardwareDetails)
	details.Firmware = getFirmwareDetails(data.Extra.Firmware)
	details.SystemVendor = getSystemVendorDetails(data.Inventory.SystemVendor)
	details.RAMMebibytes = data.MemoryMB
	details.NIC = getNICDetails(data.Inventory.Interfaces, data.AllInterfaces, data.Extra.Network)
	details.Storage = getStorageDetails(data.Inventory.Disks)
	details.CPU = getCPUDetails(&data.Inventory.CPU)
	details.Hostname = data.Inventory.Hostname
	return details
}

func updateLLDP(value interface{}, field, t string, lldp metal3v1alpha1.LLDP) metal3v1alpha1.LLDP {
	// check for empty values of int or bool type
	if field == "switch_port_untagged_vlan_id" ||
		field == "switch_port_protocol_vlan_ids" ||
		field == "switch_port_mtu" ||
		field == "switch_port_link_aggregation_support" ||
		field == "switch_port_link_aggregation_id" ||
		field == "switch_port_autonegotiation_support" {
		if t == "string" {
			return lldp
		}
	}

	// if value to be array is empty
	if field == "switch_capabilities_enabled" ||
		field == "switch_capabilities_support" ||
		field == "switch_mgmt_addresses" {
		if t != "slice" {
			if value.(string) == "" {
				return lldp
			}
		}
	}

	switch field {
	case "switch_system_name":
		lldp.SwitchSystemName = value.(string)
	case "switch_system_description":
		lldp.SwitchSystemDescription = value.(string)
	case "switch_port_untagged_vlan_id":
		lldp.SwitchPortUntaggedVlanId = value.(int)
	case "switch_port_protocol_vlan_support":
		lldp.SwitchPortProtocolVlanSupport = value.(string)
	case "switch_port_protocol_vlan_ids":
		lldp.SwitchPortProtocolVlanIds = value.(int)
	case "switch_port_protocol_vlan_enabled":
		lldp.SwitchPortProtocolVlanEnabled = value.(string)
	case "switch_port_mtu":
		lldp.SwitchPortMTU = value.(int)
	case "switch_port_link_aggregation_support":
		lldp.SwitchPortLinkAggregationSupport = value.(bool)
	case "switch_port_link_aggregation_id":
		lldp.SwitchPortLinkAggregationId = value.(int)
	case "switch_port_link_aggregation_enabled":
		lldp.SwitchPortLinkAggregationEnabled = value.(bool)
	case "switch_port_id":
		lldp.SwitchPortId = value.(string)
	case "switch_port_description":
		lldp.SwitchPortDescription = value.(string)
	case "switch_port_autonegotiation_support":
		lldp.SwitchPortAutonegotiationSupport = value.(bool)
	case "switch_port_autonegotiation_enabled":
		lldp.SwitchPortAutonegotiationEnabled = value.(bool)
	case "switch_chassis_id":
		lldp.SwitchChassisId = value.(string)
	case "switch_capabilities_enabled":
		lldp.SwitchCapabilitiesEnabled = value.([]string)
	case "switch_capabilities_support":
		lldp.SwitchCapabilitiesSupport = value.([]string)
	case "switch_mgmt_addresses":
		lldp.SwitchMgmtAddresses = value.([]string)
	default:
		log.Info("no matching cases. field : ", field)
	}
	return lldp
}

func getLLDPProcessed(intf introspection.BaseInterfaceType) metal3v1alpha1.LLDP {
	lldp := metal3v1alpha1.LLDP{}

	if intf.LLDPProcessed == nil {
		return lldp
	}
	// loop LLDPProcessed
	for field, value := range intf.LLDPProcessed {
		if field == "switch_port_vlans" {
			vlans, _ := getVLANs(intf)
			lldp.SwitchPortVlans = vlans
		}
		// check value type
		switch value.(type) {
		case string:
			lldp = updateLLDP(value, field, "string", lldp)
		case int:
			lldp = updateLLDP(value, field, "int", lldp)
		case bool:
			lldp = updateLLDP(value, field, "bool", lldp)
		case []string:
			lldp = updateLLDP(value, field, "slice", lldp)
		default:
			log.Info(fmt.Sprintf("unexpected value type : %s // field : %s", reflect.TypeOf(value).String(), field))
		}
	}
	return lldp
}

func getVLANs(intf introspection.BaseInterfaceType) (vlans []metal3v1alpha1.VLAN, vlanid metal3v1alpha1.VLANID) {
	if intf.LLDPProcessed == nil {
		return
	}
	if spvs, ok := intf.LLDPProcessed["switch_port_vlans"]; ok {
		if data, ok := spvs.([]map[string]interface{}); ok {
			vlans = make([]metal3v1alpha1.VLAN, len(data))
			for i, vlan := range data {
				vid, _ := vlan["id"].(int)
				name, _ := vlan["name"].(string)
				vlans[i] = metal3v1alpha1.VLAN{
					ID:   metal3v1alpha1.VLANID(vid),
					Name: name,
				}
			}
		}
	}
	if vid, ok := intf.LLDPProcessed["switch_port_untagged_vlan_id"].(int); ok {
		vlanid = metal3v1alpha1.VLANID(vid)
	}
	return
}

func getNICSpeedGbps(intfExtradata introspection.ExtraHardwareData) (speedGbps int) {
	if speed, ok := intfExtradata["speed"].(string); ok {
		if strings.HasSuffix(speed, "Gbps") {
			fmt.Sscanf(speed, "%d", &speedGbps)
		}
	}
	return
}

func getNICDetails(ifdata []introspection.InterfaceType,
	basedata map[string]introspection.BaseInterfaceType,
	extradata introspection.ExtraHardwareDataSection) []metal3v1alpha1.NIC {
	var nics []metal3v1alpha1.NIC
	for _, intf := range ifdata {
		baseIntf := basedata[intf.Name]
		vlans, vlanid := getVLANs(baseIntf)
		// We still store one nic even if both ips are unset
		// if both are set, we store two nics with each ip
		if intf.IPV4Address != "" || intf.IPV6Address == "" {
			nics = append(nics, metal3v1alpha1.NIC{
				Name: intf.Name,
				Model: strings.TrimLeft(fmt.Sprintf("%s %s",
					intf.Vendor, intf.Product), " "),
				MAC:       intf.MACAddress,
				IP:        intf.IPV4Address,
				VLANs:     vlans,
				VLANID:    vlanid,
				SpeedGbps: getNICSpeedGbps(extradata[intf.Name]),
				PXE:       baseIntf.PXE,
				LLDP:      getLLDPProcessed(baseIntf),
			})
		}
		if intf.IPV6Address != "" {
			nics = append(nics, metal3v1alpha1.NIC{
				Name: intf.Name,
				Model: strings.TrimLeft(fmt.Sprintf("%s %s",
					intf.Vendor, intf.Product), " "),
				MAC:       intf.MACAddress,
				IP:        intf.IPV6Address,
				VLANs:     vlans,
				VLANID:    vlanid,
				SpeedGbps: getNICSpeedGbps(extradata[intf.Name]),
				PXE:       baseIntf.PXE,
				LLDP:      getLLDPProcessed(baseIntf),
			})
		}
	}
	return nics
}

func getDiskType(diskdata introspection.RootDiskType) metal3v1alpha1.DiskType {
	if diskdata.Rotational {
		return metal3v1alpha1.HDD
	}

	if strings.HasPrefix(diskdata.Name, "/dev/nvme") {
		return metal3v1alpha1.NVME
	}

	return metal3v1alpha1.SSD
}

func getStorageDetails(diskdata []introspection.RootDiskType) []metal3v1alpha1.Storage {
	storage := make([]metal3v1alpha1.Storage, len(diskdata))
	for i, disk := range diskdata {
		storage[i] = metal3v1alpha1.Storage{
			Name:               disk.Name,
			Rotational:         disk.Rotational,
			Type:               getDiskType(disk),
			SizeBytes:          metal3v1alpha1.Capacity(disk.Size),
			Vendor:             disk.Vendor,
			Model:              disk.Model,
			SerialNumber:       disk.Serial,
			WWN:                disk.Wwn,
			WWNVendorExtension: disk.WwnVendorExtension,
			WWNWithExtension:   disk.WwnWithExtension,
			HCTL:               disk.Hctl,
		}
	}
	return storage
}

func getSystemVendorDetails(vendor introspection.SystemVendorType) metal3v1alpha1.HardwareSystemVendor {
	return metal3v1alpha1.HardwareSystemVendor{
		Manufacturer: vendor.Manufacturer,
		ProductName:  vendor.ProductName,
		SerialNumber: vendor.SerialNumber,
	}
}

func getCPUDetails(cpudata *introspection.CPUType) metal3v1alpha1.CPU {
	var freq float64
	fmt.Sscanf(cpudata.Frequency, "%f", &freq)
	freq = math.Round(freq) // Ensure freq has no fractional part
	sort.Strings(cpudata.Flags)
	cpu := metal3v1alpha1.CPU{
		Arch:           cpudata.Architecture,
		Model:          cpudata.ModelName,
		ClockMegahertz: metal3v1alpha1.ClockSpeed(freq) * metal3v1alpha1.MegaHertz,
		Count:          cpudata.Count,
		Flags:          cpudata.Flags,
	}

	return cpu
}

func getFirmwareDetails(firmwaredata introspection.ExtraHardwareDataSection) metal3v1alpha1.Firmware {

	// handle bios optionally
	var bios metal3v1alpha1.BIOS

	if biosdata, ok := firmwaredata["bios"]; ok {
		// we do not know if all fields will be supplied
		// as this is not a structured response
		// so we must handle each field conditionally
		bios.Vendor, _ = biosdata["vendor"].(string)
		bios.Version, _ = biosdata["version"].(string)
		bios.Date, _ = biosdata["date"].(string)
	}

	return metal3v1alpha1.Firmware{
		BIOS: bios,
	}

}
