package provisioner

import (
	"time"

	metal3v1alpha1 "github.com/metal3-io/baremetal-operator/pkg/apis/metal3/v1alpha1"
	"github.com/metal3-io/baremetal-operator/pkg/bmc"
)

/*
Package provisioning defines the API for talking to the provisioning backend.
*/

// EventPublisher is a function type for publishing events associated
// with provisioning.
type EventPublisher func(reason, message string)

// Factory is the interface for creating new Provisioner objects.
type Factory func(host *metal3v1alpha1.BareMetalHost, bmcCreds bmc.Credentials, publish EventPublisher) (Provisioner, error)

// UserDataSource is the interface for a function to retrieve user
// data for a host being provisioned.
type UserDataSource func() (string, error)

// Provisioner holds the state information for talking to the
// provisioning backend.
type Provisioner interface {
	// ValidateManagementAccess tests the connection information for the
	// host to verify that the location and credentials work.
	ValidateManagementAccess() (result Result, err error)

	// InspectHardware updates the HardwareDetails field of the host with
	// details of devices discovered on the hardware. It may be called
	// multiple times, and should return true for its dirty flag until the
	// inspection is completed.
	InspectHardware() (result Result, details *metal3v1alpha1.HardwareDetails, err error)

	// UpdateHardwareState fetches the latest hardware state of the
	// server and updates the HardwareDetails field of the host with
	// details. It is expected to do this in the least expensive way
	// possible, such as reading from a cache, and return dirty only
	// if any state information has changed.
	UpdateHardwareState() (result Result, err error)

	// Provision writes the image from the host spec to the host. It
	// may be called multiple times, and should return true for its
	// dirty flag until the deprovisioning operation is completed.
	Provision(getUserData UserDataSource) (result Result, err error)

	// Deprovision removes the host from the image. It may be called
	// multiple times, and should return true for its dirty flag until
	// the deprovisioning operation is completed.
	Deprovision() (result Result, err error)

	// Delete removes the host from the provisioning system. It may be
	// called multiple times, and should return true for its dirty
	// flag until the deprovisioning operation is completed.
	Delete() (result Result, err error)

	// PowerOn ensures the server is powered on independently of any image
	// provisioning operation.
	PowerOn() (result Result, err error)

	// PowerOff ensures the server is powered off independently of any image
	// provisioning operation.
	PowerOff() (result Result, err error)
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
