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

var authStrategy AuthType

// Variables for http_basic
var ironicUser string
var ironicPassword string
var inspectorUser string
var inspectorPassword string

// LoadAuth loads the Ironic and Inspector configuration from the environment
func LoadAuth() error {
	authStrategy = AuthType(os.Getenv("IRONIC_AUTH_STRATEGY"))
	switch authStrategy {
	case "":
		return fmt.Errorf("No IRONIC_AUTH_STRATEGY variable set")
	case NoAuth:
	case HTTPBasicAuth:
		ironicUser = os.Getenv("IRONIC_HTTP_BASIC_USERNAME")
		ironicPassword = os.Getenv("IRONIC_HTTP_BASIC_PASSWORD")
		inspectorUser = os.Getenv("INSPECTOR_HTTP_BASIC_USERNAME")
		inspectorPassword = os.Getenv("INSPECTOR_HTTP_BASIC_PASSWORD")
		if ironicUser == "" {
			return fmt.Errorf("No IRONIC_HTTP_BASIC_USERNAME variable set")
		}
		if ironicPassword == "" {
			return fmt.Errorf("No IRONIC_HTTP_BASIC_PASSWORD variable set")
		}
		if inspectorUser == "" {
			return fmt.Errorf("No INSPECTOR_HTTP_BASIC_USERNAME variable set")
		}
		if inspectorPassword == "" {
			return fmt.Errorf("No INSPECTOR_HTTP_BASIC_PASSWORD variable set")
		}
	default:
		return fmt.Errorf("IRONIC_AUTH_STRATEGY does not have a valid value. Set to %s or %s", NoAuth, HTTPBasicAuth)
	}
	return nil
}
