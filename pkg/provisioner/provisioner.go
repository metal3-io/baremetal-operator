package provisioner

import (
	"time"

	metalkubev1alpha1 "github.com/metalkube/baremetal-operator/pkg/apis/metalkube/v1alpha1"
	"github.com/metalkube/baremetal-operator/pkg/bmc"
)

/*
Package provisioning defines the API for talking to the provisioning backend.
*/

type ProvisionerFactory interface {
	// New creates a new Provisioner for a given host.
	New(host *metalkubev1alpha1.BareMetalHost, bmcCreds bmc.Credentials) (Provisioner, error)
}

// Provisioner holds the state information for talking to the
// provisioning backend.
type Provisioner interface {
	// ValidateManagementAccess tests the connection information for the
	// host to verify that the location and credentials work.
	ValidateManagementAccess() (dirty bool, err error)

	// InspectHardware updates the HardwareDetails field of the host with
	// details of devices discovered on the hardware. It may be called
	// multiple times, and should return true for its dirty flag until the
	// inspection is completed.
	InspectHardware() (dirty bool, err error)

	// Deprovision prepares the host to be removed from the cluster. It
	// may be called multiple times, and should return true for its dirty
	// flag until the deprovisioning operation is completed.
	Deprovision() (dirty bool, requeueDelay time.Duration, err error)

	// PowerOn ensures the server is powered on independently of any image
	// provisioning operation.
	PowerOn() (dirty bool, err error)

	// PowerOff ensures the server is powered off independently of any image
	// provisioning operation.
	PowerOff() (dirty bool, err error)
}
