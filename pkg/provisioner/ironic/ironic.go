package ironic

import (
	"time"

	"github.com/pkg/errors"

	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"

	metalkubev1alpha1 "github.com/metalkube/baremetal-operator/pkg/apis/metalkube/v1alpha1"
	"github.com/metalkube/baremetal-operator/pkg/provisioner"
)

var log = logf.Log.WithName("ironic")
var deprovisionRequeueDelay = time.Second * 10

// Provisioner implements the provisioning.Provisioner interface
// and uses Ironic to manage the host.
type ironicProvisioner struct{}

// New returns a new Ironic provisioner
func New() provisioner.Provisioner {
	return &ironicProvisioner{}
}

// Register the host with Ironic.
func (p *ironicProvisioner) ensureExists(host *metalkubev1alpha1.BareMetalHost) (dirty bool, err error) {
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
func (p *ironicProvisioner) ValidateManagementAccess(host *metalkubev1alpha1.BareMetalHost) (dirty bool, err error) {
	reqLogger := log.WithValues("host", host.Name)
	reqLogger.Info("testing management access")
	if dirty, err := p.ensureExists(host); err != nil {
		return dirty, errors.Wrap(err, "could not validate management access")
	}
	return dirty, nil
}

// InspectHardware updates the HardwareDetails field of the host with
// details of devices discovered on the hardware. It may be called
// multiple times, and should return true for its dirty flag until the
// inspection is completed.
func (p *ironicProvisioner) InspectHardware(host *metalkubev1alpha1.BareMetalHost) (dirty bool, err error) {
	reqLogger := log.WithValues("host", host.Name)
	reqLogger.Info("inspecting hardware", "status", host.OperationalStatus())

	if dirty, err = p.ensureExists(host); err != nil {
		return dirty, errors.Wrap(err, "could not inspect hardware")
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
		host.Status.HardwareDetails =
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
func (p *ironicProvisioner) Deprovision(host *metalkubev1alpha1.BareMetalHost) (dirty bool, retryDelay time.Duration, err error) {
	reqLogger := log.WithValues("host", host.Name)
	reqLogger.Info("ensuring host is removed")

	// NOTE(dhellmann): In order to simulate a multi-step process,
	// modify some of the status data structures. This is likely not
	// necessary once we really have Ironic doing the deprovisioning
	// and we can monitor it's status.

	if host.Status.HardwareDetails != nil {
		reqLogger.Info("clearing hardware details")
		host.Status.HardwareDetails = nil
		return true, deprovisionRequeueDelay, nil
	}

	if host.Status.Provisioning.ID != "" {
		reqLogger.Info("clearing provisioning id")
		host.Status.Provisioning.ID = ""
		return true, deprovisionRequeueDelay, nil
	}

	return false, 0, nil
}

// PowerOn ensures the server is powered on independently of any image
// provisioning operation.
func (p *ironicProvisioner) PowerOn(host *metalkubev1alpha1.BareMetalHost) (dirty bool, err error) {
	reqLogger := log.WithValues("host", host.Name)
	reqLogger.Info("ensuring host is powered on")

	if dirty, err = p.ensureExists(host); err != nil {
		return dirty, errors.Wrap(err, "could not power on host")
	}

	if host.Status.Provisioning.State != "powered on" {
		host.Status.Provisioning.State = "powered on"
		return true, nil
	}

	return false, nil
}

// PowerOff ensures the server is powered off independently of any image
// provisioning operation.
func (p *ironicProvisioner) PowerOff(host *metalkubev1alpha1.BareMetalHost) (dirty bool, err error) {
	reqLogger := log.WithValues("host", host.Name)
	reqLogger.Info("ensuring host is powered off")

	if dirty, err = p.ensureExists(host); err != nil {
		return dirty, errors.Wrap(err, "could not power off host")
	}

	if host.Status.Provisioning.State != "powered off" {
		host.Status.Provisioning.State = "powered off"
		return true, nil
	}

	return false, nil
}
