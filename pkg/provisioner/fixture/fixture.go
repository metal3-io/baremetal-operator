package fixture

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	"github.com/metal3-io/baremetal-operator/pkg/hardwareutils/bmc"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner"
	logz "sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var log = logz.New().WithName("provisioner").WithName("fixture")
var deprovisionRequeueDelay = time.Second * 10
var provisionRequeueDelay = time.Second * 10
var inspectionRequeueDelay = time.Second * 2

const (
	DefaultRequeueSecs    = 5
	DefaultSizeBytes      = 93
	DefaultClockMegahertz = 3.0
	DefaultRAMMebibytes   = 128
	DefaultGB             = 1024
)

type fixtureHostConfigData struct {
	userData    string
	networkData string
	metaData    string
}

// NewHostConfigData creates new host configuration data.
func NewHostConfigData(userData string, networkData string, metaData string) provisioner.HostConfigData {
	return &fixtureHostConfigData{
		userData:    userData,
		networkData: networkData,
		metaData:    metaData,
	}
}

func (cd *fixtureHostConfigData) UserData() (string, error) {
	return cd.userData, nil
}

func (cd *fixtureHostConfigData) NetworkData() (string, error) {
	return cd.networkData, nil
}

func (cd *fixtureHostConfigData) MetaData() (string, error) {
	return cd.metaData, nil
}

// fixtureProvisioner implements the provisioning.fixtureProvisioner interface
// and uses Ironic to manage the host.
type fixtureProvisioner struct {
	// the provisioning ID for this host
	provID string
	// the bmc credentials
	bmcCreds bmc.Credentials
	// a logger configured for this host
	log logr.Logger
	// an event publisher for recording significant events
	publisher provisioner.EventPublisher
	// state storage for the Host
	state *Fixture
}

type HostFirmwareSettingsMock struct {
	Settings metal3api.SettingsMap
	Schema   map[string]metal3api.SettingSchema
}

type HostFirmwareComponentsMock struct {
	Components []metal3api.FirmwareComponentStatus
}

// Fixture contains persistent state for a particular host.
type Fixture struct {
	// counter to set the provisioner as ready
	BecomeReadyCounter int
	// state to manage deletion
	Deleted bool
	// state to manage DisablePowerOff
	DisablePowerOff bool
	// state to manage power
	PoweredOn bool
	// Has reboot been called
	RebootCalled bool
	// state to manage the two-step adopt process
	adopted bool
	// state to manage provisioning
	image metal3api.Image
	// state to manage inspection
	inspectionStarted bool

	validateError string

	customDeploy *metal3api.CustomDeploy

	HostFirmwareSettings HostFirmwareSettingsMock

	HostFirmwareComponents HostFirmwareComponentsMock
}

// NewProvisioner returns a new Fixture Provisioner.
func (f *Fixture) NewProvisioner(_ context.Context, hostData provisioner.HostData, publisher provisioner.EventPublisher) (provisioner.Provisioner, error) {
	p := &fixtureProvisioner{
		provID:    hostData.ProvisionerID,
		bmcCreds:  hostData.BMCCredentials,
		log:       log.WithValues("host", hostData.ObjectMeta.Name),
		publisher: publisher,
		state:     f,
	}
	return p, nil
}

func (f *Fixture) SetValidateError(message string) {
	f.validateError = message
}

func (p *fixtureProvisioner) HasCapacity() (result bool, err error) {
	return true, nil
}

// Register tests the connection information for the
// host to verify that the location and credentials work.
func (p *fixtureProvisioner) Register(_ provisioner.ManagementAccessData, _, _ bool) (result provisioner.Result, provID string, err error) {
	p.log.Info("testing management access")

	if p.state.validateError != "" {
		result.ErrorMessage = p.state.validateError
		return
	}

	// Fill in the ID of the host in the provisioning system
	if p.provID == "" {
		provID = "temporary-fake-id"
		result.Dirty = true
		result.RequeueAfter = time.Second * DefaultRequeueSecs
		p.publisher("Registered", "Registered new host")
		return
	}

	return
}

func (p *fixtureProvisioner) PreprovisioningImageFormats() ([]metal3api.ImageFormat, error) {
	return nil, nil
}

// InspectHardware updates the HardwareDetails field of the host with
// details of devices discovered on the hardware. It may be called
// multiple times, and should return true for its dirty flag until the
// inspection is completed.
func (p *fixtureProvisioner) InspectHardware(_ provisioner.InspectData, _, _, _ bool) (result provisioner.Result, started bool, details *metal3api.HardwareDetails, err error) {
	// The inspection is ongoing. We'll need to check the fixture
	// status for the server here until it is ready for us to get the
	// inspection details. Simulate that for now by creating the
	// hardware details struct as part of a second pass.
	if p.state.inspectionStarted {
		p.log.Info("continuing inspection by setting details")
		details =
			&metal3api.HardwareDetails{
				RAMMebibytes: DefaultRAMMebibytes * DefaultGB,
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
						SizeBytes:  metal3api.TebiByte * DefaultSizeBytes,
						Model:      "Dell CFJ61",
					},
					{
						Name:       "disk-2",
						Rotational: false,
						SizeBytes:  metal3api.TebiByte * DefaultSizeBytes,
						Model:      "Dell CFJ61",
					},
				},
				CPU: metal3api.CPU{
					Arch:           "x86_64",
					Model:          "FancyPants CPU",
					ClockMegahertz: DefaultClockMegahertz * metal3api.GigaHertz,
					Flags:          []string{"fpu", "hypervisor", "sse", "vmx"},
					Count:          1,
				},
			}
		p.publisher("InspectionComplete", "Hardware inspection completed")
	} else {
		// First pass
		p.log.Info("starting inspection of hardware")
		p.state.inspectionStarted = true
		started = true

		// Introduce a delay to make the inspection more realistic
		result.RequeueAfter = inspectionRequeueDelay
		result.Dirty = true
	}

	return result, started, details, nil
}

// UpdateHardwareState fetches the latest hardware state of the server
// and updates the HardwareDetails field of the host with details. It
// is expected to do this in the least expensive way possible, such as
// reading from a cache.
func (p *fixtureProvisioner) UpdateHardwareState() (hwState provisioner.HardwareState, err error) {
	hwState.PoweredOn = &p.state.PoweredOn
	p.log.Info("updating hardware state")
	return
}

// Prepare remove existing configuration and set new configuration.
func (p *fixtureProvisioner) Prepare(_ provisioner.PrepareData, unprepared bool, _ bool) (result provisioner.Result, started bool, err error) {
	p.log.Info("preparing host", "unprepared", unprepared)
	started = unprepared
	return
}

// Service remove existing configuration and set new configuration.
func (p *fixtureProvisioner) Service(_ provisioner.ServicingData, unprepared bool, _ bool) (result provisioner.Result, started bool, err error) {
	p.log.Info("servicing host", "unprepared", unprepared)
	started = unprepared
	if started {
		result.Dirty = true
	}
	return
}

// Adopt notifies the provisioner that the state machine believes the host
// to be currently provisioned, and that it should be managed as such.
func (p *fixtureProvisioner) Adopt(_ provisioner.AdoptData, _ bool) (result provisioner.Result, err error) {
	p.log.Info("adopting host")
	if !p.state.adopted {
		p.state.adopted = true
		result.Dirty = true
		result.RequeueAfter = provisionRequeueDelay
	}
	return
}

// Provision writes the image from the host spec to the host. It may
// be called multiple times, and should return true for its dirty flag
// until the provisioning operation is completed.
func (p *fixtureProvisioner) Provision(data provisioner.ProvisionData, _ bool) (result provisioner.Result, err error) {
	p.log.Info("provisioning image to host")

	if data.CustomDeploy != nil && p.state.customDeploy == nil {
		p.publisher("ProvisioningComplete", "Custom deploy provisioning completed")
		p.log.Info("moving to done")
		p.state.customDeploy = data.CustomDeploy.DeepCopy()
		result.Dirty = true
		result.RequeueAfter = provisionRequeueDelay
		return result, nil
	}

	if data.Image.URL != "" && p.state.image.URL == "" {
		p.publisher("ProvisioningComplete", "Image provisioning completed")
		p.log.Info("moving to done")
		p.state.image = data.Image
		result.Dirty = true
		result.RequeueAfter = provisionRequeueDelay
	}

	return result, nil
}

// Deprovision removes the host from the image. It may be called
// multiple times, and should return true for its dirty flag until the
// deprovisioning operation is completed.
func (p *fixtureProvisioner) Deprovision(_ provisioner.DeprovisionData, _ bool) (result provisioner.Result, err error) {
	p.log.Info("ensuring host is deprovisioned")

	result.RequeueAfter = deprovisionRequeueDelay

	// NOTE(dhellmann): In order to simulate a multi-step process,
	// modify some of the status data structures. This is likely not
	// necessary once we really have Fixture doing the deprovisioning
	// and we can monitor it's status.

	if p.state.image.URL != "" {
		p.publisher("DeprovisionStarted", "Image deprovisioning started")
		p.log.Info("clearing hardware details")
		p.state.image = metal3api.Image{}
		result.Dirty = true
		return result, nil
	}

	if p.state.customDeploy != nil {
		p.publisher("DeprovisionStarted", "Custom deploy deprovisioning started")
		p.log.Info("clearing hardware details")
		p.state.customDeploy = nil
		result.Dirty = true
		return result, nil
	}

	p.publisher("DeprovisionComplete", "Image deprovisioning completed")
	return result, nil
}

// Delete removes the host from the provisioning system. It may be
// called multiple times, and should return true for its dirty flag
// until the deprovisioning operation is completed.
func (p *fixtureProvisioner) Delete() (result provisioner.Result, err error) {
	p.log.Info("deleting host")

	if !p.state.Deleted {
		p.log.Info("clearing provisioning id")
		p.state.Deleted = true
		result.Dirty = true
		return result, nil
	}

	return result, nil
}

// Detach removes the host from the provisioning system.
// Similar to Delete, but ensures non-interruptive behavior
// for the target system.  It may be called multiple times,
// and should return true for its dirty  flag until the
// deletion operation is completed.
func (p *fixtureProvisioner) Detach() (result provisioner.Result, err error) {
	return p.Delete()
}

// PowerOn ensures the server is powered on independently of any image
// provisioning operation.
func (p *fixtureProvisioner) PowerOn(_ bool) (result provisioner.Result, err error) {
	p.log.Info("ensuring host is powered on")

	if !p.state.PoweredOn {
		p.publisher("PowerOn", "Host powered on")
		p.log.Info("changing status")
		p.state.PoweredOn = true
		result.Dirty = true
		return result, nil
	}

	return result, nil
}

// PowerOff ensures the server is powered off independently of any image
// provisioning operation.
func (p *fixtureProvisioner) PowerOff(_ metal3api.RebootMode, _ bool) (result provisioner.Result, err error) {
	p.log.Info("ensuring host is powered off")

	if p.state.DisablePowerOff {
		p.state.RebootCalled = true
		result.Dirty = true
		return result, nil
	}

	if p.state.PoweredOn {
		p.publisher("PowerOff", "Host powered off")
		p.log.Info("changing status")
		p.state.PoweredOn = false
		result.Dirty = true
		return result, nil
	}

	return result, nil
}

// TryInit returns the current availability status of the provisioner.
func (p *fixtureProvisioner) TryInit() (result bool, err error) {
	p.log.Info("checking provisioner status")

	if p.state.BecomeReadyCounter > 0 {
		p.state.BecomeReadyCounter--
	}

	return p.state.BecomeReadyCounter == 0, nil
}

func (p *fixtureProvisioner) GetFirmwareSettings(_ bool) (settings metal3api.SettingsMap, schema map[string]metal3api.SettingSchema, err error) {
	p.log.Info("getting BIOS settings")
	return p.state.HostFirmwareSettings.Settings, p.state.HostFirmwareSettings.Schema, nil
}

func (p *fixtureProvisioner) AddBMCEventSubscriptionForNode(_ *metal3api.BMCEventSubscription, _ provisioner.HTTPHeaders) (result provisioner.Result, err error) {
	return result, nil
}

func (p *fixtureProvisioner) RemoveBMCEventSubscriptionForNode(_ metal3api.BMCEventSubscription) (result provisioner.Result, err error) {
	return result, nil
}

func (p *fixtureProvisioner) GetFirmwareComponents() (components []metal3api.FirmwareComponentStatus, err error) {
	p.log.Info("getting Firmware components")
	return p.state.HostFirmwareComponents.Components, nil
}

func (p *fixtureProvisioner) GetDataImageStatus() (isImageAttached bool, err error) {
	return false, nil
}

func (p *fixtureProvisioner) AttachDataImage(_ string) (err error) {
	return nil
}

func (p *fixtureProvisioner) DetachDataImage() (err error) {
	return nil
}
