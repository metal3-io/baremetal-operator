package provisioner

import (
	"errors"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	metal3v1alpha1 "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	"github.com/metal3-io/baremetal-operator/pkg/bmc"
	"github.com/metal3-io/baremetal-operator/pkg/hardware"
)

/*
Package provisioning defines the API for talking to the provisioning backend.
*/

// EventPublisher is a function type for publishing events associated
// with provisioning.
type EventPublisher func(reason, message string)

type HostData struct {
	ObjectMeta                     metav1.ObjectMeta
	BMCAddress                     string
	BMCCredentials                 bmc.Credentials
	DisableCertificateVerification bool
	BootMACAddress                 string
	ProvisionerID                  string
}

func BuildHostData(host metal3v1alpha1.BareMetalHost, bmcCreds bmc.Credentials) HostData {
	return HostData{
		ObjectMeta:                     *host.ObjectMeta.DeepCopy(),
		BMCAddress:                     host.Spec.BMC.Address,
		BMCCredentials:                 bmcCreds,
		DisableCertificateVerification: host.Spec.BMC.DisableCertificateVerification,
		BootMACAddress:                 host.Spec.BootMACAddress,
		ProvisionerID:                  host.Status.Provisioning.ID,
	}
}

// Factory is the interface for creating new Provisioner objects.
type Factory func(hostData HostData, publish EventPublisher) (Provisioner, error)

// HostConfigData retrieves host configuration data
type HostConfigData interface {
	// UserData is the interface for a function to retrieve user
	// data for a host being provisioned.
	UserData() (string, error)

	// NetworkData is the interface for a function to retrieve netwok
	// configuration for a host.
	NetworkData() (string, error)

	// MetaData is the interface for a function to retrieve metadata
	// configuration for a host.
	MetaData() (string, error)
}

type ManagementAccessData struct {
	BootMode              metal3v1alpha1.BootMode
	AutomatedCleaningMode metal3v1alpha1.AutomatedCleaningMode
	State                 metal3v1alpha1.ProvisioningState
	CurrentImage          *metal3v1alpha1.Image
	CurrentRAIDConfig     *metal3v1alpha1.RAIDConfig
}

type AdoptData struct {
	State metal3v1alpha1.ProvisioningState
}

type InspectData struct {
	BootMode metal3v1alpha1.BootMode
}

type PrepareData struct {
	RAIDConfig         *metal3v1alpha1.RAIDConfig
	HasRootDeviceHints bool
	FirmwareConfig     *metal3v1alpha1.FirmwareConfig
}

type ProvisionData struct {
	Image           metal3v1alpha1.Image
	HostConfig      HostConfigData
	BootMode        metal3v1alpha1.BootMode
	HardwareProfile hardware.Profile
	RootDeviceHints *metal3v1alpha1.RootDeviceHints
}

// Provisioner holds the state information for talking to the
// provisioning backend.
type Provisioner interface {
	// ValidateManagementAccess tests the connection information for
	// the host to verify that the location and credentials work. The
	// boolean argument tells the provisioner whether the current set
	// of credentials it has are different from the credentials it has
	// previously been using, without implying that either set of
	// credentials is correct.
	ValidateManagementAccess(data ManagementAccessData, credentialsChanged, force bool) (result Result, provID string, err error)

	// InspectHardware updates the HardwareDetails field of the host with
	// details of devices discovered on the hardware. It may be called
	// multiple times, and should return true for its dirty flag until the
	// inspection is completed.
	InspectHardware(data InspectData, force, refresh bool) (result Result, started bool, details *metal3v1alpha1.HardwareDetails, err error)

	// UpdateHardwareState fetches the latest hardware state of the
	// server and updates the HardwareDetails field of the host with
	// details. It is expected to do this in the least expensive way
	// possible, such as reading from a cache.
	UpdateHardwareState() (hwState HardwareState, err error)

	// Adopt brings an externally-provisioned host under management by
	// the provisioner.
	Adopt(data AdoptData, force bool) (result Result, err error)

	// Prepare remove existing configuration and set new configuration
	Prepare(data PrepareData, unprepared bool) (result Result, started bool, err error)

	// Provision writes the image from the host spec to the host. It
	// may be called multiple times, and should return true for its
	// dirty flag until the deprovisioning operation is completed.
	Provision(data ProvisionData) (result Result, err error)

	// Deprovision removes the host from the image. It may be called
	// multiple times, and should return true for its dirty flag until
	// the deprovisioning operation is completed.
	Deprovision(force bool) (result Result, err error)

	// Delete removes the host from the provisioning system. It may be
	// called multiple times, and should return true for its dirty
	// flag until the deletion operation is completed.
	Delete() (result Result, err error)

	// Detach removes the host from the provisioning system.
	// Similar to Delete, but ensures non-interruptive behavior
	// for the target system.  It may be called multiple times,
	// and should return true for its dirty  flag until the
	// deletion operation is completed.
	Detach() (result Result, err error)

	// PowerOn ensures the server is powered on independently of any image
	// provisioning operation.
	PowerOn() (result Result, err error)

	// PowerOff ensures the server is powered off independently of any image
	// provisioning operation. The boolean argument may be used to specify
	// if a hard reboot (force power off) is required - true if so.
	PowerOff(rebootMode metal3v1alpha1.RebootMode) (result Result, err error)

	// IsReady checks if the provisioning backend is available to accept
	// all the incoming requests.
	IsReady() (result bool, err error)

	// HasCapacity checks if the backend has a free (de)provisioning slot for the current host
	HasCapacity() (result bool, err error)
}

// Result holds the response from a call in the Provsioner API.
type Result struct {
	// Dirty indicates whether the host object needs to be saved.
	Dirty bool
	// RequeueAfter indicates how long to wait before making the same
	// Provisioner call again. The request should only be requeued if
	// Dirty is also true.
	RequeueAfter time.Duration
	// Any error message produced by the provisioner.
	ErrorMessage string
}

// HardwareState holds the response from an UpdateHardwareState call
type HardwareState struct {
	// PoweredOn is a pointer to a bool indicating whether the Host is currently
	// powered on. The value is nil if the power state cannot be determined.
	PoweredOn *bool
}

// ErrNeedsRegistration raised if the host is not registered
var ErrNeedsRegistration = errors.New("Host not registered")
