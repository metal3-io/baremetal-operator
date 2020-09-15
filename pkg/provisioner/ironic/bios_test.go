package ironic

import (
	"reflect"
	"testing"

	"github.com/gophercloud/gophercloud/openstack/baremetal/v1/nodes"
	metal3v1alpha1 "github.com/metal3-io/baremetal-operator/pkg/apis/metal3/v1alpha1"
)

func TestBuildBIOSCleanSteps(t *testing.T) {
	// Default settings that will be applied if the FirmwareConfig is missing for idrac
	idracDefaultSettings := []map[string]string{
		{
			"name":  "SriovGlobalEnable",
			"value": "Disabled",
		},
		{
			"name":  "LogicalProc",
			"value": "Enabled",
		},
		{
			"name":  "ProcVirtualization",
			"value": "Enabled",
		},
	}
	var FALSE bool = false
	var TRUE bool = true
	cases := []struct {
		name     string
		driver   string
		firmware *metal3v1alpha1.FirmwareConfig
		expected []nodes.CleanStep
	}{
		// Normal case with iDRAC
		{
			name:   "idrac normal",
			driver: "idrac",
			firmware: &metal3v1alpha1.FirmwareConfig{
				SriovEnabled:                       &FALSE,
				SimultaneousMultithreadingDisabled: &FALSE,
				VirtualizationDisabled:             &TRUE,
			},
			expected: []nodes.CleanStep{
				{
					Interface: "bios",
					Step:      "factory_reset",
				},
				{
					Interface: "bios",
					Step:      "apply_configuration",
					Args: map[string]interface{}{
						"settings": []map[string]string{
							{
								"name":  "SriovGlobalEnable",
								"value": "Disabled",
							},
							{
								"name":  "LogicalProc",
								"value": "Enabled",
							},
							{
								"name":  "ProcVirtualization",
								"value": "Disabled",
							},
						},
					},
				},
			},
		},
		// Firmware is nil with iDRAC
		{
			name:     "firmware is nil - idrac",
			driver:   "idrac",
			firmware: nil,
			expected: []nodes.CleanStep{
				{
					Interface: "bios",
					Step:      "factory_reset",
				},
				{
					Interface: "bios",
					Step:      "apply_configuration",
					Args: map[string]interface{}{
						"settings": idracDefaultSettings,
					},
				},
			},
		},
		// Firmware is empty with iDRAC
		{
			name:     "firmware is empty - idrac",
			driver:   "idrac",
			firmware: &metal3v1alpha1.FirmwareConfig{},
			expected: []nodes.CleanStep{
				{
					Interface: "bios",
					Step:      "factory_reset",
				},
				{
					Interface: "bios",
					Step:      "apply_configuration",
					Args: map[string]interface{}{
						"settings": idracDefaultSettings,
					},
				},
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cleanStep := buildBIOSCleanSteps(c.driver, c.firmware)
			if !reflect.DeepEqual(c.expected, cleanStep) {
				t.Errorf("expected: %v, got: %v", c.expected, cleanStep)
			}
		})
	}
}
