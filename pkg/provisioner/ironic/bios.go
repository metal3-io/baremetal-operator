package ironic

import (
	"reflect"

	"github.com/gophercloud/gophercloud/openstack/baremetal/v1/nodes"
	metal3v1alpha1 "github.com/metal3-io/baremetal-operator/pkg/apis/metal3/v1alpha1"
)

// buildBIOSCleanSteps builds clean steps for different drivers
func buildBIOSCleanSteps(driver string, firmware *metal3v1alpha1.FirmwareConfig) (cleanSteps []nodes.CleanStep) {
	switch driver {
	case "ibmc":
		// Unsupported yet
	case "idrac":
		cleanSteps = buildIDRACCleanSteps(firmware)
	case "ilo":
		// Unsupported yet
	case "ipmi":
		// Unsupported yet
	case "irmc":
		// Unsupported yet
	case "redfish":
		// Unsupported yet
	}
	return
}

// A private method to build the clean steps for IDRAC configuration from BaremetalHost spec
func buildIDRACCleanSteps(firmware *metal3v1alpha1.FirmwareConfig) (cleanSteps []nodes.CleanStep) {
	// This cleaning step resets all BIOS settings to factory default for a given node
	// NOTE(demoncoder95): Factory reset step on iDRAC can reset PXE interface, which might
	// cause provisioning failure. Known issue here https://docs.openstack.org/ironic/latest/admin/drivers/idrac.html#known-issues
	cleanSteps = append(
		cleanSteps,
		nodes.CleanStep{
			Interface: "bios",
			Step:      "factory_reset",
		},
	)

	// map CRD names to iDRAC BIOS config names
	// This will have list of all supported parameter names
	nameMap := map[string]string{
		"SimultaneousMultithreadingDisabled": "LogicalProc",
		"VirtualizationDisabled":             "ProcVirtualization",
		"SriovEnabled":                       "SriovGlobalEnable",
	}

	// map semantic value of bool to Ironic value
	// e-g virtualizationDisabled has semantic value of false, i.e if this param is True, Ironic param will be False
	valueMap := map[string]bool{
		"SimultaneousMultithreadingDisabled": false,
		"VirtualizationDisabled":             false,
		"SriovEnabled":                       true,
	}

	var settings []map[string]string
	if firmware != nil {
		settings = buildBIOSSettings(*firmware, nameMap, valueMap)
	} else {
		// The default configuration is always applied to the node, this
		// ensures an empty object is available to generate default firmware config
		dummyFirmwareConfig := &metal3v1alpha1.FirmwareConfig{}
		settings = buildBIOSSettings(*dummyFirmwareConfig, nameMap, valueMap)
	}
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

// A generic private method to build Ironic BIOS 'settings' struct for different vendors, using different name and value maps
func buildBIOSSettings(biosSettings interface{}, nameMap map[string]string, valueMap map[string]bool) (settings []map[string]string) {
	var value bool         // default is false
	var name string        // BIOS setting name
	var valueString string // bool converted to string for Ironic

	biosValues := reflect.ValueOf(biosSettings)
	biosTypes := reflect.TypeOf(biosSettings)

	for i := 0; i < biosValues.NumField(); i++ {
		// Only process the *bool type
		if biosValues.Field(i).Type().String() == "*bool" {
			// If value provided by user
			if !biosValues.Field(i).IsNil() {
				value = biosValues.Field(i).Elem().Bool()
			} else {
				// else, default value is false
				value = false
			}
		} else {
			// Ignore param types other than *bool
			continue
		}

		// Only process the implemented parameters listed in the nameMap
		if len(nameMap) == 0 || nameMap[biosTypes.Field(i).Name] == "" {
			continue
		} else {
			name = nameMap[biosTypes.Field(i).Name]
		}

		// Convert bool to string
		// Enabled if given value is same as semantic value
		if value == valueMap[biosTypes.Field(i).Name] {
			valueString = "Enabled"
		} else {
			valueString = "Disabled"
		}

		// Save the BIOS setting
		settings = append(
			settings,
			map[string]string{
				"name":  name,
				"value": valueString,
			},
		)
	}
	return
}
