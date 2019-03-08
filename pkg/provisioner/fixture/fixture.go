package fixture

import (
	"time"

	"github.com/pkg/errors"

	"github.com/go-logr/logr"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"

	metalkubev1alpha1 "github.com/metalkube/baremetal-operator/pkg/apis/metalkube/v1alpha1"
	"github.com/metalkube/baremetal-operator/pkg/bmc"
	"github.com/metalkube/baremetal-operator/pkg/provisioner"
)

var log = logf.Log.WithName("ironic")
var deprovisionRequeueDelay = time.Second * 10

// Test fixture provisioner factory
type provisionerFactory struct{}

// New returns a new test ProvisionerFactory
func NewFactory() provisioner.ProvisionerFactory {
	return &provisionerFactory{}
}

// Provisioner implements the provisioning.Provisioner interface
// and uses Ironic to manage the host.
type fixtureProvisioner struct {
	// the host to be managed by this provisioner
	host *metalkubev1alpha1.BareMetalHost
	// a logger configured for this host
	log logr.Logger
}

// New returns a new Ironic Provisioner
func (f *provisionerFactory) New(host *metalkubev1alpha1.BareMetalHost, bmcCreds bmc.Credentials) (provisioner.Provisioner, error) {
	p := &fixtureProvisioner{
		host: host,
		log:  log.WithValues("host", host.Name),
	}
	return p, nil
}

// Register the host with Fixture.
func (p *fixtureProvisioner) ensureExists() (dirty bool, err error) {
	if p.host.Status.Provisioning.ID == "" {
		p.host.Status.Provisioning.ID = "temporary-fake-id"
		p.log.Info("setting provisioning id",
			"provisioningID", p.host.Status.Provisioning.ID)
		dirty = true
	}
	return dirty, nil
}

// ValidateManagementAccess tests the connection information for the
// host to verify that the location and credentials work.
func (p *fixtureProvisioner) ValidateManagementAccess() (dirty bool, err error) {
	p.log.Info("testing management access")
	if dirty, err := p.ensureExists(); err != nil {
		return dirty, errors.Wrap(err, "could not validate management access")
	}
	return dirty, nil
}

// InspectHardware updates the HardwareDetails field of the host with
// details of devices discovered on the hardware. It may be called
// multiple times, and should return true for its dirty flag until the
// inspection is completed.
func (p *fixtureProvisioner) InspectHardware() (dirty bool, err error) {
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

	// The inspection is ongoing. We'll need to check the fixture
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
						Model:   "stone tablet",
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
func (p *fixtureProvisioner) Deprovision() (dirty bool, retryDelay time.Duration, err error) {
	p.log.Info("ensuring host is removed")

	// NOTE(dhellmann): In order to simulate a multi-step process,
	// modify some of the status data structures. This is likely not
	// necessary once we really have Fixture doing the deprovisioning
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
func (p *fixtureProvisioner) PowerOn() (dirty bool, err error) {
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
func (p *fixtureProvisioner) PowerOff() (dirty bool, err error) {
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
