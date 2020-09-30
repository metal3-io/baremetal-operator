package bmc

import (
	"reflect"
	"testing"

	"github.com/gophercloud/gophercloud/openstack/baremetal/v1/nodes"

	metal3v1alpha1 "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
)

func TestBuildBIOSCleanSteps(t *testing.T) {
	cases := []struct {
		name     string
		driver   string
		firmware *metal3v1alpha1.FirmwareConfig
		expected []nodes.CleanStep
	}{
		{
			name:   "idrac",
			driver: "idrac://192.168.122.1:6233/foo",
			firmware: &metal3v1alpha1.FirmwareConfig{
				VirtualizationEnabled:             "true",
				SimultaneousMultithreadingEnabled: "true",
				SriovEnabled:                      "true",
				BootOrderPolicy:                   "ResetAfterFailed",
				LLCPrefetchEnabled:                "true",
				CStateEnabled:                     "true",
				NUMAEnabled:                       "true",
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
								"name":  "LogicalProc",
								"value": "Enabled",
							},
							{
								"name":  "SriovGlobalEnable",
								"value": "Enabled",
							},
							{
								"name":  "ProcHwPrefetcher",
								"value": "Enabled",
							},
							{
								"name":  "ProcCStates",
								"value": "Enabled",
							},
							{
								"name":  "BootSeqRetry",
								"value": "Disabled",
							},
							{
								"name":  "SubNumaCluster",
								"value": "Enabled",
							},
							{
								"name":  "ProcTurboMode",
								"value": "Enabled",
							},
						},
					},
				},
			},
		},
		{
			name:   "ilo",
			driver: "ilo5://192.168.122.1",
			firmware: &metal3v1alpha1.FirmwareConfig{
				VirtualizationEnabled:             "true",
				SimultaneousMultithreadingEnabled: "true",
				SriovEnabled:                      "true",
				BootOrderPolicy:                   "ResetAfterFailed",
				LLCPrefetchEnabled:                "true",
				CStateEnabled:                     "true",
				NUMAEnabled:                       "true",
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
								"name":  "ProcHyperthreading",
								"value": "Enabled",
							},
							{
								"name":  "Sriov",
								"value": "Enabled",
							},
							{
								"name":  "BootOrderPolicy",
								"value": "ResetAfterFailed",
							},
							{
								"name":  "LlcPrefetch",
								"value": "Enabled",
							},
							{
								"name":  "SubNumaClustering",
								"value": "Enabled",
							},
							{
								"name":  "EnergyEfficientTurbo",
								"value": "Enabled",
							},
							{
								"name":  "MinProcIdlePower",
								"value": "C6",
							},
							{
								"name":  "WorkloadProfile",
								"value": "custom",
							},
						},
					},
				},
			},
		},
		{
			name:   "irmc",
			driver: "irmc://192.168.122.1",
			firmware: &metal3v1alpha1.FirmwareConfig{
				VirtualizationEnabled:             "true",
				SimultaneousMultithreadingEnabled: "true",
				SriovEnabled:                      "true",
				BootOrderPolicy:                   "ResetAfterFailed",
				LLCPrefetchEnabled:                "true",
				CStateEnabled:                     "true",
				NUMAEnabled:                       "true",
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
								"name":  "hyper_threading_enabled",
								"value": "true",
							},
							{
								"name":  "single_root_io_virtualization_support_enabled",
								"value": "true",
							},
							{
								"name":  "cpu_adjacent_cache_line_prefetch_enabled",
								"value": "true",
							},
						},
					},
				},
			},
		},
		{
			name:     "idrac firmware is nil",
			driver:   "idrac://192.168.122.1:6233/foo",
			firmware: nil,
			expected: []nodes.CleanStep{
				{
					Interface: "bios",
					Step:      "factory_reset",
				},
			},
		},
		{
			name:     "irmc firmware is nil",
			driver:   "irmc://192.168.122.1",
			firmware: nil,
			expected: nil,
		},
		{
			name:     "ilo firmware is empty",
			driver:   "ilo5://192.168.122.1",
			firmware: nil,
			expected: []nodes.CleanStep{
				{
					Interface: "bios",
					Step:      "factory_reset",
				},
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {

			ac, err := NewAccessDetails(c.driver, true)
			if err != nil {
				t.Fatal(err)
			}
			cleanStep := ac.BIOSCleanSteps(c.firmware)
			if !reflect.DeepEqual(c.expected, cleanStep) {
				t.Errorf("expected: %v, got: %v", c.expected, cleanStep)
			}
		})
	}
}
