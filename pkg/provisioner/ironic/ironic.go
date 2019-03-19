package ironic

import (
	"fmt"
	"strings"
	"time"

	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack/baremetal/noauth"
	"github.com/gophercloud/gophercloud/openstack/baremetal/v1/nodes"
	"github.com/gophercloud/gophercloud/openstack/baremetal/v1/ports"

	nodeutils "github.com/gophercloud/utils/openstack/baremetal/v1/nodes"

	"github.com/pkg/errors"

	"github.com/go-logr/logr"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"

	metalkubev1alpha1 "github.com/metalkube/baremetal-operator/pkg/apis/metalkube/v1alpha1"
	"github.com/metalkube/baremetal-operator/pkg/bmc"
	"github.com/metalkube/baremetal-operator/pkg/provisioner"
)

var log = logf.Log.WithName("ironic")
var deprovisionRequeueDelay = time.Second * 10
var provisionRequeueDelay = time.Second * 10

const (
	ironicEndpoint            = "http://localhost:6385/v1/"
	stateNone                 = ""
	stateRegistrationError    = "registration error"
	stateRegistering          = "registering"
	stateReady                = "ready"
	statePreparingToProvision = "preparing to provision"
	stateMakingAvailable      = "making host available"
	stateValidationError      = "validation error"
	stateProvisioning         = "provisioning"
	stateProvisioned          = "provisioned"
	stateDeprovisioning       = "deprovisioning"
)

// Provisioner implements the provisioning.Provisioner interface
// and uses Ironic to manage the host.
type ironicProvisioner struct {
	// the host to be managed by this provisioner
	host *metalkubev1alpha1.BareMetalHost
	// a shorter path to the provisioning status data structure
	status *metalkubev1alpha1.ProvisionStatus
	// access parameters for the BMC
	bmcAccess bmc.AccessDetails
	// credentials to log in to the BMC
	bmcCreds bmc.Credentials
	// a client for talking to ironic
	client *gophercloud.ServiceClient
	// a logger configured for this host
	log logr.Logger
}

// New returns a new Ironic Provisioner
func New(host *metalkubev1alpha1.BareMetalHost, bmcCreds bmc.Credentials) (provisioner.Provisioner, error) {
	client, err := noauth.NewBareMetalNoAuth(noauth.EndpointOpts{
		IronicEndpoint: ironicEndpoint,
	})
	if err != nil {
		return nil, err
	}
	bmcAccess, err := bmc.NewAccessDetails(host.Spec.BMC.Address)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse BMC address information")
	}
	// Ensure we have a microversion high enough to get the features
	// we need.
	client.Microversion = "1.50"
	p := &ironicProvisioner{
		host:      host,
		status:    &(host.Status.Provisioning),
		bmcAccess: bmcAccess,
		bmcCreds:  bmcCreds,
		client:    client,
		log:       log.WithValues("host", host.Name),
	}
	return p, nil
}

func (p *ironicProvisioner) validateNode(ironicNode *nodes.Node) (ok bool, err error) {
	var validationErrors []string

	p.log.Info("validating node settings in ironic")
	validateResult, err := nodes.Validate(p.client, ironicNode.UUID).Extract()
	if err != nil {
		return false, errors.Wrap(err, "failed to validate node settings in ironic")
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
		msg := fmt.Sprintf("host validation error: %s",
			strings.Join(validationErrors, "; "))
		ok = p.host.SetErrorMessage(msg)
		return ok, nil
	}
	return true, nil
}

// Look for an existing registration for the host in Ironic.
func (p *ironicProvisioner) findExistingHost() (ironicNode *nodes.Node, err error) {
	// Try to load the node by UUID
	if p.status.ID != "" {
		// Look for the node to see if it exists (maybe Ironic was
		// restarted)
		p.log.Info("looking for existing node by ID", "ID", p.status.ID)
		ironicNode, err = nodes.Get(p.client, p.status.ID).Extract()
		switch err.(type) {
		case nil:
			p.log.Info("found existing node by ID")
			return ironicNode, nil
		case gophercloud.ErrDefault404:
			p.log.Info("did not find existing node by ID")
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
		p.log.Info("did not find existing node by name")
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
func (p *ironicProvisioner) ValidateManagementAccess() (result provisioner.Result, err error) {
	var ironicNode *nodes.Node

	p.log.Info("validating management access")

	ironicNode, err = p.findExistingHost()
	if err != nil {
		return result, errors.Wrap(err, "failed to find existing host")
	}

	// If we have not found a node yet, we need to create one
	if ironicNode == nil {
		p.log.Info("registering host in ironic")

		driverInfo := p.bmcAccess.DriverInfo(p.bmcCreds)
		// FIXME(dhellmann): The names of the images are tied
		// to the version of ironic we are using and are
		// likely to change.
		//
		// FIXME(dhellmann): We need to get our IP on the
		// provisioning network from somewhere.
		driverInfo["deploy_kernel"] = "http://172.22.0.1/images/ironic-python-agent.kernel"
		driverInfo["deploy_ramdisk"] = "http://172.22.0.1/images/ironic-python-agent.initramfs"

		ironicNode, err = nodes.Create(
			p.client,
			nodes.CreateOpts{
				Driver:        "ipmi",
				BootInterface: "ipxe",
				Name:          p.host.Name,
				DriverInfo:    driverInfo,
			}).Extract()
		if err != nil {
			return result, errors.Wrap(err, "failed to register host in ironic")
		}

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
	}

	// Some BMC types require a MAC address, so ensure we have one
	// when we need it. If not, place the host in an error state.
	if p.bmcAccess.NeedsMAC() && p.host.Spec.BootMACAddress == "" {
		msg := fmt.Sprintf("BMC driver %s requires a BootMACAddress value", p.bmcAccess.Type())
		p.log.Info(msg)
		updatedMessage := p.host.SetErrorMessage(msg)
		result.Dirty = result.Dirty || updatedMessage
		return result, nil
	}

	// If we tried to update the node status already and it has an
	// error, store that value and stop trying to manipulate it.
	if ironicNode.LastError != "" {
		// If ironic is reporting an error that probably means it
		// cannot see the BMC or the credentials are wrong. Set the
		// error message and return dirty, if we've changed something,
		// so the status is stored.
		p.status.State = stateRegistrationError
		result.Dirty = p.host.SetErrorMessage(ironicNode.LastError) || result.Dirty
		return result, nil
	}

	// Ensure the node is marked manageable.
	//
	// FIXME(dhellmann): Do we need to check other states here?
	ironicNode, err = nodes.Get(p.client, p.status.ID).Extract()
	if err != nil {
		return result, errors.Wrap(err, "failed to get provisioning state in ironic")
	}
	if ironicNode.ProvisionState == string(nodes.Enroll) {
		p.log.Info("changing provisioning state to manage",
			"current", ironicNode.ProvisionState,
			"target", ironicNode.TargetProvisionState,
		)
		p.status.State = stateRegistering
		changeResult := nodes.ChangeProvisionState(
			p.client,
			ironicNode.UUID,
			nodes.ProvisionStateOpts{
				Target: nodes.TargetManage,
			})
		if changeResult.Err != nil {
			return result, errors.Wrap(changeResult.Err,
				"failed to change provisioning state to manage")
		}
	}

	// We don't expect the state to have changed to manageable
	// immediately if we just updated it in the previous stanza, but
	// if we didn't enter that stanza we want to make sure we have the
	// latest state, so fetch the data again.
	ironicNode, err = nodes.Get(p.client, p.status.ID).Extract()
	if err != nil {
		return result, errors.Wrap(err, "failed to check provision state")
	}
	if p.status.State == stateRegistering {
		if ironicNode.ProvisionState != nodes.Manageable {
			// If we're still waiting for the state to change in Ironic,
			// return true to indicate that we're dirty and need to be
			// reconciled again.
			p.log.Info("waiting for manageable provision state, forcing dirty state",
				"lastError", ironicNode.LastError,
				"current", ironicNode.ProvisionState,
				"target", ironicNode.TargetProvisionState,
			)
			p.status.State = stateRegistering
			result.Dirty = true
		} else {
			// Mark the node as ready to be used
			p.status.State = stateReady
			result.Dirty = true
		}
	}

	if !result.Dirty {
		// If we aren't saving any other updates, clear the error
		// status if we have one.
		result.Dirty = p.host.ClearError()
	}

	return result, nil
}

// InspectHardware updates the HardwareDetails field of the host with
// details of devices discovered on the hardware. It may be called
// multiple times, and should return true for its dirty flag until the
// inspection is completed.
func (p *ironicProvisioner) InspectHardware() (result provisioner.Result, err error) {
	p.log.Info("inspecting hardware", "status", p.host.OperationalStatus())

	_, err = p.findExistingHost()
	if err != nil {
		return result, errors.Wrap(err, "failed to find existing host")
	}
	if p.host.HasError() {
		p.log.Info("host has error, skipping hardware inspection")
		return result, nil
	}

	if p.host.OperationalStatus() != metalkubev1alpha1.OperationalStatusInspecting {
		// The inspection just started.
		p.log.Info("starting inspection by setting state")
		p.host.SetOperationalStatus(metalkubev1alpha1.OperationalStatusInspecting)
		result.Dirty = true
		return result, nil
	}

	// The inspection is ongoing. We'll need to check the ironic
	// status for the server here until it is ready for us to get the
	// inspection details. Simulate that for now by creating the
	// hardware details struct as part of a second pass.
	if p.host.Status.HardwareDetails == nil {
		p.log.Info("continuing inspection by setting details")
		p.host.Status.HardwareDetails =
			&metalkubev1alpha1.HardwareDetails{
				RAMGiB: 128,
				NIC: []metalkubev1alpha1.NIC{
					metalkubev1alpha1.NIC{
						Name:      "nic-1",
						Model:     "virt-io",
						Network:   "Pod Networking",
						MAC:       "some:mac:address",
						IP:        "192.168.100.1",
						SpeedGbps: 1,
					},
					metalkubev1alpha1.NIC{
						Name:      "nic-2",
						Model:     "e1000",
						Network:   "Pod Networking",
						MAC:       "some:other:mac:address",
						IP:        "192.168.100.2",
						SpeedGbps: 1,
					},
				},
				Storage: []metalkubev1alpha1.Storage{
					metalkubev1alpha1.Storage{
						Name:    "disk-1 (boot)",
						Type:    "SSD",
						SizeGiB: 1024 * 93,
						Model:   "Dell CFJ61",
					},
					metalkubev1alpha1.Storage{
						Name:    "disk-2",
						Type:    "SSD",
						SizeGiB: 1024 * 93,
						Model:   "Dell CFJ61",
					},
				},
				CPUs: []metalkubev1alpha1.CPU{
					metalkubev1alpha1.CPU{
						Type:     "x86",
						SpeedGHz: 3,
					},
				},
			}
		result.Dirty = true
		return result, nil
	}

	return result, nil
}

// Provision writes the image from the host spec to the host. It may
// be called multiple times, and should return true for its dirty flag
// until the deprovisioning operation is completed.
func (p *ironicProvisioner) Provision(userData string) (result provisioner.Result, err error) {
	var ironicNode *nodes.Node

	p.log.Info("provisioning image to host")

	// The last time we were here we set the host in an error state.
	if p.status.State == stateValidationError {
		p.log.Info("stopping provisioning due to validation error")
		return result, nil
	}

	if ironicNode, err = p.findExistingHost(); err != nil {
		return result, errors.Wrap(err, "could not find host to receive image")
	}

	// Since we were here ironic has recorded an error for this host.
	if ironicNode.LastError != "" {
		p.log.Info("found error", "msg", ironicNode.LastError)
		p.status.State = stateValidationError
		result.Dirty = p.host.SetErrorMessage(ironicNode.LastError)
		return result, nil
	}

	result.RequeueAfter = provisionRequeueDelay

	// Ensure the instance_info properties for the host are set to
	// tell Ironic where to get the image to be provisioned.
	var op nodes.UpdateOp
	if imageSource, ok := ironicNode.InstanceInfo["image_source"]; !ok {
		// no source, need to add
		op = nodes.AddOp
		p.log.Info("adding host settings in ironic")
	} else if imageSource != p.host.Spec.Image.URL {
		//  have a different source, need to update
		op = nodes.ReplaceOp
		p.log.Info("updating host settings in ironic")
	}
	if op != "" {
		_, err = nodes.Update(
			p.client,
			ironicNode.UUID,
			nodes.UpdateOpts{
				nodes.UpdateOperation{
					Op:    op,
					Path:  "/instance_info/image_source",
					Value: p.host.Spec.Image.URL,
				},
				nodes.UpdateOperation{
					Op:    op,
					Path:  "/instance_info/image_checksum",
					Value: p.host.Spec.Image.Checksum,
				},
				// FIXME(dhellmann): We have to provide something for
				// the disk size until
				// https://storyboard.openstack.org/#!/story/2005165
				// is fixed in ironic.
				nodes.UpdateOperation{
					Op:    op,
					Path:  "/instance_info/root_gb",
					Value: 10,
				},
				// FIXME(dhellmann): We need to specify the root
				// device to receive the image. That should come from
				// some combination of inspecting the host to see what
				// is available and the hardware profile to give us
				// instructions.
				// nodes.UpdateOperation{
				// 	Op:    nodes.AddOp,
				// 	Path:  "/properties/root_device",
				// 	Value: map[string]interface{},
				// },
			}).Extract()
		if err != nil {
			return result, errors.Wrap(err, "failed to update host settings in ironic")
		}
		p.status.State = statePreparingToProvision
		result.Dirty = true
		return result, nil
	}

	// Ironic has the settings it needs, see if it finds any issues
	// with them.
	if p.status.State == statePreparingToProvision {
		ok, err := p.validateNode(ironicNode)
		if err != nil {
			return result, errors.Wrap(err, "could not validate host during registration")
		}
		if !ok {
			p.status.State = stateValidationError
			result.Dirty = true // validateNode() would have set the errors
			return result, nil
		}

		// If validation is successful we can start moving the host
		// through the states necessary to make it "available".
		p.log.Info("making host available",
			"lastError", ironicNode.LastError,
			"current", ironicNode.ProvisionState,
			"target", ironicNode.TargetProvisionState,
			"deploy step", ironicNode.DeployStep,
		)
		changeResult := nodes.ChangeProvisionState(
			p.client,
			ironicNode.UUID,
			nodes.ProvisionStateOpts{
				Target: nodes.TargetProvide,
			})
		if changeResult.Err != nil {
			return result, errors.Wrap(changeResult.Err,
				"failed to change provisioning state to provide")
		}
		p.status.State = stateMakingAvailable
		result.Dirty = true
		return result, nil
	}

	// Wait for the host to become available
	if p.status.State == stateMakingAvailable {
		if ironicNode.ProvisionState != nodes.Available {
			p.log.Info("waiting for host to become available",
				"deploy step", ironicNode.DeployStep)
			return result, nil
		}

		// After it is available, we need to start provisioning by
		// setting the state to "active".

		// Build the config drive image using the userData we've been
		// given so we can pass it to Ironic.
		//
		// FIXME(dhellmann): The Stein version of Ironic should be
		// able to accept the user data string directly, without
		// building the ISO image first.
		var configDriveData string
		if userData != "" {
			configDrive := nodeutils.ConfigDrive{
				UserData: nodeutils.UserDataString(userData),
			}
			configDriveData, err = configDrive.ToConfigDrive()
			if err != nil {
				return result, errors.Wrap(err, "failed to build config drive")
			}
			p.log.Info("triggering provisioning with config drive")
		} else {
			p.log.Info("triggering provisioning without config drive")
		}

		changeResult := nodes.ChangeProvisionState(
			p.client,
			ironicNode.UUID,
			nodes.ProvisionStateOpts{
				Target:      nodes.TargetActive,
				ConfigDrive: configDriveData,
			},
		)
		if changeResult.Err != nil {
			return result, errors.Wrap(changeResult.Err,
				"failed to trigger provisioning")
		}
		p.status.State = stateProvisioning
		result.Dirty = true
		return result, nil
	}

	// Wait for provisioning to be completed
	if p.status.State == stateProvisioning {
		if ironicNode.ProvisionState == nodes.Active {
			p.log.Info("finished provisioning")
			p.status.Image = *p.host.Spec.Image
			p.status.State = stateProvisioned
			result.Dirty = true
			return result, nil
		}
		p.log.Info("still provisioning",
			"deploy step", ironicNode.DeployStep,
			"ironic state", ironicNode.ProvisionState,
		)
		result.Dirty = true // make sure we check back
	}

	return result, nil
}

// Deprovision prepares the host to be removed from the cluster. It
// may be called multiple times, and should return true for its dirty
// flag until the deprovisioning operation is completed.
func (p *ironicProvisioner) Deprovision(deleteIt bool) (result provisioner.Result, err error) {
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

	if ironicNode.ProvisionState == nodes.Error {
		if !ironicNode.Maintenance {
			p.log.Info("setting host maintenance flag for deleting")
			_, err = nodes.Update(
				p.client,
				ironicNode.UUID,
				nodes.UpdateOpts{
					nodes.UpdateOperation{
						Op:    nodes.ReplaceOp,
						Path:  "/maintenance",
						Value: true,
					},
				},
			).Extract()
			if err != nil {
				return result, errors.Wrap(err, "failed to set host maintenance flag")
			}
		}
		// Fake the status change to Available so that the next block
		// will delete the node now that we have placed it into
		// maintenance mode.
		ironicNode.ProvisionState = nodes.Available
	}

	if ironicNode.ProvisionState == nodes.Available {
		if !deleteIt {
			p.log.Info("deprovisioning complete")
			if p.status.Image.URL != "" {
				p.log.Info("clearing provisioning status")
				p.status.Image.URL = ""
				p.status.Image.Checksum = ""
				p.status.State = stateNone
				result.Dirty = true
			}
			return result, nil
		}
		p.log.Info("host ready to be removed")
		err = nodes.Delete(p.client, p.status.ID).ExtractErr()
		switch err.(type) {
		case nil:
			p.log.Info("removed")
		case gophercloud.ErrDefault404:
			p.log.Info("did not find host to delete, OK")
		default:
			return result, errors.Wrap(err, "failed to remove host")
		}
		return result, nil
	}

	// Most of the next steps are going to take time, so set the delay
	// now once.
	result.RequeueAfter = deprovisionRequeueDelay

	if ironicNode.ProvisionState == nodes.Deleting {
		p.log.Info("still deprovisioning")
		result.Dirty = true
		return result, nil
	}

	if ironicNode.ProvisionState == nodes.Active {
		p.log.Info("starting delete")
		changeResult := nodes.ChangeProvisionState(
			p.client,
			ironicNode.UUID,
			nodes.ProvisionStateOpts{
				Target: nodes.TargetDeleted,
			},
		)
		if changeResult.Err != nil {
			return result, errors.Wrap(changeResult.Err,
				"failed to trigger deprovisioning")
		}
		p.status.State = stateDeprovisioning
		result.Dirty = true
		return result, nil
	}

	p.log.Info("unhandled provision state", "state", ironicNode.ProvisionState)

	return result, nil
}

// PowerOn ensures the server is powered on independently of any image
// provisioning operation.
func (p *ironicProvisioner) PowerOn() (result provisioner.Result, err error) {
	p.log.Info("ensuring host is powered on")

	_, err = p.findExistingHost()
	if err != nil {
		return result, errors.Wrap(err, "failed to find existing host")
	}

	if !p.host.Status.PoweredOn {
		p.host.Status.PoweredOn = true
		result.Dirty = true
	}

	return result, nil
}

// PowerOff ensures the server is powered off independently of any image
// provisioning operation.
func (p *ironicProvisioner) PowerOff() (result provisioner.Result, err error) {
	p.log.Info("ensuring host is powered off")

	_, err = p.findExistingHost()
	if err != nil {
		return result, errors.Wrap(err, "failed to find existing host")
	}

	if p.host.Status.PoweredOn {
		p.host.Status.PoweredOn = false
		result.Dirty = true
	}

	return result, nil
}
