package ironic

import (
	"fmt"
	"reflect"

	"github.com/gophercloud/gophercloud/v2/openstack/baremetal/v1/nodes"
	"github.com/pkg/errors"

	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner/ironic/devicehints"
)

const (
	noRAIDInterface       string = "no-raid"
	softwareRAIDInterface string = "agent"
)

// setTargetRAIDCfg set the RAID settings to the ironic Node for RAID configuration steps.
func setTargetRAIDCfg(p *ironicProvisioner, raidInterface string, ironicNode *nodes.Node, data provisioner.PrepareData) (provisioner.Result, error) {
	targetRaidInterface, err := CheckRAIDInterface(raidInterface, data.TargetRAIDConfig, data.ActualRAIDConfig)
	if err != nil {
		return operationFailed(err.Error())
	}

	var logicalDisks []nodes.LogicalDisk

	// Build target for RAID configuration steps
	logicalDisks, err = BuildTargetRAIDCfg(data.TargetRAIDConfig)
	if err != nil {
		return operationFailed(err.Error())
	}
	if len(logicalDisks) == 0 {
		return provisioner.Result{}, nil
	}

	// set root volume
	if data.RootDeviceHints == nil {
		logicalDisks[0].IsRootVolume = new(bool)
		*logicalDisks[0].IsRootVolume = true
	} else {
		p.log.Info("rootDeviceHints is used, the first volume of raid will not be set to root")
	}

	updater := updateOptsBuilder(p.log)
	updater.SetTopLevelOpt("raid_interface", targetRaidInterface, ironicNode.RAIDInterface)
	ironicNode, success, result, err := p.tryUpdateNode(ironicNode, updater)
	if !success {
		return result, err
	}

	// Set target for RAID configuration steps
	err = nodes.SetRAIDConfig(
		p.ctx,
		p.client,
		ironicNode.UUID,
		nodes.RAIDConfigOpts{LogicalDisks: logicalDisks},
	).ExtractErr()
	if err != nil {
		return transientError(err)
	}
	return provisioner.Result{}, nil
}

// BuildTargetRAIDCfg build RAID logical disks, this method doesn't set the root volume.
func BuildTargetRAIDCfg(raid *metal3api.RAIDConfig) (logicalDisks []nodes.LogicalDisk, err error) {
	// Deal possible panic
	defer func() {
		r := recover()
		if r != nil {
			err = fmt.Errorf("panic in build RAID settings: %v", r)
		}
	}()

	if raid == nil {
		return
	}

	// build logicalDisks
	if len(raid.HardwareRAIDVolumes) != 0 {
		logicalDisks, err = buildTargetHardwareRAIDCfg(raid.HardwareRAIDVolumes)
	} else if len(raid.SoftwareRAIDVolumes) != 0 {
		logicalDisks, err = buildTargetSoftwareRAIDCfg(raid.SoftwareRAIDVolumes)
	}

	return
}

// A private method to build hardware RAID disks.
func buildTargetHardwareRAIDCfg(volumes []metal3api.HardwareRAIDVolume) (logicalDisks []nodes.LogicalDisk, err error) {
	var (
		logicalDisk    nodes.LogicalDisk
		nameCheckFlags = make(map[string]int)
	)

	if len(volumes) == 0 {
		return
	}

	for index, volume := range volumes {
		// Check volume's name
		if volume.Name != "" {
			i, exist := nameCheckFlags[volume.Name]
			if exist {
				return nil, errors.Errorf("the names(%s) of volume[%d] and volume[%d] are repeated", volume.Name, index, i)
			}
			nameCheckFlags[volume.Name] = index
		}

		// Check that controller field is specified if PhysicalDisks are used
		if len(volume.PhysicalDisks) != 0 {
			if volume.Controller == "" {
				return nil, errors.Errorf("'controller' must be specified if 'physicalDisks' are used")
			}
		}

		// Check numberOfPhysicalDisks is same as len(physicalDisks)
		if volume.NumberOfPhysicalDisks != nil && len(volume.PhysicalDisks) != 0 {
			if *volume.NumberOfPhysicalDisks != len(volume.PhysicalDisks) {
				return nil, errors.Errorf("the numberOfPhysicalDisks[%d] is not same as number of items in physicalDisks[%d]", *volume.NumberOfPhysicalDisks, len(volume.PhysicalDisks))
			}
		}
		// Create the physicalDisks slice for logicalDisk struct
		physicalDisks := make([]interface{}, len(volume.PhysicalDisks))
		for i, physicalDisk := range volume.PhysicalDisks {
			physicalDisks[i] = physicalDisk
		}

		// Build logicalDisk
		logicalDisk = nodes.LogicalDisk{
			SizeGB:        volume.SizeGibibytes,
			RAIDLevel:     nodes.RAIDLevel(volume.Level),
			VolumeName:    volume.Name,
			Controller:    volume.Controller,
			PhysicalDisks: physicalDisks,
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

// A private method to build software RAID disks.
func buildTargetSoftwareRAIDCfg(volumes []metal3api.SoftwareRAIDVolume) (logicalDisks []nodes.LogicalDisk, err error) {
	var (
		logicalDisk nodes.LogicalDisk
	)

	if len(volumes) == 0 {
		return
	}

	if nodes.RAIDLevel(volumes[0].Level) != nodes.RAID1 {
		return nil, errors.Errorf("the level in first volume of software raid must be RAID1")
	}

	for _, volume := range volumes {
		// Build logicalDisk
		logicalDisk = nodes.LogicalDisk{
			SizeGB:     volume.SizeGibibytes,
			RAIDLevel:  nodes.RAIDLevel(volume.Level),
			Controller: "software",
		}
		// Build physical disks hint
		for i := range volume.PhysicalDisks {
			logicalDisk.PhysicalDisks = append(logicalDisk.PhysicalDisks, devicehints.MakeHintMap(&volume.PhysicalDisks[i]))
		}
		// Add to logicalDisks
		logicalDisks = append(logicalDisks, logicalDisk)
	}

	return
}

// BuildRAIDCleanSteps build the clean steps for RAID configuration from BaremetalHost spec.
func BuildRAIDCleanSteps(raidInterface string, target *metal3api.RAIDConfig, actual *metal3api.RAIDConfig) (cleanSteps []nodes.CleanStep, err error) {
	_, err = CheckRAIDInterface(raidInterface, target, actual)
	if err != nil {
		return nil, err
	}

	// Software RAID
	if target != nil && target.SoftwareRAIDVolumes != nil {
		// Ignore HardwareRAIDVolumes
		target.HardwareRAIDVolumes = nil
		if actual != nil {
			actual.HardwareRAIDVolumes = nil
		}
		if reflect.DeepEqual(target, actual) {
			return
		}
		if len(target.SoftwareRAIDVolumes) == 0 && (actual == nil || len(actual.SoftwareRAIDVolumes) == 0) {
			return
		}

		cleanSteps = append(
			cleanSteps,
			[]nodes.CleanStep{
				{
					Interface: nodes.InterfaceRAID,
					Step:      "delete_configuration",
				},
				{
					Interface: nodes.InterfaceDeploy,
					Step:      "erase_devices_metadata",
				},
			}...,
		)

		// If software raid configuration is empty, only need to clear old configuration
		if len(target.SoftwareRAIDVolumes) == 0 {
			return
		}

		cleanSteps = append(
			cleanSteps,
			nodes.CleanStep{
				Interface: nodes.InterfaceRAID,
				Step:      "create_configuration",
			},
		)
		return
	}

	// Hardware RAID
	// If hardware RAID configuration is nil,
	// keep old hardware RAID configuration
	if raidInterface == noRAIDInterface || target == nil || target.HardwareRAIDVolumes == nil {
		return
	}

	// Ignore SoftwareRAIDVolumes
	target.SoftwareRAIDVolumes = nil
	if actual != nil {
		actual.SoftwareRAIDVolumes = nil
	}
	if reflect.DeepEqual(target, actual) {
		return
	}

	// Add ‘delete_configuration’ before ‘create_configuration’ to make sure
	// that only the desired logical disks exist in the system after manual cleaning.
	cleanSteps = append(
		cleanSteps,
		nodes.CleanStep{
			Interface: nodes.InterfaceRAID,
			Step:      "delete_configuration",
		},
	)

	// If hardware raid configuration is empty, only need to clear old configuration
	if len(target.HardwareRAIDVolumes) == 0 {
		return
	}

	// ‘create_configuration’ doesn’t remove existing disks. It is recommended
	// that only the desired logical disks exist in the system after manual cleaning.
	cleanSteps = append(
		cleanSteps,
		nodes.CleanStep{
			Interface: nodes.InterfaceRAID,
			Step:      "create_configuration",
		},
	)
	return
}

// CheckRAIDInterface checks the current RAID interface against the requested configuration.
func CheckRAIDInterface(raidInterface string, target, actual *metal3api.RAIDConfig) (string, error) {
	if target == nil {
		return raidInterface, nil
	}

	if raidInterface == noRAIDInterface && len(target.HardwareRAIDVolumes) != 0 {
		return "", fmt.Errorf("RAID settings are defined, but the node's driver %s does not support RAID", raidInterface)
	}

	// This is not a real case since no BMC driver defaults to agent, but we add it for consistency
	if raidInterface == softwareRAIDInterface && len(target.HardwareRAIDVolumes) != 0 {
		return "", fmt.Errorf("hardware RAID settings are defined, but the node's driver %s only supports software RAID", raidInterface)
	}

	// If software RAID is requested, change the RAID interface.
	// FIXME(dtantsur): if a user tries to simultaneously remove hardware RAID volumes and add software RAID volumes, it will not work.
	// The matter is complicated because Ironic cannot change the RAID interface in the middle of cleaning. We better just document it.
	if len(target.SoftwareRAIDVolumes) != 0 {
		return softwareRAIDInterface, nil
	}

	// If software RAID is being deleted, also change the interface.
	if actual != nil && len(actual.SoftwareRAIDVolumes) != 0 {
		return softwareRAIDInterface, nil
	}

	return raidInterface, nil
}
