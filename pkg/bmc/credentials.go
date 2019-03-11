package bmc

const (
	// MissingCredentialsMsg is returned as a validation failure
	// reason when there are no credentials at all.
	MissingCredentialsMsg string = "Missing BMC connection details: Credentials"

	// MissingAddressMsg is returned as a validation failure
	// reason when there is no address for the BMC.
	MissingAddressMsg string = "Missing BMC connection details: Address"

	// MissingUsernameMsg is returned as a validation failure reason
	// when the credentials do not include a "username" field.
	MissingUsernameMsg string = "Missing BMC connection details: 'username' in credentials"

	// MissingPasswordMsg is returned as a validation failure reason
	// when the credentials do not include a "password" field.
	MissingPasswordMsg string = "Missing BMC connection details: 'password' in credentials"
)

// Credentials holds the information for authenticating with the BMC.
type Credentials struct {
	Username string
	Password string
}

// AreValid returns a boolean indicating whether the credentials are
// valid, and if false a string explaining why not.
func (creds Credentials) AreValid() (bool, string) {
	if creds.Username == "" {
		return false, MissingUsernameMsg
	}
	if creds.Password == "" {
		return false, MissingPasswordMsg
	}
	return true, ""
}
