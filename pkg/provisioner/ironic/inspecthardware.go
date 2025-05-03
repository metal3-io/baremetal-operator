package ironic

import (
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack/baremetal/v1/drivers"
	"github.com/gophercloud/gophercloud/v2/openstack/baremetal/v1/nodes"
	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	"github.com/metal3-io/baremetal-operator/pkg/hardwareutils/bmc"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner/ironic/clients"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner/ironic/hardwaredetails"
)

func (p *ironicProvisioner) getInspectInterface(bmcAccess bmc.AccessDetails) (string, error) {
	driver, err := drivers.GetDriverDetails(p.ctx, p.client, bmcAccess.Driver()).Extract()
	if err != nil {
		return "", fmt.Errorf("cannot load information about driver %s: %w", bmcAccess.Driver(), err)
	}

	if slices.Contains(driver.EnabledInspectInterfaces, "agent") {
		return "agent", nil
	}

	return "inspector", nil // backward compatibility
}

func (p *ironicProvisioner) abortInspection(ironicNode *nodes.Node) (result provisioner.Result, started bool, details *metal3api.HardwareDetails, err error) {
	// Set started to let the controller know about the change
	p.log.Info("aborting inspection to force reboot of preprovisioning image")
	started, result, err = p.tryChangeNodeProvisionState(
		ironicNode,
		nodes.ProvisionStateOpts{Target: nodes.TargetAbort},
	)
	return
}

func (p *ironicProvisioner) startInspection(data provisioner.InspectData, ironicNode *nodes.Node) (result provisioner.Result, started bool, err error) {
	_, started, result, err = p.tryUpdateNode(
		ironicNode,
		clients.UpdateOptsBuilder(p.log).
			SetPropertiesOpts(clients.UpdateOptsData{
				"capabilities": buildCapabilitiesValue(ironicNode, data.BootMode),
			}, ironicNode),
	)
	if !started {
		return
	}

	p.log.Info("starting new hardware inspection")
	started, result, err = p.tryChangeNodeProvisionState(
		ironicNode,
		nodes.ProvisionStateOpts{Target: nodes.TargetInspect},
	)
	if started {
		p.publisher("InspectionStarted", "Hardware inspection started")
	}
	return
}

// InspectHardware updates the HardwareDetails field of the host with
// details of devices discovered on the hardware. It may be called
// multiple times, and should return true for its dirty flag until the
// inspection is completed.
func (p *ironicProvisioner) InspectHardware(data provisioner.InspectData, restartOnFailure, refresh, forceReboot bool) (result provisioner.Result, started bool, details *metal3api.HardwareDetails, err error) {
	p.log.Info("inspecting hardware")

	ironicNode, err := p.getNode()
	if err != nil {
		result, err = transientError(err)
		return result, started, details, err
	}

	if ironicNode.ProvisionState == string(nodes.InspectFail) && strings.Contains(ironicNode.LastError, "aborted") {
		// Inspection gets canceled when we detect a new preprovisioning image, not need to report an error, just restart.
		refresh = true
		restartOnFailure = true
	}

	switch nodes.ProvisionState(ironicNode.ProvisionState) {
	case nodes.Available:
		result, err = p.changeNodeProvisionState(
			ironicNode,
			nodes.ProvisionStateOpts{Target: nodes.TargetManage},
		)
		return result, started, details, err
	case nodes.InspectWait:
		if forceReboot {
			return p.abortInspection(ironicNode)
		}

		fallthrough
	case nodes.Inspecting:
		p.log.Info("inspection in progress")
		result, err = operationContinuing(introspectionRequeueDelay)
		return result, started, details, err
	case nodes.InspectFail:
		if !restartOnFailure {
			failure := ironicNode.LastError
			if failure == "" {
				failure = "Inspection failed"
			}
			p.log.Info("inspection failed", "error", failure)
			result, err = operationFailed(failure)
			return result, started, details, err
		}
		refresh = true
		fallthrough
	case nodes.Manageable:
		if refresh {
			result, started, err = p.startInspection(data, ironicNode)
			return result, started, details, err
		}
	default:
		p.log.Info("unexpected provisioning state for inspection",
			"provisionState", ironicNode.ProvisionState, "targetProvisionState", ironicNode.TargetProvisionState, "lastError", ironicNode.LastError)
		result, err = transientError(fmt.Errorf("unexpected provision state %s", ironicNode.ProvisionState))
		return result, started, details, err
	}

	p.log.Info("getting hardware details from inspection")
	response := nodes.GetInventory(p.ctx, p.client, ironicNode.UUID)
	introData, err := response.Extract()
	if err != nil {
		if gophercloud.ResponseCodeIs(err, http.StatusNotFound) {
			// The node has just been enrolled, inspection hasn't been started yet.
			result, started, err = p.startInspection(data, ironicNode)
			return result, started, details, err
		}
		result, err = transientError(fmt.Errorf("failed to retrieve hardware introspection data: %w", err))
		return result, started, details, err
	}

	// Introspection is done
	p.log.Info("inspection finished successfully", "data", response.Body)

	details = hardwaredetails.GetHardwareDetails(introData, p.log)
	p.publisher("InspectionComplete", "Hardware inspection completed")
	result, err = operationComplete()
	return result, started, details, err
}
