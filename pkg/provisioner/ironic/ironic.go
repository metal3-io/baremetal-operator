package ironic

import (
	"fmt"
	"strings"
	"time"

	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack/baremetal/noauth"
	"github.com/gophercloud/gophercloud/openstack/baremetal/v1/nodes"
	"github.com/gophercloud/gophercloud/openstack/baremetal/v1/ports"

	"github.com/pkg/errors"

	"github.com/go-logr/logr"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"

	metalkubev1alpha1 "github.com/metalkube/baremetal-operator/pkg/apis/metalkube/v1alpha1"
	"github.com/metalkube/baremetal-operator/pkg/bmc"
	"github.com/metalkube/baremetal-operator/pkg/provisioner"
)

var log = logf.Log.WithName("ironic")
var deprovisionRequeueDelay = time.Second * 10

const (
	ironicEndpoint = "http://localhost:6385/v1/"
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
		driverInfo["deploy_kernel"] = "http://172.22.0.1/images/tinyipa-stable-rocky.vmlinuz"
		driverInfo["deploy_ramdisk"] = "http://172.22.0.1/images/tinyipa-stable-rocky.gz"

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
	if ironicNode.ProvisionState != nodes.Manageable {
		// If we're still waiting for the state to change in Ironic,
		// return true to indicate that we're dirty and need to be
		// reconciled again.
		p.log.Info("waiting for manageable provision state, forcing dirty state",
			"lastError", ironicNode.LastError,
			"current", ironicNode.ProvisionState,
			"target", ironicNode.TargetProvisionState,
		)
		result.Dirty = true
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
func (p *ironicProvisioner) Provision() (result provisioner.Result, err error) {
	return result, nil
}
// Deprovision prepares the host to be removed from the cluster. It
// may be called multiple times, and should return true for its dirty
// flag until the deprovisioning operation is completed.
func (p *ironicProvisioner) Deprovision() (result provisioner.Result, err error) {
	p.log.Info("deprovisioning")

	// FIXME(dhellmann): Depending on the node state, it might take some
	// transitioning to move it to a state where it can be deleted. This
	// is especially true if we enable cleaning.

	if p.status.ID != "" {
		p.log.Info("removing host", "ID", p.status.ID)
		err = nodes.Delete(p.client, p.status.ID).ExtractErr()
		switch err.(type) {
		case nil:
			p.log.Info("removed")
		case gophercloud.ErrDefault404:
			p.log.Info("did not find host to delete, OK")
		default:
			return result, errors.Wrap(err, "failed to remove host")
		}
	}

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
