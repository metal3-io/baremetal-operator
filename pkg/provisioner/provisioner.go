package provisioner

import (
	"time"

	metalkubev1alpha1 "github.com/metalkube/baremetal-operator/pkg/apis/metalkube/v1alpha1"
	"github.com/metalkube/baremetal-operator/pkg/bmc"
)

/*
Package provisioning defines the API for talking to the provisioning backend.
*/

// EventPublisher is a function type for publishing events associated
// with provisioning.
type EventPublisher func(reason, message string)

// Factory is the interface for creating new Provisioner objects.
type Factory func(host *metalkubev1alpha1.BareMetalHost, bmcCreds bmc.Credentials, publish EventPublisher) (Provisioner, error)

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
	InspectHardware() (result Result, err error)

	// UpdateHardwareState fetches the latest hardware state of the
	// server and updates the HardwareDetails field of the host with
	// details. It is expected to do this in the least expensive way
	// possible, such as reading from a cache, and return dirty only
	// if any state information has changed.
	UpdateHardwareState() (result Result, err error)

	// Provision writes the image from the host spec to the host. It
	// may be called multiple times, and should return true for its
	// dirty flag until the deprovisioning operation is completed.
	Provision(userData string) (result Result, err error)

	// Deprovision prepares the host to be removed from the cluster. It
	// may be called multiple times, and should return true for its dirty
	// flag until the deprovisioning operation is completed.
	Deprovision(deleteIt bool) (result Result, err error)

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
}

const (
	// StateNone means the state is unknown
	StateNone = ""
	// StateRegistrationError means there was an error registering the
	// host with the backend
	StateRegistrationError = "registration error"
	// StateRegistering means we are telling the backend about the host
	StateRegistering = "registering"
	// StateReady means the host can be consumed
	StateReady = "ready"
	// StatePreparingToProvision means we are updating the host to
	// receive its image
	StatePreparingToProvision = "preparing to provision"
	// StateMakingAvailable means we are making the host available to
	// be provisioned
	StateMakingAvailable = "making host available"
	// StateValidationError means the provisioning instructions had an
	// error
	StateValidationError = "validation error"
	// StateProvisioning means we are writing an image to the host's
	// disk(s)
	StateProvisioning = "provisioning"
	// StateProvisioned means we have written an image to the host's
	// disk(s)
	StateProvisioned = "provisioned"
	// StateDeprovisioning means we are removing an image from the
	// host's disk(s)
	StateDeprovisioning = "deprovisioning"
)
