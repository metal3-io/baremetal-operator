package fixture

import (
	"time"

	"github.com/go-logr/logr"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"

	metalkubev1alpha1 "github.com/metalkube/baremetal-operator/pkg/apis/metalkube/v1alpha1"
	"github.com/metalkube/baremetal-operator/pkg/bmc"
	"github.com/metalkube/baremetal-operator/pkg/provisioner"
)

var log = logf.Log.WithName("fixture")
var deprovisionRequeueDelay = time.Second * 10
var provisionRequeueDelay = time.Second * 10

// Provisioner implements the provisioning.Provisioner interface
// and uses Ironic to manage the host.
type fixtureProvisioner struct {
	// the host to be managed by this provisioner
	host *metalkubev1alpha1.BareMetalHost
	// the bmc credentials
	bmcCreds bmc.Credentials
	// a logger configured for this host
	log logr.Logger
}

// New returns a new Ironic Provisioner
func New(host *metalkubev1alpha1.BareMetalHost, bmcCreds bmc.Credentials) (provisioner.Provisioner, error) {
	p := &fixtureProvisioner{
		host:     host,
		bmcCreds: bmcCreds,
		log:      log.WithValues("host", host.Name),
	}
	return p, nil
}

// ValidateManagementAccess tests the connection information for the
// host to verify that the location and credentials work.
func (p *fixtureProvisioner) ValidateManagementAccess() (result provisioner.Result, err error) {
	p.log.Info("testing management access")

	// Fill in the ID of the host in the provisioning system
	if p.host.Status.Provisioning.ID == "" {
		p.host.Status.Provisioning.ID = "temporary-fake-id"
		p.log.Info("setting provisioning id",
			"provisioningID", p.host.Status.Provisioning.ID)
		result.Dirty = true
		result.RequeueAfter = time.Second * 5
		return result, nil
	}

	// Clear any error
	result.Dirty = p.host.ClearError()

	return result, nil
}

// InspectHardware updates the HardwareDetails field of the host with
// details of devices discovered on the hardware. It may be called
// multiple times, and should return true for its dirty flag until the
// inspection is completed.
func (p *fixtureProvisioner) InspectHardware() (result provisioner.Result, err error) {
	p.log.Info("inspecting hardware", "status", p.host.OperationalStatus())

	if p.host.OperationalStatus() != metalkubev1alpha1.OperationalStatusInspecting {
		// The inspection just started.
		p.log.Info("starting inspection by setting state")
		p.host.SetOperationalStatus(metalkubev1alpha1.OperationalStatusInspecting)
		result.Dirty = true
		return result, nil
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
func (p *fixtureProvisioner) Provision(userData string) (result provisioner.Result, err error) {
	p.log.Info("provisioning image to host",
		"state", p.host.Status.Provisioning.State)

	result.RequeueAfter = provisionRequeueDelay

	// NOTE(dhellmann): This is a test class, so we simulate a
	// multi-step process to ensure that multiple cycles through the
	// reconcile loop work properly.

	if p.host.Status.Provisioning.State == "" {
		p.log.Info("moving to step1")
		p.host.Status.Provisioning.State = "step1"
		result.Dirty = true
		return result, nil
	}

	if p.host.Status.Provisioning.State == "step1" {
		p.log.Info("moving to step2")
		p.host.Status.Provisioning.State = "step2"
		result.Dirty = true
		return result, nil
	}

	if p.host.Status.Provisioning.State == "step2" {
		p.log.Info("moving to done")
		p.host.Status.Provisioning.State = "done"
		p.host.Status.Provisioning.Image = *p.host.Spec.Image
		result.Dirty = true
		return result, nil
	}

	return result, nil
}

// Deprovision prepares the host to be removed from the cluster. It
// may be called multiple times, and should return true for its dirty
// flag until the deprovisioning operation is completed.
func (p *fixtureProvisioner) Deprovision(deleteIt bool) (result provisioner.Result, err error) {
	p.log.Info("ensuring host is removed")

	result.RequeueAfter = deprovisionRequeueDelay

	// NOTE(dhellmann): In order to simulate a multi-step process,
	// modify some of the status data structures. This is likely not
	// necessary once we really have Fixture doing the deprovisioning
	// and we can monitor it's status.

	if p.host.Status.HardwareDetails != nil {
		p.log.Info("clearing hardware details")
		p.host.Status.HardwareDetails = nil
		result.Dirty = true
		return result, nil
	}

	if p.host.Status.Provisioning.ID != "" {
		p.log.Info("clearing provisioning id")
		p.host.Status.Provisioning.ID = ""
		result.Dirty = true
		return result, nil
	}

	return result, nil
}

// PowerOn ensures the server is powered on independently of any image
// provisioning operation.
func (p *fixtureProvisioner) PowerOn() (result provisioner.Result, err error) {
	p.log.Info("ensuring host is powered on")

	if !p.host.Status.PoweredOn {
		p.host.Status.PoweredOn = true
		result.Dirty = true
		return result, nil
	}

	return result, nil
}

// PowerOff ensures the server is powered off independently of any image
// provisioning operation.
func (p *fixtureProvisioner) PowerOff() (result provisioner.Result, err error) {
	p.log.Info("ensuring host is powered off")

	if p.host.Status.PoweredOn {
		p.host.Status.PoweredOn = false
		result.Dirty = true
		return result, nil
	}

	return result, nil
}
