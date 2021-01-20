package ironic

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack/baremetal/v1/nodes"
	"github.com/gophercloud/gophercloud/openstack/baremetal/v1/ports"
	"github.com/gophercloud/gophercloud/openstack/baremetalintrospection/v1/introspection"
	"github.com/pkg/errors"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/yaml"

	metal3v1alpha1 "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	"github.com/metal3-io/baremetal-operator/pkg/bmc"
	"github.com/metal3-io/baremetal-operator/pkg/hardware"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner/ironic/clients"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner/ironic/devicehints"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner/ironic/hardwaredetails"
)

var (
	log                       = logf.Log.WithName("provisioner").WithName("ironic")
	deprovisionRequeueDelay   = time.Second * 10
	provisionRequeueDelay     = time.Second * 10
	powerRequeueDelay         = time.Second * 10
	introspectionRequeueDelay = time.Second * 15
	softPowerOffTimeout       = time.Second * 180
	deployKernelURL           string
	deployRamdiskURL          string
	ironicEndpoint            string
	inspectorEndpoint         string
	ironicTrustedCAFile       string
	ironicInsecure            bool
	ironicAuth                clients.AuthConfig
	inspectorAuth             clients.AuthConfig

	// Keep pointers to ironic and inspector clients configured with
	// the global auth settings to reuse the connection between
	// reconcilers.
	clientIronicSingleton    *gophercloud.ServiceClient
	clientInspectorSingleton *gophercloud.ServiceClient
)

const (
	// See nodes.Node.PowerState for details
	powerOn      = "power on"
	powerOff     = "power off"
	softPowerOff = "soft power off"
	powerNone    = "None"
)

var bootModeCapabilities = map[metal3v1alpha1.BootMode]string{
	metal3v1alpha1.UEFI:   "boot_mode:uefi",
	metal3v1alpha1.Legacy: "boot_mode:bios",
}

func init() {
	// NOTE(dhellmann): Use Fprintf() to report errors instead of
	// logging, because logging is not configured yet in init().

	var authErr error
	ironicAuth, inspectorAuth, authErr = clients.LoadAuth()
	if authErr != nil {
		fmt.Fprintf(os.Stderr, "Cannot start: %s\n", authErr)
		os.Exit(1)
	}

	deployKernelURL = os.Getenv("DEPLOY_KERNEL_URL")
	if deployKernelURL == "" {
		fmt.Fprintf(os.Stderr, "Cannot start: No DEPLOY_KERNEL_URL variable set\n")
		os.Exit(1)
	}
	deployRamdiskURL = os.Getenv("DEPLOY_RAMDISK_URL")
	if deployRamdiskURL == "" {
		fmt.Fprintf(os.Stderr, "Cannot start: No DEPLOY_RAMDISK_URL variable set\n")
		os.Exit(1)
	}
	ironicEndpoint = os.Getenv("IRONIC_ENDPOINT")
	if ironicEndpoint == "" {
		fmt.Fprintf(os.Stderr, "Cannot start: No IRONIC_ENDPOINT variable set\n")
		os.Exit(1)
	}
	inspectorEndpoint = os.Getenv("IRONIC_INSPECTOR_ENDPOINT")
	if inspectorEndpoint == "" {
		fmt.Fprintf(os.Stderr, "Cannot start: No IRONIC_INSPECTOR_ENDPOINT variable set")
		os.Exit(1)
	}
	ironicTrustedCAFile = os.Getenv("IRONIC_CACERT_FILE")
	if ironicTrustedCAFile == "" {
		ironicTrustedCAFile = "/opt/metal3/certs/ca/crt"
	}
	ironicInsecureStr := os.Getenv("IRONIC_INSECURE")
	if strings.ToLower(ironicInsecureStr) == "true" {
		ironicInsecure = true
	}
}

// Provisioner implements the provisioning.Provisioner interface
// and uses Ironic to manage the host.
type ironicProvisioner struct {
	// the host to be managed by this provisioner
	host *metal3v1alpha1.BareMetalHost
	// a shorter path to the provisioning status data structure
	status *metal3v1alpha1.ProvisionStatus
	// access parameters for the BMC
	bmcAccess bmc.AccessDetails
	// credentials to log in to the BMC
	bmcCreds bmc.Credentials
	// a client for talking to ironic
	client *gophercloud.ServiceClient
	// a client for talking to ironic-inspector
	inspector *gophercloud.ServiceClient
	// a logger configured for this host
	log logr.Logger
	// an event publisher for recording significant events
	publisher provisioner.EventPublisher
}

// LogStartup produces useful logging information that we only want to
// emit once on startup but that is interal to this package.
func LogStartup() {
	log.Info("ironic settings",
		"endpoint", ironicEndpoint,
		"ironicAuthType", ironicAuth.Type,
		"inspectorEndpoint", inspectorEndpoint,
		"inspectorAuthType", inspectorAuth.Type,
		"deployKernelURL", deployKernelURL,
		"deployRamdiskURL", deployRamdiskURL,
	)
}

// A private function to construct an ironicProvisioner (rather than a
// Provisioner interface) in a consistent way for tests.
func newProvisionerWithSettings(host *metal3v1alpha1.BareMetalHost, bmcCreds bmc.Credentials, publisher provisioner.EventPublisher, ironicURL string, ironicAuthSettings clients.AuthConfig, inspectorURL string, inspectorAuthSettings clients.AuthConfig) (*ironicProvisioner, error) {
	tlsConf := clients.TLSConfig{
		TrustedCAFile:      ironicTrustedCAFile,
		InsecureSkipVerify: ironicInsecure,
	}
	clientIronic, err := clients.IronicClient(ironicURL, ironicAuthSettings, tlsConf)
	if err != nil {
		return nil, err
	}

	clientInspector, err := clients.InspectorClient(inspectorURL, inspectorAuthSettings, tlsConf)
	if err != nil {
		return nil, err
	}

	return newProvisionerWithIronicClients(host, bmcCreds, publisher,
		clientIronic, clientInspector)
}

func newProvisionerWithIronicClients(host *metal3v1alpha1.BareMetalHost, bmcCreds bmc.Credentials, publisher provisioner.EventPublisher, clientIronic *gophercloud.ServiceClient, clientInspector *gophercloud.ServiceClient) (*ironicProvisioner, error) {

	bmcAccess, err := bmc.NewAccessDetails(host.Spec.BMC.Address, host.Spec.BMC.DisableCertificateVerification)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse BMC address information")
	}

	// Ensure we have a microversion high enough to get the features
	// we need.
	clientIronic.Microversion = "1.56"
	p := &ironicProvisioner{
		host:      host,
		status:    &(host.Status.Provisioning),
		bmcAccess: bmcAccess,
		bmcCreds:  bmcCreds,
		client:    clientIronic,
		inspector: clientInspector,
		log:       log.WithValues("host", host.Name),
		publisher: publisher,
	}

	return p, nil
}

// New returns a new Ironic Provisioner using the global configuration
// for finding the Ironic services.
func New(host *metal3v1alpha1.BareMetalHost, bmcCreds bmc.Credentials, publisher provisioner.EventPublisher) (provisioner.Provisioner, error) {
	var err error
	if clientIronicSingleton == nil || clientInspectorSingleton == nil {
		tlsConf := clients.TLSConfig{
			TrustedCAFile:      ironicTrustedCAFile,
			InsecureSkipVerify: ironicInsecure,
		}
		clientIronicSingleton, err = clients.IronicClient(
			ironicEndpoint, ironicAuth, tlsConf)
		if err != nil {
			return nil, err
		}

		clientInspectorSingleton, err = clients.InspectorClient(
			inspectorEndpoint, inspectorAuth, tlsConf)
		if err != nil {
			return nil, err
		}
	}
	return newProvisionerWithIronicClients(host, bmcCreds, publisher,
		clientIronicSingleton, clientInspectorSingleton)
}

func (p *ironicProvisioner) validateNode(ironicNode *nodes.Node) (errorMessage string, err error) {
	var validationErrors []string

	p.log.Info("validating node settings in ironic")
	validateResult, err := nodes.Validate(p.client, ironicNode.UUID).Extract()
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

	opts := ports.ListOpts{}

	if address != "" {
		opts.Address = address
	}

	pager := ports.List(p.client, opts)

	if pager.Err != nil {
		return allPorts, pager.Err
	}

	allPages, err := pager.AllPages()

	if err != nil {
		return allPorts, err
	}

	return ports.ExtractPorts(allPages)

}

// Look for an existing registration for the host in Ironic.
func (p *ironicProvisioner) findExistingHost() (ironicNode *nodes.Node, err error) {
	// Try to load the node by UUID
	if p.status.ID != "" {
		// Look for the node to see if it exists (maybe Ironic was
		// restarted)
		ironicNode, err = nodes.Get(p.client, p.status.ID).Extract()
		switch err.(type) {
		case nil:
			p.log.Info("found existing node by ID")
			return ironicNode, nil
		case gophercloud.ErrDefault404:
			// Look by ID failed, trying to lookup by hostname in case it was
			// previously created
		default:
			return nil, errors.Wrap(err,
				fmt.Sprintf("failed to find node by ID %s", p.status.ID))
		}
	}

	// Try to load the node by name
	p.log.Info("looking for existing node by name", "name", p.host.Name)
	ironicNode, err = nodes.Get(p.client, p.host.Name).Extract()
	switch err.(type) {
	case nil:
		p.log.Info("found existing node by name")
		return ironicNode, nil
	case gophercloud.ErrDefault404:
		p.log.Info(
			fmt.Sprintf("node with name %s doesn't exist", p.host.Name))
	default:
		return nil, errors.Wrap(err,
			fmt.Sprintf("failed to find node by name %s", p.host.Name))
	}

	// Try to load the node by port address
	p.log.Info("looking for existing node by MAC", "MAC", p.host.Spec.BootMACAddress)
	allPorts, err := p.listAllPorts(p.host.Spec.BootMACAddress)

	if err != nil {
		p.log.Info("failed to find an existing port with address", "MAC", p.host.Spec.BootMACAddress)
		return nil, nil
	}

	if len(allPorts) > 0 {
		nodeUUID := allPorts[0].NodeUUID
		ironicNode, err = nodes.Get(p.client, nodeUUID).Extract()
		switch err.(type) {
		case nil:
			p.log.Info("found existing node by ID")

			// If the node has a name, this means we didn't find it above.
			if ironicNode.Name != "" {
				return nil, errors.New(fmt.Sprint("node found by MAC but has a name: ", ironicNode.Name))
			}

			return ironicNode, nil
		case gophercloud.ErrDefault404:
			return nil, errors.Wrap(err,
				fmt.Sprintf("port exists but linked node doesn't %s", nodeUUID))
		default:
			return nil, errors.Wrap(err,
				fmt.Sprintf("port exists but failed to find linked node by ID %s", nodeUUID))
		}
	} else {
		p.log.Info("port with address doesn't exist", "MAC", p.host.Spec.BootMACAddress)
	}

	return nil, nil

}

// ValidateManagementAccess registers the host with the provisioning
// system and tests the connection information for the host to verify
// that the location and credentials work.
//
// FIXME(dhellmann): We should rename this method to describe what it
// actually does.
func (p *ironicProvisioner) ValidateManagementAccess(credentialsChanged bool) (result provisioner.Result, err error) {
	var ironicNode *nodes.Node

	p.log.Info("validating management access")

	ironicNode, err = p.findExistingHost()
	if err != nil {
		return result, errors.Wrap(err, "failed to find existing host")
	}

	// Some BMC types require a MAC address, so ensure we have one
	// when we need it. If not, place the host in an error state.
	if p.bmcAccess.NeedsMAC() && p.host.Spec.BootMACAddress == "" {
		msg := fmt.Sprintf("BMC driver %s requires a BootMACAddress value", p.bmcAccess.Type())
		p.log.Info(msg)
		result.ErrorMessage = msg
		return result, nil
	}

	driverInfo := p.bmcAccess.DriverInfo(p.bmcCreds)
	// FIXME(dhellmann): We need to get our IP on the
	// provisioning network from somewhere.
	driverInfo["deploy_kernel"] = deployKernelURL
	driverInfo["deploy_ramdisk"] = deployRamdiskURL

	// If we have not found a node yet, we need to create one
	if ironicNode == nil {
		p.log.Info("registering host in ironic")

		ironicNode, err = nodes.Create(
			p.client,
			nodes.CreateOpts{
				Driver:              p.bmcAccess.Driver(),
				BootInterface:       p.bmcAccess.BootInterface(),
				Name:                p.host.Name,
				DriverInfo:          driverInfo,
				InspectInterface:    "inspector",
				ManagementInterface: p.bmcAccess.ManagementInterface(),
				PowerInterface:      p.bmcAccess.PowerInterface(),
				RAIDInterface:       p.bmcAccess.RAIDInterface(),
				VendorInterface:     p.bmcAccess.VendorInterface(),
				Properties: map[string]interface{}{
					"capabilities": bootModeCapabilities[p.host.Status.Provisioning.BootMode],
				},
			}).Extract()
		// FIXME(dhellmann): Handle 409 and 503? errors here.
		if err != nil {
			return result, errors.Wrap(err, "failed to register host in ironic")
		}
		p.publisher("Registered", "Registered new host")

		// Store the ID so other methods can assume it is set and so
		// we can find the node again later.
		p.status.ID = ironicNode.UUID
		result.Dirty = true
		p.log.Info("setting provisioning id", "ID", p.status.ID)

		// If we know the MAC, create a port. Otherwise we will have
		// to do this after we run the introspection step.
		if p.host.Spec.BootMACAddress != "" {
			enable := true
			p.log.Info("creating port for node in ironic", "MAC",
				p.host.Spec.BootMACAddress)
			_, err := ports.Create(
				p.client,
				ports.CreateOpts{
					NodeUUID:   ironicNode.UUID,
					Address:    p.host.Spec.BootMACAddress,
					PXEEnabled: &enable,
				}).Extract()
			if err != nil {
				return result, errors.Wrap(err, "failed to create port in ironic")
			}
		}

		// If there is an image to be provisioned, or an image has
		// previously been provisioned, include those details. Either
		// case may mean we are re-adopting a host that was already
		// known but removed/lost because the pod restarted.
		var imageData *metal3v1alpha1.Image
		switch {
		case p.host.Status.Provisioning.Image.URL != "":
			imageData = &p.host.Status.Provisioning.Image
		case p.host.Spec.Image != nil && p.host.Spec.Image.URL != "":
			imageData = p.host.Spec.Image
		}

		checksum, checksumType, ok := imageData.GetChecksum()

		if ok {
			p.log.Info("setting instance info",
				"image_source", imageData.URL,
				"image_os_hash_value", checksum,
				"image_os_hash_algo", checksumType,
			)

			updates := nodes.UpdateOpts{
				nodes.UpdateOperation{
					Op:    nodes.AddOp,
					Path:  "/instance_info/image_source",
					Value: imageData.URL,
				},
				nodes.UpdateOperation{
					Op:    nodes.AddOp,
					Path:  "/instance_info/image_os_hash_value",
					Value: checksum,
				},
				nodes.UpdateOperation{
					Op:    nodes.AddOp,
					Path:  "/instance_info/image_os_hash_algo",
					Value: checksumType,
				},
				nodes.UpdateOperation{
					Op:    nodes.ReplaceOp,
					Path:  "/instance_uuid",
					Value: string(p.host.ObjectMeta.UID),
				},
			}

			// image_checksum
			//
			// FIXME: For older versions of ironic that do not have
			// https://review.opendev.org/#/c/711816/ failing to
			// include the 'image_checksum' causes ironic to refuse to
			// provision the image, even if the other hash value
			// parameters are given. We only want to do that for MD5,
			// however, because those versions of ironic only support
			// MD5 checksums.
			if checksumType == string(metal3v1alpha1.MD5) {
				updates = append(
					updates,
					nodes.UpdateOperation{
						Op:    nodes.AddOp,
						Path:  "/instance_info/image_checksum",
						Value: checksum,
					},
				)
			}

			if imageData.DiskFormat != nil {
				updates = append(updates, nodes.UpdateOperation{
					Op:    nodes.AddOp,
					Path:  "/instance_info/image_disk_format",
					Value: *imageData.DiskFormat,
				})
			}
			_, err = nodes.Update(p.client, ironicNode.UUID, updates).Extract()
			switch err.(type) {
			case nil:
			case gophercloud.ErrDefault409:
				p.log.Info("could not update host settings in ironic, busy")
				result.Dirty = true
				result.RequeueAfter = provisionRequeueDelay
				return result, nil
			default:
				return result, errors.Wrap(err, "failed to update host settings in ironic")
			}
		}
	} else {
		// FIXME(dhellmann): At this point we have found an existing
		// node in ironic by looking it up. We need to check its
		// settings against what we have in the host, and change them
		// if there are differences.
		if p.status.ID != ironicNode.UUID {
			// Store the ID so other methods can assume it is set and
			// so we can find the node using that value next time.
			p.status.ID = ironicNode.UUID
			result.Dirty = true
			p.log.Info("setting provisioning id", "ID", p.status.ID)
		}

		if ironicNode.Name == "" {
			updates := nodes.UpdateOpts{
				nodes.UpdateOperation{
					Op:    nodes.ReplaceOp,
					Path:  "/name",
					Value: p.host.Name,
				},
			}
			ironicNode, err = nodes.Update(p.client, ironicNode.UUID, updates).Extract()
			switch err.(type) {
			case nil:
			case gophercloud.ErrDefault409:
				p.log.Info("could not update ironic node name, busy")
				result.Dirty = true
				result.RequeueAfter = provisionRequeueDelay
				return result, nil
			default:
				return result, errors.Wrap(err, "failed to update ironc node name")
			}
			p.log.Info("updated ironic node name")

		}

		// Look for the case where we previously enrolled this node
		// and now the credentials have changed.
		if credentialsChanged {
			updates := nodes.UpdateOpts{
				nodes.UpdateOperation{
					Op:    nodes.ReplaceOp,
					Path:  "/driver_info",
					Value: driverInfo,
				},
			}
			ironicNode, err = nodes.Update(p.client, ironicNode.UUID, updates).Extract()
			switch err.(type) {
			case nil:
			case gophercloud.ErrDefault409:
				p.log.Info("could not update host driver settings, busy")
				result.Dirty = true
				result.RequeueAfter = provisionRequeueDelay
				return result, nil
			default:
				return result, errors.Wrap(err, "failed to update host driver settings")
			}
			p.log.Info("updated host driver settings")
			// We don't return here because we also have to set the
			// target provision state to manageable, which happens
			// below.
		}
	}

	// ironicNode, err = nodes.Get(p.client, p.status.ID).Extract()
	// if err != nil {
	// 	return result, errors.Wrap(err, "failed to get provisioning state in ironic")
	// }

	p.log.Info("current provision state",
		"lastError", ironicNode.LastError,
		"current", ironicNode.ProvisionState,
		"target", ironicNode.TargetProvisionState,
	)

	// Ensure the node is marked manageable.
	switch nodes.ProvisionState(ironicNode.ProvisionState) {

	case nodes.Enroll:

		// If ironic is reporting an error, stop working on the node.
		if ironicNode.LastError != "" && !credentialsChanged {
			result.ErrorMessage = ironicNode.LastError
			return result, nil
		}

		if ironicNode.TargetProvisionState == string(nodes.TargetManage) {
			// We have already tried to manage the node and did not
			// get an error, so do nothing and keep trying.
			result.Dirty = true
			result.RequeueAfter = provisionRequeueDelay
			return result, nil
		}

		return p.changeNodeProvisionState(
			ironicNode,
			nodes.ProvisionStateOpts{Target: nodes.TargetManage},
		)

	case nodes.Verifying:
		// If we're still waiting for the state to change in Ironic,
		// return true to indicate that we're dirty and need to be
		// reconciled again.
		result.RequeueAfter = provisionRequeueDelay
		result.Dirty = true
		return result, nil

	case nodes.Manageable:
		p.log.Info("have manageable host")
		return result, nil

	case nodes.Available:
		// The host is fully registered (and probably wasn't cleanly
		// deleted previously)
		p.log.Info("have available host")
		return result, nil

	case nodes.Active:
		// The host is already running, maybe it's a master?
		p.log.Info("have active host", "image_source", ironicNode.InstanceInfo["image_source"])
		return result, nil

	default:
		return result, nil
	}
}

func (p *ironicProvisioner) tryChangeNodeProvisionState(ironicNode *nodes.Node, opts nodes.ProvisionStateOpts) (success bool, result provisioner.Result, err error) {
	p.log.Info("changing provisioning state",
		"current", ironicNode.ProvisionState,
		"existing target", ironicNode.TargetProvisionState,
		"new target", opts.Target,
	)

	changeResult := nodes.ChangeProvisionState(p.client, ironicNode.UUID, opts)
	switch changeResult.Err.(type) {
	case nil:
		success = true
	case gophercloud.ErrDefault409:
		p.log.Info("could not change state of host, busy")
	default:
		err = errors.Wrap(changeResult.Err,
			fmt.Sprintf("failed to change provisioning state to %q", opts.Target))
		return
	}

	result.Dirty = true
	result.RequeueAfter = provisionRequeueDelay
	return
}

func (p *ironicProvisioner) changeNodeProvisionState(ironicNode *nodes.Node, opts nodes.ProvisionStateOpts) (result provisioner.Result, err error) {
	_, result, err = p.tryChangeNodeProvisionState(ironicNode, opts)
	return
}

// InspectHardware updates the HardwareDetails field of the host with
// details of devices discovered on the hardware. It may be called
// multiple times, and should return true for its dirty flag until the
// inspection is completed.
func (p *ironicProvisioner) InspectHardware() (result provisioner.Result, details *metal3v1alpha1.HardwareDetails, err error) {
	p.log.Info("inspecting hardware", "status", p.host.OperationalStatus())

	ironicNode, err := p.findExistingHost()
	if err != nil {
		err = errors.Wrap(err, "failed to find existing host")
		return
	}
	if ironicNode == nil {
		return result, nil, provisioner.NeedsRegistration
	}

	status, err := introspection.GetIntrospectionStatus(p.inspector, ironicNode.UUID).Extract()
	if err != nil {
		if _, isNotFound := err.(gophercloud.ErrDefault404); isNotFound {
			switch nodes.ProvisionState(ironicNode.ProvisionState) {
			case nodes.Inspecting, nodes.InspectWait:
				p.log.Info("inspection already started")
				result.Dirty = true
				result.RequeueAfter = introspectionRequeueDelay
				err = nil
				return
			default:
				p.log.Info("updating boot mode before hardware inspection")
				op, value := buildCapabilitiesValue(ironicNode, p.host.Status.Provisioning.BootMode)
				updates := nodes.UpdateOpts{
					nodes.UpdateOperation{
						Op:    op,
						Path:  "/properties/capabilities",
						Value: value,
					},
				}
				_, err = nodes.Update(p.client, ironicNode.UUID, updates).Extract()
				switch err.(type) {
				case nil:
				case gophercloud.ErrDefault409:
					p.log.Info("could not update host settings in ironic, busy")
					result.Dirty = true
					return
				default:
					err = errors.Wrap(err, "failed to update host boot mode settings in ironic")
					return
				}

				p.log.Info("starting new hardware inspection")
				var success bool
				success, result, err = p.tryChangeNodeProvisionState(
					ironicNode,
					nodes.ProvisionStateOpts{Target: nodes.TargetInspect},
				)
				if success {
					p.publisher("InspectionStarted", "Hardware inspection started")
				}
				return
			}
		}
		err = errors.Wrap(err, "failed to extract hardware inspection status")
		return
	}
	if !status.Finished {
		p.log.Info("inspection in progress", "started_at", status.StartedAt)
		result.Dirty = true // make sure we check back
		result.RequeueAfter = introspectionRequeueDelay
		return
	}
	if status.Error != "" {
		p.log.Info("inspection failed", "error", status.Error)
		result.ErrorMessage = status.Error
		return
	}

	// Introspection is done
	p.log.Info("getting hardware details from inspection")
	introData := introspection.GetIntrospectionData(p.inspector, ironicNode.UUID)
	data, err := introData.Extract()
	if err != nil {
		err = errors.Wrap(err, "failed to retrieve hardware introspection data")
		return
	}
	p.log.Info("received introspection data", "data", introData.Body)

	details = hardwaredetails.GetHardwareDetails(data)
	p.publisher("InspectionComplete", "Hardware inspection completed")
	return
}

// UpdateHardwareState fetches the latest hardware state of the server
// and updates the HardwareDetails field of the host with details. It
// is expected to do this in the least expensive way possible, such as
// reading from a cache, and return dirty only if any state
// information has changed.
func (p *ironicProvisioner) UpdateHardwareState() (result provisioner.Result, err error) {
	p.log.Info("updating hardware state")

	ironicNode, err := p.findExistingHost()
	if err != nil {
		return result, errors.Wrap(err, "failed to find existing host")
	}
	if ironicNode == nil {
		return result, provisioner.NeedsRegistration
	}

	var discoveredVal bool
	switch ironicNode.PowerState {
	case powerOn:
		discoveredVal = true
	case powerOff:
		discoveredVal = false
	case powerNone:
		p.log.Info("could not determine power state", "value", ironicNode.PowerState)
		return result, nil
	default:
		p.log.Info("unknown power state", "value", ironicNode.PowerState)
		return result, nil
	}

	if discoveredVal != p.host.Status.PoweredOn {
		p.log.Info("updating power status", "discovered", discoveredVal)
		p.host.Status.PoweredOn = discoveredVal
		result.Dirty = true
	}
	return result, nil
}

func (p *ironicProvisioner) getUpdateOptsForNode(ironicNode *nodes.Node) (updates nodes.UpdateOpts, err error) {

	hwProf, err := hardware.GetProfile(p.host.HardwareProfile())

	if err != nil {
		return updates, errors.Wrap(err,
			fmt.Sprintf("Could not start provisioning with bad hardware profile %s",
				p.host.HardwareProfile()))
	}

	// image_source
	var op nodes.UpdateOp
	if _, ok := ironicNode.InstanceInfo["image_source"]; !ok {
		op = nodes.AddOp
		p.log.Info("adding image_source")
	} else {
		op = nodes.ReplaceOp
		p.log.Info("updating image_source")
	}
	updates = append(
		updates,
		nodes.UpdateOperation{
			Op:    op,
			Path:  "/instance_info/image_source",
			Value: p.host.Spec.Image.URL,
		},
	)

	checksum, checksumType, _ := p.host.GetImageChecksum()

	// image_os_hash_algo
	if _, ok := ironicNode.InstanceInfo["image_os_hash_algo"]; !ok {
		op = nodes.AddOp
		p.log.Info("adding image_os_hash_algo")
	} else {
		op = nodes.ReplaceOp
		p.log.Info("updating image_os_hash_algo")
	}
	updates = append(
		updates,
		nodes.UpdateOperation{
			Op:    op,
			Path:  "/instance_info/image_os_hash_algo",
			Value: checksumType,
		},
	)

	// image_os_hash_value
	if _, ok := ironicNode.InstanceInfo["image_os_hash_value"]; !ok {
		op = nodes.AddOp
		p.log.Info("adding image_os_hash_value")
	} else {
		op = nodes.ReplaceOp
		p.log.Info("updating image_os_hash_value")
	}
	updates = append(
		updates,
		nodes.UpdateOperation{
			Op:    op,
			Path:  "/instance_info/image_os_hash_value",
			Value: checksum,
		},
	)

	// image_checksum
	//
	// FIXME: For older versions of ironic that do not have
	// https://review.opendev.org/#/c/711816/ failing to include the
	// 'image_checksum' causes ironic to refuse to provision the
	// image, even if the other hash value parameters are given. We
	// only want to do that for MD5, however, because those versions
	// of ironic only support MD5 checksums.
	if checksumType == string(metal3v1alpha1.MD5) {
		if _, ok := ironicNode.InstanceInfo["image_checksum"]; !ok {
			op = nodes.AddOp
			p.log.Info("adding image_checksum")
		} else {
			op = nodes.ReplaceOp
			p.log.Info("updating image_checksum")
		}
		updates = append(
			updates,
			nodes.UpdateOperation{
				Op:    op,
				Path:  "/instance_info/image_checksum",
				Value: checksum,
			},
		)
	}

	if p.host.Spec.Image.DiskFormat != nil {
		updates = append(updates, nodes.UpdateOperation{
			Op:    nodes.AddOp,
			Path:  "/instance_info/image_disk_format",
			Value: *p.host.Spec.Image.DiskFormat,
		})
	}

	// instance_uuid
	p.log.Info("setting instance_uuid")
	updates = append(
		updates,
		nodes.UpdateOperation{
			Op:    nodes.ReplaceOp,
			Path:  "/instance_uuid",
			Value: string(p.host.ObjectMeta.UID),
		},
	)

	// root_gb
	//
	// FIXME(dhellmann): We have to provide something for the disk
	// size until https://storyboard.openstack.org/#!/story/2005165 is
	// fixed in ironic.
	if _, ok := ironicNode.InstanceInfo["root_gb"]; !ok {
		op = nodes.AddOp
		p.log.Info("adding root_gb")
	} else {
		op = nodes.ReplaceOp
		p.log.Info("updating root_gb")
	}
	updates = append(
		updates,
		nodes.UpdateOperation{
			Op:    op,
			Path:  "/instance_info/root_gb",
			Value: hwProf.RootGB,
		},
	)

	// root_device
	//
	// FIXME(dhellmann): We need to specify the root device to receive
	// the image. That should come from some combination of inspecting
	// the host to see what is available and the hardware profile to
	// give us instructions.
	if _, ok := ironicNode.Properties["root_device"]; !ok {
		op = nodes.AddOp
		p.log.Info("adding root_device")
	} else {
		op = nodes.ReplaceOp
		p.log.Info("updating root_device")
	}

	// hints
	//
	// If the user has provided explicit root device hints, they take
	// precedence. Otherwise use the values from the hardware profile.
	hints := devicehints.MakeHintMap(p.host.Status.Provisioning.RootDeviceHints)
	p.log.Info("using root device", "hints", hints)
	updates = append(
		updates,
		nodes.UpdateOperation{
			Op:    op,
			Path:  "/properties/root_device",
			Value: hints,
		},
	)

	// cpu_arch
	//
	// FIXME(dhellmann): This should come from inspecting the
	// host.
	if _, ok := ironicNode.Properties["cpu_arch"]; !ok {
		op = nodes.AddOp
		p.log.Info("adding cpu_arch")
	} else {
		op = nodes.ReplaceOp
		p.log.Info("updating cpu_arch")
	}
	updates = append(
		updates,
		nodes.UpdateOperation{
			Op:    op,
			Path:  "/properties/cpu_arch",
			Value: hwProf.CPUArch,
		},
	)

	// local_gb
	if _, ok := ironicNode.Properties["local_gb"]; !ok {
		op = nodes.AddOp
		p.log.Info("adding local_gb")
	} else {
		op = nodes.ReplaceOp
		p.log.Info("updating local_gb")
	}
	updates = append(
		updates,
		nodes.UpdateOperation{
			Op:    op,
			Path:  "/properties/local_gb",
			Value: hwProf.LocalGB,
		},
	)

	// boot_mode
	op, value := buildCapabilitiesValue(ironicNode, p.host.Status.Provisioning.BootMode)
	updates = append(
		updates,
		nodes.UpdateOperation{
			Op:    op,
			Path:  "/properties/capabilities",
			Value: value,
		},
	)

	return updates, nil
}

// We can't just replace the capabilities because we need to keep the
// values provided by inspection. We can't replace only the boot_mode
// because the API isn't fine-grained enough for that. So we have to
// look at the existing value and modify it. This function
// encapsulates the logic for building the value and knowing which
// update operation to use with the results.
func buildCapabilitiesValue(ironicNode *nodes.Node, bootMode metal3v1alpha1.BootMode) (op nodes.UpdateOp, value string) {

	capabilities, ok := ironicNode.Properties["capabilities"]
	if !ok {
		// There is no existing capabilities value
		return nodes.AddOp, bootModeCapabilities[bootMode]
	}
	existingCapabilities := capabilities.(string)

	// The capabilities value is set, so we will want to replace it.
	op = nodes.ReplaceOp

	if existingCapabilities == "" {
		// The existing value is empty so we can replace the whole
		// thing.
		value = bootModeCapabilities[bootMode]
		return
	}

	if !strings.Contains(existingCapabilities, "boot_mode") {
		// No boot_mode is set, append and return
		value = fmt.Sprintf("%s,%s", existingCapabilities, bootModeCapabilities[bootMode])
		return
	}

	// The capabilities value has format "var1:val1,var2:val2". We
	// know that boot_mode is there but not what value it has.  There
	// are only 2 values, so we can replace the "wrong" string with
	// the right one without fully parsing the string. We may want to
	// change this later when we have more boot modes.

	var fromMode metal3v1alpha1.BootMode
	if bootMode == metal3v1alpha1.UEFI {
		fromMode = metal3v1alpha1.Legacy
	} else {
		fromMode = metal3v1alpha1.UEFI
	}
	value = strings.ReplaceAll(existingCapabilities,
		bootModeCapabilities[fromMode], bootModeCapabilities[bootMode])

	return
}

func (p *ironicProvisioner) startProvisioning(ironicNode *nodes.Node, hostConf provisioner.HostConfigData) (result provisioner.Result, err error) {

	p.log.Info("starting provisioning", "node properties", ironicNode.Properties)

	updates, err := p.getUpdateOptsForNode(ironicNode)
	if err != nil {
		return result, errors.Wrap(err, "failed to update opts for node")
	}
	_, err = nodes.Update(p.client, ironicNode.UUID, updates).Extract()
	switch err.(type) {
	case nil:
	case gophercloud.ErrDefault409:
		p.log.Info("could not update host settings in ironic, busy")
		result.Dirty = true
		return result, nil
	default:
		return result, errors.Wrap(err, "failed to update host settings in ironic")
	}

	p.log.Info("validating host settings")

	errorMessage, err := p.validateNode(ironicNode)
	switch err.(type) {
	case nil:
	case gophercloud.ErrDefault409:
		p.log.Info("could not validate host during registration, busy")
		result.Dirty = true
		return result, nil
	default:
		return result, errors.Wrap(err, "failed to validate host during registration")
	}
	if errorMessage != "" {
		result.ErrorMessage = errorMessage
		result.Dirty = true // validateNode() would have set the errors
		return result, nil
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
		fmt.Sprintf("Image provisioning started for %s", p.host.Spec.Image.URL))
	return
}

// Adopt allows an externally-provisioned server to be adopted by Ironic.
func (p *ironicProvisioner) Adopt(force bool) (result provisioner.Result, err error) {
	var ironicNode *nodes.Node

	if ironicNode, err = p.findExistingHost(); err != nil {
		err = errors.Wrap(err, "could not find host to adpot")
		return
	}
	if ironicNode == nil {
		err = provisioner.NeedsRegistration
		return
	}

	switch nodes.ProvisionState(ironicNode.ProvisionState) {
	case nodes.Enroll, nodes.Verifying:
		err = fmt.Errorf("Invalid state for adopt: %s",
			ironicNode.ProvisionState)
	case nodes.Manageable:
		_, hasImageSource := ironicNode.InstanceInfo["image_source"]
		_, hasBootISO := ironicNode.InstanceInfo["boot_iso"]
		if p.status.State == metal3v1alpha1.StateDeprovisioning &&
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
		result.RequeueAfter = provisionRequeueDelay
		result.Dirty = true
	case nodes.AdoptFail:
		if force {
			return p.changeNodeProvisionState(
				ironicNode,
				nodes.ProvisionStateOpts{
					Target: nodes.TargetAdopt,
				},
			)
		} else {
			result.ErrorMessage = fmt.Sprintf("Host adoption failed: %s",
				ironicNode.LastError)
		}
	case nodes.Active:
	default:
	}
	return
}

// Provision writes the image from the host spec to the host. It may
// be called multiple times, and should return true for its dirty flag
// until the deprovisioning operation is completed.
func (p *ironicProvisioner) Provision(hostConf provisioner.HostConfigData) (result provisioner.Result, err error) {
	var ironicNode *nodes.Node

	if ironicNode, err = p.findExistingHost(); err != nil {
		return result, errors.Wrap(err, "could not find host to receive image")
	}
	if ironicNode == nil {
		return result, provisioner.NeedsRegistration
	}

	p.log.Info("provisioning image to host", "state", ironicNode.ProvisionState)

	checksum, checksumType, _ := p.host.GetImageChecksum()

	// Local variable to make it easier to test if ironic is
	// configured with the same image we are trying to provision to
	// the host.
	ironicHasSameImage := (ironicNode.InstanceInfo["image_source"] == p.host.Spec.Image.URL &&
		ironicNode.InstanceInfo["image_os_hash_algo"] == checksumType &&
		ironicNode.InstanceInfo["image_os_hash_value"] == checksum)
	p.log.Info("checking image settings",
		"source", ironicNode.InstanceInfo["image_source"],
		"image_os_hash_algo", checksumType,
		"image_os_has_value", checksum,
		"same", ironicHasSameImage,
		"provisionState", ironicNode.ProvisionState)

	result.RequeueAfter = provisionRequeueDelay

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
				result.Dirty = true
				return result, nil
			}
			p.log.Info("found error", "msg", ironicNode.LastError)
			result.ErrorMessage = fmt.Sprintf("Image provisioning failed: %s",
				ironicNode.LastError)
			return result, nil
		}
		p.log.Info("recovering from previous failure")
		if provResult, err := p.startProvisioning(ironicNode, hostConf); err != nil || provResult.Dirty || provResult.ErrorMessage != "" {
			return provResult, err
		}

		return p.changeNodeProvisionState(ironicNode,
			nodes.ProvisionStateOpts{Target: nodes.TargetActive})

	case nodes.Manageable:
		return p.changeNodeProvisionState(ironicNode,
			nodes.ProvisionStateOpts{Target: nodes.TargetProvide})

	case nodes.Available:
		if provResult, err := p.startProvisioning(ironicNode, hostConf); err != nil || provResult.Dirty || provResult.ErrorMessage != "" {
			return provResult, err
		}

		// After it is available, we need to start provisioning by
		// setting the state to "active".
		p.log.Info("making host active")

		// Retrieve cloud-init user data
		userData, err := hostConf.UserData()
		if err != nil {
			return result, errors.Wrap(err, "could not retrieve user data")
		}

		// Retrieve cloud-init network_data.json. Default value is empty
		networkDataRaw, err := hostConf.NetworkData()
		if err != nil {
			return result, errors.Wrap(err, "could not retrieve network data")
		}
		var networkData map[string]interface{}
		if err = yaml.Unmarshal([]byte(networkDataRaw), &networkData); err != nil {
			return result, errors.Wrap(err, "failed to unmarshal network_data.json from secret")
		}

		// Retrieve cloud-init meta_data.json with falback to default
		metaData := map[string]interface{}{
			"uuid":             string(p.host.ObjectMeta.UID),
			"metal3-namespace": p.host.ObjectMeta.Namespace,
			"metal3-name":      p.host.ObjectMeta.Name,
			"local-hostname":   p.host.ObjectMeta.Name,
			"local_hostname":   p.host.ObjectMeta.Name,
		}
		metaDataRaw, err := hostConf.MetaData()
		if err != nil {
			return result, errors.Wrap(err, "could not retrieve metadata")
		}
		if metaDataRaw != "" {
			if err = yaml.Unmarshal([]byte(metaDataRaw), &metaData); err != nil {
				return result, errors.Wrap(err, "failed to unmarshal metadata from secret")
			}
		}

		var configDrive nodes.ConfigDrive
		if userData != "" {
			configDrive = nodes.ConfigDrive{
				UserData:    userData,
				MetaData:    metaData,
				NetworkData: networkData,
			}
			if err != nil {
				return result, errors.Wrap(err, "failed to build config drive")
			}
			p.log.Info("triggering provisioning with config drive")
		} else {
			p.log.Info("triggering provisioning without config drive")
		}

		return p.changeNodeProvisionState(
			ironicNode,
			nodes.ProvisionStateOpts{
				Target:      nodes.TargetActive,
				ConfigDrive: configDrive,
			},
		)

	case nodes.Active:
		// provisioning is done
		p.publisher("ProvisioningComplete",
			fmt.Sprintf("Image provisioning completed for %s", p.host.Spec.Image.URL))
		p.log.Info("finished provisioning")
		return result, nil

	default:
		// wait states like cleaning and clean wait
		p.log.Info("waiting for host to become available",
			"state", ironicNode.ProvisionState,
			"deploy step", ironicNode.DeployStep)
		result.Dirty = true
		return result, nil
	}
}

func (p *ironicProvisioner) setMaintenanceFlag(ironicNode *nodes.Node, value bool) (result provisioner.Result, err error) {
	_, err = nodes.Update(
		p.client,
		ironicNode.UUID,
		nodes.UpdateOpts{
			nodes.UpdateOperation{
				Op:    nodes.ReplaceOp,
				Path:  "/maintenance",
				Value: value,
			},
		},
	).Extract()
	switch err.(type) {
	case nil:
	case gophercloud.ErrDefault409:
		p.log.Info("could not set host maintenance flag, busy")
	default:
		return result, errors.Wrap(err, "failed to set host maintenance flag")
	}
	result.Dirty = true
	return result, nil
}

// Deprovision removes the host from the image. It may be called
// multiple times, and should return true for its dirty flag until the
// deprovisioning operation is completed.
func (p *ironicProvisioner) Deprovision() (result provisioner.Result, err error) {
	p.log.Info("deprovisioning")

	ironicNode, err := p.findExistingHost()
	if err != nil {
		return result, errors.Wrap(err, "failed to find existing host")
	}
	if ironicNode == nil {
		return result, provisioner.NeedsRegistration
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
		return p.changeNodeProvisionState(
			ironicNode,
			nodes.ProvisionStateOpts{Target: nodes.TargetManage},
		)

	case nodes.CleanFail:
		p.log.Info("cleaning failed")
		if ironicNode.Maintenance {
			p.log.Info("clearing maintenance flag")
			return p.setMaintenanceFlag(ironicNode, false)
		}
		return p.changeNodeProvisionState(
			ironicNode,
			nodes.ProvisionStateOpts{Target: nodes.TargetManage},
		)

	case nodes.Manageable:
		// We end up here after CleanFail. Because cleaning happens in the
		// process of moving from manageable to available, the node will still
		// get cleaned before we provision it again. Therefore, just declare
		// deprovisioning complete.
		p.log.Info("deprovisioning node is in manageable state")
		return operationComplete()

	case nodes.Available:
		p.publisher("DeprovisioningComplete", "Image deprovisioning completed")
		return result, nil

	case nodes.Deleting:
		p.log.Info("deleting")
		// Transitions to Cleaning upon completion
		result.Dirty = true
		result.RequeueAfter = deprovisionRequeueDelay
		return result, nil

	case nodes.Cleaning:
		p.log.Info("cleaning")
		// Transitions to Available upon completion
		result.Dirty = true
		result.RequeueAfter = deprovisionRequeueDelay
		return result, nil

	case nodes.CleanWait:
		p.log.Info("cleaning")
		result.Dirty = true
		result.RequeueAfter = deprovisionRequeueDelay
		return result, nil

	case nodes.Active:
		p.log.Info("starting deprovisioning")
		p.publisher("DeprovisioningStarted", "Image deprovisioning started")
		return p.changeNodeProvisionState(
			ironicNode,
			nodes.ProvisionStateOpts{Target: nodes.TargetDeleted},
		)

	default:
		return result, fmt.Errorf("Unhandled ironic state %s", ironicNode.ProvisionState)
	}
}

// Delete removes the host from the provisioning system. It may be
// called multiple times, and should return true for its dirty flag
// until the deprovisioning operation is completed.
func (p *ironicProvisioner) Delete() (result provisioner.Result, err error) {

	ironicNode, err := p.findExistingHost()
	if err != nil {
		return result, errors.Wrap(err, "failed to find existing host")
	}
	if ironicNode == nil {
		p.log.Info("no node found, already deleted")
		return result, nil
	}

	p.log.Info("deleting host",
		"ID", ironicNode.UUID,
		"lastError", ironicNode.LastError,
		"current", ironicNode.ProvisionState,
		"target", ironicNode.TargetProvisionState,
		"deploy step", ironicNode.DeployStep,
	)

	if nodes.ProvisionState(ironicNode.ProvisionState) == nodes.Available {
		// Move back to manageable so we can delete it cleanly.
		return p.changeNodeProvisionState(
			ironicNode,
			nodes.ProvisionStateOpts{Target: nodes.TargetManage},
		)
	}

	if !ironicNode.Maintenance {
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
		return p.setMaintenanceFlag(ironicNode, true)
	}

	p.log.Info("host ready to be removed")
	err = nodes.Delete(p.client, ironicNode.UUID).ExtractErr()
	switch err.(type) {
	case nil:
		p.log.Info("removed")
	case gophercloud.ErrDefault409:
		p.log.Info("could not remove host, busy")
	case gophercloud.ErrDefault404:
		p.log.Info("did not find host to delete, OK")
	default:
		return result, errors.Wrap(err, "failed to remove host")
	}

	result.Dirty = true
	return result, nil
}

func (p *ironicProvisioner) changePower(ironicNode *nodes.Node, target nodes.TargetPowerState) (result provisioner.Result, err error) {
	p.log.Info("changing power state")

	if ironicNode.TargetProvisionState != "" {
		p.log.Info("host in state that does not allow power change, try again after delay",
			"state", ironicNode.ProvisionState,
			"target state", ironicNode.TargetProvisionState,
		)
		result.Dirty = true
		result.RequeueAfter = powerRequeueDelay
		return result, nil
	}

	powerStateOpts := nodes.PowerStateOpts{
		Target: target,
	}
	if target == softPowerOff {
		powerStateOpts.Timeout = int(softPowerOffTimeout.Seconds())
	}

	changeResult := nodes.ChangePowerState(
		p.client,
		ironicNode.UUID,
		powerStateOpts)

	switch changeResult.Err.(type) {
	case nil:
		result.Dirty = true
		p.log.Info("power change OK")
	case gophercloud.ErrDefault409:
		p.log.Info("host is locked, trying again after delay", "delay", powerRequeueDelay)
		result.Dirty = true
		result.RequeueAfter = powerRequeueDelay
		return result, HostLockedError{Address: p.host.Spec.BMC.Address}
	case gophercloud.ErrDefault400:
		// Error 400 Bad Request means target power state is not supported by vendor driver
		p.log.Info("power change error", "message", changeResult.Err)
		return result, SoftPowerOffUnsupportedError{Address: p.host.Spec.BMC.Address}
	default:
		p.log.Info("power change error", "message", changeResult.Err)
		return result, errors.Wrap(changeResult.Err, "failed to change power state")
	}

	return result, nil
}

// PowerOn ensures the server is powered on independently of any image
// provisioning operation.
func (p *ironicProvisioner) PowerOn() (result provisioner.Result, err error) {
	p.log.Info("ensuring host is powered on")

	ironicNode, err := p.findExistingHost()
	if err != nil {
		return result, errors.Wrap(err, "failed to find existing host")
	}

	p.log.Info("checking current state",
		"current", p.host.Status.PoweredOn,
		"target", ironicNode.TargetPowerState)

	if ironicNode.PowerState != powerOn {
		if ironicNode.TargetPowerState == powerOn {
			p.log.Info("waiting for power status to change")
			result.RequeueAfter = powerRequeueDelay
			result.Dirty = true
			return result, nil
		}
		result, err = p.changePower(ironicNode, nodes.PowerOn)
		if err != nil {
			return result, errors.Wrap(err, "failed to power on host")
		}
		p.publisher("PowerOn", "Host powered on")
	}

	return result, nil
}

// PowerOff ensures the server is powered off independently of any image
// provisioning operation.
func (p *ironicProvisioner) PowerOff() (result provisioner.Result, err error) {
	p.log.Info("ensuring host is powered off")

	result, err = p.softPowerOff()
	if err != nil {
		switch err.(type) {
		// In case of soft power off is unsupported or has failed,
		// we activate hard power off.
		case SoftPowerOffUnsupportedError, SoftPowerOffFailed:
			return p.hardPowerOff()
		case HostLockedError:
			result.RequeueAfter = powerRequeueDelay
			result.Dirty = true
			return result, nil
		default:
			result.RequeueAfter = powerRequeueDelay
			return result, err
		}
	}
	return result, nil
}

// hardPowerOff sends 'power off' request to BM node and waits for the result
func (p *ironicProvisioner) hardPowerOff() (result provisioner.Result, err error) {
	p.log.Info("ensuring host is powered off by \"hard power off\" command")

	ironicNode, err := p.findExistingHost()
	if err != nil {
		return result, errors.Wrap(err, "failed to find existing host")
	}

	if ironicNode.PowerState != powerOff {
		if ironicNode.TargetPowerState == powerOff {
			p.log.Info("waiting for power status to change")
			result.RequeueAfter = powerRequeueDelay
			result.Dirty = true
			return result, nil
		}
		result, err = p.changePower(ironicNode, nodes.PowerOff)
		if err != nil {
			return result, errors.Wrap(err, "failed to power off host")
		}
		p.publisher("PowerOff", "Host powered off")
	}

	return result, nil
}

// softPowerOff sends 'soft power off' request to BM node.
// If soft power off is not supported, the request ends with an error.
// Otherwise the request ends with no error and the result should be
// checked later via node fields "power_state", "target_power_state"
// and "last_error".
func (p *ironicProvisioner) softPowerOff() (result provisioner.Result, err error) {
	p.log.Info("ensuring host is powered off by \"soft power off\" command")

	ironicNode, err := p.findExistingHost()
	if err != nil {
		return result, errors.Wrap(err, "failed to find existing host")
	}

	if ironicNode.PowerState != powerOff {
		targetState := ironicNode.TargetPowerState
		// If the target state is either powerOff or softPowerOff, then we should wait
		if targetState == powerOff || targetState == softPowerOff {
			p.log.Info("waiting for power status to change")
			result.RequeueAfter = powerRequeueDelay
			result.Dirty = true
			return result, nil
		}
		// If the target state is unset while the last error is set,
		// then the last execution of soft power off has failed.
		if targetState == "" && ironicNode.LastError != "" {
			return result, SoftPowerOffFailed{Address: p.host.Spec.BMC.Address}
		}
		result, err = p.changePower(ironicNode, nodes.SoftPowerOff)
		if err != nil {
			result.RequeueAfter = powerRequeueDelay
			return result, err
		}
		p.publisher("PowerOff", "Host soft powered off")
	}

	return result, nil
}

// IsReady checks if the provisioning backend is available
func (p *ironicProvisioner) IsReady() (result bool, err error) {
	p.log.Info("verifying ironic provisioner dependencies")

	checker := newIronicDependenciesChecker(p.client, p.inspector, p.log)
	return checker.IsReady()
}
