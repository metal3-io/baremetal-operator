package bmc

const (
	MissingCredentialsMsg string = "Missing BMC connection details: Credentials"
	MissingUsernameMsg    string = "Missing BMC connection details: 'username' in credentials"
	MissingPasswordMsg    string = "Missing BMC connection details: 'password' in credentials"
	MissingAddressMsg     string = "Missing BMC connection details: Address"
)

type Credentials struct {
	Username string
	Password string
}

func (creds Credentials) AreValid() (bool, string) {
	if creds.Username == "" {
		return false, MissingUsernameMsg
	}
	if creds.Password == "" {
		return false, MissingPasswordMsg
	}
	return true, ""
}
