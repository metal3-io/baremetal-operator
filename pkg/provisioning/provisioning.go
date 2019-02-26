package provisioning

import (
	"time"

	metalkubev1alpha1 "github.com/metalkube/baremetal-operator/pkg/apis/metalkube/v1alpha1"
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
	// DeprovisionRequeueDelay controls the amount of time the reconciler
	// waits between attempts to determine if the deprovisioning operation
	// has been completed.
	DeprovisionRequeueDelay time.Duration
}

// Register the host with Ironic.
func (p *Provisioner) ensureExists(host *metalkubev1alpha1.BareMetalHost) (dirty bool, err error) {
	reqLogger := log.WithValues("host", host.Name)
	if host.Status.Provisioning.ID == "" {
		host.Status.Provisioning.ID = "temporary-fake-id"
		reqLogger.Info("setting provisioning id",
			"provisioningID", host.Status.Provisioning.ID)
		dirty = true
	}
	return dirty, nil
}

// ValidateManagementAccess tests the connection information for the
// host to verify that the location and credentials work.
func (p *Provisioner) ValidateManagementAccess(host *metalkubev1alpha1.BareMetalHost) (dirty bool, err error) {
	reqLogger := log.WithValues("host", host.Name)
	reqLogger.Info("testing management access")
	if dirty, err := p.ensureExists(host); err != nil {
		return dirty, err
	}
	return dirty, nil
}

// InspectHardware updates the HardwareDetails field of the host with
// details of devices discovered on the hardware. It may be called
// multiple times, and should return true for its dirty flag until the
// inspection is completed.
func (p *Provisioner) InspectHardware(host *metalkubev1alpha1.BareMetalHost) (dirty bool, err error) {
	reqLogger := log.WithValues("host", host.Name)
	reqLogger.Info("inspecting hardware", "status", host.OperationalStatus())

	if dirty, err = p.ensureExists(host); err != nil {
		return dirty, err
	}

	if host.OperationalStatus() != metalkubev1alpha1.OperationalStatusInspecting {
		// The inspection just started.
		reqLogger.Info("starting inspection by setting state")
		host.SetOperationalStatus(metalkubev1alpha1.OperationalStatusInspecting)
		return true, nil
	}

	// The inspection is ongoing. We'll need to check the ironic
	// status for the server here until it is ready for us to get the
	// inspection details. Simulate that for now by creating the
	// hardware details struct as part of a second pass.
	if host.Status.HardwareDetails == nil {
		reqLogger.Info("continuing inspection by setting details")
		host.Status.HardwareDetails = &metalkubev1alpha1.HardwareDetails{}
		return true, nil
	}

	return false, nil
}

// Deprovision prepares the host to be removed from the cluster. It
// may be called multiple times, and should return true for its dirty
// flag until the deprovisioning operation is completed.
func (p *Provisioner) Deprovision(host *metalkubev1alpha1.BareMetalHost) (dirty bool, err error) {
	reqLogger := log.WithValues("host", host.Name)
	reqLogger.Info("ensuring host is removed")

	if host.Status.HardwareDetails != nil {
		reqLogger.Info("clearing hardware details")
		host.Status.HardwareDetails = nil
		return true, nil
	}

	if host.Status.Provisioning.ID != "" {
		reqLogger.Info("clearing provisioning id")
		host.Status.Provisioning.ID = ""
		return true, nil
	}

	return false, nil
}

// PowerOn ensures the server is powered on independently of any image
// provisioning operation.
func (p *Provisioner) PowerOn(host *metalkubev1alpha1.BareMetalHost) (dirty bool, err error) {
	reqLogger := log.WithValues("host", host.Name)
	reqLogger.Info("ensuring host is powered on")

	if dirty, err = p.ensureExists(host); err != nil {
		return dirty, err
	}

	if host.Status.Provisioning.State != "powered on" {
		host.Status.Provisioning.State = "powered on"
		return true, nil
	}

	return false, nil
}

// PowerOff ensures the server is powered off independently of any image
// provisioning operation.
func (p *Provisioner) PowerOff(host *metalkubev1alpha1.BareMetalHost) (dirty bool, err error) {
	reqLogger := log.WithValues("host", host.Name)
	reqLogger.Info("ensuring host is powered off")

	if dirty, err = p.ensureExists(host); err != nil {
		return dirty, err
	}

	if host.Status.Provisioning.State != "powered off" {
		host.Status.Provisioning.State = "powered off"
		return true, nil
	}

	return false, nil
}
