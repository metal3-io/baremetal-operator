package clients

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strings"
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

func authRoot() string {
	env := os.Getenv("METAL3_AUTH_ROOT_DIR")
	if env != "" {
		return filepath.Clean(env)
	}
	return "/opt/metal3/auth"
}

func readAuthFile(filename string) (string, error) {
	content, err := ioutil.ReadFile(filepath.Clean(filename))
	return strings.TrimSpace(string(content)), err
}

func load(clientType string) (auth AuthConfig, err error) {
	authPath := path.Join(authRoot(), clientType)

	if _, err := os.Stat(authPath); err != nil {
		if os.IsNotExist(err) {
			auth.Type = NoAuth
			return auth, nil
		}
		return auth, err
	}
	auth.Type = HTTPBasicAuth

	auth.Username, err = readAuthFile(path.Join(authPath, "username"))
	if err != nil {
		return
	}

	auth.Password, err = readAuthFile(path.Join(authPath, "password"))
	if err != nil {
		return
	}

	if auth.Username == "" {
		err = fmt.Errorf("Empty HTTP Basic Auth username")
	} else if auth.Password == "" {
		err = fmt.Errorf("Empty HTTP Basic Auth password")
	}
	return
}

// LoadAuth loads the Ironic and Inspector configuration from the environment
func LoadAuth() (ironicAuth, inspectorAuth AuthConfig, err error) {
	ironicAuth, err = load("ironic")
	if err != nil {
		return
	}
	inspectorAuth, err = load("ironic-inspector")
	return
}
