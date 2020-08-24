package ironic

import (
	"reflect"
	"testing"

	"github.com/gophercloud/gophercloud/openstack/baremetal/v1/nodes"
	metal3v1alpha1 "github.com/metal3-io/baremetal-operator/pkg/apis/metal3/v1alpha1"
)

func TestBuildTargetRAIDCfg(t *testing.T) {
	var TRUE bool = true
	var FALSE bool = false
	cases := []struct {
		name          string
		raid          *metal3v1alpha1.RAIDConfig
		expected      []nodes.LogicalDisk
		expectedError string
	}{
		{
			name: "hardware raid",
			raid: &metal3v1alpha1.RAIDConfig{
				HardwareRAIDVolumes: []metal3v1alpha1.HardwareRAIDVolume{
					{
						Name:       "root",
						Level:      "1",
						Rotational: &FALSE,
					},
					{
						Name:       "v1",
						Level:      "1",
						Rotational: &TRUE,
					},
				},
				SoftwareRAIDVolumes: []metal3v1alpha1.SoftwareRAIDVolume{
					{
						Level: "1",
					},
					{
						Level: "1",
					},
				},
			},
			expected: []nodes.LogicalDisk{
				{
					RAIDLevel:    "1",
					VolumeName:   "root",
					DiskType:     nodes.SSD,
					IsRootVolume: &TRUE,
				},
				{
					RAIDLevel:  "1",
					DiskType:   nodes.HDD,
					VolumeName: "v1",
				},
			},
		},
		{
			name: "software raid",
			raid: &metal3v1alpha1.RAIDConfig{
				SoftwareRAIDVolumes: []metal3v1alpha1.SoftwareRAIDVolume{
					{
						Level: "1",
						PhysicalDisks: []metal3v1alpha1.RootDeviceHints{
							{
								MinSizeGigabytes: 100,
							},
							{
								MinSizeGigabytes: 200,
							},
						},
					},
					{
						Level: "1",
					},
				},
			},
			expected: []nodes.LogicalDisk{
				{
					RAIDLevel:    "1",
					IsRootVolume: &TRUE,
					Controller:   "software",
					PhysicalDisks: []interface{}{
						map[string]string{
							"size": ">= 100",
						},
						map[string]string{
							"size": ">= 200",
						},
					},
				},
				{
					RAIDLevel:  "1",
					Controller: "software",
				},
			},
		},
		{
			name: "hardware raid, same volume's name",
			raid: &metal3v1alpha1.RAIDConfig{
				HardwareRAIDVolumes: []metal3v1alpha1.HardwareRAIDVolume{
					{
						Name:  "v1",
						Level: "1",
					},
					{
						Name:  "v1",
						Level: "1",
					},
				},
			},
			expectedError: "name of volume must be unique",
		},
		{
			name: "hardware raid, volume's name is empty",
			raid: &metal3v1alpha1.RAIDConfig{
				HardwareRAIDVolumes: []metal3v1alpha1.HardwareRAIDVolume{
					{
						Name:  "",
						Level: "1",
					},
					{
						Name:  "",
						Level: "1",
					},
				},
			},
			expected: []nodes.LogicalDisk{
				{
					RAIDLevel:    "1",
					VolumeName:   "",
					IsRootVolume: &TRUE,
				},
				{
					RAIDLevel:  "1",
					VolumeName: "",
				},
			},
		},
		{
			name:     "raid is nil",
			raid:     nil,
			expected: nil,
		},
		{
			name: "volumes is nil",
			raid: &metal3v1alpha1.RAIDConfig{
				HardwareRAIDVolumes: nil,
				SoftwareRAIDVolumes: nil,
			},
			expected: nil,
		},
		{
			name: "volumes is empty",
			raid: &metal3v1alpha1.RAIDConfig{
				HardwareRAIDVolumes: []metal3v1alpha1.HardwareRAIDVolume{},
				SoftwareRAIDVolumes: []metal3v1alpha1.SoftwareRAIDVolume{},
			},
			expected: nil,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cfg, err := buildTargetRAIDCfg(c.raid)
			if c.expectedError != "" {
				if err == nil || err.Error() != c.expectedError {
					t.Errorf("expectError: %v, got: %v", c.expectedError, err)
				}
				return
			}
			if !reflect.DeepEqual(c.expected, cfg) {
				t.Errorf("expected: %v, got: %v", c.expected, cfg)
			}
		})
	}
}

func TestBuildRAIDCleanSteps(t *testing.T) {
	cases := []struct {
		name          string
		raid          *metal3v1alpha1.RAIDConfig
		expected      []nodes.CleanStep
		expectedError string
	}{
		{
			name: "hardware raid",
			raid: &metal3v1alpha1.RAIDConfig{
				HardwareRAIDVolumes: []metal3v1alpha1.HardwareRAIDVolume{
					{
						Name:  "root",
						Level: "1",
					},
					{
						Name:  "v1",
						Level: "1",
					},
				},
				SoftwareRAIDVolumes: []metal3v1alpha1.SoftwareRAIDVolume{
					{
						Level: "1",
					},
					{
						Level: "1",
					},
				},
			},
			expected: []nodes.CleanStep{
				{
					Interface: "raid",
					Step:      "delete_configuration",
				},
				{
					Interface: "raid",
					Step:      "create_configuration",
				},
			},
		},
		{
			name: "software raid",
			raid: &metal3v1alpha1.RAIDConfig{
				SoftwareRAIDVolumes: []metal3v1alpha1.SoftwareRAIDVolume{
					{
						Level: "1",
					},
					{
						Level: "1",
					},
				},
			},
			expected: []nodes.CleanStep{
				{
					Interface: "raid",
					Step:      "delete_configuration",
				},
				{
					Interface: "deploy",
					Step:      "erase_devices_metadata",
				},
				{
					Interface: "raid",
					Step:      "create_configuration",
				},
			},
		},
		{
			name: "raid is nil",
			raid: nil,
			expected: []nodes.CleanStep{
				{
					Interface: "raid",
					Step:      "delete_configuration",
				},
			},
		},
		{
			name: "volumes is nil",
			raid: &metal3v1alpha1.RAIDConfig{
				HardwareRAIDVolumes: nil,
				SoftwareRAIDVolumes: nil,
			},
			expected: []nodes.CleanStep{
				{
					Interface: "raid",
					Step:      "delete_configuration",
				},
			},
		},
		{
			name: "volumes is empty",
			raid: &metal3v1alpha1.RAIDConfig{
				HardwareRAIDVolumes: []metal3v1alpha1.HardwareRAIDVolume{},
				SoftwareRAIDVolumes: []metal3v1alpha1.SoftwareRAIDVolume{},
			},
			expected: []nodes.CleanStep{
				{
					Interface: "raid",
					Step:      "delete_configuration",
				},
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			step := buildRAIDCleanSteps(c.raid)
			if !reflect.DeepEqual(c.expected, step) {
				t.Errorf("expected: %v, got: %v", c.expected, step)
			}
		})
	}
}
