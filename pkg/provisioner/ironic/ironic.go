package ironic

import (
	"fmt"
	"net/url"
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
type provisionerFactory struct{}

// New returns a new Ironic ProvisionerFactory
func NewFactory() provisioner.ProvisionerFactory {
	return &provisionerFactory{}
}

// Provisioner implements the provisioning.Provisioner interface
// and uses Ironic to manage the host.
type ironicProvisioner struct {
	// the host to be managed by this provisioner
	host *metalkubev1alpha1.BareMetalHost
	// a shorter path to the provisioning status data structure
	status *metalkubev1alpha1.ProvisionStatus
	// access parameters for the BMC
	bmcAccess *url.URL
	// credentials to log in to the BMC
	bmcCreds bmc.Credentials
	// a client for talking to ironic
	client *gophercloud.ServiceClient
	// a logger configured for this host
	log logr.Logger
}

// New returns a new Ironic Provisioner
func (f *provisionerFactory) New(host *metalkubev1alpha1.BareMetalHost, bmcCreds bmc.Credentials) (provisioner.Provisioner, error) {
	client, err := noauth.NewBareMetalNoAuth(noauth.EndpointOpts{
		// FIXME(dhellmann): We need to get this URL from the caller
		// somehow, maybe from the factory?
		IronicEndpoint: "http://localhost:6385/v1/",
	})
	if err != nil {
		return nil, err
	}
	bmcAccess, err := url.Parse(host.Spec.BMC.Address)
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
		ironicNode, err = nodes.Create(
			p.client,
			nodes.CreateOpts{
				Driver:        "ipmi",
				BootInterface: "pxe",
				Name:          p.host.Name,
				DriverInfo: map[string]interface{}{
					// FIXME(dhellmann): The names of the parameters
					// depend on the ironic driver. We are going to
					// need to isolate the logic for building the
					// DriverInfo dict somewhere.
					"ipmi_port":     p.bmcAccess.Port(),
					"ipmi_username": p.bmcCreds.Username,
					"ipmi_password": p.bmcCreds.Password,
					"ipmi_address":  p.bmcAccess.Hostname(),
					// FIXME(dhellmann): The names of the images are tied
					// to the version of ironic we are using and are
					// likely to change.
					//
					// FIXME(dhellmann): We need to get our IP on the
					// provisioning network from somewhere.
					"deploy_kernel":  "http://172.22.0.1/images/tinyipa-stable-rocky.vmlinuz",
					"deploy_ramdisk": "http://172.22.0.1/images/tinyipa-stable-rocky.gz",
				},
			}).Extract()
		if err != nil {
			return false, errors.Wrap(err, "failed to register host in ironic")
		}

		// Store the ID so other methods can assume it is set and so
		// we can find the node again later.
		p.status.ID = ironicNode.UUID
		dirty = true
		p.log.Info("setting provisioning id", "ID", p.status.ID)

		// NOTE(dhellmann): libvirt-based hosts used for dev and testing
		// require a MAC address and network name, specified as query
		// parameters in the URL.
		params := p.bmcAccess.Query()
		if len(params["mac"]) >= 1 {
			// FIXME(dhellmann): There is probably a safer way to
			// get the values out of the query dict, which might be empty.
			mac := params["mac"][0]
			net := params["net"][0]
			p.log.Info("creating port for node in ironic", "mac", mac, "net", net)
			_, err := ports.Create(
				p.client,
				ports.CreateOpts{
					NodeUUID:        ironicNode.UUID,
					Address:         mac,
					PhysicalNetwork: net,
				}).Extract()
			if err != nil {
				return false, errors.Wrap(err, "failed to create port in ironic")
			}
		}

		// FIXME(dhellmann): The URLs and image names used here also
		// change based on deployment and version. These should come from
		// the host, via the Machine object and actuator. We should test
		// whether the incoming values match the values on the node in
		// ironic before updating them. For now, using "dirty" flag as an
		// indicator that we created the node so we need to set these
		// values.
		p.log.Info("updating host settings in ironic")
		_, err = nodes.Update(
			p.client,
			ironicNode.UUID,
			nodes.UpdateOpts{
				nodes.UpdateOperation{
					Op:    nodes.AddOp,
					Path:  "/instance_info/image_source",
					Value: "http://172.22.0.1/images/redhat-coreos-maipo-latest.qcow2",
				},
				nodes.UpdateOperation{
					Op:    nodes.AddOp,
					Path:  "/instance_info/image_checksum",
					Value: "97830b21ed272a3d854615beb54cf004",
				},
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
	if ironicNode.ProvisionState == string(nodes.Enroll) && ironicNode.TargetProvisionState != nodes.Manageable {
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
	p.log.Info("testing management access")
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
						MAC:       "some:mac:address",
						IP:        "192.168.100.1",
						SpeedGbps: 1,
					},
					metalkubev1alpha1.NIC{
						MAC:       "some:other:mac:address",
						IP:        "192.168.100.2",
						SpeedGbps: 1,
					},
				},
				Storage: []metalkubev1alpha1.Storage{
					metalkubev1alpha1.Storage{
						SizeGiB: 1024,
						Info:    "Some information about this disk.",
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
	p.log.Info("ensuring host is removed")

	// NOTE(dhellmann): In order to simulate a multi-step process,
	// modify some of the status data structures. This is likely not
	// necessary once we really have Ironic doing the deprovisioning
	// and we can monitor it's status.

	if p.host.Status.HardwareDetails != nil {
		p.log.Info("clearing hardware details")
		p.host.Status.HardwareDetails = nil
		return true, deprovisionRequeueDelay, nil
	}

	if p.host.Status.Provisioning.ID != "" {
		p.log.Info("clearing provisioning id")
		p.host.Status.Provisioning.ID = ""
		return true, deprovisionRequeueDelay, nil
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
