package controllers

import (
	"fmt"
)

// EmptyBMCAddressError is returned when the BMC address field
// for a host is empty.
type EmptyBMCAddressError struct {
	message string
}

func (e EmptyBMCAddressError) Error() string {
	return "Empty BMC address " + e.message
}

// EmptyBMCSecretError is returned when the BMC secret
// for a host is empty.
type EmptyBMCSecretError struct {
	message string
}

func (e EmptyBMCSecretError) Error() string {
	return "No BMC CredentialsName defined " + e.message
}

// ResolveBMCSecretRefError is returned when the BMC secret
// for a host is defined but cannot be found.
type ResolveBMCSecretRefError struct {
	message string
}

func (e ResolveBMCSecretRefError) Error() string {
	return "BMC CredentialsName secret doesn't exist " + e.message
}

// NoDataInSecretError is returned when host configuration
// data were not found in referenced secret.
type NoDataInSecretError struct {
	secret string
	key    string
}

func (e NoDataInSecretError) Error() string {
	return fmt.Sprintf("Secret %s does not contain key %s", e.secret, e.key)
}

// SecretAccessError is returned when a Secret referenced by the host for
// configuration data (userData, networkData, or metaData) cannot be retrieved
// because it does not exist or is not accessible. It wraps the underlying error
// so callers can still inspect it with errors.Is/As (for example
// k8serrors.IsNotFound).
type SecretAccessError struct {
	secret string
	key    string
	err    error
}

func (e SecretAccessError) Error() string {
	return fmt.Sprintf("could not retrieve %s secret %q: %v", e.key, e.secret, e.err)
}

func (e SecretAccessError) Unwrap() error {
	return e.err
}
