package ironic

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"sigs.k8s.io/yaml"

	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack/baremetal/noauth"
	"github.com/gophercloud/gophercloud/openstack/baremetal/v1/nodes"
	"github.com/gophercloud/gophercloud/openstack/baremetal/v1/ports"
	noauthintrospection "github.com/gophercloud/gophercloud/openstack/baremetalintrospection/noauth"
	"github.com/gophercloud/gophercloud/openstack/baremetalintrospection/v1/introspection"

	"github.com/pkg/errors"

	"github.com/go-logr/logr"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"

	metal3v1alpha1 "github.com/metal3-io/baremetal-operator/pkg/apis/metal3/v1alpha1"
	"github.com/metal3-io/baremetal-operator/pkg/bmc"
	"github.com/metal3-io/baremetal-operator/pkg/hardware"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner"
)

var log = logf.Log.WithName("baremetalhost_ironic")
var deprovisionRequeueDelay = time.Second * 10
var provisionRequeueDelay = time.Second * 10
var powerRequeueDelay = time.Second * 10
var introspectionRequeueDelay = time.Second * 15
var deployKernelURL string
var deployRamdiskURL string
var ironicEndpoint string
var inspectorEndpoint string

const (
	// See nodes.Node.PowerState for details
	powerOn   = "power on"
	powerOff  = "power off"
	powerNone = "None"
)

func init() {
	// NOTE(dhellmann): Use Fprintf() to report errors instead of
	// logging, because logging is not configured yet in init().
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
		"inspectorEndpoint", inspectorEndpoint,
		"deployKernelURL", deployKernelURL,
		"deployRamdiskURL", deployRamdiskURL,
	)
}

// A private function to construct an ironicProvisioner (rather than a
// Provisioner interface) in a consistent way for tests.
func newProvisioner(host *metal3v1alpha1.BareMetalHost, bmcCreds bmc.Credentials, publisher provisioner.EventPublisher) (*ironicProvisioner, error) {
	client, err := noauth.NewBareMetalNoAuth(noauth.EndpointOpts{
		IronicEndpoint: ironicEndpoint,
	})
	if err != nil {
		return nil, err
	}
	bmcAccess, err := bmc.NewAccessDetails(host.Spec.BMC.Address, host.Spec.BMC.DisableCertificateVerification)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse BMC address information")
	}
	inspector, err := noauthintrospection.NewBareMetalIntrospectionNoAuth(
		noauthintrospection.EndpointOpts{
			IronicInspectorEndpoint: inspectorEndpoint,
		})
	if err != nil {
		return nil, err
	}
	// Ensure we have a microversion high enough to get the features
	// we need.
	client.Microversion = "1.56"
	p := &ironicProvisioner{
		host:      host,
		status:    &(host.Status.Provisioning),
		bmcAccess: bmcAccess,
		bmcCreds:  bmcCreds,
		client:    client,
		inspector: inspector,
		log:       log.WithValues("host", host.Name),
		publisher: publisher,
	}
	return p, nil
}

// New returns a new Ironic Provisioner
func New(host *metal3v1alpha1.BareMetalHost, bmcCreds bmc.Credentials, publisher provisioner.EventPublisher) (provisioner.Provisioner, error) {
	return newProvisioner(host, bmcCreds, publisher)
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
		return nil, nil
	default:
		return nil, errors.Wrap(err,
			fmt.Sprintf("failed to find node by name %s", p.host.Name))
	}
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
		result.Dirty = true
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

		checksum, checksumType, ok := p.host.GetImageChecksum()

		if ok {
			p.log.Info("setting instance info",
				"image_source", p.host.Spec.Image.URL,
				"image_os_hash_value", checksum,
				"image_os_hash_algo", checksumType,
			)

			updates := nodes.UpdateOpts{
				nodes.UpdateOperation{
					Op:    nodes.AddOp,
					Path:  "/instance_info/image_source",
					Value: p.host.Spec.Image.URL,
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
		// If we're still waiting for the state to change in Ironic,
		// return true to indicate that we're dirty and need to be
		// reconciled again.
		result.Dirty = true
		return result, nil
	}
}

func (p *ironicProvisioner) changeNodeProvisionState(ironicNode *nodes.Node, opts nodes.ProvisionStateOpts) (result provisioner.Result, err error) {
	p.log.Info("changing provisioning state",
		"current", ironicNode.ProvisionState,
		"existing target", ironicNode.TargetProvisionState,
		"new target", opts.Target,
	)

	changeResult := nodes.ChangeProvisionState(p.client, ironicNode.UUID, opts)
	switch changeResult.Err.(type) {
	case nil:
	case gophercloud.ErrDefault409:
		p.log.Info("could not change state of host, busy")
	default:
		return result, errors.Wrap(changeResult.Err,
			fmt.Sprintf("failed to change provisioning state to %q", opts.Target))
	}

	result.Dirty = true
	result.RequeueAfter = provisionRequeueDelay
	return result, nil
}

func getVLANs(intf introspection.BaseInterfaceType) (vlans []metal3v1alpha1.VLAN, vlanid metal3v1alpha1.VLANID) {
	if intf.LLDPProcessed == nil {
		return
	}
	if spvs, ok := intf.LLDPProcessed["switch_port_vlans"]; ok {
		if data, ok := spvs.([]map[string]interface{}); ok {
			vlans = make([]metal3v1alpha1.VLAN, len(data))
			for i, vlan := range data {
				vid, _ := vlan["id"].(int)
				name, _ := vlan["name"].(string)
				vlans[i] = metal3v1alpha1.VLAN{
					ID:   metal3v1alpha1.VLANID(vid),
					Name: name,
				}
			}
		}
	}
	if vid, ok := intf.LLDPProcessed["switch_port_untagged_vlan_id"].(int); ok {
		vlanid = metal3v1alpha1.VLANID(vid)
	}
	return
}

func getNICSpeedGbps(intfExtradata introspection.ExtraHardwareData) (speedGbps int) {
	if speed, ok := intfExtradata["speed"].(string); ok {
		if strings.HasSuffix(speed, "Gbps") {
			fmt.Sscanf(speed, "%d", &speedGbps)
		}
	}
	return
}

func getNICDetails(ifdata []introspection.InterfaceType,
	basedata map[string]introspection.BaseInterfaceType,
	extradata introspection.ExtraHardwareDataSection) []metal3v1alpha1.NIC {
	nics := make([]metal3v1alpha1.NIC, len(ifdata))
	for i, intf := range ifdata {
		baseIntf := basedata[intf.Name]
		vlans, vlanid := getVLANs(baseIntf)
		ip := intf.IPV4Address
		if ip == "" {
			ip = intf.IPV6Address
		}
		nics[i] = metal3v1alpha1.NIC{
			Name: intf.Name,
			Model: strings.TrimLeft(fmt.Sprintf("%s %s",
				intf.Vendor, intf.Product), " "),
			MAC:       intf.MACAddress,
			IP:        ip,
			VLANs:     vlans,
			VLANID:    vlanid,
			SpeedGbps: getNICSpeedGbps(extradata[intf.Name]),
			PXE:       baseIntf.PXE,
		}
	}
	return nics
}

func getStorageDetails(diskdata []introspection.RootDiskType) []metal3v1alpha1.Storage {
	storage := make([]metal3v1alpha1.Storage, len(diskdata))
	for i, disk := range diskdata {
		storage[i] = metal3v1alpha1.Storage{
			Name:               disk.Name,
			Rotational:         disk.Rotational,
			SizeBytes:          metal3v1alpha1.Capacity(disk.Size),
			Vendor:             disk.Vendor,
			Model:              disk.Model,
			SerialNumber:       disk.Serial,
			WWN:                disk.Wwn,
			WWNVendorExtension: disk.WwnVendorExtension,
			WWNWithExtension:   disk.WwnWithExtension,
			HCTL:               disk.Hctl,
		}
	}
	return storage
}

func getSystemVendorDetails(vendor introspection.SystemVendorType) metal3v1alpha1.HardwareSystemVendor {
	return metal3v1alpha1.HardwareSystemVendor{
		Manufacturer: vendor.Manufacturer,
		ProductName:  vendor.ProductName,
		SerialNumber: vendor.SerialNumber,
	}
}

func getCPUDetails(cpudata *introspection.CPUType) metal3v1alpha1.CPU {
	var freq float64
	fmt.Sscanf(cpudata.Frequency, "%f", &freq)
	sort.Strings(cpudata.Flags)
	cpu := metal3v1alpha1.CPU{
		Arch:           cpudata.Architecture,
		Model:          cpudata.ModelName,
		ClockMegahertz: metal3v1alpha1.ClockSpeed(freq) * metal3v1alpha1.MegaHertz,
		Count:          cpudata.Count,
		Flags:          cpudata.Flags,
	}

	return cpu
}

func getFirmwareDetails(firmwaredata introspection.ExtraHardwareDataSection) metal3v1alpha1.Firmware {

	// handle bios optionally
	var bios metal3v1alpha1.BIOS

	if biosdata, ok := firmwaredata["bios"]; ok {
		// we do not know if all fields will be supplied
		// as this is not a structured response
		// so we must handle each field conditionally
		bios.Vendor, _ = biosdata["vendor"].(string)
		bios.Version, _ = biosdata["version"].(string)
		bios.Date, _ = biosdata["date"].(string)
	}

	return metal3v1alpha1.Firmware{
		BIOS: bios,
	}

}

func getHardwareDetails(data *introspection.Data) *metal3v1alpha1.HardwareDetails {
	details := new(metal3v1alpha1.HardwareDetails)
	details.Firmware = getFirmwareDetails(data.Extra.Firmware)
	details.SystemVendor = getSystemVendorDetails(data.Inventory.SystemVendor)
	details.RAMMebibytes = data.MemoryMB
	details.NIC = getNICDetails(data.Inventory.Interfaces, data.AllInterfaces, data.Extra.Network)
	details.Storage = getStorageDetails(data.Inventory.Disks)
	details.CPU = getCPUDetails(&data.Inventory.CPU)
	details.Hostname = data.Inventory.Hostname
	return details
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
		return result, nil, fmt.Errorf("no ironic node for host")
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
				p.log.Info("starting new hardware inspection")
				result, err = p.changeNodeProvisionState(
					ironicNode,
					nodes.ProvisionStateOpts{Target: nodes.TargetInspect},
				)
				if err == nil {
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

	details = getHardwareDetails(data)
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
		return result, fmt.Errorf("no ironic node for host")
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
	hints := map[string]string{}
	switch {
	case hwProf.RootDeviceHints.DeviceName != "":
		hints["name"] = hwProf.RootDeviceHints.DeviceName
	case hwProf.RootDeviceHints.HCTL != "":
		hints["hctl"] = hwProf.RootDeviceHints.HCTL
	}
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

	return updates, nil
}

func (p *ironicProvisioner) startProvisioning(ironicNode *nodes.Node, hostConf provisioner.HostConfigData) (result provisioner.Result, err error) {

	p.log.Info("starting provisioning")

	updates, err := p.getUpdateOptsForNode(ironicNode)
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

	var opts nodes.ProvisionStateOpts
	if nodes.ProvisionState(ironicNode.ProvisionState) == nodes.DeployFail {
		opts = nodes.ProvisionStateOpts{Target: nodes.TargetActive}
	} else {
		opts = nodes.ProvisionStateOpts{Target: nodes.TargetProvide}
	}
	return p.changeNodeProvisionState(ironicNode, opts)
}

// Adopt allows an externally-provisioned server to be adopted by Ironic.
func (p *ironicProvisioner) Adopt() (result provisioner.Result, err error) {
	var ironicNode *nodes.Node

	if ironicNode, err = p.findExistingHost(); err != nil {
		err = errors.Wrap(err, "could not find host to adpot")
		return
	}
	if ironicNode == nil {
		// The node does not exist, but we were called so the
		// controller thinks that the node existed at one time. That
		// likely means data loss from restarting the database, so
		// pass through the validation process to register the node
		// again. Pass true to indicate that we need to re-test the
		// credentials, just in case.
		p.log.Info("re-registering host")
		return p.ValidateManagementAccess(true)
	}

	switch nodes.ProvisionState(ironicNode.ProvisionState) {
	case nodes.Enroll:
		err = fmt.Errorf("Invalid state for adopt: %s",
			ironicNode.ProvisionState)
	case nodes.Manageable:
		return p.changeNodeProvisionState(
			ironicNode,
			nodes.ProvisionStateOpts{
				Target: nodes.TargetAdopt,
			},
		)
	case nodes.Adopting, nodes.Verifying:
		result.RequeueAfter = provisionRequeueDelay
		result.Dirty = true
	case nodes.AdoptFail:
		result.ErrorMessage = fmt.Sprintf("Host adoption failed: %s",
			ironicNode.LastError)
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
		return result, fmt.Errorf("no ironic node for host")
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
		return p.startProvisioning(ironicNode, hostConf)

	case nodes.Manageable:
		return p.startProvisioning(ironicNode, hostConf)

	case nodes.Available:
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
		p.log.Info("no node found, already deleted")
		return result, nil
	}

	p.log.Info("deprovisioning host",
		"ID", ironicNode.UUID,
		"lastError", ironicNode.LastError,
		"current", ironicNode.ProvisionState,
		"target", ironicNode.TargetProvisionState,
		"deploy step", ironicNode.DeployStep,
	)

	switch nodes.ProvisionState(ironicNode.ProvisionState) {

	case nodes.Error, nodes.CleanFail:
		if !ironicNode.Maintenance {
			p.log.Info("setting host maintenance flag to force image delete")
			return p.setMaintenanceFlag(ironicNode, true)
		}
		// Once it's in maintenance, we can start the delete process.
		return p.changeNodeProvisionState(
			ironicNode,
			nodes.ProvisionStateOpts{Target: nodes.TargetDeleted},
		)

	case nodes.Available:
		// Move back to manageable
		return p.changeNodeProvisionState(
			ironicNode,
			nodes.ProvisionStateOpts{Target: nodes.TargetManage},
		)

	case nodes.Inspecting:
		p.log.Info("waiting for inspection to complete")
		result.Dirty = true
		result.RequeueAfter = introspectionRequeueDelay
		return result, nil

	case nodes.InspectWait:
		p.log.Info("cancelling inspection")
		return p.changeNodeProvisionState(
			ironicNode,
			nodes.ProvisionStateOpts{Target: nodes.TargetAbort},
		)

	case nodes.InspectFail:
		p.log.Info("inspection failed or cancelled")
		return p.changeNodeProvisionState(
			ironicNode,
			nodes.ProvisionStateOpts{Target: nodes.TargetManage},
		)

	case nodes.Deleting:
		p.log.Info("deleting")
		result.Dirty = true
		result.RequeueAfter = deprovisionRequeueDelay
		return result, nil

	case nodes.Cleaning:
		p.log.Info("cleaning")
		result.Dirty = true
		result.RequeueAfter = deprovisionRequeueDelay
		return result, nil

	case nodes.CleanWait:
		p.log.Info("cleaning")
		result.Dirty = true
		result.RequeueAfter = deprovisionRequeueDelay
		return result, nil

	case nodes.Manageable, nodes.Enroll, nodes.Verifying:
		p.publisher("DeprovisioningComplete", "Image deprovisioning completed")
		return result, nil

	default:
		p.log.Info("starting deprovisioning")
		p.publisher("DeprovisioningStarted", "Image deprovisioning started")
		return p.changeNodeProvisionState(
			ironicNode,
			nodes.ProvisionStateOpts{Target: nodes.TargetDeleted},
		)
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
	err = nodes.Delete(p.client, p.status.ID).ExtractErr()
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

	changeResult := nodes.ChangePowerState(
		p.client,
		ironicNode.UUID,
		nodes.PowerStateOpts{
			Target: target,
		})

	switch changeResult.Err.(type) {
	case nil:
		result.Dirty = true
		p.log.Info("power change OK")
	case gophercloud.ErrDefault409:
		p.log.Info("host is locked, trying again after delay", "delay", powerRequeueDelay)
		result.Dirty = true
		result.RequeueAfter = powerRequeueDelay
		return result, nil
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
			result.RequeueAfter = powerRequeueDelay
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
			result.RequeueAfter = powerRequeueDelay
			return result, errors.Wrap(err, "failed to power off host")
		}
		p.publisher("PowerOff", "Host powered off")
	}

	return result, nil
}
