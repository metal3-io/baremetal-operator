package ironic

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/gophercloud/gophercloud/v2/openstack/baremetal/v1/nodes"
	"github.com/stretchr/testify/assert"

	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
)

func TestBuildTargetRAIDCfg(t *testing.T) {
	var TRUE = true
	var FALSE = false
	physicalDisks := make([]interface{}, 0)
	physicalDisks2 := []string{
		"Disk-1",
		"Disk-2",
	}
	numberOfPhysicalDisks := 3

	cases := []struct {
		name          string
		raid          *metal3api.RAIDConfig
		expected      []nodes.LogicalDisk
		expectedError string
	}{
		{
			name: "hardware raid, physicalDisks without controller",
			raid: &metal3api.RAIDConfig{
				HardwareRAIDVolumes: []metal3api.HardwareRAIDVolume{
					{
						Name:          "volume1",
						Level:         "1",
						Rotational:    &FALSE,
						PhysicalDisks: physicalDisks2,
					}, // end of RAID volume
				}, // end of RAID volumes slice
			}, // end of RAID config
			expectedError: "'controller' must be specified if 'physicalDisks' are used",
		}, // end of test case
		{
			name: "hardware raid, len(physicalDisks) != numberOfPhysicalDisks",
			raid: &metal3api.RAIDConfig{
				HardwareRAIDVolumes: []metal3api.HardwareRAIDVolume{
					{
						Name:                  "volume1",
						Level:                 "1",
						Rotational:            &TRUE,
						PhysicalDisks:         physicalDisks2,         // slice of 2 disks
						NumberOfPhysicalDisks: &numberOfPhysicalDisks, // defined as 3 above
						Controller:            "Controller-1",
					}, // end of RAID volume
				}, // end of RAID volumes slice
			}, // end of RAID config
			expectedError: fmt.Sprintf("the numberOfPhysicalDisks[%d] is not same as number of items in physicalDisks[%d]", numberOfPhysicalDisks, len(physicalDisks2)),
		},
		{
			name: "hardware raid",
			raid: &metal3api.RAIDConfig{
				HardwareRAIDVolumes: []metal3api.HardwareRAIDVolume{
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
				SoftwareRAIDVolumes: []metal3api.SoftwareRAIDVolume{
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
					RAIDLevel:     "1",
					VolumeName:    "root",
					DiskType:      nodes.SSD,
					PhysicalDisks: physicalDisks,
				},
				{
					RAIDLevel:     "1",
					DiskType:      nodes.HDD,
					VolumeName:    "v1",
					PhysicalDisks: physicalDisks,
				},
			},
		},
		{
			name: "hardware raid, same volume's name",
			raid: &metal3api.RAIDConfig{
				HardwareRAIDVolumes: []metal3api.HardwareRAIDVolume{
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
			expectedError: "the names(v1) of volume[1] and volume[0] are repeated",
		},
		{
			name: "hardware raid, volume's name is empty",
			raid: &metal3api.RAIDConfig{
				HardwareRAIDVolumes: []metal3api.HardwareRAIDVolume{
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
					RAIDLevel:     "1",
					VolumeName:    "",
					PhysicalDisks: physicalDisks,
				},
				{
					RAIDLevel:     "1",
					VolumeName:    "",
					PhysicalDisks: physicalDisks,
				},
			},
		},
		{
			name: "software raid",
			raid: &metal3api.RAIDConfig{
				SoftwareRAIDVolumes: []metal3api.SoftwareRAIDVolume{
					{
						Level: "1",
						PhysicalDisks: []metal3api.RootDeviceHints{
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
					RAIDLevel:  "1",
					Controller: "software",
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
			name: "software raid, the level in first volume isn't RAID1",
			raid: &metal3api.RAIDConfig{
				SoftwareRAIDVolumes: []metal3api.SoftwareRAIDVolume{
					{
						Level: "0",
					},
					{
						Level: "1",
					},
				},
			},
			expectedError: "the level in first volume of software raid must be RAID1",
		},
		{
			name:     "raid is nil",
			raid:     nil,
			expected: nil,
		},
		{
			name: "volumes is nil",
			raid: &metal3api.RAIDConfig{
				HardwareRAIDVolumes: nil,
				SoftwareRAIDVolumes: nil,
			},
			expected: nil,
		},
		{
			name: "volumes is empty",
			raid: &metal3api.RAIDConfig{
				HardwareRAIDVolumes: []metal3api.HardwareRAIDVolume{},
				SoftwareRAIDVolumes: []metal3api.SoftwareRAIDVolume{},
			},
			expected: nil,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cfg, err := BuildTargetRAIDCfg(c.raid)
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
		raidInterface string
		target        *metal3api.RAIDConfig
		actual        *metal3api.RAIDConfig
		expected      []nodes.CleanStep
		expectedError bool
	}{
		{
			name:          "keep hardware RAID",
			raidInterface: "irmc",
			target:        nil,
		},
		{
			name:          "keep hardware RAID",
			raidInterface: "irmc",
			target:        &metal3api.RAIDConfig{},
		},
		{
			name:          "configure hardware RAID",
			raidInterface: "irmc",
			target: &metal3api.RAIDConfig{
				HardwareRAIDVolumes: []metal3api.HardwareRAIDVolume{
					{
						Name:  "root",
						Level: "1",
					},
				},
			},
			expected: []nodes.CleanStep{
				{
					Interface: nodes.InterfaceRAID,
					Step:      "delete_configuration",
				},
				{
					Interface: nodes.InterfaceRAID,
					Step:      "create_configuration",
				},
			},
		},
		{
			name:          "have same hardware RAID",
			raidInterface: "irmc",
			target: &metal3api.RAIDConfig{
				HardwareRAIDVolumes: []metal3api.HardwareRAIDVolume{
					{
						Name:  "root",
						Level: "1",
					},
				},
			},
			actual: &metal3api.RAIDConfig{
				HardwareRAIDVolumes: []metal3api.HardwareRAIDVolume{
					{
						Name:  "root",
						Level: "1",
					},
				},
			},
		},
		{
			name:          "clear hardware RAID",
			raidInterface: "irmc",
			target: &metal3api.RAIDConfig{
				HardwareRAIDVolumes: []metal3api.HardwareRAIDVolume{},
			},
			actual: &metal3api.RAIDConfig{
				HardwareRAIDVolumes: []metal3api.HardwareRAIDVolume{
					{
						Name:  "root",
						Level: "1",
					},
				},
			},
			expected: []nodes.CleanStep{
				{
					Interface: nodes.InterfaceRAID,
					Step:      "delete_configuration",
				},
			},
		},
		{
			name:          "configure software RAID",
			raidInterface: "agent",
			target: &metal3api.RAIDConfig{
				SoftwareRAIDVolumes: []metal3api.SoftwareRAIDVolume{
					{
						Level: "1",
					},
				},
			},
			expected: []nodes.CleanStep{
				{
					Interface: nodes.InterfaceRAID,
					Step:      "delete_configuration",
				},
				{
					Interface: nodes.InterfaceDeploy,
					Step:      "erase_devices_metadata",
				},
				{
					Interface: nodes.InterfaceRAID,
					Step:      "create_configuration",
				},
			},
		},
		{
			name:          "have same software RAID",
			raidInterface: "agent",
			target: &metal3api.RAIDConfig{
				SoftwareRAIDVolumes: []metal3api.SoftwareRAIDVolume{
					{
						Level: "1",
					},
				},
			},
			actual: &metal3api.RAIDConfig{
				SoftwareRAIDVolumes: []metal3api.SoftwareRAIDVolume{
					{
						Level: "1",
					},
				},
			},
		},
		{
			name:          "keep missing software RAID",
			raidInterface: "agent",
			target: &metal3api.RAIDConfig{
				SoftwareRAIDVolumes: []metal3api.SoftwareRAIDVolume{},
			},
		},
		{
			name:          "keep empty software RAID",
			raidInterface: "agent",
			target: &metal3api.RAIDConfig{
				SoftwareRAIDVolumes: []metal3api.SoftwareRAIDVolume{},
			},
			actual: &metal3api.RAIDConfig{},
		},
		{
			name:          "clear software RAID",
			raidInterface: "agent",
			target: &metal3api.RAIDConfig{
				SoftwareRAIDVolumes: []metal3api.SoftwareRAIDVolume{},
			},
			actual: &metal3api.RAIDConfig{
				SoftwareRAIDVolumes: []metal3api.SoftwareRAIDVolume{
					{
						Level: "1",
					},
				},
			},
			expected: []nodes.CleanStep{
				{
					Interface: nodes.InterfaceRAID,
					Step:      "delete_configuration",
				},
				{
					Interface: nodes.InterfaceDeploy,
					Step:      "erase_devices_metadata",
				},
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			step, err := BuildRAIDCleanSteps(c.raidInterface, c.target, c.actual)
			if !reflect.DeepEqual(c.expected, step) {
				t.Errorf("expected: %v, got: %v", c.expected, step)
			}
			if (err != nil) != c.expectedError {
				t.Errorf("got unexpected error: %v", err)
			}
		})
	}
}

func TestCheckRAIDConfigure(t *testing.T) {
	cases := []struct {
		raidInterface        string
		RAID                 *metal3api.RAIDConfig
		expectedError        bool
		expectedNewInterface string
		currentRAID          *metal3api.RAIDConfig
	}{
		{
			raidInterface: "no-raid",
		},
		{
			raidInterface: "no-raid",
			RAID:          &metal3api.RAIDConfig{},
		},
		{
			raidInterface: "no-raid",
			RAID: &metal3api.RAIDConfig{
				HardwareRAIDVolumes: []metal3api.HardwareRAIDVolume{
					{
						Name:  "root",
						Level: "1",
					},
				},
			},
			expectedError: true,
		},
		{
			raidInterface: "no-raid",
			RAID: &metal3api.RAIDConfig{
				SoftwareRAIDVolumes: []metal3api.SoftwareRAIDVolume{
					{
						Level: "1",
					},
				},
			},
			expectedNewInterface: "agent",
		},
		{
			raidInterface: "agent",
		},
		{
			raidInterface: "agent",
			RAID:          &metal3api.RAIDConfig{},
		},
		{
			raidInterface: "agent",
			RAID: &metal3api.RAIDConfig{
				HardwareRAIDVolumes: []metal3api.HardwareRAIDVolume{
					{
						Name:  "root",
						Level: "1",
					},
				},
			},
			expectedError: true,
		},
		{
			raidInterface: "agent",
			RAID: &metal3api.RAIDConfig{
				SoftwareRAIDVolumes: []metal3api.SoftwareRAIDVolume{
					{
						Level: "1",
					},
				},
			},
		},
		{
			raidInterface: "hardware",
		},
		{
			raidInterface: "hardware",
			RAID:          &metal3api.RAIDConfig{},
		},
		{
			raidInterface: "hardware",
			RAID: &metal3api.RAIDConfig{
				HardwareRAIDVolumes: []metal3api.HardwareRAIDVolume{
					{
						Name:  "root",
						Level: "1",
					},
				},
			},
		},
		{
			raidInterface: "hardware",
			RAID: &metal3api.RAIDConfig{
				SoftwareRAIDVolumes: []metal3api.SoftwareRAIDVolume{
					{
						Level: "1",
					},
				},
			},
			expectedNewInterface: "agent",
		},
		{
			raidInterface: "hardware",
			RAID: &metal3api.RAIDConfig{
				SoftwareRAIDVolumes: []metal3api.SoftwareRAIDVolume{},
			},
			currentRAID: &metal3api.RAIDConfig{
				SoftwareRAIDVolumes: []metal3api.SoftwareRAIDVolume{
					{
						Level: "1",
					},
				},
			},
			expectedNewInterface: "agent",
		},
	}

	for _, c := range cases {
		t.Run(c.raidInterface, func(t *testing.T) {
			newInterface, err := CheckRAIDInterface(c.raidInterface, c.RAID, c.currentRAID)
			if (err != nil) != c.expectedError {
				t.Errorf("Got unexpected error: %v", err)
			}
			if !c.expectedError {
				if c.expectedNewInterface != "" {
					assert.Equal(t, c.expectedNewInterface, newInterface)
				} else {
					assert.Equal(t, c.raidInterface, newInterface)
				}
			}
		})
	}
}
