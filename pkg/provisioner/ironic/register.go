package ironic

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"regexp"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack/baremetal/v1/nodes"
	"github.com/gophercloud/gophercloud/v2/openstack/baremetal/v1/ports"
	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	"github.com/metal3-io/baremetal-operator/pkg/hardwareutils/bmc"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner/ironic/clients"
	"sigs.k8s.io/yaml"
)

const (
	defaultInspectInterface = "agent"
)

func bmcAddressMatches(ironicNode *nodes.Node, driverInfo map[string]any) bool {
	newAddress := make(map[string]any)
	ironicAddress := make(map[string]any)
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
func (p *ironicProvisioner) Register(ctx context.Context, data provisioner.ManagementAccessData, credentialsChanged, restartOnFailure bool) (result provisioner.Result, provID string, err error) {
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

	ironicNode, err = p.findExistingHost(ctx, p.bootMACAddress)
	if err != nil {
		var target macAddressConflictError
		if errors.As(err, &target) {
			result, err = operationFailed(target.Error())
		} else {
			result, err = transientError(fmt.Errorf("failed to find existing host: %w", err))
		}
		return result, "", err
	}

	// Some BMC types require a MAC address regardless of inspection (for
	// example VirtualBMC), and any host requires one when inspection is
	// disabled because the MAC cannot be discovered. Ensure we have one when
	// we need it; if not, place the host in an error state. This mirrors the
	// combined rule enforced by the validating webhook.
	if (bmcAccess.NeedsMAC() || data.DisableInspection) && p.bootMACAddress == "" {
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
		ironicNode, retry, err = p.enrollNode(ctx, data, bmcAccess, driverInfo)
		if err != nil {
			result, err = transientError(err)
			return result, "", err
		}
		if retry {
			result, err = retryAfterDelay(shortRetryDelay)
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

		// Update cpu_arch in Properties if specified.
		// This is important for multi-arch deployments to ensure the correct
		// architecture-specific IPA kernel/ramdisk is used via deploy_kernel_by_arch.
		if data.CPUArchitecture != "" {
			updater.SetPropertiesOpts(clients.UpdateOptsData{
				"cpu_arch": data.CPUArchitecture,
			}, ironicNode)
		}

		// We don't return here because we also have to set the
		// target provision state to manageable, which happens
		// below.
	}

	// NOTE(dtantsur): don't try to create ports in states where it's
	// either impossible because of a lock or potentially disruptive.
	switch nodes.ProvisionState(ironicNode.ProvisionState) {
	case nodes.Enroll, nodes.Manageable, nodes.Available, nodes.Active,
		// A failure can be caused by wrong ports, so allow creating.
		// TODO(dtantsur): add Hold states once they're supported by Gophercloud
		nodes.AdoptFail, nodes.InspectFail, nodes.CleanFail, nodes.DeployFail, nodes.ServiceFail, nodes.Error:
		// Try to create ports from two sources
		// bootMACAddress if available
		// HardwareData whenever inspection data is available.
		err = p.createPortsForNode(ctx, ironicNode, data.HardwareData)
		if err != nil {
			result, err = transientError(err)
			return result, provID, err
		}
	default:
	}

	// If no PreprovisioningImage builder is enabled we set the Node network_data
	// this enables Ironic to inject the network_data into the ramdisk image
	if !p.config.havePreprovImgBuilder {
		networkDataRaw := data.PreprovisioningNetworkData
		if networkDataRaw != "" {
			var networkData map[string]any
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

	ironicNode, success, result, err := p.tryUpdateNode(ctx, ironicNode, updater)
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
			result, err = operationContinuing(shortRetryDelay)
			return result, provID, err
		}

		result, err = p.changeNodeProvisionState(
			ctx,
			ironicNode,
			nodes.ProvisionStateOpts{Target: nodes.TargetManage},
		)
		return result, provID, err

	case nodes.Verifying:
		// If we're still waiting for the state to change in Ironic,
		// return true to indicate that we're dirty and need to be
		// reconciled again.
		result, err = operationContinuing(shortRetryDelay)
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
		result, err = p.configureNode(ctx, data, ironicNode, bmcAccess)
		return result, provID, err
	}
}

func (p *ironicProvisioner) enrollNode(ctx context.Context, data provisioner.ManagementAccessData, bmcAccess bmc.AccessDetails, driverInfo map[string]any) (ironicNode *nodes.Node, retry bool, err error) {
	nodeCreateOpts := nodes.CreateOpts{
		Driver:              bmcAccess.Driver(),
		BIOSInterface:       bmcAccess.BIOSInterface(),
		BootInterface:       bmcAccess.BootInterface(),
		Name:                ironicNodeName(p.objectMeta),
		DriverInfo:          driverInfo,
		FirmwareInterface:   bmcAccess.FirmwareInterface(),
		DeployInterface:     p.deployInterface(data),
		InspectInterface:    defaultInspectInterface,
		ManagementInterface: bmcAccess.ManagementInterface(),
		PowerInterface:      bmcAccess.PowerInterface(),
		RAIDInterface:       bmcAccess.RAIDInterface(),
		VendorInterface:     bmcAccess.VendorInterface(),
		DisablePowerOff:     &data.DisablePowerOff,
		Properties: map[string]any{
			"capabilities": buildCapabilitiesValue(nil, data.BootMode),
			"cpu_arch":     data.CPUArchitecture,
		},
	}

	ironicNode, err = nodes.Create(ctx, p.client, nodeCreateOpts).Extract()
	if err == nil {
		p.publisher("Registered", "Registered new host")
	} else if gophercloud.ResponseCodeIs(err, http.StatusConflict) {
		p.log.Info("could not register host in ironic, busy")
		return nil, true, nil
	} else {
		return nil, true, fmt.Errorf("failed to register host in ironic: %w", err)
	}

	return ironicNode, false, nil
}

func (p *ironicProvisioner) createPortsForNode(ctx context.Context, ironicNode *nodes.Node, hardwareData *metal3api.HardwareData) error {
	var nics []metal3api.NIC
	if hardwareData != nil && hardwareData.Spec.HardwareDetails != nil {
		nics = hardwareData.Spec.HardwareDetails.NIC
	}

	if p.bootMACAddress == "" && len(nics) == 0 {
		// we don't have anything to process, gracefully returning
		return nil
	}

	ironicNodePorts, err := p.getPorts(ctx, ironicNode.UUID, "")
	if err != nil {
		return err
	}

	ironicNodePortsList := map[string]ports.Port{}
	for _, port := range ironicNodePorts {
		ironicNodePortsList[port.Address] = port
	}

	// Mac/PXE status map
	portMacsToCreate := map[string]bool{}
	for _, nic := range nics {
		if _, ok := ironicNodePortsList[nic.MAC]; nic.MAC != "" && !ok {
			portMacsToCreate[nic.MAC] = nic.PXE
		}
	}

	if _, ok := ironicNodePortsList[p.bootMACAddress]; p.bootMACAddress != "" && !ok {
		portMacsToCreate[p.bootMACAddress] = true
	}

	p.log.Info("creating ports for node", "nodeUUID", ironicNode.UUID, "MACs", portMacsToCreate)

	for mac, pxe := range portMacsToCreate {
		err := p.createNodePort(ctx, ironicNode.UUID, mac, pxe)
		if err != nil {
			return err
		}
	}

	return nil
}
