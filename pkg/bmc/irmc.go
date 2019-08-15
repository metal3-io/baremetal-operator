package bmc

import "reflect"

type iRMCAccessDetails struct {
	bmcType  string
	portNum  string
	hostname string
}

func (a *iRMCAccessDetails) Type() string {
	return a.bmcType
}

// NeedsMAC returns true when the host is going to need a separate
// port created rather than having it discovered.
func (a *iRMCAccessDetails) NeedsMAC() bool {
	return false
}

func (a *iRMCAccessDetails) Driver() string {
	return "irmc"
}

// DriverInfo returns a data structure to pass as the DriverInfo
// parameter when creating a node in Ironic. The structure is
// pre-populated with the access information, and the caller is
// expected to add any other information that might be needed (such as
// the kernel and ramdisk locations).
func (a *iRMCAccessDetails) DriverInfo(bmcCreds Credentials) map[string]interface{} {
	result := map[string]interface{}{
		"irmc_username": bmcCreds.Username,
		"irmc_password": bmcCreds.Password,
		"irmc_address":  a.hostname,
	}

	if a.portNum != "" {
		result["irmc_port"] = a.portNum
	}
	
	return result
}

func (a *iRMCAccessDetails) BootInterface() string {
	return "pxe"
}
// GetBIOSConfigDetails return the mapping of supported BIOS configuration keys between Metal3 and iRMC.
// If the user use the key that does not belong to this map, this key wil be marked as unsupported in iRMC
func (a *iRMCAccessDetails) GetBIOSConfigDetails() map[string]VendorBIOSConfigSpec {
	return iRMCBiosConfigDetails
}

var iRMCBiosConfigDetails = map[string]VendorBIOSConfigSpec {
		// Specifies from which drives can be booted
		"bootOptionFilter": {
			VendorKey: "boot_option_filter",
			ValueType: reflect.String,
			SupportedValues: []interface{}{"UefiAndLegacy", "LegacyOnly", "UefiOnly"},
		},
		// The UEFI FW checks the controller health status.
		"checkControllersHealthStatusEnabled": {
			VendorKey: "check_controllers_health_status_enabled",
			ValueType: reflect.Bool,
		},
		// The number of active processor cores 1…n. Option 0 indicates that all
		// available processor cores are active.
		"cpuActiveProcessorCores": {
			VendorKey: "cpu_active_processor_cores",
			ValueType: reflect.Int,
		},
		// The processor loads the requested cache line and the adjacent cache line
		"cpuAdjacentCacheLinePrefetchEnabled": {
			VendorKey: "cpu_adjacent_cache_line_prefetch_enabled",
			ValueType: reflect.Bool,
		},
		// Supports the virtualization of platform hardware and several software
		// environments, based on Virtual Machine Extensions to support the use of
		// several software environments using virtual computers
		"cpuVtEnabled": {
			VendorKey: "cpu_vt_enabled",
			ValueType: reflect.Bool,
		},
		// The system BIOS can be written. Flash BIOS update is possible
		"flashWriteEnabled": {
			VendorKey: "flash_write_enabled",
			ValueType: reflect.Bool,
		},
		// Hyper-threading technology allows a single physical processor core to
		// appear as several logical processors.
		"hyperThreadingEnabled": {
			VendorKey: "hyper_threading_enabled",
			ValueType: reflect.Bool,
		},
		// Boot Options will not be removed from “Boot Option Priority” list
		"keepVoidBootOptionsEnabled":{
			VendorKey: "keep_void_boot_options_enabled",
			ValueType: reflect.Bool,
		},
		// Specifies whether the Compatibility Support Module (CSM) is executed
		"launchCsmEnabled": {
			VendorKey: "launch_csm_enabled",
			ValueType: reflect.Bool,
		},
		// Prevents the OS from overruling any energy efficiency policy setting of the setup
		"osEnergyPerformanceOverrideEnabled":{
			VendorKey: "os_energy_performance_override_enabled",
			ValueType: reflect.Bool,
		},
		// Active State Power Management (ASPM) is used to power-manage the PCI
		// Express links, thus consuming less power
		"pciAspmSupport": {
			VendorKey: "pci_aspm_support",
			ValueType: reflect.String,
			SupportedValues: []interface{}{"Disabled", "Auto", "L0Limited", "L1only", "L0Force"},
		},
		// Specifies if memory resources above the 4GB address boundary can be assigned to PCI devices
		"pciAbove4gDecodingEnabled": {
			VendorKey: "pci_above_4g_decoding_enabled",
			ValueType: reflect.Bool,
		},
		// Specifies whether the switch on sources for the system are managed by
		// the BIOS or the ACPI operating system
		"powerOnSource": {
			VendorKey: "power_on_source",
			ValueType: reflect.String,
			SupportedValues: []interface{}{"BiosControlled", "AcpiControlled"},
		},
		// Single Root IO Virtualization Support is active
		"singleRootIoVirtualizationSupportEnabled": {
			VendorKey: "single_root_io_virtualization_support_enabled",
			ValueType: reflect.Bool,
		},
	}
