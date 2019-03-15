package provisioner

import (
	"time"

	metalkubev1alpha1 "github.com/metalkube/baremetal-operator/pkg/apis/metalkube/v1alpha1"
	"github.com/metalkube/baremetal-operator/pkg/bmc"
)

/*
Package provisioning defines the API for talking to the provisioning backend.
*/

// Factory is the interface for creating new Provisioner objects.
type Factory func(host *metalkubev1alpha1.BareMetalHost, bmcCreds bmc.Credentials) (Provisioner, error)

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
