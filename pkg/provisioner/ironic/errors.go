package ironic

// SoftPowerOffUnsupportedError is returned when the BMC does not
// support soft power off.
type SoftPowerOffUnsupportedError struct {
}

func (e SoftPowerOffUnsupportedError) Error() string {
	return "soft power off is unsupported on BMC"
}

// SoftPowerOffFailed is returned when the soft power off command
// finishes with failure.
type SoftPowerOffFailed struct {
}

func (e SoftPowerOffFailed) Error() string {
	return "Soft power off has failed on BMC"
}

// HostLockedError is returned when the BMC host is
// locked.
type HostLockedError struct {
}

func (e HostLockedError) Error() string {
	return "BMC host is locked"
}
