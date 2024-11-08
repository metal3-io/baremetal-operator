package ironic

import (
	"fmt"

	"github.com/gophercloud/gophercloud/v2/openstack/baremetal/v1/nodes"
	"github.com/metal3-io/baremetal-operator/pkg/hardwareutils/bmc"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner"
)

func (p *ironicProvisioner) buildServiceSteps(bmcAccess bmc.AccessDetails, data provisioner.ServicingData) (serviceSteps []nodes.ServiceStep, err error) {
	// Get the subset (currently 3) of vendor specific BIOS settings converted from common names
	var firmwareConfig *bmc.FirmwareConfig
	if data.FirmwareConfig != nil {
		bmcConfig := bmc.FirmwareConfig(*data.FirmwareConfig)
		firmwareConfig = &bmcConfig
	}
	fwConfigSettings, err := bmcAccess.BuildBIOSSettings(firmwareConfig)
	if err != nil {
		return nil, err
	}

	newSettings := p.getNewFirmwareSettings(data.ActualFirmwareSettings, data.TargetFirmwareSettings, fwConfigSettings)
	if len(newSettings) != 0 {
		p.log.Info("Applying BIOS config clean steps", "settings", newSettings)
		serviceSteps = append(
			serviceSteps,
			nodes.ServiceStep{
				Interface: nodes.InterfaceBIOS,
				Step:      "apply_configuration",
				Args: map[string]interface{}{
					"settings": newSettings,
				},
			},
		)
	}

	newUpdates := p.getFirmwareComponentsUpdates(data.TargetFirmwareComponents)
	if len(newUpdates) != 0 {
		p.log.Info("Applying Firmware Update clean steps", "settings", newUpdates)
		serviceSteps = append(
			serviceSteps,
			nodes.ServiceStep{
				Interface: nodes.InterfaceFirmware,
				Step:      "update",
				Args: map[string]interface{}{
					"settings": newUpdates,
				},
			},
		)
	}

	return serviceSteps, nil
}

func (p *ironicProvisioner) startServicing(bmcAccess bmc.AccessDetails, ironicNode *nodes.Node, data provisioner.ServicingData) (success bool, result provisioner.Result, err error) {
	// Build service steps
	serviceSteps, err := p.buildServiceSteps(bmcAccess, data)
	if err != nil {
		result, err = operationFailed(err.Error())
		return
	}

	// Start servicing
	if len(serviceSteps) != 0 {
		p.log.Info("remove existing configuration and set new configuration", "serviceSteps", serviceSteps)
		return p.tryChangeNodeProvisionState(
			ironicNode,
			nodes.ProvisionStateOpts{
				Target:       nodes.TargetService,
				ServiceSteps: serviceSteps,
			},
		)
	}
	result, err = operationComplete()
	return
}

func (p *ironicProvisioner) Service(data provisioner.ServicingData, unprepared, restartOnFailure bool) (result provisioner.Result, started bool, err error) {
	if !p.availableFeatures.HasServicing() {
		result, err = operationFailed(fmt.Sprintf("servicing not supported: requires API version 1.87, available is 1.%d", p.availableFeatures.MaxVersion))
		return result, started, err
	}

	bmcAccess, err := p.bmcAccess()
	if err != nil {
		result, err = transientError(err)
		return result, started, err
	}

	ironicNode, err := p.getNode()
	if err != nil {
		result, err = transientError(err)
		return result, started, err
	}

	switch nodes.ProvisionState(ironicNode.ProvisionState) {
	case nodes.ServiceFail:
		// When servicing failed, we need to clean host provisioning settings.
		// If restartOnFailure is false, it means the settings aren't cleared.
		if !restartOnFailure {
			result, err = operationFailed(ironicNode.LastError)
			return result, started, err
		}

		if ironicNode.Maintenance {
			p.log.Info("clearing maintenance flag after a servicing failure")
			result, err = p.setMaintenanceFlag(ironicNode, false, "")
			return result, started, err
		}

		p.log.Info("restarting servicing because of a previous failure")
		unprepared = true
		fallthrough
	case nodes.Active:
		if unprepared {
			started, result, err = p.startServicing(bmcAccess, ironicNode, data)
			if started || result.Dirty || result.ErrorMessage != "" || err != nil {
				return result, started, err
			}
			// nothing to do
			started = true
		}
		// Servicing finished
		p.log.Info("servicing finished on the host")
		result, err = operationComplete()
	case nodes.Servicing, nodes.ServiceWait:
		p.log.Info("waiting for host to become active",
			"state", ironicNode.ProvisionState,
			"serviceStep", ironicNode.ServiceStep)
		result, err = operationContinuing(provisionRequeueDelay)

	default:
		result, err = transientError(fmt.Errorf("have unexpected ironic node state %s", ironicNode.ProvisionState))
	}
	return result, started, err
}
