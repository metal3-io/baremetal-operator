package ironic

import (
	"reflect"
	"testing"

	"github.com/gophercloud/gophercloud/openstack/baremetal/v1/nodes"
	metal3v1alpha1 "github.com/metal3-io/baremetal-operator/pkg/apis/metal3/v1alpha1"
)

func TestBuildBIOSCleanSteps(t *testing.T) {
	var settings []map[string]string
	cases := []struct {
		name     string
		driver   string
		firmware *metal3v1alpha1.FirmwareConfig
		expected []nodes.CleanStep
	}{
		// normal
		{
			name:   "idrac",
			driver: "idrac",
			firmware: &metal3v1alpha1.FirmwareConfig{
				VirtualizationEnabled: "true",
				IDRAC: &metal3v1alpha1.IDRACConfig{
					DcuIPPrefetcher: "Enabled",
					RtidSetting:     "Enabled",
				},
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
								"name":  "ProcVirtualization",
								"value": "Enabled",
							},
							{
								"name":  "DcuIpPrefetcher",
								"value": "Enabled",
							},
							{
								"name":  "RtidSetting",
								"value": "Enabled",
							},
						},
					},
				},
			},
		},
		{
			name:   "irmc",
			driver: "irmc",
			firmware: &metal3v1alpha1.FirmwareConfig{
				VirtualizationEnabled: "true",
				IRMC: &metal3v1alpha1.IRMCConfig{
					CPUActiveProcessorCores:             "true",
					CPUAdjacentCacheLinePrefetchEnabled: 0,
				},
			},
			expected: []nodes.CleanStep{
				{
					Interface: "bios",
					Step:      "apply_configuration",
					Args: map[string]interface{}{
						"settings": []map[string]string{
							{
								"name":  "cpu_vt_enabled",
								"value": "true",
							},
							{
								"name":  "cpu_active_processor_cores",
								"value": "true",
							},
							{
								"name":  "cpu_adjacent_cache_line_prefetch_enabled",
								"value": "0",
							},
						},
					},
				},
			},
		},
		// nil
		{
			name:   "idrac is nil",
			driver: "idrac",
			firmware: &metal3v1alpha1.FirmwareConfig{
				IDRAC: nil,
			},
			expected: []nodes.CleanStep{
				{
					Interface: "bios",
					Step:      "factory_reset",
				},
			},
		},
		{
			name:   "irmc is nil",
			driver: "irmc",
			firmware: &metal3v1alpha1.FirmwareConfig{
				IRMC: nil,
			},
			expected: nil,
		},
		// empty
		{
			name:   "idrac is empty",
			driver: "idrac",
			firmware: &metal3v1alpha1.FirmwareConfig{
				IDRAC: &metal3v1alpha1.IDRACConfig{},
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
						"settings": settings,
					},
				},
			},
		},
		{
			name:   "irmc is empty",
			driver: "irmc",
			firmware: &metal3v1alpha1.FirmwareConfig{
				IRMC: &metal3v1alpha1.IRMCConfig{},
			},
			expected: []nodes.CleanStep{
				{
					Interface: "bios",
					Step:      "apply_configuration",
					Args: map[string]interface{}{
						"settings": []map[string]string{
							{
								"name":  "cpu_adjacent_cache_line_prefetch_enabled",
								"value": "0",
							},
						},
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
