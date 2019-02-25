package provisioning

import (
	"github.com/metalkube/baremetal-operator/pkg/bmc"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var log = logf.Log.WithName("provisioner")

/*
Package provisioning defines the API for talking to the provisioning backend.
*/

// Provisioner holds the state information for talking to the
// provisioning backend.
//
// NOTE(dhellmann): Provisioner will eventually become an interface,
// but not until we need to have more than one backend.
type Provisioner struct {
}

// ProvisionStatus holds the state information for a single target.
type ProvisionStatus struct {
	// FIXME(dhellmann): This should be an enum of some sort.
	State string `json:"state"`
	// UUID in ironic
	ProvisioningID string `json:"provisioningID"`
}

// Target holds the details for a single host that we're going to
// manipulate.
type Target struct {
	// Name of the target, for logging
	Name string
	// Connection string for BMC (IP, URL, etc.)
	BMCLocation string
	// Credentials for logging in to the BMC
	BMCCredentials bmc.Credentials
	// State tracking
	Status *ProvisionStatus

	// Has the Status been modified?
	dirty bool
}

// StatusDirty returns true when the Status field of a Target has been
// modified and needs to be saved.
func (t Target) StatusDirty() bool {
	return t.dirty
}

// Register the host with Ironic.
func (p *Provisioner) ensureExists(host *Target) error {
	reqLogger := log.WithValues("host", host.Name)
	reqLogger.Info("ensuring host exists")
	host.Status.ProvisioningID = "temporary-fake-id"
	host.dirty = true
	return nil
}

// ValidateManagementAccess tests the connection information for the
// host to verify that the location and credentials work.
func (p *Provisioner) ValidateManagementAccess(host *Target) error {
	reqLogger := log.WithValues("host", host.Name)
	reqLogger.Info("testing management access")
	if err := p.ensureExists(host); err != nil {
		return err
	}
	return nil
}
