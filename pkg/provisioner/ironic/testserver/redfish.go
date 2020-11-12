package testserver

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
)

// RedfishMock is a test server that implements Redfish semantics
type RedfishMock struct {
	*MockServer
}

// NewRedfish builds a Redfish mock server
func NewRedfish(t *testing.T) *RedfishMock {
	return &RedfishMock{
		MockServer: NewTLS(t, "redfish"),
	}
}

// Endpoint returns the URL to the server
func (m *RedfishMock) Endpoint() string {
	return m.MockServer.RawEndpoint()
}

func (m *RedfishMock) buildURL(url string, method string) string {
	return fmt.Sprintf("%s:%s", url, method)
}

// RootService configures the server with a valid response for /redfish/v1/
func (m *RedfishMock) RootService() *RedfishMock {
	m.ResponseWithCode("/redfish/v1/", `
	{
		"@odata.type": "#ServiceRoot.v1_9_0.ServiceRoot",
		"Id": "RootService",
		"Name": "Root Service",
		"RedfishVersion": "1.6.0",
		"UUID": "92384634-2938-2342-8820-489239905423",
		"Systems": {
			"@odata.id": "/redfish/v1/Systems"
		},
		"Chassis": {
			"@odata.id": "/redfish/v1/Chassis"
		},
		"Managers": {
			"@odata.id": "/redfish/v1/Managers"
		},
		"Tasks": {
			"@odata.id": "/redfish/v1/TaskService"
		},
		"SessionService": {
			"@odata.id": "/redfish/v1/SessionService"
		},
		"AccountService": {
			"@odata.id": "/redfish/v1/AccountService"
		},
		"EventService": {
			"@odata.id": "/redfish/v1/EventService"
		},
		"Registries": {
			"@odata.id": "/redfish/v1/Registries"
		},
		"UpdateService": {
			"@odata.id": "/redfish/v1/UpdateService"
		},
		"CertificateService": {
			"@odata.id": "/redfish/v1/CertificateService"
		},
		"Links": {
			"Sessions": {
				"@odata.id": "/redfish/v1/SessionService/Sessions"
			}
		},
		"Oem": { },
		"@odata.id": "redfish/v1/"
	}`, http.StatusOK)
	return m
}

// Unauthorized configures the server with a http 401 Unauthorized response for /redfish/v1/
func (m *RedfishMock) Unauthorized() *RedfishMock {
	m.ErrorResponse("/redfish/v1/", http.StatusUnauthorized)
	return m
}

func (m *RedfishMock) NoConnection() *RedfishMock {
	m.ErrorResponse("/redfish/v1", http.StatusBadRequest)
	return m
}

// buildMembers generates the members information based on the number of members and endpoint
// the prefix is used to generate unique members name together with the number of members information
func buildOdata(odataType string, numElements int, endpoint string, prefix string) (string, string) {
	var count = fmt.Sprintf(`"%s@odata.count": %d`, odataType, numElements)
	var list = ``
	for element := 1; element <= numElements; element++ {
		list += fmt.Sprintf(`{ "@odata.id": "%s/%s%d" },`, endpoint, prefix, element)
	}
	list = fmt.Sprintf(`"%s": [%s],`, odataType, strings.TrimSuffix(list, ","))
	return list, count
}

// Systems configures the server with a valid response for /redfish/v1/Systems with
// a specific number of system members
func (m *RedfishMock) Systems(systemsCount int) *RedfishMock {
	membersList, odataCount := buildOdata("Members", systemsCount, "/redfish/v1/Systems", "System-")
	m.ResponseWithCode("/redfish/v1/Systems", `
	{
		"@odata.context": "/redfish/v1/$metadata#ComputerSystemCollection.ComputerSystemCollection",
		"@odata.id": "/redfish/v1/Systems",
		"@odata.type": "#ComputerSystemCollection.ComputerSystemCollection",
		"Description": "Collection of Computer Systems",
		`+membersList+`
		`+odataCount+`,
		"Name": "Computer System Collection"
	}`, http.StatusOK)
	return m
}

// GetSystem configures the server with a valid response for /redfish/v1/Systems/<ID>
func (m *RedfishMock) GetSystem(systemURL string, systemPayload string, hasError bool, code int) *RedfishMock {
	if hasError {
		m.ErrorResponse(systemURL, code)
	} else {
		m.ResponseWithCode(systemURL, systemPayload, http.StatusOK)
	}
	return m
}

// GetManager configures the server with a valid response
// for /redfish/v1/Managers/<ID>
func (m *RedfishMock) GetManager(managerURL string, managerPayload string, hasError bool, code int) *RedfishMock {
	m.ResponseWithCode(managerURL, managerPayload, http.StatusOK)
	return m
}

// VirtualMediaInfo represents the basic information to represent a VirtualMedia in Redfish
type VirtualMediaInfo struct {
	Id             string
	MediaTypes     []string
	MissingActions bool
}

// VirtualMedia configures the server with all responses according to the virtual media scenario
func (m *RedfishMock) VirtualMedia(vmediaList []VirtualMediaInfo) *RedfishMock {
	m.ListVirtualMedia(len(vmediaList))
	for _, vmediaInfo := range vmediaList {
		m.CreateVirtualMedia(vmediaInfo)
	}
	return m
}

// ListVirtualMedia configures the server with a valid response for /redfish/v1/Systems with
// a specific number of system members
func (m *RedfishMock) ListVirtualMedia(vmediaMembers int) *RedfishMock {
	membersList, odata := buildOdata("Members", vmediaMembers, "/redfish/v1/Managers/BMC-1/VirtualMedia", "")
	m.ResponseWithCode("/redfish/v1/Managers/BMC-1/VirtualMedia", `
	{
		"@odata.context": "/redfish/v1/$metadata#VirtualMediaCollection.VirtualMediaCollection",
		"@odata.id": "/redfish/v1/Managers/BMC-1/VirtualMedia",
		"@odata.type": "#VirtualMediaCollection.VirtualMediaCollection",
		"Description": "Virtual Media Services Settings",
		`+membersList+`
		`+odata+`,
		"Name": "Virtual Media Services"
	}`, http.StatusOK)
	return m
}

func virtualMediaBody(vmediaInfo VirtualMediaInfo) string {
	var actionsBody = `"Actions": {},`
	if !vmediaInfo.MissingActions {
		ejectTarget := fmt.Sprintf(`"target": "/redfish/v1/Managers/BMC-1/VirtualMedia/%s/Actions/VirtualMedia.EjectMedia"`, vmediaInfo.Id)
		injectTarget := fmt.Sprintf(`"target": "/redfish/v1/Managers/BMC-1/VirtualMedia/%s/Actions/VirtualMedia.InsertMedia"`, vmediaInfo.Id)
		actionsBody = `
		"Actions": {
			"#VirtualMedia.EjectMedia": {
			  ` + ejectTarget + `
			},
			"#VirtualMedia.InsertMedia": {
			  ` + injectTarget + `
			}
		},`
	}
	vmediaOdataId := fmt.Sprintf(`"@odata.id": "/redfish/v1/Managers/BMC-1/VirtualMedia/%s",`, vmediaInfo.Id)
	vmediaID := fmt.Sprintf(`"Id": "%s",`, vmediaInfo.Id)
	vmediaTypes := fmt.Sprintf(`"MediaTypes": [%s],`, strings.Join(vmediaInfo.MediaTypes, ", "))
	vmediaCount := fmt.Sprintf(`"MediaTypes@odata.count": %d`, len(vmediaInfo.MediaTypes))
	var Body = `{
		"@odata.context": "/redfish/v1/$metadata#VirtualMedia.VirtualMedia",
		` + vmediaOdataId + `
		"@odata.type": "#VirtualMedia.v1_2_0.VirtualMedia",
		` + actionsBody + `
		"Description": "Virtual Media Services Settings",
		` + vmediaID + `
		` + vmediaTypes + `
		` + vmediaCount + `
	}`
	return Body
}

// CreateVirtualMedia configures the server with a valid response for the given VirtualMediaInfo
func (m *RedfishMock) CreateVirtualMedia(info VirtualMediaInfo) *RedfishMock {
	responseBody := virtualMediaBody(info)
	m.ResponseWithCode("/redfish/v1/Managers/BMC-1/VirtualMedia/"+info.Id, responseBody, http.StatusOK)
	return m

}
