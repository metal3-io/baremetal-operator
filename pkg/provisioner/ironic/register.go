package ironic

import (
	"fmt"
	"net/http"
	"reflect"
	"regexp"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack/baremetal/v1/nodes"
	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	"github.com/metal3-io/baremetal-operator/pkg/hardwareutils/bmc"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner/ironic/clients"
	"sigs.k8s.io/yaml"
)

func bmcAddressMatches(ironicNode *nodes.Node, driverInfo map[string]interface{}) bool {
	newAddress := make(map[string]interface{})
	ironicAddress := make(map[string]interface{})
	reg := regexp.MustCompile("_address$")
	for key, value := range driverInfo {
		if reg.MatchString(key) {
			newAddress[key] = value
			break
		}
	}
	for key, value := range ironicNode.DriverInfo {
		if reg.MatchString(key) {
			ironicAddress[key] = value
			break
		}
	}
	return reflect.DeepEqual(newAddress, ironicAddress)
}

// Register registers the host in the internal database if it does not
// exist, updates the existing host if needed, and tests the connection
// information for the host to verify that the credentials work.
// The credentialsChanged argument tells the provisioner whether the
// current set of credentials it has are different from the credentials
// it has previously been using, without implying that either set of
// credentials is correct.
func (p *ironicProvisioner) Register(data provisioner.ManagementAccessData, credentialsChanged, restartOnFailure bool) (result provisioner.Result, provID string, err error) {
	bmcAccess, err := p.bmcAccess()
	if err != nil {
		result, err = operationFailed(err.Error())
		return result, "", err
	}

	if data.BootMode == metal3api.UEFISecureBoot && !bmcAccess.SupportsSecureBoot() {
		msg := fmt.Sprintf("BMC driver %s does not support secure boot", bmcAccess.Type())
		p.log.Info(msg)
		result, err = operationFailed(msg)
		return result, "", err
	}

	if bmcAccess.RequiresProvisioningNetwork() && p.config.provNetDisabled {
		msg := fmt.Sprintf("BMC driver %s requires a provisioning network", bmcAccess.Type())
		p.log.Info(msg)
		result, err = operationFailed(msg)
		return result, "", err
	}

	// Refuse to manage a node that has Disabled Power off if not supported by ironic,
	// accidentally powering it off would require a arctic expedition to the data center
	if data.DisablePowerOff && !p.availableFeatures.HasDisablePowerOff() {
		msg := "current ironic version does not support DisablePowerOff, refusing to manage node"
		p.log.Info(msg)
		result, err = operationFailed(msg)
		return result, "", err
	}

	var ironicNode *nodes.Node
	updater := clients.UpdateOptsBuilder(p.log)

	p.debugLog.Info("validating management access")

	ironicNode, err = p.findExistingHost(p.bootMACAddress)
	if err != nil {
		switch err.(type) {
		case macAddressConflictError:
			result, err = operationFailed(err.Error())
		default:
			result, err = transientError(fmt.Errorf("failed to find existing host: %w", err))
		}
		return result, "", err
	}

	// Some BMC types require a MAC address, so ensure we have one
	// when we need it. If not, place the host in an error state.
	if bmcAccess.NeedsMAC() && p.bootMACAddress == "" {
		msg := fmt.Sprintf("BMC driver %s requires a BootMACAddress value", bmcAccess.Type())
		p.log.Info(msg)
		result, err = operationFailed(msg)
		return result, "", err
	}

	driverInfo := bmcAccess.DriverInfo(p.bmcCreds)
	driverInfo = setExternalURL(p, driverInfo)

	// If we have not found a node yet, we need to create one
	if ironicNode == nil {
		p.log.Info("registering host in ironic")
		var retry bool
		ironicNode, retry, err = p.enrollNode(data, bmcAccess, driverInfo)
		if err != nil {
			result, err = transientError(err)
			return result, "", err
		}
		if retry {
			result, err = retryAfterDelay(provisionRequeueDelay)
			return result, "", err
		}
		// Store the ID so other methods can assume it is set and so
		// we can find the node again later.
		provID = ironicNode.UUID
	} else {
		// FIXME(dhellmann): At this point we have found an existing
		// node in ironic by looking it up. We need to check its
		// settings against what we have in the host, and change them
		// if there are differences.
		provID = ironicNode.UUID

		updater.SetTopLevelOpt("name", ironicNodeName(p.objectMeta), ironicNode.Name)

		// When node exists but has no assigned port to it by Ironic and actuall address (MAC) is present
		// in host config and is not allocated to different node lets try to create port for this node.
		if p.bootMACAddress != "" {
			err = p.ensurePort(ironicNode)
			if err != nil {
				result, err = transientError(err)
				return result, provID, err
			}
		}

		bmcAddressChanged := !bmcAddressMatches(ironicNode, driverInfo)

		// The actual password is not returned from ironic, so we want to
		// update the whole DriverInfo only if the credentials or BMC address
		// has changed, otherwise we will be writing on every call to this
		// function.
		if credentialsChanged || bmcAddressChanged {
			p.log.Info("Updating driver info because the credentials and/or the BMC address changed")
			updater.SetTopLevelOpt("driver_info", driverInfo, ironicNode.DriverInfo)
		}

		// The updater only updates disable_power_off if it has changed
		updater.SetTopLevelOpt("disable_power_off", data.DisablePowerOff, ironicNode.DisablePowerOff)

		// We don't return here because we also have to set the
		// target provision state to manageable, which happens
		// below.
	}

	// If no PreprovisioningImage builder is enabled we set the Node network_data
	// this enables Ironic to inject the network_data into the ramdisk image
	if !p.config.havePreprovImgBuilder {
		networkDataRaw := data.PreprovisioningNetworkData
		if networkDataRaw != "" {
			var networkData map[string]interface{}
			if yamlErr := yaml.Unmarshal([]byte(networkDataRaw), &networkData); yamlErr != nil {
				p.log.Info("failed to unmarshal networkData from PreprovisioningNetworkData")
				result, err = transientError(fmt.Errorf("invalid preprovisioningNetworkData: %w", yamlErr))
				return result, provID, err
			}
			numUpdates := len(updater.Updates)
			updater.SetTopLevelOpt("network_data", networkData, ironicNode.NetworkData)
			if len(updater.Updates) != numUpdates {
				p.log.Info("adding preprovisioning network_data for node", "node", ironicNode.UUID)
			}
		}
	}

	ironicNode, success, result, err := p.tryUpdateNode(ironicNode, updater)
	if !success {
		return result, provID, err
	}

	p.log.Info("current provision state",
		"lastError", ironicNode.LastError,
		"current", ironicNode.ProvisionState,
		"target", ironicNode.TargetProvisionState,
	)

	// Ensure the node is marked manageable.
	switch nodes.ProvisionState(ironicNode.ProvisionState) {
	case nodes.Enroll:

		// If ironic is reporting an error, stop working on the node.
		if ironicNode.LastError != "" && !(credentialsChanged || restartOnFailure) {
			result, err = operationFailed(ironicNode.LastError)
			return result, provID, err
		}

		if ironicNode.TargetProvisionState == string(nodes.TargetManage) {
			// We have already tried to manage the node and did not
			// get an error, so do nothing and keep trying.
			result, err = operationContinuing(provisionRequeueDelay)
			return result, provID, err
		}

		result, err = p.changeNodeProvisionState(
			ironicNode,
			nodes.ProvisionStateOpts{Target: nodes.TargetManage},
		)
		return result, provID, err

	case nodes.Verifying:
		// If we're still waiting for the state to change in Ironic,
		// return true to indicate that we're dirty and need to be
		// reconciled again.
		result, err = operationContinuing(provisionRequeueDelay)
		return result, provID, err

	case nodes.CleanWait,
		nodes.Cleaning,
		nodes.DeployWait,
		nodes.Deploying,
		nodes.Inspecting:
		// Do not try to update the node if it's in a transient state other than InspectWait - will fail anyway.
		result, err = operationComplete()
		return result, provID, err

	case nodes.Active:
		// The host is already running, maybe it's a controlplane host?
		p.debugLog.Info("have active host", "image_source", ironicNode.InstanceInfo["image_source"])
		fallthrough

	default:
		result, err = p.configureNode(data, ironicNode, bmcAccess)
		return result, provID, err
	}
}

func (p *ironicProvisioner) enrollNode(data provisioner.ManagementAccessData, bmcAccess bmc.AccessDetails, driverInfo map[string]interface{}) (ironicNode *nodes.Node, retry bool, err error) {
	inspectInterface, err := p.getInspectInterface(bmcAccess)
	if err != nil {
		return nil, true, err
	}

	nodeCreateOpts := nodes.CreateOpts{
		Driver:              bmcAccess.Driver(),
		BIOSInterface:       bmcAccess.BIOSInterface(),
		BootInterface:       bmcAccess.BootInterface(),
		Name:                ironicNodeName(p.objectMeta),
		DriverInfo:          driverInfo,
		DeployInterface:     p.deployInterface(data),
		InspectInterface:    inspectInterface,
		ManagementInterface: bmcAccess.ManagementInterface(),
		PowerInterface:      bmcAccess.PowerInterface(),
		RAIDInterface:       bmcAccess.RAIDInterface(),
		VendorInterface:     bmcAccess.VendorInterface(),
		DisablePowerOff:     &data.DisablePowerOff,
		Properties: map[string]interface{}{
			"capabilities": buildCapabilitiesValue(nil, data.BootMode),
			"cpu_arch":     data.CPUArchitecture,
		},
	}

	if p.availableFeatures.HasFirmwareUpdates() {
		nodeCreateOpts.FirmwareInterface = bmcAccess.FirmwareInterface()
	}

	ironicNode, err = nodes.Create(p.ctx, p.client, nodeCreateOpts).Extract()
	if err == nil {
		p.publisher("Registered", "Registered new host")
	} else if gophercloud.ResponseCodeIs(err, http.StatusConflict) {
		p.log.Info("could not register host in ironic, busy")
		return nil, true, nil
	} else {
		return nil, true, fmt.Errorf("failed to register host in ironic: %s", err)
	}

	// If we know the MAC, create a port. Otherwise we will have
	// to do this after we run the introspection step.
	if p.bootMACAddress != "" {
		err = p.createPXEEnabledNodePort(ironicNode.UUID, p.bootMACAddress)
		if err != nil {
			return nil, true, err
		}
	}

	return ironicNode, false, nil
}

func (p *ironicProvisioner) ensurePort(ironicNode *nodes.Node) error {
	nodeHasAssignedPort, err := p.nodeHasAssignedPort(ironicNode)
	if err != nil {
		return err
	}

	if !nodeHasAssignedPort {
		addressIsAllocatedToPort, err := p.isAddressAllocatedToPort(p.bootMACAddress)
		if err != nil {
			return err
		}

		if !addressIsAllocatedToPort {
			err = p.createPXEEnabledNodePort(ironicNode.UUID, p.bootMACAddress)
			if err != nil {
				return err
			}
		}
	}

	return nil
}
