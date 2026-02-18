package ironic

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack/baremetal/v1/nodes"
	"github.com/gophercloud/gophercloud/v2/openstack/baremetal/v1/ports"
	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	"github.com/metal3-io/baremetal-operator/pkg/hardwareutils/bmc"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner/ironic/clients"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner/ironic/devicehints"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/yaml"
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
	havePreprovImgBuilder                 bool
	deployKernelURL                       string
	deployRamdiskURL                      string
	deployISOURL                          string
	liveISOForcePersistentBootDevice      string
	directDeployForcePersistentBootDevice string
	maxBusyHosts                          int
	externalURL                           string
	provNetDisabled                       bool
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

// FIXME(hroyrh) : move this to gophercloud when implementing
// VirtualMediaGetResult Extract function.
type VirtualMedia struct {
	Image    string `json:"image"`
	Inserted bool   `json:"inserted"`
	//nolint:tagliatelle
	MediaTypes []string `json:"media_types"`
}

func (p *ironicProvisioner) bmcAccess() (bmc.AccessDetails, error) {
	bmcAccess, err := bmc.NewAccessDetails(p.bmcAddress, p.disableCertVerification)
	if err != nil {
		return nil, fmt.Errorf("failed to parse BMC address information: %w", err)
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
		errorMessage = "host validation error: " + strings.Join(validationErrors, "; ")
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
	if err == nil {
		p.debugLog.Info("found existing node by ID")
		return ironicNode, nil
	}

	if gophercloud.ResponseCodeIs(err, http.StatusNotFound) {
		// Look by ID failed, trying to lookup by hostname in case it was
		// previously created
		return nil, provisioner.ErrNeedsRegistration
	}

	return nil, fmt.Errorf("failed to find node by ID %s: %w", p.nodeID, err)
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
		return false, fmt.Errorf("failed to page over list of ports: %w", err)
	}

	empty, err := allPages.IsEmpty()
	if err != nil {
		return false, fmt.Errorf("failed to check port list status: %w", err)
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
		return false, fmt.Errorf("failed to list ports for %s: %w", address, err)
	}

	if len(allPorts) == 0 {
		p.debugLog.Info("address does not have allocated ports", "address", address)
		return false, nil
	}

	p.debugLog.Info("address is allocated to port", "address", address, "node", allPorts[0].NodeUUID)
	return true, nil
}

// Look for an existing registration for the host in Ironic.
// Returns nil, nil if no host exists with MAC.
func (p *ironicProvisioner) findExistingHost(bootMACAddress string) (ironicNode *nodes.Node, err error) {
	// Try to load the node by UUID
	ironicNode, err = p.getNode()
	if !errors.Is(err, provisioner.ErrNeedsRegistration) {
		return ironicNode, err
	}

	// Try to load the node by name
	nodeSearchList := []string{ironicNodeName(p.objectMeta)}
	if !strings.Contains(p.objectMeta.Name, nameSeparator) {
		nodeSearchList = append(nodeSearchList, p.objectMeta.Name)
	}

	for _, nodeName := range nodeSearchList {
		p.debugLog.Info("looking for existing node by name", "name", nodeName)
		ironicNode, err = nodes.Get(p.ctx, p.client, nodeName).Extract()
		if err == nil {
			p.debugLog.Info("found existing node by name", "name", nodeName, "node", ironicNode.UUID)
			return ironicNode, nil
		}

		if gophercloud.ResponseCodeIs(err, http.StatusNotFound) {
			p.log.Info(fmt.Sprintf("node with name %s doesn't exist", nodeName))
		} else {
			return nil, fmt.Errorf("failed to find node by name %s: %w", nodeName, err)
		}
	}

	// Try to load the node by port address
	// Skip MAC-based lookup if bootMACAddress is empty to avoid false conflicts
	if bootMACAddress != "" {
		p.log.Info("looking for existing node by MAC", "MAC", bootMACAddress)
		allPorts, err := p.listAllPorts(bootMACAddress)

		if err != nil {
			p.log.Info("failed to find an existing port with address", "MAC", bootMACAddress)
			return nil, nil //nolint:nilerr,nilnil
		}

		if len(allPorts) > 0 {
			nodeUUID := allPorts[0].NodeUUID
			ironicNode, err = nodes.Get(p.ctx, p.client, nodeUUID).Extract()
			if err == nil {
				p.debugLog.Info("found existing node by MAC", "MAC", bootMACAddress, "node", ironicNode.UUID, "name", ironicNode.Name)

				// If the node has a name, this means we didn't find it above.
				if ironicNode.Name != "" {
					return nil, NewMacAddressConflictError(bootMACAddress, ironicNode.Name)
				}

				return ironicNode, nil
			}
			if gophercloud.ResponseCodeIs(err, http.StatusNotFound) {
				return nil, fmt.Errorf("port %s exists but linked node %s doesn't: %w", bootMACAddress, nodeUUID, err)
			}
			return nil, fmt.Errorf("port %s exists but failed to find linked node %s by ID: %w", bootMACAddress, nodeUUID, err)
		}

		p.log.Info("port with address doesn't exist", "MAC", bootMACAddress)
	}
	// Either the node was never created or the Ironic database has
	// been dropped.
	return nil, nil //nolint:nilnil
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
		return fmt.Errorf("failed to create ironic port for node %s, MAC: %s: %w", uuid, macAddress, err)
	}

	return nil
}

// configureNode configures Node properties that are not related to any specific provisioning phase.
// It populates the AutomatedClean field, as well as capabilities and architecture in Properties.
// It also calls setDeployImage to populate IPA parameters in DriverInfo and
// checks if the required PreprovisioningImage is provided and ready.
func (p *ironicProvisioner) configureNode(data provisioner.ManagementAccessData, ironicNode *nodes.Node, bmcAccess bmc.AccessDetails) (result provisioner.Result, err error) {
	updater := clients.UpdateOptsBuilder(p.log)

	deployImageInfo := setDeployImage(p.config, bmcAccess, data.PreprovisioningImage)
	updater.SetDriverInfoOpts(deployImageInfo, ironicNode)

	updater.SetTopLevelOpt("automated_clean",
		data.AutomatedCleaningMode != metal3api.CleaningModeDisabled,
		ironicNode.AutomatedClean)

	opts := clients.UpdateOptsData{
		"capabilities": buildCapabilitiesValue(ironicNode, data.BootMode),
	}
	if data.CPUArchitecture != "" {
		opts["cpu_arch"] = data.CPUArchitecture
	}
	updater.SetPropertiesOpts(opts, ironicNode)

	_, success, result, err := p.tryUpdateNode(ironicNode, updater)
	if !success {
		return result, err
	}

	result, err = operationComplete()
	if err != nil {
		return result, err
	}

	if data.State == metal3api.StateProvisioning && data.CurrentImage.IsLiveISO() {
		// Live ISO doesn't need pre-provisioning image
		return result, nil
	}

	if data.State == metal3api.StateDeprovisioning && data.AutomatedCleaningMode == metal3api.CleaningModeDisabled {
		// No need for pre-provisioning image if cleaning disabled
		return result, nil
	}

	switch data.State {
	case metal3api.StateDeprovisioning,
		metal3api.StateInspecting,
		metal3api.StatePreparing:
		if deployImageInfo == nil && p.config.havePreprovImgBuilder {
			result, err = transientError(provisioner.ErrNeedsPreprovisioningImage)
		}
		return result, err
	case metal3api.StateProvisioned,
		metal3api.StateExternallyProvisioned:
		if data.OperationalStatus == metal3api.OperationalStatusServicing &&
			deployImageInfo == nil && p.config.havePreprovImgBuilder {
			result, err = transientError(provisioner.ErrNeedsPreprovisioningImage)
		}
		return result, err
	default:
	}

	return result, nil
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

func setExternalURL(p *ironicProvisioner, driverInfo map[string]any) map[string]any {
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
		ips, err := net.DefaultResolver.LookupIPAddr(context.Background(), parsedURL.Hostname())
		if err != nil {
			p.log.Info("Failed to look up the IP address for BMC hostname", "hostname", p.bmcAddress)
			return driverInfo
		}

		if len(ips) == 0 {
			p.log.Info("Zero IP addresses for BMC hostname", "hostname", p.bmcAddress)
			return driverInfo
		}

		ip = ips[0].IP
	}

	// In the case of IPv4, we don't have to do anything.
	if ip.To4() != nil {
		return driverInfo
	}

	driverInfo["external_http_url"] = p.config.externalURL

	return driverInfo
}

// setDeployImage configures the IPA ramdisk parameters in the Node's DriverInfo.
// It can use either the provided PreprovisioningImage or the global configuration from ironicConfig.
func setDeployImage(config ironicConfig, accessDetails bmc.AccessDetails, hostImage *provisioner.PreprovisioningImage) clients.UpdateOptsData {
	deployImageInfo := clients.UpdateOptsData{
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
				deployImageInfo[kernelParamsKey] = "%default% " + hostImage.ExtraKernelParams
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

func (p *ironicProvisioner) tryUpdateNode(ironicNode *nodes.Node, updater *clients.NodeUpdater) (updatedNode *nodes.Node, success bool, result provisioner.Result, err error) {
	if len(updater.Updates) == 0 {
		updatedNode = ironicNode
		success = true
		return
	}

	p.log.Info("updating node settings in ironic", "updateCount", len(updater.Updates))
	updatedNode, err = nodes.Update(p.ctx, p.client, ironicNode.UUID, updater.Updates).Extract()
	if err == nil {
		success = true
	} else if gophercloud.ResponseCodeIs(err, http.StatusConflict) {
		p.log.Info("could not update node settings in ironic, busy or update cannot be applied in the current state")
		result, err = retryAfterDelay(provisionRequeueDelay)
	} else {
		result, err = transientError(fmt.Errorf("failed to update host settings in ironic: %w", err))
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
		return success, result, err
	}
	if ironicNode.Maintenance {
		p.log.Info("trying to change a provision state for a node in maintenance, removing maintenance first", "reason", ironicNode.MaintenanceReason)
		result, err = p.setMaintenanceFlag(ironicNode, false, "")
		return success, result, err
	}

	changeResult := nodes.ChangeProvisionState(p.ctx, p.client, ironicNode.UUID, opts)
	if changeResult.Err == nil {
		success = true
	} else if gophercloud.ResponseCodeIs(changeResult.Err, http.StatusConflict) {
		p.log.Info("could not change state of host, busy")
		result, err = retryAfterDelay(provisionRequeueDelay)
		return success, result, err
	} else {
		result, err = transientError(fmt.Errorf("failed to change provisioning state to %q: %w", opts.Target, changeResult.Err))
		return success, result, err
	}

	result, err = operationContinuing(provisionRequeueDelay)
	return success, result, err
}

func (p *ironicProvisioner) changeNodeProvisionState(ironicNode *nodes.Node, opts nodes.ProvisionStateOpts) (result provisioner.Result, err error) {
	_, result, err = p.tryChangeNodeProvisionState(ironicNode, opts)
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

func (p *ironicProvisioner) setLiveIsoUpdateOptsForNode(ironicNode *nodes.Node, imageData *metal3api.Image, updater *clients.NodeUpdater) {
	optValues := clients.UpdateOptsData{
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

	driverOptValues := clients.UpdateOptsData{"force_persistent_boot_device": "Default"}
	if p.config.liveISOForcePersistentBootDevice != "" {
		driverOptValues = clients.UpdateOptsData{
			"force_persistent_boot_device": p.config.liveISOForcePersistentBootDevice,
		}
	}
	updater.SetDriverInfoOpts(driverOptValues, ironicNode)
}

func (p *ironicProvisioner) setDirectDeployUpdateOptsForNode(ironicNode *nodes.Node, imageData *metal3api.Image, updater *clients.NodeUpdater) {
	checksum, checksumType, err := imageData.GetChecksum()
	if err != nil {
		p.log.Info("image/checksum not found for host", "message", err)
		return
	}

	optValues := clients.UpdateOptsData{
		// Remove any boot_iso field
		"boot_iso":          nil,
		"image_source":      imageData.URL,
		"image_disk_format": imageData.DiskFormat,
	}

	// For OCI images without checksum, don't set checksum fields
	if checksum == "" && checksumType == "" {
		optValues["image_checksum"] = nil
		optValues["image_os_hash_algo"] = nil
		optValues["image_os_hash_value"] = nil
	} else if checksumType == "" {
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

	driverOptValues := clients.UpdateOptsData{
		"force_persistent_boot_device": "Default",
	}
	if p.config.directDeployForcePersistentBootDevice != "" {
		driverOptValues = clients.UpdateOptsData{
			"force_persistent_boot_device": p.config.directDeployForcePersistentBootDevice,
		}
	}
	updater.SetDriverInfoOpts(driverOptValues, ironicNode)
}

func (p *ironicProvisioner) setCustomDeployUpdateOptsForNode(ironicNode *nodes.Node, imageData *metal3api.Image, updater *clients.NodeUpdater) {
	var optValues clients.UpdateOptsData
	if imageData != nil && imageData.URL != "" {
		checksum, checksumType, err := imageData.GetChecksum()
		// NOTE(dtantsur): all fields are optional for custom deploy
		if err == nil {
			optValues = clients.UpdateOptsData{
				"boot_iso":            nil,
				"image_checksum":      nil,
				"image_source":        imageData.URL,
				"image_os_hash_algo":  checksumType,
				"image_os_hash_value": checksum,
				"image_disk_format":   imageData.DiskFormat,
			}
		} else {
			optValues = clients.UpdateOptsData{
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
		optValues = clients.UpdateOptsData{
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

// getInstanceUpdateOpts constructs InstanceInfo options required to provision a Node in Ironic.
func (p *ironicProvisioner) getInstanceUpdateOpts(ironicNode *nodes.Node, data provisioner.ProvisionData) *clients.NodeUpdater {
	updater := clients.UpdateOptsBuilder(p.log)

	hasCustomDeploy := data.CustomDeploy != nil && data.CustomDeploy.Method != ""

	// instance_uuid
	updater.SetTopLevelOpt("instance_uuid", string(p.objectMeta.UID), ironicNode.InstanceUUID)

	updater.SetInstanceInfoOpts(clients.UpdateOptsData{
		"capabilities": buildInstanceInfoCapabilities(data.BootMode),
		"root_device":  devicehints.MakeHintMap(data.RootDeviceHints),
	}, ironicNode)

	if hasCustomDeploy {
		// Custom deploy process
		p.setCustomDeployUpdateOptsForNode(ironicNode, &data.Image, updater)
	} else if data.Image.IsLiveISO() {
		// Set live-iso format options
		p.setLiveIsoUpdateOptsForNode(ironicNode, &data.Image, updater)
	} else {
		// Set deploy_interface direct options when not booting a live-iso
		p.setDirectDeployUpdateOptsForNode(ironicNode, &data.Image, updater)
	}

	return updater
}

// GetFirmwareSettings gets the BIOS settings and optional schema from the host and returns maps.
func (p *ironicProvisioner) GetFirmwareSettings(includeSchema bool) (settings metal3api.SettingsMap, schema map[string]metal3api.SettingSchema, err error) {
	ironicNode, err := p.getNode()
	if err != nil {
		return nil, nil, fmt.Errorf("could not get node for BIOS settings: %w", err)
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
		return nil, nil, fmt.Errorf("could not get BIOS settings for node %s: %w", ironicNode.UUID, biosListErr)
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

	// We support bmc, bios, and multiple NICs components. Starting with 3 slots.
	componentsInfo := make([]metal3api.FirmwareComponentStatus, 0, 3) //nolint:mnd

	if ironicNode.FirmwareInterface == "no-firmware" {
		return componentsInfo, provisioner.ErrFirmwareUpdateUnsupported
	}
	// Get the components from Ironic via Gophercloud
	componentList, componentListErr := nodes.ListFirmware(p.ctx, p.client, ironicNode.UUID).Extract()

	if componentListErr != nil {
		return nil, fmt.Errorf("could not get firmware components for node %s: %w", ironicNode.UUID, componentListErr)
	}

	// Iterate over the list of components to extract their information and update the list.
	for _, fwc := range componentList {
		if fwc.Component != "bios" && fwc.Component != "bmc" && !strings.HasPrefix(fwc.Component, metal3api.NICComponentPrefix) {
			p.log.Info("ignoring firmware component for node", "component", fwc.Component, "node", ironicNode.UUID)
			continue
		}
		component := metal3api.FirmwareComponentStatus{
			Component:          fwc.Component,
			InitialVersion:     fwc.InitialVersion,
			CurrentVersion:     fwc.CurrentVersion,
			LastVersionFlashed: fwc.LastVersionFlashed,
		}
		// Check if UpdatedAt is nil before adding it.
		if fwc.UpdatedAt != nil {
			component.UpdatedAt = metav1.Time{
				Time: *fwc.UpdatedAt,
			}
		}
		componentsInfo = append(componentsInfo, component)
		p.log.V(1).Info("firmware component found for node", "component", fwc.Component, "node", ironicNode.UUID)
	}

	return componentsInfo, componentListErr
}

func (p *ironicProvisioner) setUpForProvisioning(ironicNode *nodes.Node, data provisioner.ProvisionData) (result provisioner.Result, err error) {
	p.log.Info("starting provisioning", "node properties", ironicNode.Properties)

	ironicNode, success, result, err := p.tryUpdateNode(ironicNode,
		p.getInstanceUpdateOpts(ironicNode, data))
	if !success {
		return result, err
	}

	errorMessage, err := p.validateNode(ironicNode)
	if gophercloud.ResponseCodeIs(err, http.StatusConflict) {
		p.log.Info("could not validate host during registration, busy")
		return retryAfterDelay(provisionRequeueDelay)
	} else if err != nil {
		return transientError(fmt.Errorf("failed to validate host during registration: %w", err))
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
		"Image provisioning started for "+data.Image.URL)
	return result, nil
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
		if data.State == metal3api.StateDeprovisioning {
			// During deprovisioning we may get here because of either a fresh
			// registration or a failure during cleaning. Either way, the
			// Deprovision() function will change move the state to Available,
			// which will retry automated cleaning.
			p.log.Info("deprovisioning in progress; not adopting",
				"state", ironicNode.ProvisionState)
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
		return operationFailed("Host adoption failed: " +
			ironicNode.LastError)
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
		// For OCI images without checksum, only compare the URL
		if image.IsOCI() && checksum == "" {
			sameImage = (ironicNode.InstanceInfo["image_source"] == image.URL)
		} else if checksumType == "" {
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

func (p *ironicProvisioner) getNewFirmwareSettings(actualFirmwareSettings metal3api.SettingsMap, targetFirmwareSettings metal3api.DesiredSettingsMap, fwConfigSettings []map[string]string) (newSettings []map[string]any) {
	if actualFirmwareSettings != nil {
		// If we have the current settings from Ironic, update the settings to contain:
		// 1. settings converted by BMC drivers that are different than current settings
		for _, fwConfigSetting := range fwConfigSettings {
			if val, exists := actualFirmwareSettings[fwConfigSetting["name"]]; exists {
				if fwConfigSetting["value"] != val {
					newSettings = buildFirmwareSettings(newSettings, fwConfigSetting["name"], intstr.FromString(fwConfigSetting["value"]))
				}
			} else {
				p.log.Info("name converted from bmc driver not found in firmware settings", "name", fwConfigSetting["name"], "node", p.nodeID)
			}
		}

		// 2. target settings that are different than current settings
		for k, v := range targetFirmwareSettings {
			if actualFirmwareSettings[k] != v.String() {
				// Skip changing this setting if it was defined in the vendor specific settings
				for _, fwConfigSetting := range fwConfigSettings {
					if fwConfigSetting["name"] == k {
						continue
					}
				}
				newSettings = buildFirmwareSettings(newSettings, k, v)
			}
		}
	} else {
		// use only the settings converted by bmc driver. Note that these settings are all strings
		for _, fwConfigSetting := range fwConfigSettings {
			newSettings = buildFirmwareSettings(newSettings, fwConfigSetting["name"], intstr.FromString(fwConfigSetting["value"]))
		}
	}

	return newSettings
}

// getFirmwareComponentsUpdates extract the updates in a format that ironic accepts  [{"component":"...", "url":"..."}, {"component":"...","url":".."}].
func (p *ironicProvisioner) getFirmwareComponentsUpdates(targetFirmwareComponents []metal3api.FirmwareUpdate) (newUpdates []map[string]string) {
	for _, update := range targetFirmwareComponents {
		newComponentUpdate := map[string]string{
			"component": update.Component,
			"url":       update.URL,
		}
		newUpdates = append(newUpdates, newComponentUpdate)
	}
	return newUpdates
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

	newSettings := p.getNewFirmwareSettings(data.ActualFirmwareSettings, data.TargetFirmwareSettings, fwConfigSettings)
	if len(newSettings) != 0 {
		p.log.Info("Applying BIOS config clean steps", "settings", newSettings)
		cleanSteps = append(
			cleanSteps,
			nodes.CleanStep{
				Interface: nodes.InterfaceBIOS,
				Step:      "apply_configuration",
				Args: map[string]any{
					"settings": newSettings,
				},
			},
		)
	}

	newUpdates := p.getFirmwareComponentsUpdates(data.TargetFirmwareComponents)

	if len(newUpdates) != 0 {
		p.log.Info("Applying Firmware Update clean steps", "settings", newUpdates)
		cleanSteps = append(
			cleanSteps,
			nodes.CleanStep{
				Interface: nodes.InterfaceFirmware,
				Step:      "update",
				Args: map[string]any{
					"settings": newUpdates,
				},
			},
		)
	}

	// TODO: Add manual cleaning steps for host configuration

	return cleanSteps, nil
}

func buildFirmwareSettings(settings []map[string]any, name string, value intstr.IntOrString) []map[string]any {
	// if name already exists, don't add it
	for _, setting := range settings {
		if setting["name"] == name {
			return settings
		}
	}

	if value.Type == intstr.Int {
		settings = append(settings, map[string]any{"name": name, "value": value.IntValue()})
	} else {
		settings = append(settings, map[string]any{"name": name, "value": value.String()})
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
		return result, started, err
	}

	ironicNode, err := p.getNode()
	if err != nil {
		result, err = transientError(err)
		return result, started, err
	}

	switch nodes.ProvisionState(ironicNode.ProvisionState) {
	case nodes.Available:
		if unprepared {
			var cleanSteps []nodes.CleanStep
			cleanSteps, err = p.buildManualCleaningSteps(bmcAccess, data)
			if err != nil {
				result, err = operationFailed(err.Error())
				return result, started, err
			}
			if len(cleanSteps) != 0 {
				p.log.Info("the node needs to be reconfigured", "clean steps", cleanSteps)
				result, err = p.changeNodeProvisionState(
					ironicNode,
					nodes.ProvisionStateOpts{Target: nodes.TargetManage},
				)
				return result, started, err
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
				return result, started, err
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
			return result, started, err
		}
		if ironicNode.Maintenance {
			p.log.Info("clearing maintenance flag")
			result, err = p.setMaintenanceFlag(ironicNode, false, "")
			return result, started, err
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
	return result, started, err
}

func (p *ironicProvisioner) getConfigDrive(data provisioner.ProvisionData) (configDrive nodes.ConfigDrive, err error) {
	// In theory, Ironic can support configdrive with live ISO by attaching
	// it to another virtual media slot. However, some hardware does not
	// support two virtual media devices at the same time, so we shouldn't
	// try it.
	if data.Image.IsLiveISO() {
		p.log.Info("not providing config drive for live ISO")
		return configDrive, nil
	}

	// Retrieve instance specific user data (cloud-init, ignition, etc).
	userData, err := data.HostConfig.UserData()
	if err != nil {
		return configDrive, fmt.Errorf("could not retrieve user data: %w", err)
	}
	if userData != "" {
		configDrive.UserData = userData
	}

	// Retrieve OpenStack network_data. Default value is empty.
	networkDataRaw, err := data.HostConfig.NetworkData()
	if err != nil {
		return configDrive, fmt.Errorf("could not retrieve network data: %w", err)
	}
	if networkDataRaw != "" {
		var networkData map[string]any
		if err = yaml.Unmarshal([]byte(networkDataRaw), &networkData); err != nil {
			return configDrive, fmt.Errorf("failed to unmarshal network_data.json from secret: %w", err)
		}
		configDrive.NetworkData = networkData
	}

	// Retrieve meta data with fallback to defaults from provisioner.
	configDrive.MetaData = map[string]any{
		"uuid":             string(p.objectMeta.UID),
		"metal3-namespace": p.objectMeta.Namespace,
		"metal3-name":      p.objectMeta.Name,
		"local-hostname":   p.objectMeta.Name,
		"local_hostname":   p.objectMeta.Name,
		"name":             p.objectMeta.Name,
	}
	metaDataRaw, err := data.HostConfig.MetaData()
	if err != nil {
		return configDrive, fmt.Errorf("could not retrieve metadata: %w", err)
	}
	if metaDataRaw != "" {
		if err = yaml.Unmarshal([]byte(metaDataRaw), &configDrive.MetaData); err != nil {
			return configDrive, fmt.Errorf("failed to unmarshal metadata from secret: %w", err)
		}
	}

	return configDrive, nil
}

func (p *ironicProvisioner) getCustomDeploySteps(customDeploy *metal3api.CustomDeploy) (deploySteps []nodes.DeployStep) {
	if customDeploy != nil && customDeploy.Method != "" {
		deploySteps = append(deploySteps, nodes.DeployStep{
			Interface: nodes.InterfaceDeploy,
			Step:      customDeploy.Method,
			Args:      map[string]any{},
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

	if err = validateProvisionData(data); err != nil {
		errorMessage := "Validation failed: " + err.Error()
		return operationFailed(errorMessage)
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
		var provResult provisioner.Result
		var configDrive nodes.ConfigDrive
		if ironicHasSameImage {
			// Save me from "eventually consistent" systems built on
			// top of relational databases...
			if ironicNode.LastError == "" {
				p.log.Info("failed but error message not available")
				return retryAfterDelay(0)
			}
			p.log.Info("found error", "msg", ironicNode.LastError)
			return operationFailed("Image provisioning failed: " + ironicNode.LastError)
		}
		p.log.Info("recovering from previous failure")
		if provResult, err = p.setUpForProvisioning(ironicNode, data); err != nil || provResult.Dirty || provResult.ErrorMessage != "" {
			return provResult, err
		}

		configDrive, err = p.getConfigDrive(data)
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
		var provResult provisioner.Result
		var configDrive nodes.ConfigDrive

		if provResult, err = p.setUpForProvisioning(ironicNode, data); err != nil || provResult.Dirty || provResult.ErrorMessage != "" {
			return provResult, err
		}

		// After it is available, we need to start provisioning by
		// setting the state to "active".

		configDrive, err = p.getConfigDrive(data)
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
			"Image provisioning completed for "+data.Image.URL)
		p.log.Info("finished provisioning")
		return operationComplete()

	case nodes.DeployWait:
		if forceReboot {
			p.log.Info("aborting provisioning to force reboot of preprovisioning image")
			_, result, err = p.tryChangeNodeProvisionState(
				ironicNode,
				nodes.ProvisionStateOpts{Target: nodes.TargetDeleted},
			)
			return result, err
		}

		fallthrough

	default:
		// wait states like cleaning and clean wait
		p.log.Info("waiting for host to become active or available",
			"state", ironicNode.ProvisionState,
			"deploy step", ironicNode.DeployStep, "clean step", ironicNode.CleanStep)
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

	if err == nil {
		result, err = operationContinuing(0)
	} else if gophercloud.ResponseCodeIs(err, http.StatusConflict) {
		p.log.Info("could not update maintenance in ironic, busy")
		result, err = retryAfterDelay(provisionRequeueDelay)
	} else {
		err = fmt.Errorf("failed to set host maintenance flag to %v (%w)", value, err)
		result, err = transientError(err)
	}
	return
}

// syncAutomatedClean updates the Ironic node's automated_clean field if it doesn't match the desired state.
// Returns true if an update was needed and applied.
func (p *ironicProvisioner) syncAutomatedClean(ironicNode *nodes.Node, automatedCleaningMode metal3api.AutomatedCleaningMode) (updated bool, err error) {
	desiredAutomatedClean := automatedCleaningMode != metal3api.CleaningModeDisabled

	// Check if update is needed
	if ironicNode.AutomatedClean != nil && *ironicNode.AutomatedClean == desiredAutomatedClean {
		return false, nil
	}

	p.log.Info("synchronizing automatedClean before deprovisioning",
		"current", ironicNode.AutomatedClean,
		"desired", automatedCleaningMode)

	updater := clients.UpdateOptsBuilder(p.log)
	updater.SetTopLevelOpt("automated_clean", desiredAutomatedClean, ironicNode.AutomatedClean)

	_, err = nodes.Update(p.ctx, p.client, ironicNode.UUID, updater.Updates).Extract()
	if err != nil {
		return false, fmt.Errorf("failed to update automatedClean: %w", err)
	}

	return true, nil
}

// Deprovision removes the host from the image. It may be called
// multiple times, and should return true for its dirty flag until the
// deprovisioning operation is completed.
func (p *ironicProvisioner) Deprovision(restartOnFailure bool, automatedCleaningMode metal3api.AutomatedCleaningMode) (result provisioner.Result, err error) {
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
			return operationFailed("Cleaning failed: " + ironicNode.LastError)
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
		// We also end up here if the node has to be re-registered, because we
		// don't attempt to adopt once deprovisioning has started.
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
		// Before starting deprovisioning, ensure Ironic's automated_clean matches the BMH spec.
		// This prevents the PPI deletion race where the spec is changed right before deletion
		// but Ironic hasn't been updated yet.
		updated, err := p.syncAutomatedClean(ironicNode, automatedCleaningMode)
		if err != nil {
			return transientError(err)
		}
		if updated {
			return operationContinuing(0)
		}

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

	// Handle verifying state specially: Ironic holds an exclusive lock during
	// verification, so we can't set maintenance mode or delete until it completes.
	// Just wait for the verification to finish (success or timeout).
	if currentProvState == nodes.Verifying {
		p.log.Info("node is verifying, waiting for verification to complete before deletion")
		return operationContinuing(provisionRequeueDelay)
	}

	// For enroll state, the node can be deleted directly without maintenance mode
	// since it has no Nova associations and isn't locked.
	if currentProvState == nodes.Enroll {
		p.log.Info("node is in enroll state, proceeding to delete directly")
		// Fall through to deletion
	} else if currentProvState == nodes.Available || currentProvState == nodes.Manageable {
		// Make sure we don't have a stale instance UUID
		if ironicNode.InstanceUUID != "" {
			var success bool
			p.log.Info("removing stale instance UUID before deletion", "instanceUUID", ironicNode.InstanceUUID)
			updater := clients.UpdateOptsBuilder(p.log)
			updater.SetTopLevelOpt("instance_uuid", nil, ironicNode.InstanceUUID)
			_, success, result, err = p.tryUpdateNode(ironicNode, updater)
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
	if err == nil {
		p.log.Info("removed")
	} else if gophercloud.ResponseCodeIs(err, http.StatusConflict) {
		p.log.Info("could not remove host, busy")
		return retryAfterDelay(provisionRequeueDelay)
	} else if gophercloud.ResponseCodeIs(err, http.StatusNotFound) {
		p.log.Info("did not find host to delete, OK")
	} else {
		return transientError(fmt.Errorf("failed to remove host: %w", err))
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
	p.log.Info("removing the node for detachment", "node", p.nodeID)
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

	if changeResult.Err == nil {
		p.log.Info("power change OK")
		event := map[nodes.TargetPowerState]struct{ Event, Reason string }{
			nodes.PowerOn:      {Event: "PowerOn", Reason: "Host powered on"},
			nodes.PowerOff:     {Event: "PowerOff", Reason: "Host powered off"},
			nodes.SoftPowerOff: {Event: "PowerOff", Reason: "Host soft powered off"},
		}[target]
		p.publisher(event.Event, event.Reason)
		return operationContinuing(0)
	} else if gophercloud.ResponseCodeIs(changeResult.Err, http.StatusConflict) {
		p.log.Info("host is locked, trying again after delay", "delay", powerRequeueDelay)
		return retryAfterDelay(powerRequeueDelay)
	} else if gophercloud.ResponseCodeIs(changeResult.Err, http.StatusBadRequest) {
		// Error 400 Bad Request means target power state is not supported by vendor driver
		if target == nodes.SoftPowerOff {
			changeResult.Err = softPowerOffUnsupportedError{changeResult.Err}
		}
	}
	p.log.Info("power change error", "message", changeResult.Err)
	return transientError(fmt.Errorf("failed to %s node: %w", target, changeResult.Err))
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
			return operationFailed("PowerOn operation failed: " +
				ironicNode.LastError)
		}
		return p.changePower(ironicNode, nodes.PowerOn)
	}
	return result, nil
}

// abortInspectionOrCleaning aborts inspection or cleaning if the node is in one of those states.
// This is necessary to allow power changes when the node is being deleted.
// Abort only works in *Wait states; for *ing states we return Dirty to retry later.
// For cleaning, we abort manual cleaning or automated cleaning if it's been disabled.
func (p *ironicProvisioner) abortInspectionOrCleaning(ironicNode *nodes.Node, automatedCleaningMode metal3api.AutomatedCleaningMode) (result provisioner.Result, err error) {
	provState := nodes.ProvisionState(ironicNode.ProvisionState)

	switch provState {
	case nodes.InspectWait:
		p.log.Info("aborting inspection to allow power off during deletion")
		return p.changeNodeProvisionState(
			ironicNode,
			nodes.ProvisionStateOpts{Target: nodes.TargetAbort},
		)
	case nodes.Inspecting:
		p.log.Info("inspection in progress, waiting for it to reach inspect wait state")
		return operationContinuing(provisionRequeueDelay)
	case nodes.InspectFail:
		// After aborting inspection, node transitions to InspectFail but Ironic does not
		// automatically clear TargetProvisionState. We need to explicitly transition to
		// manageable to clear it, which then allows power changes.
		if ironicNode.TargetProvisionState != "" {
			p.log.Info("transitioning from inspect failed to manageable to allow power off",
				"targetState", ironicNode.TargetProvisionState)
			return p.changeNodeProvisionState(
				ironicNode,
				nodes.ProvisionStateOpts{Target: nodes.TargetManage},
			)
		}
		// TargetProvisionState is clear, we can proceed with power off
		return operationComplete()
	case nodes.CleanWait:
		// Abort manual cleaning or automated cleaning if disabled.
		// Use automatedCleaningMode from BMH spec rather than ironicNode.AutomatedClean
		// which may be stale if BMO couldn't update it during cleaning states.
		isManualCleaning := ironicNode.TargetProvisionState == string(nodes.TargetClean)
		if isManualCleaning || automatedCleaningMode == metal3api.CleaningModeDisabled {
			if isManualCleaning {
				p.log.Info("aborting manual cleaning to allow power off during deletion")
			} else {
				p.log.Info("aborting automated cleaning (disabled) to allow power off during deletion")
			}
			return p.changeNodeProvisionState(
				ironicNode,
				nodes.ProvisionStateOpts{Target: nodes.TargetAbort},
			)
		}
		// Automated cleaning is enabled - let it finish
		p.log.Info("automated cleaning in progress, waiting for it to complete")
		return operationContinuing(provisionRequeueDelay)
	case nodes.Cleaning:
		// Check if it's manual cleaning or automated cleaning that's been disabled
		if ironicNode.TargetProvisionState == string(nodes.TargetClean) ||
			automatedCleaningMode == metal3api.CleaningModeDisabled {
			p.log.Info("cleaning in progress, waiting for it to reach clean wait state before aborting")
		} else {
			p.log.Info("automated cleaning in progress, waiting for it to complete")
		}
		return operationContinuing(provisionRequeueDelay)
	default:
		// Node is not in a state that needs aborting
		return operationComplete()
	}
}

// PowerOff ensures the server is powered off independently of any image
// provisioning operation.
func (p *ironicProvisioner) PowerOff(rebootMode metal3api.RebootMode, force bool, automatedCleaningMode metal3api.AutomatedCleaningMode) (result provisioner.Result, err error) {
	p.log.Info(fmt.Sprintf("ensuring host is powered off (mode: %s)", rebootMode))

	ironicNode, err := p.getNode()
	if err != nil {
		return transientError(err)
	}

	// If the node is in inspection or cleaning, we need to handle it before we can power off.
	// For *Wait states, we can abort. For *ing states, we wait for them to transition to *Wait.
	// This is especially important during deletion to avoid getting stuck waiting for inspection/cleaning to complete.
	result, err = p.abortInspectionOrCleaning(ironicNode, automatedCleaningMode)
	if err != nil || result.Dirty {
		return result, err
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
		// However, if the error is from an aborted operation (e.g., inspection abort),
		// this is expected and we should proceed with power off rather than failing.
		if targetState == "" && ironicNode.LastError != "" && !force {
			if !strings.Contains(ironicNode.LastError, "aborted") {
				p.log.Info("power off error", "msg", ironicNode.LastError)
				return operationFailed(ironicNode.LastError)
			}
			// Error is from an abort operation, not a power-off failure - proceed
			p.log.Info("ignoring abort error, proceeding with power off", "msg", ironicNode.LastError)
		}

		if rebootMode == metal3api.RebootModeSoft && !force {
			powerTarget := nodes.SoftPowerOff
			if ironicNode.DisablePowerOff {
				powerTarget = nodes.SoftRebooting
			}
			result, err = p.changePower(ironicNode, powerTarget)
			if !errors.As(err, &softPowerOffUnsupportedError{}) {
				return result, err
			}
		}
		// Reboot mode is hard, force flag is set, or soft power off is not supported
		powerTarget := nodes.PowerOff
		if ironicNode.DisablePowerOff {
			powerTarget = nodes.Rebooting
		}
		return p.changePower(ironicNode, powerTarget)
	}

	return result, nil
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
		default:
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

// Uses the Ironic Virtual Media Get API which synchronously fetches the
// virtual media details for the given node.
// We return only the bool isImageAttached because the url in the response
// body is always the same - based on the node uuid, so it is not useful
// for any comparison purpose.
func (p *ironicProvisioner) GetDataImageStatus() (isImageAttached bool, err error) {
	// Check if Ironic API version supports Virtual Media Get API
	// Needs version >= 1.93
	if !p.availableFeatures.HasVirtualMediaGetAPI() {
		return false, fmt.Errorf("ironic version=%d doesn't support Virtual Media Get API, needs version>=1.93", p.availableFeatures.MaxVersion)
	}

	node, err := p.getNode()
	if err != nil {
		return true, err
	}

	// If the node is busy, return error, since the get request will anyways
	// fail
	// TODO(hroyrh): is there a better way to do this ?
	isNodeBusy := node.Reservation != ""
	if isNodeBusy {
		p.log.Info("node is busy, retry GetVirtualMedia request in some time", "node", node.UUID)
		return false, provisioner.ErrNodeIsBusy
	}

	result := nodes.GetVirtualMedia(p.ctx, p.client, p.nodeID)
	err = result.Err
	if err != nil {
		return false, err
	}

	// FIXME (hroyrh) : implement Extract function for VirtualMediaGetResult
	// in gophercloud
	var vmediaList []VirtualMedia
	err = result.ExtractInto(&vmediaList)
	if err != nil {
		p.log.Info("GetDataImage, failed to extract result body", "Error", err)
		return false, err
	}

	for _, vmedia := range vmediaList {
		if vmedia.Inserted {
			p.log.Info("GetDataImage, vmedia attached", "URL", vmedia.Image)
			return true, nil
		}
	}

	p.log.Info("GetDataImage, no vmedia is attached")
	return false, nil
}

func (p *ironicProvisioner) AttachDataImage(url string) (err error) {
	err = nodes.AttachVirtualMedia(p.ctx, p.client, p.nodeID, nodes.AttachVirtualMediaOpts{
		DeviceType: nodes.VirtualMediaCD,
		ImageURL:   url,
	}).ExtractErr()
	if err != nil {
		return err
	}

	return nil
}

func (p *ironicProvisioner) DetachDataImage() (err error) {
	err = nodes.DetachVirtualMedia(p.ctx, p.client, p.nodeID, nodes.DetachVirtualMediaOpts{
		DeviceTypes: []nodes.VirtualMediaDeviceType{nodes.VirtualMediaCD},
	}).ExtractErr()
	if err != nil {
		return err
	}

	return nil
}

func (p *ironicProvisioner) HasPowerFailure() bool {
	node, err := p.getNode()
	if err != nil {
		p.log.Error(err, "Ignored error while checking for Power Failure")
		return false
	}
	return node.Fault == "power failure"
}
