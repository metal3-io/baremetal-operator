package ironic

// SoftPowerOffUnsupportedError is returned when the BMC does not
// support soft power off.
type SoftPowerOffUnsupportedError struct {
}

func (e SoftPowerOffUnsupportedError) Error() string {
	return "soft power off is unsupported on BMC"
}
