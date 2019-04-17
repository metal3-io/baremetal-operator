package baremetalhost

import (
	"fmt"
)

// EmptyBMCAddressError is returned when the BMC address field
// for a host is empty
type EmptyBMCAddressError struct {
	message string
}

func (e EmptyBMCAddressError) Error() string {
	return fmt.Sprintf("Empty BMC address %s",
		e.message)
}

// EmptyBMCSecretError is returned when the BMC secret
// for a host is empty
type EmptyBMCSecretError struct {
	message string
}

func (e EmptyBMCSecretError) Error() string {
	return fmt.Sprintf("No BMC CredentialsName defined %s",
		e.message)
}

// ResolveBMCSecretRefError is returned when the BMC secret
// for a host is defined but cannot be found
type ResolveBMCSecretRefError struct {
	message string
}

func (e ResolveBMCSecretRefError) Error() string {
	return fmt.Sprintf("BMC CredentialsName secret doesn't exist %s",
		e.message)
}

// SaveBMCSecretOwnerError is returned when we
// fail to set the owner of a secret
type SaveBMCSecretOwnerError struct {
	message string
}

func (e SaveBMCSecretOwnerError) Error() string {
	return fmt.Sprintf("Failed to set owner of BMC secret %s",
		e.message)
}
