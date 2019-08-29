package ironic

import (
	"fmt"
)

// SoftPowerOffUnsupportedError is returned when the BMC does not
// support soft power off.
type SoftPowerOffUnsupportedError struct {
	Address string
}

func (e SoftPowerOffUnsupportedError) Error() string {
	return fmt.Sprintf("Soft power off is unsupported on BMC %s",
		e.Address)
}

// SoftPowerOffFailed is returned when the soft power off command
// finishes with failure.
type SoftPowerOffFailed struct {
	Address string
}

func (e SoftPowerOffFailed) Error() string {
	return fmt.Sprintf("Soft power off has failed on BMC %s",
		e.Address)
}

// HostLockedError is returned when the BMC host is
// locked.
type HostLockedError struct {
	Address string
}

func (e HostLockedError) Error() string {
	return fmt.Sprintf("BMC %s host is locked",
		e.Address)
}
