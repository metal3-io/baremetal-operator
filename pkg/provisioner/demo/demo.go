package demo

import (
	"time"

	"github.com/go-logr/logr"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"

	metal3v1alpha1 "github.com/metal3-io/baremetal-operator/pkg/apis/metal3/v1alpha1"
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
	host *metal3v1alpha1.BareMetalHost
	// the bmc credentials
	bmcCreds bmc.Credentials
	// a logger configured for this host
	log logr.Logger
	// an event publisher for recording significant events
	publisher provisioner.EventPublisher
}

// New returns a new Ironic Provisioner
func New(host *metal3v1alpha1.BareMetalHost, bmcCreds bmc.Credentials, publisher provisioner.EventPublisher) (provisioner.Provisioner, error) {
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
func (p *demoProvisioner) ValidateManagementAccess(credentialsChanged bool) (result provisioner.Result, err error) {
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
func (p *demoProvisioner) InspectHardware() (result provisioner.Result, details *metal3v1alpha1.HardwareDetails, err error) {
	p.log.Info("inspecting hardware", "status", p.host.OperationalStatus())

	hostName := p.host.ObjectMeta.Name

	if hostName == InspectingHost {
		// set dirty so we don't allow the host to progress past this
		// state in Reconcile()
		result.Dirty = true
		result.RequeueAfter = time.Second * 5
		return
	}

	// The inspection is ongoing. We'll need to check the demo
	// status for the server here until it is ready for us to get the
	// inspection details. Simulate that for now by creating the
	// hardware details struct as part of a second pass.
	if p.host.Status.HardwareDetails == nil {
		p.log.Info("continuing inspection by setting details")
		details =
			&metal3v1alpha1.HardwareDetails{
				RAMMebibytes: 128 * 1024,
				NIC: []metal3v1alpha1.NIC{
					{
						Name:      "nic-1",
						Model:     "virt-io",
						MAC:       "some:mac:address",
						IP:        "192.168.100.1",
						SpeedGbps: 1,
						PXE:       true,
					},
					{
						Name:      "nic-2",
						Model:     "e1000",
						MAC:       "some:other:mac:address",
						IP:        "192.168.100.2",
						SpeedGbps: 1,
						PXE:       false,
					},
				},
				Storage: []metal3v1alpha1.Storage{
					{
						Name:       "disk-1 (boot)",
						Rotational: false,
						SizeBytes:  metal3v1alpha1.TebiByte * 93,
						Model:      "Dell CFJ61",
					},
					{
						Name:       "disk-2",
						Rotational: false,
						SizeBytes:  metal3v1alpha1.TebiByte * 93,
						Model:      "Dell CFJ61",
					},
				},
				CPU: metal3v1alpha1.CPU{
					Arch:           "x86_64",
					Model:          "Core 2 Duo",
					ClockMegahertz: 3.0 * metal3v1alpha1.GigaHertz,
					Flags:          []string{"lm", "hypervisor", "vmx"},
					Count:          1,
				},
			}
		p.publisher("InspectionComplete", "Hardware inspection completed")
		p.host.SetOperationalStatus(metal3v1alpha1.OperationalStatusOK)
	}

	return
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

// Adopt allows an externally-provisioned server to be adopted.
func (p *demoProvisioner) Adopt() (result provisioner.Result, err error) {
	p.log.Info("adopting host")
	result.Dirty = false
	return
}

// Provision writes the image from the host spec to the host. It may
// be called multiple times, and should return true for its dirty flag
// until the deprovisioning operation is completed.
func (p *demoProvisioner) Provision(hostConf provisioner.HostConfigData) (result provisioner.Result, err error) {

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

// Deprovision removes the host from the image. It may be called
// multiple times, and should return true for its dirty flag until the
// deprovisioning operation is completed.
func (p *demoProvisioner) Deprovision() (result provisioner.Result, err error) {

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

// Delete removes the host from the provisioning system. It may be
// called multiple times, and should return true for its dirty flag
// until the deprovisioning operation is completed.
func (p *demoProvisioner) Delete() (result provisioner.Result, err error) {
	p.log.Info("deleting host")
	return result, nil
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
