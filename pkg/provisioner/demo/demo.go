package demo

import (
	"context"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/go-logr/logr"
	logz "sigs.k8s.io/controller-runtime/pkg/log/zap"

	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	"github.com/metal3-io/baremetal-operator/pkg/hardwareutils/bmc"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner"
)

var log = logz.New().WithName("provisioner").WithName("demo")

const (
	// RegistrationErrorHost is a host that fails the registration
	// process.
	RegistrationErrorHost string = "demo-registration-error"

	// RegisteringHost is a host that is in the process of being
	// registered.
	RegisteringHost string = "demo-registering"

	// AvailableHost is a host that is available to be used.
	AvailableHost string = "demo-available"

	// InspectingHost is a host that is having its hardware scanned.
	InspectingHost string = "demo-inspecting"

	// PreparingErrorHost is a host that started preparing but failed.
	PreparingErrorHost string = "demo-preparing-error"

	// PreparingHost is a host that is in the middle of preparing.
	PreparingHost string = "demo-preparing"

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
	// the object metadata of the BareMetalHost resource
	objectMeta metav1.ObjectMeta
	// the provisioning ID for this host
	provID string
	// the bmc credentials
	bmcCreds bmc.Credentials
	// a logger configured for this host
	log logr.Logger
	// an event publisher for recording significant events
	publisher provisioner.EventPublisher
}

type Demo struct{}

// NewProvisioner returns a new demo Provisioner.
func (d Demo) NewProvisioner(_ context.Context, hostData provisioner.HostData, publisher provisioner.EventPublisher) (provisioner.Provisioner, error) {
	p := &demoProvisioner{
		objectMeta: hostData.ObjectMeta,
		provID:     hostData.ProvisionerID,
		bmcCreds:   hostData.BMCCredentials,
		log:        log.WithValues("host", hostData.ObjectMeta.Name),
		publisher:  publisher,
	}
	return p, nil
}

func (p *demoProvisioner) HasCapacity() (result bool, err error) {
	return true, nil
}

// ValidateManagementAccess tests the connection information for the
// host to verify that the location and credentials work.
func (p *demoProvisioner) ValidateManagementAccess(_ provisioner.ManagementAccessData, _, _ bool) (result provisioner.Result, provID string, err error) {
	p.log.Info("testing management access")

	hostName := p.objectMeta.Name

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
		if p.provID == "" {
			provID = p.objectMeta.Name
			p.log.Info("setting provisioning id", "provisioningID", provID)
			result.Dirty = true
		}
	}

	return
}

func (p *demoProvisioner) PreprovisioningImageFormats() ([]metal3api.ImageFormat, error) {
	return nil, nil
}

// InspectHardware updates the HardwareDetails field of the host with
// details of devices discovered on the hardware. It may be called
// multiple times, and should return true for its dirty flag until the
// inspection is completed.
func (p *demoProvisioner) InspectHardware(_ provisioner.InspectData, _, _, _ bool) (result provisioner.Result, started bool, details *metal3api.HardwareDetails, err error) {
	started = true
	hostName := p.objectMeta.Name

	if hostName == InspectingHost {
		// set dirty so we don't allow the host to progress past this
		// state in Reconcile()
		result.Dirty = true
		result.RequeueAfter = time.Second * 5
		return
	}

	p.log.Info("continuing inspection by setting details")
	details =
		&metal3api.HardwareDetails{
			RAMMebibytes: 128 * 1024,
			NIC: []metal3api.NIC{
				{
					Name:      "nic-1",
					Model:     "virt-io",
					MAC:       "ab:cd:12:34:56:78",
					IP:        "192.168.100.1",
					SpeedGbps: 1,
					PXE:       true,
				},
				{
					Name:      "nic-2",
					Model:     "e1000",
					MAC:       "12:34:56:78:ab:cd",
					IP:        "192.168.100.2",
					SpeedGbps: 1,
					PXE:       false,
				},
			},
			Storage: []metal3api.Storage{
				{
					Name:       "disk-1 (boot)",
					Rotational: false,
					SizeBytes:  metal3api.TebiByte * 93,
					Model:      "Dell CFJ61",
				},
				{
					Name:       "disk-2",
					Rotational: false,
					SizeBytes:  metal3api.TebiByte * 93,
					Model:      "Dell CFJ61",
				},
			},
			CPU: metal3api.CPU{
				Arch:           "x86_64",
				Model:          "Core 2 Duo",
				ClockMegahertz: 3.0 * metal3api.GigaHertz,
				Flags:          []string{"lm", "hypervisor", "vmx"},
				Count:          1,
			},
		}
	p.publisher("InspectionComplete", "Hardware inspection completed")

	return
}

// UpdateHardwareState fetches the latest hardware state of the server
// and updates the HardwareDetails field of the host with details. It
// is expected to do this in the least expensive way possible, such as
// reading from a cache.
func (p *demoProvisioner) UpdateHardwareState() (hwState provisioner.HardwareState, err error) {
	p.log.Info("updating hardware state")
	return
}

// Prepare remove existing configuration and set new configuration.
func (p *demoProvisioner) Prepare(_ provisioner.PrepareData, unprepared bool, _ bool) (result provisioner.Result, started bool, err error) {
	hostName := p.objectMeta.Name

	switch hostName {
	case PreparingErrorHost:
		p.log.Info("preparing error host")
		result.ErrorMessage = "preparing failed"

	case PreparingHost:
		p.log.Info("preparing host")
		started = unprepared
		result.Dirty = true
		result.RequeueAfter = time.Second * 5

	default:
		p.log.Info("finished preparing")
		started = true
	}

	return
}

// Adopt notifies the provisioner that the state machine believes the host
// to be currently provisioned, and that it should be managed as such.
func (p *demoProvisioner) Adopt(_ provisioner.AdoptData, _ bool) (result provisioner.Result, err error) {
	p.log.Info("adopting host")
	result.Dirty = false
	return
}

// Provision writes the image from the host spec to the host. It may
// be called multiple times, and should return true for its dirty flag
// until the provisioning operation is completed.
func (p *demoProvisioner) Provision(_ provisioner.ProvisionData, _ bool) (result provisioner.Result, err error) {
	hostName := p.objectMeta.Name
	p.log.Info("provisioning image to host")

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
func (p *demoProvisioner) Deprovision(_ bool) (result provisioner.Result, err error) {
	p.log.Info("deprovisioning host")
	return result, nil
}

// Delete removes the host from the provisioning system. It may be
// called multiple times, and should return true for its dirty flag
// until the deprovisioning operation is completed.
func (p *demoProvisioner) Delete() (result provisioner.Result, err error) {
	p.log.Info("deleting host")
	return result, nil
}

// Detach removes the host from the provisioning system.
// Similar to Delete, but ensures non-interruptive behavior
// for the target system.  It may be called multiple times,
// and should return true for its dirty  flag until the
// deletion operation is completed.
func (p *demoProvisioner) Detach() (result provisioner.Result, err error) {
	p.log.Info("detaching host")
	return result, nil
}

// PowerOn ensures the server is powered on independently of any image
// provisioning operation.
func (p *demoProvisioner) PowerOn(_ bool) (result provisioner.Result, err error) {
	p.log.Info("powering on host")
	return result, nil
}

// PowerOff ensures the server is powered off independently of any image
// provisioning operation.
func (p *demoProvisioner) PowerOff(_ metal3api.RebootMode, _ bool) (result provisioner.Result, err error) {
	p.log.Info("powering off host")
	return result, nil
}

// TryInit always returns true for the demo provisioner.
func (p *demoProvisioner) TryInit() (result bool, err error) {
	return true, nil
}

func (p *demoProvisioner) GetFirmwareSettings(_ bool) (settings metal3api.SettingsMap, schema map[string]metal3api.SettingSchema, err error) {
	p.log.Info("getting BIOS settings")
	return
}

func (p *demoProvisioner) AddBMCEventSubscriptionForNode(_ *metal3api.BMCEventSubscription, _ provisioner.HTTPHeaders) (result provisioner.Result, err error) {
	return result, nil
}

func (p *demoProvisioner) RemoveBMCEventSubscriptionForNode(_ metal3api.BMCEventSubscription) (result provisioner.Result, err error) {
	return result, nil
}

func (p *demoProvisioner) GetFirmwareComponents() (components []metal3api.FirmwareComponentStatus, err error) {
	return components, nil
}
