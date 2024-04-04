package hardwaredetails

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/go-logr/logr"
	"github.com/gophercloud/gophercloud/v2/openstack/baremetal/inventory"
	"github.com/gophercloud/gophercloud/v2/openstack/baremetal/v1/nodes"
	"github.com/gophercloud/gophercloud/v2/openstack/baremetalintrospection/v1/introspection"

	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
)

// GetHardwareDetails converts Ironic introspection data into BareMetalHost HardwareDetails.
func GetHardwareDetails(data *nodes.InventoryData, logger logr.Logger) *metal3api.HardwareDetails {
	ironicData, inspectorData, err := data.PluginData.GuessFormat()
	if err != nil {
		logger.Error(err, "cannot get plugin data from inventory, some fields will not be available")
	}

	details := new(metal3api.HardwareDetails)
	details.Firmware = getFirmwareDetails(data.Inventory.SystemVendor.Firmware)
	details.SystemVendor = getSystemVendorDetails(data.Inventory.SystemVendor)
	details.RAMMebibytes = data.Inventory.Memory.PhysicalMb
	details.NIC = getNICDetails(data.Inventory.Interfaces, ironicData, inspectorData)
	details.Storage = getStorageDetails(data.Inventory.Disks)
	details.CPU = getCPUDetails(&data.Inventory.CPU)
	details.Hostname = data.Inventory.Hostname
	return details
}

func getVLANs(lldp map[string]interface{}) (vlans []metal3api.VLAN, vlanid metal3api.VLANID) {
	if lldp == nil {
		return
	}
	if spvs, ok := lldp["switch_port_vlans"]; ok {
		if data, ok := spvs.([]map[string]interface{}); ok {
			vlans = make([]metal3api.VLAN, len(data))
			for i, vlan := range data {
				vid, _ := vlan["id"].(int)
				name, _ := vlan["name"].(string)
				vlans[i] = metal3api.VLAN{
					ID:   metal3api.VLANID(vid),
					Name: name,
				}
			}
		}
	}
	if vid, ok := lldp["switch_port_untagged_vlan_id"].(int); ok {
		vlanid = metal3api.VLANID(vid)
	}
	return
}

func getNICDetails(ifdata []inventory.InterfaceType,
	ironicData *inventory.StandardPluginData,
	inspectorData *introspection.Data) []metal3api.NIC {
	var nics []metal3api.NIC
	for _, intf := range ifdata {
		var lldp map[string]interface{}
		var pxeEnabled bool
		if ironicData != nil {
			pxeEnabled = ironicData.AllInterfaces[intf.Name].PXEEnabled
			lldp = ironicData.ParsedLLDP[intf.Name]
		} else {
			pxeEnabled = inspectorData.AllInterfaces[intf.Name].PXE
			lldp = inspectorData.AllInterfaces[intf.Name].LLDPProcessed
		}

		vlans, vlanid := getVLANs(lldp)
		// We still store one nic even if both ips are unset
		// if both are set, we store two nics with each ip
		if intf.IPV4Address != "" || intf.IPV6Address == "" {
			nics = append(nics, metal3api.NIC{
				Name: intf.Name,
				Model: strings.TrimLeft(fmt.Sprintf("%s %s",
					intf.Vendor, intf.Product), " "),
				MAC:       intf.MACAddress,
				IP:        intf.IPV4Address,
				VLANs:     vlans,
				VLANID:    vlanid,
				SpeedGbps: intf.SpeedMbps / 1000,
				PXE:       pxeEnabled,
			})
		}
		if intf.IPV6Address != "" {
			nics = append(nics, metal3api.NIC{
				Name: intf.Name,
				Model: strings.TrimLeft(fmt.Sprintf("%s %s",
					intf.Vendor, intf.Product), " "),
				MAC:       intf.MACAddress,
				IP:        intf.IPV6Address,
				VLANs:     vlans,
				VLANID:    vlanid,
				SpeedGbps: intf.SpeedMbps / 1000,
				PXE:       pxeEnabled,
			})
		}
	}
	return nics
}

func getDiskType(diskdata inventory.RootDiskType) metal3api.DiskType {
	if diskdata.Rotational {
		return metal3api.HDD
	}

	if strings.HasPrefix(diskdata.Name, "/dev/nvme") {
		return metal3api.NVME
	}

	return metal3api.SSD
}

func getStorageDetails(diskdata []inventory.RootDiskType) []metal3api.Storage {
	storage := make([]metal3api.Storage, len(diskdata))
	for i, disk := range diskdata {
		device := disk.Name
		allNames := []string{device}
		if disk.ByPath != "" {
			device = disk.ByPath
			allNames = append(allNames, device)
		}
		storage[i] = metal3api.Storage{
			Name:               device,
			AlternateNames:     allNames,
			Rotational:         disk.Rotational,
			Type:               getDiskType(disk),
			SizeBytes:          metal3api.Capacity(disk.Size),
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

func getSystemVendorDetails(vendor inventory.SystemVendorType) metal3api.HardwareSystemVendor {
	return metal3api.HardwareSystemVendor{
		Manufacturer: vendor.Manufacturer,
		ProductName:  vendor.ProductName,
		SerialNumber: vendor.SerialNumber,
	}
}

func getCPUDetails(cpudata *inventory.CPUType) metal3api.CPU {
	var freq float64
	fmt.Sscanf(cpudata.Frequency, "%f", &freq)
	freq = math.Round(freq) // Ensure freq has no fractional part
	sort.Strings(cpudata.Flags)
	cpu := metal3api.CPU{
		Arch:           cpudata.Architecture,
		Model:          cpudata.ModelName,
		ClockMegahertz: metal3api.ClockSpeed(freq) * metal3api.MegaHertz,
		Count:          cpudata.Count,
		Flags:          cpudata.Flags,
	}

	return cpu
}

func getFirmwareDetails(firmwaredata inventory.SystemFirmwareType) metal3api.Firmware {
	return metal3api.Firmware{
		BIOS: metal3api.BIOS{
			Vendor:  firmwaredata.Vendor,
			Version: firmwaredata.Version,
			Date:    firmwaredata.BuildDate,
		},
	}
}
