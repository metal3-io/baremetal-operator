package controllers

import (
	"fmt"
	"strings"

	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
)

// secretAccessErrorPrefix is the start of SecretAccessError.Error().
// Provisioning status messages with this shape are retried in place.
const secretAccessErrorPrefix = "could not retrieve "

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
	return fmt.Sprintf("%s%s secret %q: %v", secretAccessErrorPrefix, e.key, e.secret, e.err)
}

func (e SecretAccessError) Unwrap() error {
	return e.err
}

// isSecretAccessErrorMessage reports whether message was produced by
// SecretAccessError.Error.
func isSecretAccessErrorMessage(message string) bool {
	return strings.HasPrefix(message, secretAccessErrorPrefix) && strings.Contains(message, " secret ")
}

// isRetryableSecretAccessStatus reports whether the host status was recorded
// because configuration Secret data could not be read. That condition is
// retried without leaving the provisioning state. Other provisioning errors
// are fatal and move the host to deprovisioning.
func isRetryableSecretAccessStatus(host *metal3api.BareMetalHost) bool {
	return host.Status.ErrorType == metal3api.ProvisioningError &&
		isSecretAccessErrorMessage(host.Status.ErrorMessage)
}
