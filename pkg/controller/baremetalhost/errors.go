package baremetalhost

import (
	"fmt"
)

// InvalidBMCAddressError is returned when the BMC address field
// for a host is invalid or empty
type InvalidBMCAddressError struct {
	message string
}

func (e InvalidBMCAddressError) Error() string {
	return fmt.Sprintf("Invalid BMC address %s",
		e.message)
}

// InvalidBMCSecretError is returned when the BMC secret
// for a host is invalid or empty
type InvalidBMCSecretError struct {
	message string
}

func (e InvalidBMCSecretError) Error() string {
	return fmt.Sprintf("Invalid BMC Secret %s",
		e.message)
}
