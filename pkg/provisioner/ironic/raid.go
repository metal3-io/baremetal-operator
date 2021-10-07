package ironic

import (
	"github.com/gophercloud/gophercloud/openstack/baremetal/v1/nodes"

	"github.com/metal3-io/baremetal-operator/ironic"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner"
)

// setTargetRAIDCfg set the RAID settings to the ironic Node for RAID configuration steps
func setTargetRAIDCfg(p *ironicProvisioner, raidInterface string, ironicNode *nodes.Node, data provisioner.PrepareData) (provisioner.Result, error) {
	err := ironic.CheckRAIDConfigure(raidInterface, data.TargetRAIDConfig)
	if err != nil {
		return operationFailed(err.Error())
	}

	var logicalDisks []nodes.LogicalDisk

	// Build target for RAID configuration steps
	logicalDisks, err = ironic.BuildTargetRAIDCfg(data.TargetRAIDConfig)
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

	// Set target for RAID configuration steps
	err = nodes.SetRAIDConfig(
		p.client,
		ironicNode.UUID,
		nodes.RAIDConfigOpts{LogicalDisks: logicalDisks},
	).ExtractErr()
	if err != nil {
		return transientError(err)
	}
	return provisioner.Result{}, nil
}
