package bmc

import (
	"fmt"
)

// UnknownBMCTypeError is returned when the provided BMC address cannot be
// mapped to a driver.
type UnknownBMCTypeError struct {
	address string
	bmcType string
}

func (e UnknownBMCTypeError) Error() string {
	return fmt.Sprintf("Unknown BMC type '%s' for address %s",
		e.bmcType, e.address)
}

// ValidationError is returned when the provided BMC credentials
// are invalid (e.g. null)
type ValidationError struct {
	message string
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("Validation error with BMC credentials: %s",
		e.message)
}
