package ironic

// SoftPowerOffUnsupportedError is returned when the BMC does not
// support soft power off.
type SoftPowerOffUnsupportedError struct {
}

func (e SoftPowerOffUnsupportedError) Error() string {
	return "soft power off is unsupported on BMC"
}

// HostLockedError is returned when the BMC host is
// locked.
type HostLockedError struct {
}

func (e HostLockedError) Error() string {
	return "BMC host is locked"
}
