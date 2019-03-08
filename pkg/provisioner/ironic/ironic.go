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

// Ironic-specific provisioner factory
type provisionerFactory struct {
	// Where is ironic?
	ironicEndpoint string
	// The image to deploy on new hosts
	instanceImageSource string
	// The checksum for the instanceImageSource
	instanceImageChecksum string
}

// New returns a new Ironic ProvisionerFactory
func NewFactory(ironicEndpoint, instanceImageSource, instanceImageChecksum string) provisioner.ProvisionerFactory {
	return &provisionerFactory{
		ironicEndpoint:        ironicEndpoint,
		instanceImageSource:   instanceImageSource,
		instanceImageChecksum: instanceImageChecksum,
	}
}

// Provisioner implements the provisioning.Provisioner interface
// and uses Ironic to manage the host.
type ironicProvisioner struct {
	// the host to be managed by this provisioner
	host *metalkubev1alpha1.BareMetalHost
	// a shorter path to the provisioning status data structure
	status *metalkubev1alpha1.ProvisionStatus
	// access parameters for the BMC
	bmcAccess *bmc.AccessDetails
	// credentials to log in to the BMC
	bmcCreds bmc.Credentials
	// a client for talking to ironic
	client *gophercloud.ServiceClient
	// a logger configured for this host
	log logr.Logger
	// The image to deploy on new hosts
	instanceImageSource string
	// The checksum for the instanceImageSource
	instanceImageChecksum string
}

// New returns a new Ironic Provisioner
func (f *provisionerFactory) New(host *metalkubev1alpha1.BareMetalHost, bmcCreds bmc.Credentials) (provisioner.Provisioner, error) {
	client, err := noauth.NewBareMetalNoAuth(noauth.EndpointOpts{
		IronicEndpoint: f.ironicEndpoint,
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
		host:                  host,
		status:                &(host.Status.Provisioning),
		bmcAccess:             bmcAccess,
		bmcCreds:              bmcCreds,
		client:                client,
		log:                   log.WithValues("host", host.Name),
		instanceImageSource:   f.instanceImageSource,
		instanceImageChecksum: f.instanceImageChecksum,
	}
	return p, nil
}

// Register the host with Ironic.
func (p *ironicProvisioner) ensureExists() (dirty bool, err error) {
	var ironicNode *nodes.Node

	// Try to load the node by UUID
	if p.status.ID != "" {
		// Look for the node to see if it exists (maybe Ironic was
		// restarted)
		p.log.Info("looking for existing node by ID", "ID", p.status.ID)
		ironicNode, err = nodes.Get(p.client, p.status.ID).Extract()
		switch err.(type) {
		case nil:
			p.log.Info("found existing node by ID")
		case gophercloud.ErrDefault404:
			p.log.Info("did not find existing node by ID")
		default:
			return false, errors.Wrap(err,
				fmt.Sprintf("failed to find node by ID %s", p.status.ID))
		}
	}

	// Try to load the node by name
	if ironicNode == nil {
		p.log.Info("looking for existing node by name", "name", p.host.Name)
		ironicNode, err = nodes.Get(p.client, p.host.Name).Extract()
		switch err.(type) {
		case nil:
			p.log.Info("found existing node by name")
			// Store the ID so other methods can assume it is set and
			// so we can find the node using that value next time.
			p.status.ID = ironicNode.UUID
			dirty = true
			p.log.Info("setting provisioning id", "ID", p.status.ID)
		case gophercloud.ErrDefault404:
			p.log.Info("did not find existing node by name")
			ironicNode = nil
		default:
			return false, errors.Wrap(err,
				fmt.Sprintf("failed to find node by name %s", p.host.Name))
		}
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
			return false, errors.Wrap(err, "failed to register host in ironic")
		}

		// Store the ID so other methods can assume it is set and so
		// we can find the node again later.
		p.status.ID = ironicNode.UUID
		dirty = true
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
				return false, errors.Wrap(err, "failed to create port in ironic")
			}
		}

		// FIXME(dhellmann): We should test whether the incoming
		// values match the values on the node in ironic before
		// updating them. For now, using "dirty" flag as an indicator
		// that we created the node so we need to set these values.
		p.log.Info("updating host settings in ironic")
		_, err = nodes.Update(
			p.client,
			ironicNode.UUID,
			nodes.UpdateOpts{
				nodes.UpdateOperation{
					Op:    nodes.AddOp,
					Path:  "/instance_info/image_source",
					Value: p.instanceImageSource,
				},
				nodes.UpdateOperation{
					Op:    nodes.AddOp,
					Path:  "/instance_info/image_checksum",
					Value: p.instanceImageChecksum,
				},
				// FIXME(dhellmann): We have to provide something for
				// the disk size until
				// https://storyboard.openstack.org/#!/story/2005165
				// is fixed in ironic.
				nodes.UpdateOperation{
					Op:    nodes.AddOp,
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
			return false, errors.Wrap(err, "failed to update host settings in ironic")
		}

		// Validate the host settings
		p.log.Info("validating node settings in ironic")
		validateResult, err := nodes.Validate(p.client, ironicNode.UUID).Extract()
		if err != nil {
			return false, errors.Wrap(err, "failed to validate node settings in ironic")
		}
		var validationErrors []string
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
			dirty = p.host.SetErrorMessage(msg) || dirty
			return dirty, nil
		}
	} else {
		// FIXME(dhellmann): At this point we have found an existing
		// node in ironic by looking it up. We need to check its
		// settings against what we have in the host, and change them
		// if there are differences.
	}

	// Some BMC types require a MAC address, so ensure we have one
	// when we need it. If not, place the host in an error state.
	if p.bmcAccess.NeedsMAC() && p.host.Spec.BootMACAddress == "" {
		msg := fmt.Sprintf("BMC driver %s requires a BootMACAddress value", p.bmcAccess.Type)
		p.log.Info(msg)
		updatedMessage := p.host.SetErrorMessage(msg)
		return dirty || updatedMessage, nil
	}

	// If we tried to update the node status already and it has an
	// error, store that value and stop trying to manipulate it.
	if ironicNode.LastError != "" {
		// If ironic is reporting an error that probably means it
		// cannot see the BMC or the credentials are wrong. Set the
		// error message and return dirty, if we've changed something,
		// so the status is stored.
		dirty = p.host.SetErrorMessage(ironicNode.LastError) || dirty
		return dirty, nil
	}

	// Ensure the node is marked manageable.
	//
	// FIXME(dhellmann): Do we need to check other states here?
	ironicNode, err = nodes.Get(p.client, p.status.ID).Extract()
	if err != nil {
		return false, errors.Wrap(err, "failed to get provisioning state in ironic")
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
			return false, errors.Wrap(changeResult.Err,
				"failed to change provisioning state to manage")
		}
	}

	// We don't expect the state to have changed to manageable
	// immediately if we just updated it in the previous stanza, but
	// if we didn't enter that stanza we want to make sure we have the
	// latest state, so fetch the data again.
	ironicNode, err = nodes.Get(p.client, p.status.ID).Extract()
	if err != nil {
		return false, errors.Wrap(err, "failed to check provision state")
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
		dirty = true
	}

	return dirty, nil
}

// ValidateManagementAccess tests the connection information for the
// host to verify that the location and credentials work.
func (p *ironicProvisioner) ValidateManagementAccess() (dirty bool, err error) {
	p.log.Info("validating management access")
	if dirty, err = p.ensureExists(); err != nil {
		return dirty, errors.Wrap(err, "could not validate management access")
	}
	return dirty, nil
}

// InspectHardware updates the HardwareDetails field of the host with
// details of devices discovered on the hardware. It may be called
// multiple times, and should return true for its dirty flag until the
// inspection is completed.
func (p *ironicProvisioner) InspectHardware() (dirty bool, err error) {
	p.log.Info("inspecting hardware", "status", p.host.OperationalStatus())

	if dirty, err = p.ensureExists(); err != nil {
		return dirty, errors.Wrap(err, "could not inspect hardware")
	}
	if p.host.HasError() {
		p.log.Info("host has error, skipping hardware inspection")
		return dirty, nil
	}

	if p.host.OperationalStatus() != metalkubev1alpha1.OperationalStatusInspecting {
		// The inspection just started.
		p.log.Info("starting inspection by setting state")
		p.host.SetOperationalStatus(metalkubev1alpha1.OperationalStatusInspecting)
		return true, nil
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
		return true, nil
	}

	return false, nil
}

// Deprovision prepares the host to be removed from the cluster. It
// may be called multiple times, and should return true for its dirty
// flag until the deprovisioning operation is completed.
func (p *ironicProvisioner) Deprovision() (dirty bool, retryDelay time.Duration, err error) {
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
			return false, 0, errors.Wrap(err, "failed to remove host")
		}
	}

	return false, 0, nil
}

// PowerOn ensures the server is powered on independently of any image
// provisioning operation.
func (p *ironicProvisioner) PowerOn() (dirty bool, err error) {
	p.log.Info("ensuring host is powered on")

	if dirty, err = p.ensureExists(); err != nil {
		return dirty, errors.Wrap(err, "could not power on host")
	}

	if p.host.Status.Provisioning.State != "powered on" {
		p.host.Status.Provisioning.State = "powered on"
		return true, nil
	}

	return false, nil
}

// PowerOff ensures the server is powered off independently of any image
// provisioning operation.
func (p *ironicProvisioner) PowerOff() (dirty bool, err error) {
	p.log.Info("ensuring host is powered off")

	if dirty, err = p.ensureExists(); err != nil {
		return dirty, errors.Wrap(err, "could not power off host")
	}

	if p.host.Status.Provisioning.State != "powered off" {
		p.host.Status.Provisioning.State = "powered off"
		return true, nil
	}

	return false, nil
}
