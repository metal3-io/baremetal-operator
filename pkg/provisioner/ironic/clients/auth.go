package clients

import (
	"fmt"
	"os"
)

// AuthType is the method of authenticating requests to the server
type AuthType string

const (
	// NoAuth uses no authentication
	NoAuth AuthType = "noauth"
	// HTTPBasicAuth uses HTTP Basic Authentication
	HTTPBasicAuth AuthType = "http_basic"
)

// AuthConfig contains data needed to configure authentication in the client
type AuthConfig struct {
	Type     AuthType
	Username string
	Password string
}

// LoadAuth loads the Ironic and Inspector configuration from the environment
func LoadAuth() (ironicAuth, inspectorAuth AuthConfig, err error) {
	strategy := AuthType(os.Getenv("IRONIC_AUTH_STRATEGY"))
	ironicAuth.Type = strategy
	inspectorAuth.Type = strategy
	switch strategy {
	case "":
		err = fmt.Errorf("No IRONIC_AUTH_STRATEGY variable set")
	case NoAuth:
	case HTTPBasicAuth:
		ironicAuth.Username = os.Getenv("IRONIC_HTTP_BASIC_USERNAME")
		ironicAuth.Password = os.Getenv("IRONIC_HTTP_BASIC_PASSWORD")
		inspectorAuth.Username = os.Getenv("INSPECTOR_HTTP_BASIC_USERNAME")
		inspectorAuth.Password = os.Getenv("INSPECTOR_HTTP_BASIC_PASSWORD")
		switch {
		case ironicAuth.Username == "":
			err = fmt.Errorf("No IRONIC_HTTP_BASIC_USERNAME variable set")
		case ironicAuth.Password == "":
			err = fmt.Errorf("No IRONIC_HTTP_BASIC_PASSWORD variable set")
		case inspectorAuth.Username == "":
			err = fmt.Errorf("No INSPECTOR_HTTP_BASIC_USERNAME variable set")
		case inspectorAuth.Password == "":
			err = fmt.Errorf("No INSPECTOR_HTTP_BASIC_PASSWORD variable set")
		}
	default:
		err = fmt.Errorf("IRONIC_AUTH_STRATEGY does not have a valid value. Set to %s or %s", NoAuth, HTTPBasicAuth)
	}
	return
}
