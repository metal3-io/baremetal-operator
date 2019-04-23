package demo

import (
	"time"

	"github.com/go-logr/logr"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"

	metalkubev1alpha1 "github.com/metal3-io/baremetal-operator/pkg/apis/metalkube/v1alpha1"
	"github.com/metal3-io/baremetal-operator/pkg/bmc"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner"
)

var log = logf.Log.WithName("demo")
var deprovisionRequeueDelay = time.Second * 10
var provisionRequeueDelay = time.Second * 10

const (
	// RegistrationErrorHost is a host that fails the registration
	// process.
	RegistrationErrorHost string = "demo-registration-error"

	// RegisteringHost is a host that is in the process of being
	// registered.
	RegisteringHost string = "demo-registering"

	// ReadyHost is a host that is ready to be used.
	ReadyHost string = "demo-ready"

	// InspectingHost is a host that is having its hardware scanned.
	InspectingHost string = "demo-inspecting"

	// ValidationErrorHost is a host that started provisioning but
	// failed validation.
	ValidationErrorHost string = "demo-validation-error"

	// ProvisioningHost is a host that is in the middle of
	// provisioning.
	ProvisioningHost string = "demo-provisioning"

	// ProvisionedHost is a host that has had an image provisioned.
	ProvisionedHost string = "demo-provisioned"
)

// Provisioner implements the provisioning.Provisioner interface
// and uses Ironic to manage the host.
type demoProvisioner struct {
	// the host to be managed by this provisioner
	host *metalkubev1alpha1.BareMetalHost
	// the bmc credentials
	bmcCreds bmc.Credentials
	// a logger configured for this host
	log logr.Logger
	// an event publisher for recording significant events
	publisher provisioner.EventPublisher
}

// New returns a new Ironic Provisioner
func New(host *metalkubev1alpha1.BareMetalHost, bmcCreds bmc.Credentials, publisher provisioner.EventPublisher) (provisioner.Provisioner, error) {
	p := &demoProvisioner{
		host:      host,
		bmcCreds:  bmcCreds,
		log:       log.WithValues("host", host.Name),
		publisher: publisher,
	}
	return p, nil
}

// ValidateManagementAccess tests the connection information for the
// host to verify that the location and credentials work.
func (p *demoProvisioner) ValidateManagementAccess() (result provisioner.Result, err error) {
	p.log.Info("testing management access")

	hostName := p.host.ObjectMeta.Name

	switch hostName {

	case RegistrationErrorHost:
		// We have set an error, so Reconcile() will stop
		result.ErrorMessage = "failed to register new host"
		p.log.Info("setting registration error")

	case RegisteringHost:
		// Always mark the host as dirty so it never moves past this
		// point.
		result.Dirty = true
		result.RequeueAfter = time.Second * 5

	default:
		if p.host.Status.Provisioning.ID == "" {
			p.host.Status.Provisioning.ID = p.host.ObjectMeta.Name
			p.log.Info("setting provisioning id",
				"provisioningID", p.host.Status.Provisioning.ID)
			result.Dirty = true
		}
	}

	return result, nil
}

// InspectHardware updates the HardwareDetails field of the host with
// details of devices discovered on the hardware. It may be called
// multiple times, and should return true for its dirty flag until the
// inspection is completed.
func (p *demoProvisioner) InspectHardware() (result provisioner.Result, err error) {
	p.log.Info("inspecting hardware", "status", p.host.OperationalStatus())

	hostName := p.host.ObjectMeta.Name

	if hostName == InspectingHost {
		p.host.Status.Provisioning.State = metalkubev1alpha1.StateInspecting
		// set dirty so we don't allow the host to progress past this
		// state in Reconcile()
		result.Dirty = true
		result.RequeueAfter = time.Second * 5
		return result, nil
	}

	// The inspection is ongoing. We'll need to check the demo
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
		p.publisher("InspectionComplete", "Hardware inspection completed")
		p.host.SetOperationalStatus(metalkubev1alpha1.OperationalStatusOK)
		result.Dirty = true
		return result, nil
	}

	return result, nil
}

// UpdateHardwareState fetches the latest hardware state of the server
// and updates the HardwareDetails field of the host with details. It
// is expected to do this in the least expensive way possible, such as
// reading from a cache, and return dirty only if any state
// information has changed.
func (p *demoProvisioner) UpdateHardwareState() (result provisioner.Result, err error) {
	p.log.Info("updating hardware state")
	result.Dirty = false
	return result, nil
}

// Provision writes the image from the host spec to the host. It may
// be called multiple times, and should return true for its dirty flag
// until the deprovisioning operation is completed.
func (p *demoProvisioner) Provision(getUserData provisioner.UserDataSource) (result provisioner.Result, err error) {

	hostName := p.host.ObjectMeta.Name
	p.log.Info("provisioning image to host", "state", p.host.Status.Provisioning.State)

	switch hostName {

	case ValidationErrorHost:
		p.log.Info("setting validation error")
		result.ErrorMessage = "validation failed"

	case ProvisioningHost:
		p.log.Info("provisioning host")
		result.Dirty = true
		result.RequeueAfter = time.Second * 5

	default:
		p.log.Info("finished provisioning")
	}

	return result, nil
}

// Deprovision prepares the host to be removed from the cluster. It
// may be called multiple times, and should return true for its dirty
// flag until the deprovisioning operation is completed.
func (p *demoProvisioner) Deprovision(deleteIt bool) (result provisioner.Result, err error) {

	hostName := p.host.ObjectMeta.Name
	switch hostName {
	default:
		return result, nil
	}

	// p.log.Info("ensuring host is removed")

	// result.RequeueAfter = deprovisionRequeueDelay

	// // NOTE(dhellmann): In order to simulate a multi-step process,
	// // modify some of the status data structures. This is likely not
	// // necessary once we really have Demo doing the deprovisioning
	// // and we can monitor it's status.

	// if p.host.Status.HardwareDetails != nil {
	// 	p.publisher("DeprovisionStarted", "Image deprovisioning started")
	// 	p.log.Info("clearing hardware details")
	// 	p.host.Status.HardwareDetails = nil
	// 	result.Dirty = true
	// 	return result, nil
	// }

	// if p.host.Status.Provisioning.ID != "" {
	// 	p.log.Info("clearing provisioning id")
	// 	p.host.Status.Provisioning.ID = ""
	// 	result.Dirty = true
	// 	return result, nil
	// }

	// p.publisher("DeprovisionComplete", "Image deprovisioning completed")
	// return result, nil
}

// PowerOn ensures the server is powered on independently of any image
// provisioning operation.
func (p *demoProvisioner) PowerOn() (result provisioner.Result, err error) {

	hostName := p.host.ObjectMeta.Name
	switch hostName {
	default:
		return result, nil
	}

	// p.log.Info("ensuring host is powered on")

	// if !p.host.Status.PoweredOn {
	// 	p.host.Status.PoweredOn = true
	// 	result.Dirty = true
	// 	return result, nil
	// }

	// return result, nil
}

// PowerOff ensures the server is powered off independently of any image
// provisioning operation.
func (p *demoProvisioner) PowerOff() (result provisioner.Result, err error) {

	hostName := p.host.ObjectMeta.Name
	switch hostName {
	default:
		return result, nil
	}

	// p.log.Info("ensuring host is powered off")

	// if p.host.Status.PoweredOn {
	// 	p.host.Status.PoweredOn = false
	// 	result.Dirty = true
	// 	return result, nil
	// }

	// return result, nil
}
