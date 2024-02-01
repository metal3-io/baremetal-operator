package ironic

import (
	"context"
	"fmt"
	"net"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack/baremetal/v1/drivers"
	"github.com/gophercloud/gophercloud/v2/openstack/baremetal/v1/nodes"
	"github.com/gophercloud/gophercloud/v2/openstack/baremetal/v1/ports"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/yaml"

	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	"github.com/metal3-io/baremetal-operator/pkg/hardwareutils/bmc"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner/ironic/clients"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner/ironic/devicehints"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner/ironic/hardwaredetails"
)

var (
	deprovisionRequeueDelay   = time.Second * 10
	provisionRequeueDelay     = time.Second * 10
	powerRequeueDelay         = time.Second * 10
	subscriptionRequeueDelay  = time.Second * 10
	introspectionRequeueDelay = time.Second * 15
	softPowerOffTimeout       = time.Second * 180
)

const (
	// See nodes.Node.PowerState for details.
	powerOn              = string(nodes.PowerOn)
	powerOff             = string(nodes.PowerOff)
	softPowerOff         = string(nodes.SoftPowerOff)
	powerNone            = "None"
	nameSeparator        = "~"
	customDeployPriority = 80

	deployKernelKey  = "deploy_kernel"
	deployRamdiskKey = "deploy_ramdisk"
	deployISOKey     = "deploy_iso"
	kernelParamsKey  = "kernel_append_params"
)

var bootModeCapabilities = map[metal3api.BootMode]string{
	metal3api.UEFI:           "boot_mode:uefi",
	metal3api.UEFISecureBoot: "boot_mode:uefi,secure_boot:true",
	metal3api.Legacy:         "boot_mode:bios",
}

type macAddressConflictError struct {
	Address      string
	ExistingNode string
}

func (e macAddressConflictError) Error() string {
	return fmt.Sprintf("MAC address %s conflicts with existing node %s", e.Address, e.ExistingNode)
}

// NewMacAddressConflictError is a wrap for macAddressConflictError error.
func NewMacAddressConflictError(address, node string) error {
	return macAddressConflictError{Address: address, ExistingNode: node}
}

type ironicConfig struct {
	havePreprovImgBuilder            bool
	deployKernelURL                  string
	deployRamdiskURL                 string
	deployISOURL                     string
	liveISOForcePersistentBootDevice string
	maxBusyHosts                     int
	externalURL                      string
}

// Provisioner implements the provisioning.Provisioner interface
// and uses Ironic to manage the host.
type ironicProvisioner struct {
	// the global ironic settings
	config ironicConfig
	// the object metadata of the BareMetalHost resource
	objectMeta metav1.ObjectMeta
	// the UUID of the node in Ironic
	nodeID string
	// the address of the BMC
	bmcAddress string
	// whether to disable SSL certificate verification
	disableCertVerification bool
	// credentials to log in to the BMC
	bmcCreds bmc.Credentials
	// the MAC address of the PXE boot interface
	bootMACAddress string
	// a client for talking to ironic
	client *gophercloud.ServiceClient
	// a logger configured for this host
	log logr.Logger
	// a debug logger configured for this host
	debugLog logr.Logger
	// an event publisher for recording significant events
	publisher provisioner.EventPublisher
	// available API features
	availableFeatures clients.AvailableFeatures
	// request context
	ctx context.Context
}

func (p *ironicProvisioner) bmcAccess() (bmc.AccessDetails, error) {
	bmcAccess, err := bmc.NewAccessDetails(p.bmcAddress, p.disableCertVerification)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse BMC address information")
	}
	return bmcAccess, nil
}

func (p *ironicProvisioner) validateNode(ironicNode *nodes.Node) (errorMessage string, err error) {
	var validationErrors []string

	p.log.Info("validating node settings in ironic")
	validateResult, err := nodes.Validate(p.ctx, p.client, ironicNode.UUID).Extract()
	if err != nil {
		return "", err // do not wrap error so we can check type in caller
	}
	if !validateResult.Boot.Result {
		validationErrors = append(validationErrors, validateResult.Boot.Reason)
	}
	if !validateResult.Deploy.Result {
		validationErrors = append(validationErrors, validateResult.Deploy.Reason)
	}
	if len(validationErrors) > 0 {
		// We expect to see errors of this nature sometimes, so rather
		// than reporting it as a reconcile error we record the error
		// status on the host and return.
		errorMessage = fmt.Sprintf("host validation error: %s",
			strings.Join(validationErrors, "; "))
		return errorMessage, nil
	}
	return "", nil
}

func (p *ironicProvisioner) listAllPorts(address string) ([]ports.Port, error) {
	var allPorts []ports.Port

	opts := ports.ListOpts{
		Fields: []string{"node_uuid"},
	}

	if address != "" {
		opts.Address = address
	}

	pager := ports.List(p.client, opts)

	allPages, err := pager.AllPages(p.ctx)

	if err != nil {
		return allPorts, err
	}

	return ports.ExtractPorts(allPages)
}

func (p *ironicProvisioner) getNode() (*nodes.Node, error) {
	if p.nodeID == "" {
		return nil, provisioner.ErrNeedsRegistration
	}

	ironicNode, err := nodes.Get(p.ctx, p.client, p.nodeID).Extract()
	switch err.(type) {
	case nil:
		p.debugLog.Info("found existing node by ID")
		return ironicNode, nil
	case gophercloud.ErrDefault404:
		// Look by ID failed, trying to lookup by hostname in case it was
		// previously created
		return nil, provisioner.ErrNeedsRegistration
	default:
		return nil, errors.Wrap(err,
			fmt.Sprintf("failed to find node by ID %s", p.nodeID))
	}
}

// Verifies that node has port assigned by Ironic.
func (p *ironicProvisioner) nodeHasAssignedPort(ironicNode *nodes.Node) (bool, error) {
	opts := ports.ListOpts{
		Fields:   []string{"node_uuid"},
		NodeUUID: ironicNode.UUID,
	}

	pager := ports.List(p.client, opts)

	allPages, err := pager.AllPages(p.ctx)
	if err != nil {
		return false, errors.Wrap(err, "failed to page over list of ports")
	}

	empty, err := allPages.IsEmpty()
	if err != nil {
		return false, errors.Wrap(err, "failed to check port list status")
	}

	if empty {
		p.debugLog.Info("node has no assigned port", "node", ironicNode.UUID)
		return false, nil
	}

	p.debugLog.Info("node has assigned port", "node", ironicNode.UUID)
	return true, nil
}

// Verify that MAC is already allocated to some node port.
func (p *ironicProvisioner) isAddressAllocatedToPort(address string) (bool, error) {
	allPorts, err := p.listAllPorts(address)
	if err != nil {
		return false, errors.Wrap(err, fmt.Sprintf("failed to list ports for %s", address))
	}

	if len(allPorts) == 0 {
		p.debugLog.Info("address does not have allocated ports", "address", address)
		return false, nil
	}

	p.debugLog.Info("address is allocated to port", "address", address, "node", allPorts[0].NodeUUID)
	return true, nil
}

// Look for an existing registration for the host in Ironic.
func (p *ironicProvisioner) findExistingHost(bootMACAddress string) (ironicNode *nodes.Node, err error) {
	// Try to load the node by UUID
	ironicNode, err = p.getNode()
	if !errors.Is(err, provisioner.ErrNeedsRegistration) {
		return
	}

	// Try to load the node by name
	nodeSearchList := []string{ironicNodeName(p.objectMeta)}
	if !strings.Contains(p.objectMeta.Name, nameSeparator) {
		nodeSearchList = append(nodeSearchList, p.objectMeta.Name)
	}

	for _, nodeName := range nodeSearchList {
		p.debugLog.Info("looking for existing node by name", "name", nodeName)
		ironicNode, err = nodes.Get(p.ctx, p.client, nodeName).Extract()
		switch err.(type) {
		case nil:
			p.debugLog.Info("found existing node by name", "name", nodeName, "node", ironicNode.UUID)
			return ironicNode, nil
		case gophercloud.ErrDefault404:
			p.log.Info(
				fmt.Sprintf("node with name %s doesn't exist", nodeName))
		default:
			return nil, errors.Wrap(err,
				fmt.Sprintf("failed to find node by name %s", nodeName))
		}
	}

	// Try to load the node by port address
	p.log.Info("looking for existing node by MAC", "MAC", bootMACAddress)
	allPorts, err := p.listAllPorts(bootMACAddress)

	if err != nil {
		p.log.Info("failed to find an existing port with address", "MAC", bootMACAddress)
		return nil, nil
	}

	if len(allPorts) > 0 {
		nodeUUID := allPorts[0].NodeUUID
		ironicNode, err = nodes.Get(p.ctx, p.client, nodeUUID).Extract()
		switch err.(type) {
		case nil:
			p.debugLog.Info("found existing node by MAC", "MAC", bootMACAddress, "node", ironicNode.UUID, "name", ironicNode.Name)

			// If the node has a name, this means we didn't find it above.
			if ironicNode.Name != "" {
				return nil, NewMacAddressConflictError(bootMACAddress, ironicNode.Name)
			}

			return ironicNode, nil
		case gophercloud.ErrDefault404:
			return nil, errors.Wrap(err,
				fmt.Sprintf("port %s exists but linked node doesn't %s", bootMACAddress, nodeUUID))
		default:
			return nil, errors.Wrap(err,
				fmt.Sprintf("port %s exists but failed to find linked node by ID %s", bootMACAddress, nodeUUID))
		}
	} else {
		p.log.Info("port with address doesn't exist", "MAC", bootMACAddress)
	}

	// Either the node was never created or the Ironic database has
	// been dropped.
	return nil, nil
}

func (p *ironicProvisioner) createPXEEnabledNodePort(uuid, macAddress string) error {
	p.log.Info("creating PXE enabled ironic port for node", "NodeUUID", uuid, "MAC", macAddress)

	enable := true

	_, err := ports.Create(
		p.ctx,
		p.client,
		ports.CreateOpts{
			NodeUUID:   uuid,
			Address:    macAddress,
			PXEEnabled: &enable,
		}).Extract()
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("failed to create ironic port for node: %s, MAC: %s", uuid, macAddress))
	}

	return nil
}

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

// ValidateManagementAccess registers the host with the provisioning
// system and tests the connection information for the host to verify
// that the location and credentials work.
//
// FIXME(dhellmann): We should rename this method to describe what it
// actually does.
func (p *ironicProvisioner) ValidateManagementAccess(data provisioner.ManagementAccessData, credentialsChanged, restartOnFailure bool) (result provisioner.Result, provID string, err error) {
	bmcAccess, err := p.bmcAccess()
	if err != nil {
		result, err = operationFailed(err.Error())
		return
	}

	var ironicNode *nodes.Node
	updater := updateOptsBuilder(p.log)

	p.debugLog.Info("validating management access")

	ironicNode, err = p.findExistingHost(p.bootMACAddress)
	if err != nil {
		switch err.(type) {
		case macAddressConflictError:
			result, err = operationFailed(err.Error())
		default:
			result, err = transientError(errors.Wrap(err, "failed to find existing host"))
		}
		return
	}

	// Some BMC types require a MAC address, so ensure we have one
	// when we need it. If not, place the host in an error state.
	if bmcAccess.NeedsMAC() && p.bootMACAddress == "" {
		msg := fmt.Sprintf("BMC driver %s requires a BootMACAddress value", bmcAccess.Type())
		p.log.Info(msg)
		result, err = operationFailed(msg)
		return
	}

	driverInfo := bmcAccess.DriverInfo(p.bmcCreds)
	driverInfo = setExternalURL(p, driverInfo)

	// If we have not found a node yet, we need to create one
	if ironicNode == nil {
		p.log.Info("registering host in ironic")

		if data.BootMode == metal3api.UEFISecureBoot && !bmcAccess.SupportsSecureBoot() {
			msg := fmt.Sprintf("BMC driver %s does not support secure boot", bmcAccess.Type())
			p.log.Info(msg)
			result, err = operationFailed(msg)
			return
		}

		inspectInterface, driverErr := p.getInspectInterface(bmcAccess)
		if driverErr != nil {
			result, err = transientError(driverErr)
			return
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
			Properties: map[string]interface{}{
				"capabilities": bootModeCapabilities[data.BootMode],
			},
		}

		if p.availableFeatures.HasFirmwareUpdates() {
			nodeCreateOpts.FirmwareInterface = bmcAccess.FirmwareInterface()
		}

		ironicNode, err = nodes.Create(p.ctx, p.client, nodeCreateOpts).Extract()
		switch err.(type) {
		case nil:
			p.publisher("Registered", "Registered new host")
		case gophercloud.ErrDefault409:
			p.log.Info("could not register host in ironic, busy")
			result, err = retryAfterDelay(provisionRequeueDelay)
			return
		default:
			result, err = transientError(errors.Wrap(err, "failed to register host in ironic"))
			return
		}

		// Store the ID so other methods can assume it is set and so
		// we can find the node again later.
		provID = ironicNode.UUID

		// If we know the MAC, create a port. Otherwise we will have
		// to do this after we run the introspection step.
		if p.bootMACAddress != "" {
			err = p.createPXEEnabledNodePort(ironicNode.UUID, p.bootMACAddress)
			if err != nil {
				result, err = transientError(err)
				return
			}
		}
	} else {
		// FIXME(dhellmann): At this point we have found an existing
		// node in ironic by looking it up. We need to check its
		// settings against what we have in the host, and change them
		// if there are differences.
		provID = ironicNode.UUID

		updater.SetTopLevelOpt("name", ironicNodeName(p.objectMeta), ironicNode.Name)

		var bmcAddressChanged bool
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
		if !reflect.DeepEqual(newAddress, ironicAddress) {
			bmcAddressChanged = true
		}

		// When node exists but has no assigned port to it by Ironic and actuall address (MAC) is present
		// in host config and is not allocated to different node lets try to create port for this node.
		if p.bootMACAddress != "" {
			var nodeHasAssignedPort, addressIsAllocatedToPort bool

			nodeHasAssignedPort, err = p.nodeHasAssignedPort(ironicNode)
			if err != nil {
				result, err = transientError(err)
				return
			}

			if !nodeHasAssignedPort {
				addressIsAllocatedToPort, err = p.isAddressAllocatedToPort(p.bootMACAddress)
				if err != nil {
					result, err = transientError(err)
					return
				}

				if !addressIsAllocatedToPort {
					err = p.createPXEEnabledNodePort(ironicNode.UUID, p.bootMACAddress)
					if err != nil {
						result, err = transientError(err)
						return
					}
				}
			}
		}

		// The actual password is not returned from ironic, so we want to
		// update the whole DriverInfo only if the credentials or BMC address
		// has changed, otherwise we will be writing on every call to this
		// function.
		if credentialsChanged || bmcAddressChanged {
			p.log.Info("Updating driver info because the credentials and/or the BMC address changed")
			updater.SetTopLevelOpt("driver_info", driverInfo, ironicNode.DriverInfo)
		}

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
				return
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
		return
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
			return
		}

		if ironicNode.TargetProvisionState == string(nodes.TargetManage) {
			// We have already tried to manage the node and did not
			// get an error, so do nothing and keep trying.
			result, err = operationContinuing(provisionRequeueDelay)
			return
		}

		result, err = p.changeNodeProvisionState(
			ironicNode,
			nodes.ProvisionStateOpts{Target: nodes.TargetManage},
		)
		return

	case nodes.Verifying:
		// If we're still waiting for the state to change in Ironic,
		// return true to indicate that we're dirty and need to be
		// reconciled again.
		result, err = operationContinuing(provisionRequeueDelay)
		return

	case nodes.CleanWait,
		nodes.Cleaning,
		nodes.DeployWait,
		nodes.Deploying,
		nodes.Inspecting:
		// Do not try to update the node if it's in a transient state other than InspectWait - will fail anyway.
		result, err = operationComplete()
		return

	case nodes.Active:
		// The host is already running, maybe it's a controlplane host?
		p.debugLog.Info("have active host", "image_source", ironicNode.InstanceInfo["image_source"])
		fallthrough

	default:
		result, err = p.configureImages(data, ironicNode, bmcAccess)
		return
	}
}

func (p *ironicProvisioner) configureImages(data provisioner.ManagementAccessData, ironicNode *nodes.Node, bmcAccess bmc.AccessDetails) (result provisioner.Result, err error) {
	updater := updateOptsBuilder(p.log)

	deployImageInfo := setDeployImage(p.config, bmcAccess, data.PreprovisioningImage)
	updater.SetDriverInfoOpts(deployImageInfo, ironicNode)

	// NOTE(dtantsur): It is risky to update image information for active nodes since it may affect the ability to clean up.
	if (data.CurrentImage != nil || data.HasCustomDeploy) && ironicNode.ProvisionState != string(nodes.Active) {
		p.getImageUpdateOptsForNode(ironicNode, data.CurrentImage, data.BootMode, data.HasCustomDeploy, updater)
	}
	updater.SetTopLevelOpt("automated_clean",
		data.AutomatedCleaningMode != metal3api.CleaningModeDisabled,
		ironicNode.AutomatedClean)

	_, success, result, err := p.tryUpdateNode(ironicNode, updater)
	if !success {
		return
	}

	result, err = operationComplete()

	switch data.State {
	case metal3api.StateProvisioning,
		metal3api.StateDeprovisioning:
		if data.State == metal3api.StateProvisioning {
			if data.CurrentImage.IsLiveISO() {
				// Live ISO doesn't need pre-provisioning image
				return
			}
		} else {
			if data.AutomatedCleaningMode == metal3api.CleaningModeDisabled {
				// No need for pre-provisioning image if cleaning disabled
				return
			}
		}
		fallthrough
	case metal3api.StateInspecting,
		metal3api.StatePreparing:
		if deployImageInfo == nil {
			if p.config.havePreprovImgBuilder {
				result, err = transientError(provisioner.ErrNeedsPreprovisioningImage)
			} else {
				result, err = operationFailed("no preprovisioning image available")
			}
			return
		}
	}

	return
}

// PreprovisioningImageFormats returns a list of acceptable formats for a
// pre-provisioning image to be built by a PreprovisioningImage object. The
// list should be nil if no image build is requested.
func (p *ironicProvisioner) PreprovisioningImageFormats() ([]metal3api.ImageFormat, error) {
	if !p.config.havePreprovImgBuilder {
		return nil, nil
	}

	accessDetails, err := p.bmcAccess()
	if err != nil {
		return nil, err
	}

	var formats []metal3api.ImageFormat
	if accessDetails.SupportsISOPreprovisioningImage() {
		formats = append(formats, metal3api.ImageFormatISO)
	}
	formats = append(formats, metal3api.ImageFormatInitRD)

	return formats, nil
}

func setExternalURL(p *ironicProvisioner, driverInfo map[string]interface{}) map[string]interface{} {
	if _, ok := driverInfo["external_http_url"]; ok {
		driverInfo["external_http_url"] = nil
	}

	if p.config.externalURL == "" {
		return driverInfo
	}

	parsedURL, err := bmc.GetParsedURL(p.bmcAddress)
	if err != nil {
		p.log.Info("Failed to parse BMC address", "bmcAddress", p.bmcAddress, "err", err)
		return driverInfo
	}

	ip := net.ParseIP(parsedURL.Hostname())
	if ip == nil {
		// Maybe it's a hostname?
		ips, err := net.LookupIP(p.bmcAddress)
		if err != nil {
			p.log.Info("Failed to look up the IP address for BMC hostname", "hostname", p.bmcAddress)
			return driverInfo
		}

		if len(ips) == 0 {
			p.log.Info("Zero IP addresses for BMC hostname", "hostname", p.bmcAddress)
			return driverInfo
		}

		ip = ips[0]
	}

	// In the case of IPv4, we don't have to do anything.
	if ip.To4() != nil {
		return driverInfo
	}

	driverInfo["external_http_url"] = p.config.externalURL

	return driverInfo
}

func setDeployImage(config ironicConfig, accessDetails bmc.AccessDetails, hostImage *provisioner.PreprovisioningImage) optionsData {
	deployImageInfo := optionsData{
		deployKernelKey:  nil,
		deployRamdiskKey: nil,
		deployISOKey:     nil,
		kernelParamsKey:  nil,
	}

	allowISO := accessDetails.SupportsISOPreprovisioningImage()

	if hostImage != nil {
		switch hostImage.Format {
		case metal3api.ImageFormatISO:
			if allowISO {
				deployImageInfo[deployISOKey] = hostImage.ImageURL
				return deployImageInfo
			}
		case metal3api.ImageFormatInitRD:
			if hostImage.KernelURL != "" {
				deployImageInfo[deployKernelKey] = hostImage.KernelURL
			} else if config.deployKernelURL == "" {
				return nil
			} else {
				deployImageInfo[deployKernelKey] = config.deployKernelURL
			}
			deployImageInfo[deployRamdiskKey] = hostImage.ImageURL
			if hostImage.ExtraKernelParams != "" {
				// Using %default% prevents overriding the config in ironic-image
				deployImageInfo[kernelParamsKey] = fmt.Sprintf("%%default%% %s", hostImage.ExtraKernelParams)
			}
			return deployImageInfo
		}
	}

	if !config.havePreprovImgBuilder {
		if allowISO && config.deployISOURL != "" {
			deployImageInfo[deployISOKey] = config.deployISOURL
			return deployImageInfo
		}
		if config.deployKernelURL != "" && config.deployRamdiskURL != "" {
			deployImageInfo[deployKernelKey] = config.deployKernelURL
			deployImageInfo[deployRamdiskKey] = config.deployRamdiskURL
			return deployImageInfo
		}
	}

	return nil
}

func (p *ironicProvisioner) tryUpdateNode(ironicNode *nodes.Node, updater *nodeUpdater) (updatedNode *nodes.Node, success bool, result provisioner.Result, err error) {
	if len(updater.Updates) == 0 {
		updatedNode = ironicNode
		success = true
		return
	}

	p.log.Info("updating node settings in ironic", "updateCount", len(updater.Updates))
	updatedNode, err = nodes.Update(p.ctx, p.client, ironicNode.UUID, updater.Updates).Extract()
	switch err.(type) {
	case nil:
		success = true
	case gophercloud.ErrDefault409:
		p.log.Info("could not update node settings in ironic, busy or update cannot be applied in the current state")
		result, err = retryAfterDelay(provisionRequeueDelay)
	default:
		result, err = transientError(errors.Wrap(err, "failed to update host settings in ironic"))
	}

	return
}

func (p *ironicProvisioner) tryChangeNodeProvisionState(ironicNode *nodes.Node, opts nodes.ProvisionStateOpts) (success bool, result provisioner.Result, err error) {
	p.log.Info("changing provisioning state",
		"current", ironicNode.ProvisionState,
		"existing target", ironicNode.TargetProvisionState,
		"new target", opts.Target,
	)

	// Changing provision state in maintenance mode will not work.
	if ironicNode.Fault != "" {
		p.log.Info("node has a fault, will retry", "fault", ironicNode.Fault, "reason", ironicNode.MaintenanceReason)
		result, err = retryAfterDelay(provisionRequeueDelay)
		return
	}
	if ironicNode.Maintenance {
		p.log.Info("trying to change a provision state for a node in maintenance, removing maintenance first", "reason", ironicNode.MaintenanceReason)
		result, err = p.setMaintenanceFlag(ironicNode, false, "")
		return
	}

	changeResult := nodes.ChangeProvisionState(p.ctx, p.client, ironicNode.UUID, opts)
	switch changeResult.Err.(type) {
	case nil:
		success = true
	case gophercloud.ErrDefault409:
		p.log.Info("could not change state of host, busy")
		result, err = retryAfterDelay(provisionRequeueDelay)
		return
	default:
		result, err = transientError(errors.Wrap(changeResult.Err,
			fmt.Sprintf("failed to change provisioning state to %q", opts.Target)))
		return
	}

	result, err = operationContinuing(provisionRequeueDelay)
	return
}

func (p *ironicProvisioner) changeNodeProvisionState(ironicNode *nodes.Node, opts nodes.ProvisionStateOpts) (result provisioner.Result, err error) {
	_, result, err = p.tryChangeNodeProvisionState(ironicNode, opts)
	return
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
		updateOptsBuilder(p.log).
			SetPropertiesOpts(optionsData{
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
		return
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
		return
	case nodes.InspectWait:
		if forceReboot {
			return p.abortInspection(ironicNode)
		}

		fallthrough
	case nodes.Inspecting:
		p.log.Info("inspection in progress")
		result, err = operationContinuing(introspectionRequeueDelay)
		return
	case nodes.InspectFail:
		if !restartOnFailure {
			failure := ironicNode.LastError
			if failure == "" {
				failure = "Inspection failed"
			}
			p.log.Info("inspection failed", "error", failure)
			result, err = operationFailed(failure)
			return
		}
		refresh = true
		fallthrough
	case nodes.Manageable:
		if refresh {
			result, started, err = p.startInspection(data, ironicNode)
			return
		}
	default:
		p.log.Info("unexpected provisioning state for inspection",
			"provisionState", ironicNode.ProvisionState, "targetProvisionState", ironicNode.TargetProvisionState, "lastError", ironicNode.LastError)
		result, err = transientError(errors.Errorf("unexpected provision state %s", ironicNode.ProvisionState))
		return
	}

	p.log.Info("getting hardware details from inspection")
	response := nodes.GetInventory(p.ctx, p.client, ironicNode.UUID)
	introData, err := response.Extract()
	if err != nil {
		if _, isNotFound := err.(gophercloud.ErrDefault404); isNotFound {
			// The node has just been enrolled, inspection hasn't been started yet.
			result, started, err = p.startInspection(data, ironicNode)
			return
		}
		result, err = transientError(errors.Wrap(err, "failed to retrieve hardware introspection data"))
		return
	}

	// Introspection is done
	p.log.Info("inspection finished successfully", "data", response.Body)

	details = hardwaredetails.GetHardwareDetails(introData, p.log)
	p.publisher("InspectionComplete", "Hardware inspection completed")
	result, err = operationComplete()
	return
}

// UpdateHardwareState fetches the latest hardware state of the server
// and updates the HardwareDetails field of the host with details. It
// is expected to do this in the least expensive way possible, such as
// reading from a cache.
func (p *ironicProvisioner) UpdateHardwareState() (hwState provisioner.HardwareState, err error) {
	p.debugLog.Info("updating hardware state")

	ironicNode, err := p.getNode()
	if err != nil {
		return
	}

	switch ironicNode.PowerState {
	case powerOn, powerOff:
		discoveredVal := ironicNode.PowerState == powerOn
		hwState.PoweredOn = &discoveredVal
	case powerNone:
		p.log.Info("could not determine power state", "value", ironicNode.PowerState)
	default:
		p.log.Info("unknown power state", "value", ironicNode.PowerState)
	}
	return
}

func (p *ironicProvisioner) setLiveIsoUpdateOptsForNode(ironicNode *nodes.Node, imageData *metal3api.Image, updater *nodeUpdater) {
	optValues := optionsData{
		"boot_iso": imageData.URL,

		// remove any image_source or checksum options
		"image_source":        nil,
		"image_os_hash_value": nil,
		"image_os_hash_algo":  nil,
		"image_checksum":      nil,
	}
	updater.
		SetInstanceInfoOpts(optValues, ironicNode).
		SetTopLevelOpt("deploy_interface", "ramdisk", ironicNode.DeployInterface)

	driverOptValues := optionsData{"force_persistent_boot_device": "Default"}
	if p.config.liveISOForcePersistentBootDevice != "" {
		driverOptValues = optionsData{
			"force_persistent_boot_device": p.config.liveISOForcePersistentBootDevice,
		}
	}
	updater.SetDriverInfoOpts(driverOptValues, ironicNode)
}

func (p *ironicProvisioner) setDirectDeployUpdateOptsForNode(ironicNode *nodes.Node, imageData *metal3api.Image, updater *nodeUpdater) {
	checksum, checksumType, ok := imageData.GetChecksum()
	if !ok {
		p.log.Info("image/checksum not found for host")
		return
	}

	optValues := optionsData{
		// Remove any boot_iso field
		"boot_iso":          nil,
		"image_source":      imageData.URL,
		"image_disk_format": imageData.DiskFormat,
	}

	if checksumType == "" {
		optValues["image_checksum"] = checksum
		optValues["image_os_hash_algo"] = nil
		optValues["image_os_hash_value"] = nil
	} else {
		optValues["image_checksum"] = nil
		optValues["image_os_hash_algo"] = checksumType
		optValues["image_os_hash_value"] = checksum
	}
	updater.
		SetInstanceInfoOpts(optValues, ironicNode)

	if ironicNode.DeployInterface == "ramdisk" || ironicNode.DeployInterface == "custom-agent" {
		updater.SetTopLevelOpt("deploy_interface", nil, ironicNode.DeployInterface)
	}

	driverOptValues := optionsData{
		"force_persistent_boot_device": "Default",
	}
	updater.SetDriverInfoOpts(driverOptValues, ironicNode)
}

func (p *ironicProvisioner) setCustomDeployUpdateOptsForNode(ironicNode *nodes.Node, imageData *metal3api.Image, updater *nodeUpdater) {
	var optValues optionsData
	if imageData != nil && imageData.URL != "" {
		checksum, checksumType, ok := imageData.GetChecksum()
		// NOTE(dtantsur): all fields are optional for custom deploy
		if ok {
			optValues = optionsData{
				"boot_iso":            nil,
				"image_checksum":      nil,
				"image_source":        imageData.URL,
				"image_os_hash_algo":  checksumType,
				"image_os_hash_value": checksum,
				"image_disk_format":   imageData.DiskFormat,
			}
		} else {
			optValues = optionsData{
				"boot_iso":            nil,
				"image_checksum":      nil,
				"image_source":        imageData.URL,
				"image_os_hash_algo":  nil,
				"image_os_hash_value": nil,
				"image_disk_format":   imageData.DiskFormat,
			}
		}
	} else {
		// Clean up everything
		optValues = optionsData{
			"boot_iso":            nil,
			"image_checksum":      nil,
			"image_source":        nil,
			"image_os_hash_algo":  nil,
			"image_os_hash_value": nil,
			"image_disk_format":   nil,
		}
	}

	updater.
		SetInstanceInfoOpts(optValues, ironicNode).
		SetTopLevelOpt("deploy_interface", "custom-agent", ironicNode.DeployInterface)
}

func (p *ironicProvisioner) getImageUpdateOptsForNode(ironicNode *nodes.Node, imageData *metal3api.Image, bootMode metal3api.BootMode, hasCustomDeploy bool, updater *nodeUpdater) {
	// instance_uuid
	updater.SetTopLevelOpt("instance_uuid", string(p.objectMeta.UID), ironicNode.InstanceUUID)

	// Secure boot is a normal capability that goes into instance_info (we
	// also put it to properties for consistency, although it's not
	// strictly required in our case).

	// Instance info capabilities were invented later and
	// use a normal JSON mapping instead of a custom
	// string value.
	capabilitiesII := map[string]string{}
	if bootMode == metal3api.UEFISecureBoot {
		capabilitiesII["secure_boot"] = "true"
	}

	updater.SetInstanceInfoOpts(optionsData{"capabilities": capabilitiesII}, ironicNode)

	if hasCustomDeploy {
		// Custom deploy process
		p.setCustomDeployUpdateOptsForNode(ironicNode, imageData, updater)
	} else if imageData.IsLiveISO() {
		// Set live-iso format options
		p.setLiveIsoUpdateOptsForNode(ironicNode, imageData, updater)
	} else {
		// Set deploy_interface direct options when not booting a live-iso
		p.setDirectDeployUpdateOptsForNode(ironicNode, imageData, updater)
	}
}

func (p *ironicProvisioner) getUpdateOptsForNode(ironicNode *nodes.Node, data provisioner.ProvisionData) *nodeUpdater {
	updater := updateOptsBuilder(p.log)

	hasCustomDeploy := data.CustomDeploy != nil && data.CustomDeploy.Method != ""
	p.getImageUpdateOptsForNode(ironicNode, &data.Image, data.BootMode, hasCustomDeploy, updater)

	opts := optionsData{
		"root_device":  devicehints.MakeHintMap(data.RootDeviceHints),
		"capabilities": buildCapabilitiesValue(ironicNode, data.BootMode),
	}
	if data.CPUArchitecture != "" {
		opts["cpu_arch"] = data.CPUArchitecture
	}
	updater.SetPropertiesOpts(opts, ironicNode)

	return updater
}

// GetFirmwareSettings gets the BIOS settings and optional schema from the host and returns maps.
func (p *ironicProvisioner) GetFirmwareSettings(includeSchema bool) (settings metal3api.SettingsMap, schema map[string]metal3api.SettingSchema, err error) {
	ironicNode, err := p.getNode()
	if err != nil {
		return nil, nil, errors.Wrap(err, "could not get node for BIOS settings")
	}

	// Get the settings from Ironic via Gophercloud
	var settingsList []nodes.BIOSSetting
	var biosListErr error
	if includeSchema {
		opts := nodes.ListBIOSSettingsOpts{Detail: true}
		settingsList, biosListErr = nodes.ListBIOSSettings(p.ctx, p.client, ironicNode.UUID, opts).Extract()
	} else {
		settingsList, biosListErr = nodes.ListBIOSSettings(p.ctx, p.client, ironicNode.UUID, nil).Extract()
	}
	if biosListErr != nil {
		return nil, nil, errors.Wrap(biosListErr,
			fmt.Sprintf("could not get BIOS settings for node %s", ironicNode.UUID))
	}
	p.log.Info("retrieved BIOS settings for node", "node", ironicNode.UUID, "size", len(settingsList))

	settings = make(map[string]string)
	schema = make(map[string]metal3api.SettingSchema)

	for _, v := range settingsList {
		settings[v.Name] = v.Value

		if includeSchema {
			// add to schema
			schema[v.Name] = metal3api.SettingSchema{
				AttributeType:   v.AttributeType,
				AllowableValues: v.AllowableValues,
				LowerBound:      v.LowerBound,
				UpperBound:      v.UpperBound,
				MinLength:       v.MinLength,
				MaxLength:       v.MaxLength,
				ReadOnly:        v.ReadOnly,
				Unique:          v.Unique,
			}
		}
	}

	return settings, schema, nil
}

// GetFirmwareComponents gets all available firmware components for a node and return a list.
func (p *ironicProvisioner) GetFirmwareComponents() ([]metal3api.FirmwareComponentStatus, error) {
	ironicNode, err := p.getNode()
	if err != nil {
		return nil, fmt.Errorf("could not get node to retrieve firmware components: %w", err)
	}

	if !p.availableFeatures.HasFirmwareUpdates() {
		return nil, fmt.Errorf("current ironic version does not support firmware updates")
	}
	// Get the components from Ironic via Gophercloud
	componentList, componentListErr := nodes.ListFirmware(p.ctx, p.client, ironicNode.UUID).Extract()

	if componentListErr != nil {
		bmcAccess, _ := p.bmcAccess()
		if bmcAccess.FirmwareInterface() == "no-firmware" {
			return nil, fmt.Errorf("node %s is using firmware interface %s: %w", ironicNode.UUID, bmcAccess.FirmwareInterface(), componentListErr)
		}

		return nil, fmt.Errorf("could not get firmware components for node %s: %w", ironicNode.UUID, componentListErr)
	}

	// Setting to 2 since we only support bmc and bios
	componentsInfo := make([]metal3api.FirmwareComponentStatus, 0, 2)

	// Iterate over the list of components to extract their information and update the list.
	for _, fwc := range componentList {
		if fwc.Component != "bios" && fwc.Component != "bmc" {
			p.log.Info("ignoring firmware component for node", "component", fwc.Component, "node", ironicNode.UUID)
			continue
		}
		component := metal3api.FirmwareComponentStatus{
			Component:          fwc.Component,
			InitialVersion:     fwc.InitialVersion,
			CurrentVersion:     fwc.CurrentVersion,
			LastVersionFlashed: fwc.LastVersionFlashed,
			UpdatedAt: metav1.Time{
				Time: *fwc.UpdatedAt,
			},
		}
		componentsInfo = append(componentsInfo, component)
		p.log.Info("firmware component added for node", "component", fwc.Component, "node", ironicNode.UUID)
	}

	return componentsInfo, componentListErr
}

// We can't just replace the capabilities because we need to keep the
// values provided by inspection. We can't replace only the boot_mode
// because the API isn't fine-grained enough for that. So we have to
// look at the existing value and modify it. This function
// encapsulates the logic for building the value and knowing which
// update operation to use with the results.
func buildCapabilitiesValue(ironicNode *nodes.Node, bootMode metal3api.BootMode) string {
	capabilities, ok := ironicNode.Properties["capabilities"]
	if !ok {
		// There is no existing capabilities value
		return bootModeCapabilities[bootMode]
	}
	existingCapabilities := capabilities.(string)

	if existingCapabilities == "" {
		// The existing value is empty so we can replace the whole
		// thing.
		return bootModeCapabilities[bootMode]
	}

	var filteredCapabilities []string
	for _, item := range strings.Split(existingCapabilities, ",") {
		if !strings.HasPrefix(item, "boot_mode:") && !strings.HasPrefix(item, "secure_boot:") {
			filteredCapabilities = append(filteredCapabilities, item)
		}
	}
	filteredCapabilities = append(filteredCapabilities, bootModeCapabilities[bootMode])

	return strings.Join(filteredCapabilities, ",")
}

func (p *ironicProvisioner) setUpForProvisioning(ironicNode *nodes.Node, data provisioner.ProvisionData) (result provisioner.Result, err error) {
	p.log.Info("starting provisioning", "node properties", ironicNode.Properties)

	ironicNode, success, result, err := p.tryUpdateNode(ironicNode,
		p.getUpdateOptsForNode(ironicNode, data))
	if !success {
		return
	}

	p.log.Info("validating host settings")

	errorMessage, err := p.validateNode(ironicNode)
	switch err.(type) {
	case nil:
	case gophercloud.ErrDefault409:
		p.log.Info("could not validate host during registration, busy")
		return retryAfterDelay(provisionRequeueDelay)
	default:
		return transientError(errors.Wrap(err, "failed to validate host during registration"))
	}
	if errorMessage != "" {
		return operationFailed(errorMessage)
	}

	// If validation is successful we can start moving the host
	// through the states necessary to make it "available".
	p.log.Info("starting provisioning",
		"lastError", ironicNode.LastError,
		"current", ironicNode.ProvisionState,
		"target", ironicNode.TargetProvisionState,
		"deploy step", ironicNode.DeployStep,
	)
	p.publisher("ProvisioningStarted",
		fmt.Sprintf("Image provisioning started for %s", data.Image.URL))
	return
}

func (p *ironicProvisioner) deployInterface(data provisioner.ManagementAccessData) (result string) {
	if data.CurrentImage.IsLiveISO() {
		result = "ramdisk"
	}
	if data.HasCustomDeploy {
		result = "custom-agent"
	}
	return result
}

// Adopt notifies the provisioner that the state machine believes the host
// to be currently provisioned, and that it should be managed as such.
func (p *ironicProvisioner) Adopt(data provisioner.AdoptData, restartOnFailure bool) (result provisioner.Result, err error) {
	ironicNode, err := p.getNode()
	if err != nil {
		return transientError(err)
	}

	switch nodes.ProvisionState(ironicNode.ProvisionState) {
	case nodes.Enroll, nodes.Verifying:
		return transientError(fmt.Errorf("invalid state for adopt: %s",
			ironicNode.ProvisionState))
	case nodes.Manageable:
		_, hasImageSource := ironicNode.InstanceInfo["image_source"]
		_, hasBootISO := ironicNode.InstanceInfo["boot_iso"]
		if data.State == metal3api.StateDeprovisioning &&
			!(hasImageSource || hasBootISO) {
			// If we got here after a fresh registration and image data is
			// available, it should have been added to the node during
			// registration. If it isn't present then we got here due to a
			// failed cleaning on deprovision. The node will be cleaned again
			// before the next provisioning, so just allow the controller to
			// continue without adopting.
			p.log.Info("no image info; not adopting", "state", ironicNode.ProvisionState)
			return operationComplete()
		}
		return p.changeNodeProvisionState(
			ironicNode,
			nodes.ProvisionStateOpts{
				Target: nodes.TargetAdopt,
			},
		)
	case nodes.Adopting:
		return operationContinuing(provisionRequeueDelay)
	case nodes.AdoptFail:
		if restartOnFailure {
			return p.changeNodeProvisionState(
				ironicNode,
				nodes.ProvisionStateOpts{
					Target: nodes.TargetAdopt,
				},
			)
		}
		return operationFailed(fmt.Sprintf("Host adoption failed: %s",
			ironicNode.LastError))
	case nodes.Active:
		// Empty Fault means that maintenance was set manually, not by Ironic
		if ironicNode.Maintenance && ironicNode.Fault == "" && data.State != metal3api.StateDeleting {
			p.log.Info("active node was found to be in maintenance, updating", "state", data.State)
			return p.setMaintenanceFlag(ironicNode, false, "")
		}
	default:
	}
	return operationComplete()
}

func (p *ironicProvisioner) ironicHasSameImage(ironicNode *nodes.Node, image metal3api.Image) (sameImage bool) {
	// To make it easier to test if ironic is configured with
	// the same image we are trying to provision to the host.
	if image.IsLiveISO() {
		sameImage = (ironicNode.InstanceInfo["boot_iso"] == image.URL)
		p.log.Info("checking image settings",
			"boot_iso", ironicNode.InstanceInfo["boot_iso"],
			"same", sameImage,
			"provisionState", ironicNode.ProvisionState)
	} else {
		checksum, checksumType, _ := image.GetChecksum()
		if checksumType == "" {
			sameImage = (ironicNode.InstanceInfo["image_source"] == image.URL &&
				ironicNode.InstanceInfo["image_checksum"] == checksum &&
				ironicNode.InstanceInfo["image_os_hash_algo"] == nil &&
				ironicNode.InstanceInfo["image_os_hash_value"] == nil)
		} else {
			sameImage = (ironicNode.InstanceInfo["image_source"] == image.URL &&
				ironicNode.InstanceInfo["image_checksum"] == nil &&
				ironicNode.InstanceInfo["image_os_hash_algo"] == checksumType &&
				ironicNode.InstanceInfo["image_os_hash_value"] == checksum)
		}
		p.log.Info("checking image settings",
			"source", ironicNode.InstanceInfo["image_source"],
			"checksumType", checksumType,
			"checksum", checksum,
			"same", sameImage,
			"provisionState", ironicNode.ProvisionState,
			"iinfo", ironicNode.InstanceInfo)
	}
	return sameImage
}

func (p *ironicProvisioner) buildManualCleaningSteps(bmcAccess bmc.AccessDetails, data provisioner.PrepareData) (cleanSteps []nodes.CleanStep, err error) {
	// Build raid clean steps
	raidCleanSteps, err := BuildRAIDCleanSteps(bmcAccess.RAIDInterface(), data.TargetRAIDConfig, data.ActualRAIDConfig)
	if err != nil {
		return nil, err
	}
	cleanSteps = append(cleanSteps, raidCleanSteps...)

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

	var newSettings []map[string]interface{}
	if data.ActualFirmwareSettings != nil {
		// If we have the current settings from Ironic, update the settings to contain:
		// 1. settings converted by BMC drivers that are different than current settings
		for _, fwConfigSetting := range fwConfigSettings {
			if val, exists := data.ActualFirmwareSettings[fwConfigSetting["name"]]; exists {
				if fwConfigSetting["value"] != val {
					newSettings = buildFirmwareSettings(newSettings, fwConfigSetting["name"], intstr.FromString(fwConfigSetting["value"]))
				}
			} else {
				p.log.Info("name converted from bmc driver not found in firmware settings", "name", fwConfigSetting["name"], "node", p.nodeID)
			}
		}

		// 2. target settings that are different than current settings
		if data.TargetFirmwareSettings != nil {
			for k, v := range data.TargetFirmwareSettings {
				if data.ActualFirmwareSettings[k] != v.String() {
					// Skip changing this setting if it was defined in the vendor specific settings
					for _, fwConfigSetting := range fwConfigSettings {
						if fwConfigSetting["name"] == k {
							continue
						}
					}
					newSettings = buildFirmwareSettings(newSettings, k, v)
				}
			}
		}
	} else {
		// use only the settings converted by bmc driver. Note that these settings are all strings
		for _, fwConfigSetting := range fwConfigSettings {
			newSettings = buildFirmwareSettings(newSettings, fwConfigSetting["name"], intstr.FromString(fwConfigSetting["value"]))
		}
	}

	if len(newSettings) != 0 {
		p.log.Info("Applying BIOS config clean steps", "settings", newSettings)
		cleanSteps = append(
			cleanSteps,
			nodes.CleanStep{
				Interface: nodes.InterfaceBIOS,
				Step:      "apply_configuration",
				Args: map[string]interface{}{
					"settings": newSettings,
				},
			},
		)
	}

	// TODO: Add manual cleaning steps for host configuration

	return
}

func buildFirmwareSettings(settings []map[string]interface{}, name string, value intstr.IntOrString) []map[string]interface{} {
	// if name already exists, don't add it
	for _, setting := range settings {
		if setting["name"] == name {
			return settings
		}
	}

	if value.Type == intstr.Int {
		settings = append(settings, map[string]interface{}{"name": name, "value": value.IntValue()})
	} else {
		settings = append(settings, map[string]interface{}{"name": name, "value": value.String()})
	}

	return settings
}

func (p *ironicProvisioner) startManualCleaning(bmcAccess bmc.AccessDetails, ironicNode *nodes.Node, data provisioner.PrepareData) (success bool, result provisioner.Result, err error) {
	// Set raid configuration
	result, err = setTargetRAIDCfg(p, bmcAccess.RAIDInterface(), ironicNode, data)
	if result.Dirty || result.ErrorMessage != "" || err != nil {
		return
	}

	// Build manual clean steps
	cleanSteps, err := p.buildManualCleaningSteps(bmcAccess, data)
	if err != nil {
		result, err = operationFailed(err.Error())
		return
	}

	// Start manual clean
	if len(cleanSteps) != 0 {
		p.log.Info("remove existing configuration and set new configuration", "clean steps", cleanSteps)
		return p.tryChangeNodeProvisionState(
			ironicNode,
			nodes.ProvisionStateOpts{
				Target:     nodes.TargetClean,
				CleanSteps: cleanSteps,
			},
		)
	}
	result, err = operationComplete()
	return
}

// Prepare remove existing configuration and set new configuration.
// If `started` is true,  it means that we successfully executed `tryChangeNodeProvisionState`.
func (p *ironicProvisioner) Prepare(data provisioner.PrepareData, unprepared bool, restartOnFailure bool) (result provisioner.Result, started bool, err error) {
	bmcAccess, err := p.bmcAccess()
	if err != nil {
		result, err = transientError(err)
		return
	}

	ironicNode, err := p.getNode()
	if err != nil {
		result, err = transientError(err)
		return
	}

	switch nodes.ProvisionState(ironicNode.ProvisionState) {
	case nodes.Available:
		if unprepared {
			var cleanSteps []nodes.CleanStep
			cleanSteps, err = p.buildManualCleaningSteps(bmcAccess, data)
			if err != nil {
				result, err = operationFailed(err.Error())
				return
			}
			if len(cleanSteps) != 0 {
				p.log.Info("the node needs to be reconfigured", "clean steps", cleanSteps)
				result, err = p.changeNodeProvisionState(
					ironicNode,
					nodes.ProvisionStateOpts{Target: nodes.TargetManage},
				)
				return
			}
			// nothing to do
			started = true
		}
		// Automated clean finished

		result, err = operationComplete()

	case nodes.Manageable:
		if unprepared {
			started, result, err = p.startManualCleaning(bmcAccess, ironicNode, data)
			if started || result.Dirty || result.ErrorMessage != "" || err != nil {
				return
			}
			// nothing to do
			started = true
		}
		// Manual clean finished
		result, err = operationComplete()

	case nodes.CleanFail:
		// When clean failed, we need to clean host provisioning settings.
		// If restartOnFailure is false, it means the settings aren't cleared.
		// So we can't set the node's state to manageable, until the settings are cleared.
		if !restartOnFailure {
			result, err = operationFailed(ironicNode.LastError)
			return
		}
		if ironicNode.Maintenance {
			p.log.Info("clearing maintenance flag")
			result, err = p.setMaintenanceFlag(ironicNode, false, "")
			return
		}
		result, err = p.changeNodeProvisionState(
			ironicNode,
			nodes.ProvisionStateOpts{Target: nodes.TargetManage},
		)

	case nodes.Cleaning, nodes.CleanWait:
		p.log.Info("waiting for host to become manageable",
			"state", ironicNode.ProvisionState,
			"deploy step", ironicNode.DeployStep)
		result, err = operationContinuing(provisionRequeueDelay)

	default:
		result, err = transientError(fmt.Errorf("have unexpected ironic node state %s", ironicNode.ProvisionState))
	}
	return
}

func (p *ironicProvisioner) getConfigDrive(data provisioner.ProvisionData) (configDrive nodes.ConfigDrive, err error) {
	// In theory, Ironic can support configdrive with live ISO by attaching
	// it to another virtual media slot. However, some hardware does not
	// support two virtual media devices at the same time, so we shouldn't
	// try it.
	if data.Image.IsLiveISO() {
		p.log.Info("not providing config drive for live ISO")
		return
	}

	// Retrieve instance specific user data (cloud-init, ignition, etc).
	userData, err := data.HostConfig.UserData()
	if err != nil {
		return configDrive, errors.Wrap(err, "could not retrieve user data")
	}
	if userData != "" {
		configDrive.UserData = userData
	}

	// Retrieve OpenStack network_data. Default value is empty.
	networkDataRaw, err := data.HostConfig.NetworkData()
	if err != nil {
		return configDrive, errors.Wrap(err, "could not retrieve network data")
	}
	if networkDataRaw != "" {
		var networkData map[string]interface{}
		if err = yaml.Unmarshal([]byte(networkDataRaw), &networkData); err != nil {
			return configDrive, errors.Wrap(err, "failed to unmarshal network_data.json from secret")
		}
		configDrive.NetworkData = networkData
	}

	// Retrieve meta data with fallback to defaults from provisioner.
	configDrive.MetaData = map[string]interface{}{
		"uuid":             string(p.objectMeta.UID),
		"metal3-namespace": p.objectMeta.Namespace,
		"metal3-name":      p.objectMeta.Name,
		"local-hostname":   p.objectMeta.Name,
		"local_hostname":   p.objectMeta.Name,
		"name":             p.objectMeta.Name,
	}
	metaDataRaw, err := data.HostConfig.MetaData()
	if err != nil {
		return configDrive, errors.Wrap(err, "could not retrieve metadata")
	}
	if metaDataRaw != "" {
		if err = yaml.Unmarshal([]byte(metaDataRaw), &configDrive.MetaData); err != nil {
			return configDrive, errors.Wrap(err, "failed to unmarshal metadata from secret")
		}
	}

	return
}

func (p *ironicProvisioner) getCustomDeploySteps(customDeploy *metal3api.CustomDeploy) (deploySteps []nodes.DeployStep) {
	if customDeploy != nil && customDeploy.Method != "" {
		deploySteps = append(deploySteps, nodes.DeployStep{
			Interface: nodes.InterfaceDeploy,
			Step:      customDeploy.Method,
			Args:      map[string]interface{}{},
			Priority:  customDeployPriority,
		})
	}

	return
}

// Provision writes the image from the host spec to the host. It may
// be called multiple times, and should return true for its dirty flag
// until the provisioning operation is completed.
func (p *ironicProvisioner) Provision(data provisioner.ProvisionData, forceReboot bool) (result provisioner.Result, err error) {
	ironicNode, err := p.getNode()
	if err != nil {
		return transientError(err)
	}

	p.log.Info("provisioning image to host", "state", ironicNode.ProvisionState)

	ironicHasSameImage := p.ironicHasSameImage(ironicNode, data.Image)

	// Ironic has the settings it needs, see if it finds any issues
	// with them.
	switch nodes.ProvisionState(ironicNode.ProvisionState) {
	case nodes.DeployFail:
		// Since we were here ironic has recorded an error for this host,
		// with the image and checksum we have been trying to use, so we
		// should stop. (If the image values do not match, we want to try
		// again.)
		if ironicHasSameImage {
			// Save me from "eventually consistent" systems built on
			// top of relational databases...
			if ironicNode.LastError == "" {
				p.log.Info("failed but error message not available")
				return retryAfterDelay(0)
			}
			p.log.Info("found error", "msg", ironicNode.LastError)
			return operationFailed(fmt.Sprintf("Image provisioning failed: %s",
				ironicNode.LastError))
		}
		p.log.Info("recovering from previous failure")
		if provResult, err := p.setUpForProvisioning(ironicNode, data); err != nil || provResult.Dirty || provResult.ErrorMessage != "" {
			return provResult, err
		}

		configDrive, err := p.getConfigDrive(data)
		if err != nil {
			return transientError(err)
		}

		return p.changeNodeProvisionState(
			ironicNode,
			nodes.ProvisionStateOpts{
				Target:      nodes.TargetActive,
				ConfigDrive: configDrive,
				DeploySteps: p.getCustomDeploySteps(data.CustomDeploy),
			},
		)

	case nodes.Manageable:
		return p.changeNodeProvisionState(ironicNode,
			nodes.ProvisionStateOpts{Target: nodes.TargetProvide})

	case nodes.CleanFail:
		if ironicNode.Maintenance {
			p.log.Info("clearing maintenance flag")
			return p.setMaintenanceFlag(ironicNode, false, "")
		}
		return p.changeNodeProvisionState(
			ironicNode,
			nodes.ProvisionStateOpts{Target: nodes.TargetManage},
		)

	case nodes.Available:
		if provResult, err := p.setUpForProvisioning(ironicNode, data); err != nil || provResult.Dirty || provResult.ErrorMessage != "" {
			return provResult, err
		}

		// After it is available, we need to start provisioning by
		// setting the state to "active".
		p.log.Info("making host active")

		configDrive, err := p.getConfigDrive(data)
		if err != nil {
			return transientError(err)
		}

		return p.changeNodeProvisionState(
			ironicNode,
			nodes.ProvisionStateOpts{
				Target:      nodes.TargetActive,
				ConfigDrive: configDrive,
				DeploySteps: p.getCustomDeploySteps(data.CustomDeploy),
			},
		)

	case nodes.Active:
		// provisioning is done
		p.publisher("ProvisioningComplete",
			fmt.Sprintf("Image provisioning completed for %s", data.Image.URL))
		p.log.Info("finished provisioning")
		return operationComplete()

	case nodes.DeployWait:
		if forceReboot {
			p.log.Info("aborting provisioning to force reboot of preprovisioning image")
			_, result, err = p.tryChangeNodeProvisionState(
				ironicNode,
				nodes.ProvisionStateOpts{Target: nodes.TargetDeleted},
			)
			return
		}

		fallthrough

	default:
		// wait states like cleaning and clean wait
		p.log.Info("waiting for host to become available",
			"state", ironicNode.ProvisionState,
			"deploy step", ironicNode.DeployStep)
		return operationContinuing(provisionRequeueDelay)
	}
}

func (p *ironicProvisioner) setMaintenanceFlag(ironicNode *nodes.Node, value bool, reason string) (result provisioner.Result, err error) {
	p.log.Info("updating maintenance in ironic", "newValue", value, "reason", reason)
	if value {
		err = nodes.SetMaintenance(p.ctx, p.client, ironicNode.UUID, nodes.MaintenanceOpts{Reason: reason}).ExtractErr()
	} else {
		err = nodes.UnsetMaintenance(p.ctx, p.client, ironicNode.UUID).ExtractErr()
	}

	switch err.(type) {
	case nil:
		result, err = operationContinuing(0)
	case gophercloud.ErrDefault409:
		p.log.Info("could not update maintenance in ironic, busy")
		result, err = retryAfterDelay(provisionRequeueDelay)
	default:
		err = fmt.Errorf("failed to set host maintenance flag to %v (%w)", value, err)
		result, err = transientError(err)
	}
	return
}

// Deprovision removes the host from the image. It may be called
// multiple times, and should return true for its dirty flag until the
// deprovisioning operation is completed.
func (p *ironicProvisioner) Deprovision(restartOnFailure bool) (result provisioner.Result, err error) {
	p.log.Info("deprovisioning")

	ironicNode, err := p.getNode()
	if err != nil {
		return transientError(err)
	}

	p.log.Info("deprovisioning host",
		"ID", ironicNode.UUID,
		"lastError", ironicNode.LastError,
		"current", ironicNode.ProvisionState,
		"target", ironicNode.TargetProvisionState,
		"deploy step", ironicNode.DeployStep,
		"instance_info", ironicNode.InstanceInfo,
	)

	switch nodes.ProvisionState(ironicNode.ProvisionState) {
	case nodes.Error:
		if !restartOnFailure {
			p.log.Info("deprovisioning failed")
			if ironicNode.LastError == "" {
				result.ErrorMessage = "Deprovisioning failed"
			} else {
				result.ErrorMessage = ironicNode.LastError
			}
			return result, nil
		}
		p.log.Info("retrying deprovisioning")
		p.publisher("DeprovisioningStarted", "Image deprovisioning restarted")
		return p.changeNodeProvisionState(
			ironicNode,
			nodes.ProvisionStateOpts{Target: nodes.TargetDeleted},
		)

	case nodes.CleanFail:
		if !restartOnFailure {
			p.log.Info("cleaning failed", "lastError", ironicNode.LastError)
			return operationFailed(fmt.Sprintf("Cleaning failed: %s", ironicNode.LastError))
		}
		p.log.Info("retrying cleaning")
		if ironicNode.Maintenance {
			p.log.Info("clearing maintenance flag", "maintenanceReason", ironicNode.MaintenanceReason)
			return p.setMaintenanceFlag(ironicNode, false, "")
		}
		// Move to manageable for retrying.
		return p.changeNodeProvisionState(
			ironicNode,
			nodes.ProvisionStateOpts{Target: nodes.TargetManage},
		)

	case nodes.Manageable:
		// We end up here after CleanFail, retry cleaning. If a user
		// wants to delete a host without cleaning, they can always set
		// automatedCleaningMode: disabled.
		p.log.Info("deprovisioning node is in manageable state", "automatedClean", ironicNode.AutomatedClean)
		return p.changeNodeProvisionState(
			ironicNode,
			nodes.ProvisionStateOpts{Target: nodes.TargetProvide},
		)

	case nodes.Available:
		p.publisher("DeprovisioningComplete", "Image deprovisioning completed")
		return operationComplete()

	case nodes.Deleting:
		p.log.Info("deleting")
		// Transitions to Cleaning upon completion
		return operationContinuing(deprovisionRequeueDelay)

	case nodes.Cleaning:
		p.log.Info("cleaning")
		// Transitions to Available upon completion
		return operationContinuing(deprovisionRequeueDelay)

	case nodes.CleanWait:
		p.log.Info("cleaning")
		return operationContinuing(deprovisionRequeueDelay)

	case nodes.Deploying:
		p.log.Info("previous deploy running")
		// Deploying cannot be stopped, wait for DeployWait or Active
		return operationContinuing(deprovisionRequeueDelay)

	case nodes.Active, nodes.DeployFail, nodes.DeployWait:
		p.log.Info("starting deprovisioning", "automatedClean", ironicNode.AutomatedClean)
		p.publisher("DeprovisioningStarted", "Image deprovisioning started")
		return p.changeNodeProvisionState(
			ironicNode,
			nodes.ProvisionStateOpts{Target: nodes.TargetDeleted},
		)

	default:
		// FIXME(zaneb): this error is unlikely to actually be transient
		return transientError(fmt.Errorf("unhandled ironic state %s", ironicNode.ProvisionState))
	}
}

// Delete removes the host from the provisioning system. It may be
// called multiple times, and should return true for its dirty flag
// until the deprovisioning operation is completed.
func (p *ironicProvisioner) Delete() (result provisioner.Result, err error) {
	ironicNode, err := p.getNode()
	if err != nil {
		if errors.Is(err, provisioner.ErrNeedsRegistration) {
			p.log.Info("no node found, already deleted")
			return operationComplete()
		}
		return transientError(err)
	}

	p.log.Info("deleting host",
		"ID", ironicNode.UUID,
		"lastError", ironicNode.LastError,
		"current", ironicNode.ProvisionState,
		"target", ironicNode.TargetProvisionState,
		"deploy step", ironicNode.DeployStep,
	)

	currentProvState := nodes.ProvisionState(ironicNode.ProvisionState)
	if currentProvState == nodes.Available || currentProvState == nodes.Manageable {
		// Make sure we don't have a stale instance UUID
		if ironicNode.InstanceUUID != "" {
			p.log.Info("removing stale instance UUID before deletion", "instanceUUID", ironicNode.InstanceUUID)
			updater := updateOptsBuilder(p.log)
			updater.SetTopLevelOpt("instance_uuid", nil, ironicNode.InstanceUUID)
			_, success, result, err := p.tryUpdateNode(ironicNode, updater)
			if !success {
				return result, err
			}
		}
	} else if !ironicNode.Maintenance {
		// If we see an active node and the controller doesn't think
		// we need to deprovision it, that means the node was
		// ExternallyProvisioned and we should remove it from Ironic
		// without deprovisioning it.
		//
		// If we see a node with an error, we will have to set the
		// maintenance flag before deleting it.
		//
		// Any other state requires us to use maintenance mode to
		// delete while bypassing Ironic's internal checks related to
		// Nova.
		p.log.Info("setting host maintenance flag to force image delete")
		return p.setMaintenanceFlag(ironicNode, true, "forcing deletion in baremetal-operator")
	}

	p.log.Info("host ready to be removed")
	err = nodes.Delete(p.ctx, p.client, ironicNode.UUID).ExtractErr()
	switch err.(type) {
	case nil:
		p.log.Info("removed")
	case gophercloud.ErrDefault409:
		p.log.Info("could not remove host, busy")
		return retryAfterDelay(provisionRequeueDelay)
	case gophercloud.ErrDefault404:
		p.log.Info("did not find host to delete, OK")
	default:
		return transientError(errors.Wrap(err, "failed to remove host"))
	}

	return operationContinuing(0)
}

// Detach removes the host from the provisioning system.
// Similar to Delete, but ensures non-interruptive behavior
// for the target system.  It may be called multiple times,
// and should return true for its dirty  flag until the
// deletion operation is completed.
func (p *ironicProvisioner) Detach() (result provisioner.Result, err error) {
	// Currently the same behavior as Delete()
	return p.Delete()
}

// softPowerOffUnsupportedError is returned when the BMC does not
// support soft power off.
type softPowerOffUnsupportedError struct {
	cause error
}

func (e softPowerOffUnsupportedError) Unwrap() error {
	return e.cause
}

func (e softPowerOffUnsupportedError) Error() string {
	return "soft power off is unsupported on BMC"
}

func (p *ironicProvisioner) changePower(ironicNode *nodes.Node, target nodes.TargetPowerState) (result provisioner.Result, err error) {
	p.log.Info("changing power state")

	if ironicNode.TargetProvisionState != "" {
		p.log.Info("host in state that does not allow power change, try again after delay",
			"state", ironicNode.ProvisionState,
			"target state", ironicNode.TargetProvisionState,
		)
		return operationContinuing(powerRequeueDelay)
	}

	powerStateOpts := nodes.PowerStateOpts{
		Target: target,
	}
	if target == nodes.SoftPowerOff {
		powerStateOpts.Timeout = int(softPowerOffTimeout.Seconds())
	}

	changeResult := nodes.ChangePowerState(
		p.ctx,
		p.client,
		ironicNode.UUID,
		powerStateOpts)

	switch changeResult.Err.(type) {
	case nil:
		p.log.Info("power change OK")
		event := map[nodes.TargetPowerState]struct{ Event, Reason string }{
			nodes.PowerOn:      {Event: "PowerOn", Reason: "Host powered on"},
			nodes.PowerOff:     {Event: "PowerOff", Reason: "Host powered off"},
			nodes.SoftPowerOff: {Event: "PowerOff", Reason: "Host soft powered off"},
		}[target]
		p.publisher(event.Event, event.Reason)
		return operationContinuing(0)
	case gophercloud.ErrDefault409:
		p.log.Info("host is locked, trying again after delay", "delay", powerRequeueDelay)
		return retryAfterDelay(powerRequeueDelay)
	case gophercloud.ErrDefault400:
		// Error 400 Bad Request means target power state is not supported by vendor driver
		if target == nodes.SoftPowerOff {
			changeResult.Err = softPowerOffUnsupportedError{changeResult.Err}
		}
	}
	p.log.Info("power change error", "message", changeResult.Err)
	return transientError(errors.Wrap(changeResult.Err,
		fmt.Sprintf("failed to %s node", target)))
}

// PowerOn ensures the server is powered on independently of any image
// provisioning operation.
func (p *ironicProvisioner) PowerOn(force bool) (result provisioner.Result, err error) {
	p.log.Info("ensuring host is powered on")

	ironicNode, err := p.getNode()
	if err != nil {
		return transientError(err)
	}

	p.log.Info("checking current state",
		"target", ironicNode.TargetPowerState)

	if ironicNode.PowerState != powerOn {
		if ironicNode.TargetPowerState == powerOn {
			p.log.Info("waiting for power status to change")
			return operationContinuing(powerRequeueDelay)
		}
		if ironicNode.LastError != "" && !force {
			p.log.Info("PowerOn operation failed", "msg", ironicNode.LastError)
			return operationFailed(fmt.Sprintf("PowerOn operation failed: %s",
				ironicNode.LastError))
		}
		return p.changePower(ironicNode, nodes.PowerOn)
	}
	return result, nil
}

// PowerOff ensures the server is powered off independently of any image
// provisioning operation.
func (p *ironicProvisioner) PowerOff(rebootMode metal3api.RebootMode, force bool) (result provisioner.Result, err error) {
	p.log.Info(fmt.Sprintf("ensuring host is powered off (mode: %s)", rebootMode))

	ironicNode, err := p.getNode()
	if err != nil {
		return transientError(err)
	}

	if ironicNode.PowerState != powerOff {
		targetState := ironicNode.TargetPowerState
		// If the target state is either powerOff or softPowerOff, then we should wait
		if targetState == powerOff || targetState == softPowerOff {
			p.log.Info("waiting for power status to change")
			return operationContinuing(powerRequeueDelay)
		}
		// If the target state is unset while the last error is set,
		// then the last execution of power off has failed.
		if targetState == "" && ironicNode.LastError != "" && !force {
			p.log.Info("power off error", "msg", ironicNode.LastError)
			return operationFailed(ironicNode.LastError)
		}

		if rebootMode == metal3api.RebootModeSoft && !force {
			result, err = p.changePower(ironicNode, nodes.SoftPowerOff)
			if !errors.As(err, &softPowerOffUnsupportedError{}) {
				return result, err
			}
		}
		// Reboot mode is hard, force flag is set, or soft power off is not supported
		return p.changePower(ironicNode, nodes.PowerOff)
	}

	return operationComplete()
}

func ironicNodeName(objMeta metav1.ObjectMeta) string {
	return objMeta.Namespace + nameSeparator + objMeta.Name
}

func (p *ironicProvisioner) HasCapacity() (result bool, err error) {
	bmcAccess, err := p.bmcAccess()
	if err != nil {
		return false, err // shouldn't actually happen so late in the process
	}

	// If the current host uses virtual media, do not limit it. Virtual
	// media deployments may work without DHCP and can share the same image.
	if bmcAccess.SupportsISOPreprovisioningImage() {
		return true, nil
	}

	hosts, err := p.loadBusyHosts()
	if err != nil {
		p.log.Error(err, "Unable to get hosts for determining current provisioner capacity")
		return false, err
	}

	// If the current host is already under processing then let's skip the test
	if _, ok := hosts[ironicNodeName(p.objectMeta)]; ok {
		return true, nil
	}

	return len(hosts) < p.config.maxBusyHosts, nil
}

func (p *ironicProvisioner) loadBusyHosts() (hosts map[string]struct{}, err error) {
	hosts = make(map[string]struct{})
	pager := nodes.List(p.client, nodes.ListOpts{
		Fields: []string{"uuid,name,provision_state,boot_interface"},
	})

	page, err := pager.AllPages(p.ctx)
	if err != nil {
		return nil, err
	}

	allNodes, err := nodes.ExtractNodes(page)
	if err != nil {
		return nil, err
	}

	for _, node := range allNodes {
		switch nodes.ProvisionState(node.ProvisionState) {
		case nodes.Cleaning, nodes.CleanWait,
			nodes.Inspecting, nodes.InspectWait,
			nodes.Deploying, nodes.DeployWait,
			nodes.Deleting:
			// FIXME(dtantsur): this is a bit silly, but we don't have an easy way
			// to reconstruct AccessDetails from a DriverInfo.
			if !strings.Contains(node.BootInterface, "virtual-media") {
				hosts[node.Name] = struct{}{}
			}
		}
	}

	return hosts, nil
}

func (p *ironicProvisioner) AddBMCEventSubscriptionForNode(subscription *metal3api.BMCEventSubscription, httpHeaders provisioner.HTTPHeaders) (result provisioner.Result, err error) {
	newSubscription, err := nodes.CreateSubscription(
		p.ctx,
		p.client,
		p.nodeID,
		nodes.CallVendorPassthruOpts{
			Method: "create_subscription",
		},
		nodes.CreateSubscriptionOpts{
			Destination: subscription.Spec.Destination,
			Context:     subscription.Spec.Context,
			HttpHeaders: httpHeaders,
		}).Extract()
	if err != nil {
		return provisioner.Result{}, err
	}

	subscription.Status.SubscriptionID = newSubscription.Id
	return operationComplete()
}

func (p *ironicProvisioner) RemoveBMCEventSubscriptionForNode(subscription metal3api.BMCEventSubscription) (result provisioner.Result, err error) {
	method := nodes.CallVendorPassthruOpts{
		Method: "delete_subscription",
	}
	opts := nodes.DeleteSubscriptionOpts{
		Id: subscription.Status.SubscriptionID,
	}
	err = nodes.DeleteSubscription(p.ctx, p.client, p.nodeID, method, opts).ExtractErr()

	if err != nil {
		return provisioner.Result{RequeueAfter: subscriptionRequeueDelay}, err
	}
	return operationComplete()
}
