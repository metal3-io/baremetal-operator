package ironic

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack/baremetal/v1/nodes"
	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner/ironic/clients"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner/ironic/hardwaredetails"
)

func (p *ironicProvisioner) abortInspection(ctx context.Context, ironicNode *nodes.Node) (result provisioner.Result, started bool, err error) {
	// Set started to let the controller know about the change
	p.log.Info("aborting inspection to force reboot of preprovisioning image")
	started, result, err = p.tryChangeNodeProvisionState(
		ctx,
		ironicNode,
		nodes.ProvisionStateOpts{Target: nodes.TargetAbort},
	)
	return
}

func (p *ironicProvisioner) startInspection(ctx context.Context, data provisioner.InspectData, ironicNode *nodes.Node) (result provisioner.Result, started bool, err error) {
	opts := clients.UpdateOptsData{
		"capabilities": buildCapabilitiesValue(ironicNode, data.BootMode),
	}
	if data.CPUArchitecture != "" {
		opts["cpu_arch"] = data.CPUArchitecture
	}

	updater := clients.UpdateOptsBuilder(p.log).
		SetPropertiesOpts(opts, ironicNode)

	bmcAccess, bmcErr := p.bmcAccess()
	if bmcErr != nil {
		result, err = operationFailed(bmcErr.Error())
		return result, started, err
	}
	if data.InspectionMode == metal3api.InspectionModeFast && bmcAccess.InspectInterface() == "" {
		result, err = operationFailed(fmt.Sprintf("BMC driver %s does not support fast (out-of-band) inspection", bmcAccess.Type()))
		return result, started, err
	}
	updater.SetTopLevelOpt("inspect_interface",
		inspectInterfaceForMode(data.InspectionMode, bmcAccess),
		ironicNode.InspectInterface)

	_, started, result, err = p.tryUpdateNode(
		ctx,
		ironicNode,
		updater,
	)
	if !started {
		return result, started, err
	}

	p.log.Info("starting new hardware inspection")
	started, result, err = p.tryChangeNodeProvisionState(
		ctx,
		ironicNode,
		nodes.ProvisionStateOpts{Target: nodes.TargetInspect},
	)
	if started {
		p.publisher("InspectionStarted", "Hardware inspection started")
	}
	return result, started, err
}

// InspectHardware updates the HardwareDetails field of the host with
// details of devices discovered on the hardware. It may be called
// multiple times, and should return true for its dirty flag until the
// inspection is completed.
func (p *ironicProvisioner) InspectHardware(ctx context.Context, data provisioner.InspectData, restartOnFailure, refresh, forceReboot bool) (result provisioner.Result, started bool, details *metal3api.HardwareDetails, err error) {
	p.log.Info("inspecting hardware")

	ironicNode, err := p.getNode(ctx)
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
			ctx,
			ironicNode,
			nodes.ProvisionStateOpts{Target: nodes.TargetManage},
		)
		return result, started, details, err
	case nodes.InspectWait:
		if forceReboot {
			result, started, err = p.abortInspection(ctx, ironicNode)
			return result, started, details, err
		}

		fallthrough
	case nodes.Inspecting:
		p.log.Info("inspection in progress")
		delay := longRetryDelay
		if data.InspectionMode == metal3api.InspectionModeFast {
			delay = shortRetryDelay
		}
		result, err = operationContinuing(delay)
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
			result, started, err = p.startInspection(ctx, data, ironicNode)
			return result, started, details, err
		}
	default:
		p.log.Info("unexpected provisioning state for inspection",
			"provisionState", ironicNode.ProvisionState, "targetProvisionState", ironicNode.TargetProvisionState, "lastError", ironicNode.LastError)
		result, err = transientError(fmt.Errorf("unexpected provision state %s", ironicNode.ProvisionState))
		return result, started, details, err
	}

	inventoryData, result, err := p.getInventory(ctx, ironicNode)
	if result.Dirty || result.ErrorMessage != "" || err != nil {
		return result, started, details, err
	} else if inventoryData == nil {
		// The node has just been enrolled, inspection hasn't been started yet.
		result, started, err = p.startInspection(ctx, data, ironicNode)
		return result, started, details, err
	}

	details = hardwaredetails.GetHardwareDetails(inventoryData, ironicNode.Properties, p.log)
	p.publisher("InspectionComplete", "Hardware inspection completed")
	result, err = operationComplete()
	return result, started, details, err
}

// getInventory fetches inventory data from Ironic. It returns nil when data is
// not present, a non-empty result on a permanent failure, and an error on a
// transient error. Partial data may be returned even on failure.
func (p *ironicProvisioner) getInventory(ctx context.Context, ironicNode *nodes.Node) (inventoryData *nodes.InventoryData, result provisioner.Result, err error) {
	p.log.Info("getting hardware details from inspection")
	response := nodes.GetInventory(ctx, p.client, ironicNode.UUID)
	if response.Err != nil {
		if gophercloud.ResponseCodeIs(response.Err, http.StatusNotFound) {
			// Not a failure, the data is simply not there.
			return nil, result, nil
		}
		// This branch captures not just networking problems but also conditions like SyntaxError from the JSON library,
		// when Ironic returns something that cannot be interpreted as JSON at all (which is hopefully transient).
		result, err = transientError(fmt.Errorf("failed to retrieve hardware introspection data: %w", response.Err))
		return nil, result, err
	}

	inventoryData = new(nodes.InventoryData)
	err = response.ExtractInto(inventoryData)
	unmarshalTypeError := &json.UnmarshalTypeError{}
	if errors.As(err, &unmarshalTypeError) {
		// TODO(dtantsur): at this point, inventoryData may be partially constructed and contain useful information
		// (see https://pkg.go.dev/encoding/json#Unmarshal for details on this behavior).
		// Until we decide if that's good enough to declare success (and how to communicate the error),
		// report a fatal failure since it's not going to improve on retry.
		p.log.Error(err, "unable to parse inventory JSON as InventoryData; it can be a bug in Ironic or GopherCloud")
		result, err = operationFailed("Unable to parse inventory JSON, cannot finish inspection")
		return inventoryData, result, err
	} else if err != nil {
		result, err = transientError(fmt.Errorf("failed to parse hardware introspection data: %w", err))
		return nil, result, err
	}

	p.log.Info("inspection finished successfully", "data", response.Body)
	result, err = operationComplete()
	return inventoryData, result, err
}
