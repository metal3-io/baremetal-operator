package bmc

// Credentials holds the information for authenticating with the BMC.
type Credentials struct {
	Username string
	Password string
}

// AreValid returns a boolean indicating whether the credentials are
// valid, and if false a string explaining why not.
func (creds Credentials) AreValid() (bool, error) {
	if creds.Username == "" {
		return false, &ValidationError{message: "Missing BMC connection detail 'username' in credentials"}
	}
	if creds.Password == "" {
		return false, &ValidationError{message: "Missing BMC connection details 'password' in credentials"}
	}
	return true, nil
}
