package bmc

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/stmcginnis/gofish"
	"github.com/stmcginnis/gofish/redfish"
)

// contains tells whether the array of redfish.VirtualMediaType
// has the element redfish.VirtualMediaType.
func contains(array []redfish.VirtualMediaType, element redfish.VirtualMediaType) bool {
	for _, n := range array {
		if element == n {
			return true
		}
	}
	return false
}

// checkVirtualMedia tells if all virtual media of a given Manager
// has the correct type and actions to work with virtual media boot.
func checkVirtualMedia(m *redfish.Manager) error {
	vmediaList, err := m.VirtualMedia()
	if err != nil {
		return err

	}
	if len(vmediaList) == 0 {
		return fmt.Errorf("no virtual media found for manager %s", m.ID)
	}

	var vmediaError error
	for _, vmedia := range vmediaList {
		if !contains(vmedia.MediaTypes, redfish.CDMediaType) {
			vmediaError = fmt.Errorf("virtual media %s from manager %s does not contain "+
				"CD MediaType", vmedia.ID, m.ID)
			continue
		}
		if !vmedia.SupportsMediaEject || !vmedia.SupportsMediaInsert {
			vmediaError = fmt.Errorf("virtual media %s from manager %s does not support "+
				"at least one of following Actions: InsertMedia, EjectMedia", vmedia.ID, m.ID)
			continue
		}
		return nil
	}
	return vmediaError
}

// checkUnauthorized tells if an error message contains 401 HTTP code.
func checkUnauthorized(errorMessage string) bool {
	if strings.Contains(errorMessage, "401") {
		return true
	}
	return false
}

// checkHPUnauthorized tells if an error message contains 400 HTTP code
// and UnauthorizedLoginAttempt string
func checkHPUnauthorized(errorMessage string) bool {
	if strings.Contains(errorMessage, "400") && strings.Contains(errorMessage, "UnauthorizedLoginAttempt") {
		return true
	}
	return false
}

// ValidateVirtualMedia returns nil when the BMC has support
// for VirtualMedia and error when the BMC does not have support.
func ValidateVirtualMedia(address string, user string, password string, systemURI string, insecure bool, httpClient *http.Client) error {
	bmcConfig := gofish.ClientConfig{
		Endpoint:   address,
		Username:   user,
		Password:   password,
		Insecure:   insecure,
		HTTPClient: httpClient,
	}

	client, err := gofish.Connect(bmcConfig)
	if err != nil {
		if strings.Contains(err.Error(), "i/o timeout") {
			return &AccessVirtualMediaValidationError{message: "timeout reached when trying to access redfish BMC"}
		}
		if checkUnauthorized(err.Error()) || checkHPUnauthorized(err.Error()) {
			return fmt.Errorf("invalid credentials to access the BMC")
		}
		return &AccessVirtualMediaValidationError{message: "could not connect to redfish BMC"}
	}
	defer client.Logout()

	system, err := redfish.GetComputerSystem(client, systemURI)
	if err != nil {
		return fmt.Errorf("system %s not found", systemURI)
	}

	if len(system.ManagedBy) == 0 {
		return fmt.Errorf("system %s is not managed by any manager", system.ID)
	}

	var validationError error
	for _, availableManager := range system.ManagedBy {
		manager, err := redfish.GetManager(client, availableManager)
		if err != nil {
			validationError = fmt.Errorf("could not access manager %s for system %s",
				manager.ID, system.ID)
			continue
		}
		validationError = checkVirtualMedia(manager)
		if validationError == nil {
			return validationError
		}

	}
	return validationError

}
