package ironic

import (
	"reflect"
	"strconv"

	"github.com/gophercloud/gophercloud/openstack/baremetal/v1/nodes"
	metal3v1alpha1 "github.com/metal3-io/baremetal-operator/pkg/apis/metal3/v1alpha1"
)

// buildBIOSCleanSteps build clean steps for different driver
func buildBIOSCleanSteps(driver string, firmware *metal3v1alpha1.FirmwareConfig) (cleanSteps []nodes.CleanStep) {
	switch driver {
	case "ibmc":
		// Unsupported yet
	case "idrac":
		cleanSteps = buildIDRACCleanSteps(firmware)
	case "ilo":
		cleanSteps = buildILOCleanSteps(firmware)
	case "ipmi":
		// Unsupported yet
	case "irmc":
		cleanSteps = buildIRMCCleanSteps(firmware)
	case "redfish":
		// Unsupported yet
	}
	return
}

// A private method to build the clean steps for IDRAC configuration from BaremetalHost spec
func buildIDRACCleanSteps(firmware *metal3v1alpha1.FirmwareConfig) (cleanSteps []nodes.CleanStep) {
	// This cleaning step resets all BIOS settings to factory default for a given node
	cleanSteps = append(
		cleanSteps,
		nodes.CleanStep{
			Interface: "bios",
			Step:      "factory_reset",
		},
	)
	// If not configure IDRAC, only need to clear old configuration
	if firmware == nil || firmware.IDRAC == nil {
		return
	}
	// Build bios settings
	// Build public bios settings
	settings := buildBIOSSettings(*firmware,
		map[string]string{
			"SimultaneousMultithreadingEnabled": "LogicalProc",
			"VirtualizationEnabled":             "ProcVirtualization",
		},
		map[string]string{
			"true":  "Enabled",
			"false": "Disabled",
		},
	)
	// build private bios settings
	settings = append(settings, buildBIOSSettings(*firmware.IDRAC,
		map[string]string{
			"CPUInterconnectBusLinkPower": "CpuInterconnectBusLinkPower",
			"DcuIPPrefetcher":             "DcuIpPrefetcher",
		},
		nil,
	)...)
	// This cleaning step applies a set of BIOS settings for a node
	cleanSteps = append(
		cleanSteps,
		nodes.CleanStep{
			Interface: "bios",
			Step:      "apply_configuration",
			Args: map[string]interface{}{
				"settings": settings,
			},
		},
	)
	return
}

// A private method to build the clean steps for ILO configuration from BaremetalHost spec
func buildILOCleanSteps(firmware *metal3v1alpha1.FirmwareConfig) (cleanSteps []nodes.CleanStep) {
	// This cleaning step resets all BIOS settings to factory default for a given node
	cleanSteps = append(
		cleanSteps,
		nodes.CleanStep{
			Interface: "bios",
			Step:      "factory_reset",
		},
	)
	// If not configure ILO, only need to clear old configuration
	if firmware == nil || firmware.ILO == nil {
		return
	}
	// Build bios settings
	// Build public bios settings
	settings := buildBIOSSettings(*firmware,
		map[string]string{
			"SimultaneousMultithreadingEnabled": "ProcHyperthreading",
			"VirtualizationEnabled":             "ProcVirtualization",
		},
		map[string]string{
			"true":  "Enabled",
			"false": "Disabled",
		},
	)
	// build private bios settings
	settings = append(settings, buildBIOSSettings(*firmware.ILO, nil, nil)...)
	// This cleaning step applies a set of BIOS settings for a node
	cleanSteps = append(
		cleanSteps,
		nodes.CleanStep{
			Interface: "bios",
			Step:      "apply_configuration",
			Args: map[string]interface{}{
				"settings": settings,
			},
		},
	)
	return
}

// A private method to build the clean steps for IRMC configuration from BaremetalHost spec
func buildIRMCCleanSteps(firmware *metal3v1alpha1.FirmwareConfig) (cleanSteps []nodes.CleanStep) {
	// If not configure irmc, only need to clear old configuration,
	// but irmc bios interface does not support factory_reset.
	if firmware == nil || firmware.IRMC == nil {
		return
	}
	// Build bios settings
	// Build public bios settings
	settings := buildBIOSSettings(*firmware,
		map[string]string{
			"SimultaneousMultithreadingEnabled": "hyper_threading_enabled",
			"VirtualizationEnabled":             "cpu_vt_enabled",
		},
		nil,
	)
	// build private bios settings
	settings = append(settings, buildBIOSSettings(*firmware.IRMC,
		map[string]string{
			"BootOptionFilter":                         "boot_option_filter",
			"CheckControllersHealthStatusEnabled":      "check_controllers_health_status_enabled",
			"CPUActiveProcessorCores":                  "cpu_active_processor_cores",
			"CPUAdjacentCacheLinePrefetchEnabled":      "cpu_adjacent_cache_line_prefetch_enabled",
			"FlashWriteEnabled":                        "flash_write_enabled",
			"KeepVoidBootOptionsEnabled":               "keep_void_boot_options_enabled",
			"LaunchCsmEnabled":                         "launch_csm_enabled",
			"OsEnergyPerformanceOverrideEnabled":       "os_energy_performance_override_enabled",
			"PciAspmSupport":                           "pci_aspm_support",
			"PciAbove4gDecodingEnabled":                "pci_above_4g_decoding_enabled",
			"PowerOnSource":                            "power_on_source",
			"SingleRootIoVirtualizationSupportEnabled": "single_root_io_virtualization_support_enabled",
		},
		nil,
	)...)
	// This cleaning step applies a set of BIOS settings for a node
	cleanSteps = append(
		cleanSteps,
		nodes.CleanStep{
			Interface: "bios",
			Step:      "apply_configuration",
			Args: map[string]interface{}{
				"settings": settings,
			},
		},
	)
	return
}

// A private method to build bios settings from different driver
func buildBIOSSettings(biosSettings interface{}, nameMap map[string]string, valueMap map[string]string) (settings []map[string]string) {
	var value string
	var name string
	v := reflect.ValueOf(biosSettings)
	t := reflect.TypeOf(biosSettings)
	for i := 0; v.NumField() > i; i++ {
		switch v.Field(i).Kind() {
		case reflect.String:
			value = v.Field(i).String()
		case reflect.Bool:
			value = strconv.FormatBool(v.Field(i).Bool())
		case reflect.Float32, reflect.Float64:
			value = strconv.FormatFloat(v.Field(i).Float(), 'f', -1, 64)
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			value = strconv.FormatInt(v.Field(i).Int(), 10)
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			value = strconv.FormatUint(v.Field(i).Uint(), 10)
		default:
			value = ""
		}
		if value == "" {
			continue
		}
		name = t.Field(i).Name
		if len(nameMap) != 0 && nameMap[t.Field(i).Name] != "" {
			name = nameMap[t.Field(i).Name]
		}
		if len(valueMap) != 0 && valueMap[value] != "" {
			value = valueMap[value]
		}
		settings = append(
			settings,
			map[string]string{
				"name":  name,
				"value": value,
			},
		)
	}
	return
}
