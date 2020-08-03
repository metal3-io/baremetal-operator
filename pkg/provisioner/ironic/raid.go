package ironic

import (
	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack/baremetal/v1/nodes"
	metal3v1alpha1 "github.com/metal3-io/baremetal-operator/pkg/apis/metal3/v1alpha1"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner/ironic/devicehints"
	"github.com/pkg/errors"
)

// setTargetRAIDCfg set target for RAID configuration steps
func setTargetRAIDCfg(client *gophercloud.ServiceClient, ironicNode *nodes.Node, raid *metal3v1alpha1.RAIDConfig) (err error) {
	// Build target for RAID configuration steps
	logicalDisks, err := buildTargetRAIDCfg(raid)
	if len(logicalDisks) == 0 || err != nil {
		return
	}
	// Set target for RAID configuration steps
	return nodes.SetRAIDConfig(
		client,
		ironicNode.UUID,
		nodes.RAIDConfigOpts{LogicalDisks: logicalDisks},
	).ExtractErr()
}

// A private method to build RAID disks
func buildTargetRAIDCfg(raid *metal3v1alpha1.RAIDConfig) (logicalDisks []nodes.LogicalDisk, err error) {
	if raid != nil {
		// build logicalDisks
		if len(raid.HardwareRAIDVolumes) != 0 {
			logicalDisks, err = buildTargetHardwareRAIDCfg(raid.HardwareRAIDVolumes)
		} else if len(raid.SoftwareRAIDVolumes) != 0 {
			logicalDisks = buildTargetSoftwareRAIDCfg(raid.SoftwareRAIDVolumes)
		}
		// set root volume
		if len(logicalDisks) != 0 {
			logicalDisks[0].IsRootVolume = new(bool)
			*logicalDisks[0].IsRootVolume = true
		}
	}

	return
}

// A private method to build hardware RAID disks
func buildTargetHardwareRAIDCfg(volumes []metal3v1alpha1.HardwareRAIDVolume) (logicalDisks []nodes.LogicalDisk, err error) {
	var (
		logicalDisk    nodes.LogicalDisk
		nameCheckFlags map[string]struct{} = make(map[string]struct{})
	)

	for _, volume := range volumes {
		// Check volume's name
		if volume.Name != "" {
			_, exist := nameCheckFlags[volume.Name]
			if exist {
				err = errors.Errorf("name of volume must be unique")
				return nil, err
			}
			nameCheckFlags[volume.Name] = struct{}{}
		}
		// Build logicalDisk
		logicalDisk = nodes.LogicalDisk{
			SizeGB:     volume.SizeGibibytes,
			RAIDLevel:  nodes.RAIDLevel(volume.Level),
			VolumeName: volume.Name,
		}
		if volume.Rotational != nil {
			if *volume.Rotational {
				logicalDisk.DiskType = nodes.HDD
			} else {
				logicalDisk.DiskType = nodes.SSD
			}
		}
		if volume.NumberOfPhysicalDisks != nil {
			logicalDisk.NumberOfPhysicalDisks = *volume.NumberOfPhysicalDisks
		}
		// Add to logicalDisks
		logicalDisks = append(logicalDisks, logicalDisk)
	}

	return
}

// A private method to build software RAID disks
func buildTargetSoftwareRAIDCfg(volumes []metal3v1alpha1.SoftwareRAIDVolume) (logicalDisks []nodes.LogicalDisk) {
	var (
		logicalDisk nodes.LogicalDisk
	)

	for _, volume := range volumes {
		// Build logicalDisk
		logicalDisk = nodes.LogicalDisk{
			SizeGB:     volume.SizeGibibytes,
			RAIDLevel:  nodes.RAIDLevel(volume.Level),
			Controller: "software",
		}
		physicalDisks := makePhysicalDisks(volume.PhysicalDisks)
		for _, physicalDisk := range physicalDisks {
			logicalDisk.PhysicalDisks = append(logicalDisk.PhysicalDisks, physicalDisk)
		}
		// Add to logicalDisks
		logicalDisks = append(logicalDisks, logicalDisk)
	}

	return
}

// A private method to make physical disks hints for software raid
func makePhysicalDisks(hintslist []metal3v1alpha1.RootDeviceHints) (physicalDisks []map[string]string) {
	var physicalDisk map[string]string
	for _, hints := range hintslist {
		physicalDisk = devicehints.MakeHintMap(&hints)
		physicalDisks = append(physicalDisks, physicalDisk)
	}
	return
}

// buildRAIDCleanSteps build the clean steps for RAID configuration from BaremetalHost spec
func buildRAIDCleanSteps(raid *metal3v1alpha1.RAIDConfig) (cleanSteps []nodes.CleanStep) {
	// To add ‘delete_configuration’ before ‘create_configuration’ to make sure
	cleanSteps = append(
		cleanSteps,
		nodes.CleanStep{
			Interface: "raid",
			Step:      "delete_configuration",
		},
	)
	// If not configure raid, only need to clear old configuration
	if raid == nil || (len(raid.HardwareRAIDVolumes) == 0 && len(raid.SoftwareRAIDVolumes) == 0) {
		return
	}
	if len(raid.HardwareRAIDVolumes) == 0 && len(raid.SoftwareRAIDVolumes) != 0 {
		cleanSteps = append(
			cleanSteps,
			nodes.CleanStep{
				Interface: "deploy",
				Step:      "erase_devices_metadata",
			},
		)
	}
	// ‘create_configuration’ doesn’t remove existing disks. It is recommended
	// that only the desired logical disks exist in the system after manual cleaning.
	cleanSteps = append(
		cleanSteps,
		nodes.CleanStep{
			Interface: "raid",
			Step:      "create_configuration",
		},
	)
	return
}
