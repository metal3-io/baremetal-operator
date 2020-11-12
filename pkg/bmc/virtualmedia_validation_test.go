package bmc

import (
	"fmt"
	"strings"
	"testing"

	"github.com/metal3-io/baremetal-operator/pkg/provisioner/ironic/testserver"

	"github.com/stretchr/testify/assert"
)

type redfishSystem struct {
	ID          string
	hasManageBy bool
	hasErrors   bool
	httpCode    int
}

type redfishManager struct {
	ID              string
	hasVirtualMedia bool
	hasErrors       bool
	httpCode        int
}

func (rs *redfishSystem) build() (url string, payload string, hasErrors bool, code int) {
	url = "/redfish/v1/Systems/" + rs.ID
	hasErrors = rs.hasErrors
	code = rs.httpCode
	var links string

	if rs.hasManageBy {
		var managedByList, odataCount = buildOdata("ManagedBy", 1, "/redfish/v1/Managers", "BMC-")
		links = `"Links": {` + managedByList + `` + odataCount + `}`
	} else {
		var managedByList, odataCount = buildOdata("ManagedBy", 0, "/redfish/v1/Managers", "BMC-")
		links = `"Links": {` + managedByList + `` + odataCount + `}`
	}
	odataID := fmt.Sprintf(`"@odata.id": "/redfish/v1/Systems/%s",`, rs.ID)
	id := fmt.Sprintf(`"Id": "%s",`, rs.ID)
	payload = `{
		"@odata.context": "/redfish/v1/$metadata#ComputerSystem.ComputerSystem",
		` + odataID + `
		"@odata.type": "#ComputerSystem.v1_5_0.ComputerSystem",
		` + id + `
		` + links + `
	}`
	return
}

func (rm *redfishManager) build() (url string, payload string, hasErrors bool, code int) {
	url = "/redfish/v1/Managers/" + rm.ID
	hasErrors = rm.hasErrors
	code = rm.httpCode

	var vmedia string
	if rm.hasVirtualMedia {
		vmedia = `"VirtualMedia": {"@odata.id": "/redfish/v1/Managers/BMC-1/VirtualMedia"}`
	} else {
		vmedia = `"VirtualMedia": {}`
	}
	odataID := fmt.Sprintf(`"@odata.id": "/redfish/v1/Managers/%s",`, rm.ID)
	id := fmt.Sprintf(`"Id": "%s",`, rm.ID)
	payload = `{
		"@odata.context": "/redfish/v1/$metadata#Manager.Manager",
		` + odataID + `
		"@odata.type": "#Manager.v1_3_3.Manager",
		` + id + `
		` + vmedia + `
	  }`
	return
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

func TestValidateVirtualMedia(t *testing.T) {

	// Pre-build Systems
	systemNotFound := &redfishSystem{
		ID:        "System-2",
		hasErrors: true,
		httpCode:  404,
	}
	systemNoManagedBy := &redfishSystem{
		ID:          "System-1",
		hasManageBy: false,
		hasErrors:   false,
		httpCode:    200,
	}

	systemManagedBy := &redfishSystem{
		ID:          "System-1",
		hasManageBy: true,
		hasErrors:   false,
		httpCode:    200,
	}

	//Pre-build Managers
	managerNoVM := &redfishManager{
		ID:              "BMC-1",
		hasErrors:       false,
		hasVirtualMedia: false,
		httpCode:        200,
	}
	managerVM := &redfishManager{
		ID:              "BMC-1",
		hasErrors:       false,
		hasVirtualMedia: true,
		httpCode:        200,
	}

	cases := []struct {
		name    string
		redfish *testserver.RedfishMock

		user         string
		password     string
		systemURI    string
		errorMessage string
	}{
		{
			name:         "invalid credentials",
			redfish:      testserver.NewRedfish(t).Unauthorized(),
			user:         "",
			password:     "",
			errorMessage: "invalid credentials to access the BMC",
		},
		{
			name:         "could not connect to BMC",
			redfish:      testserver.NewRedfish(t).NoConnection(),
			errorMessage: "could not connect to redfish BMC",
		},
		{
			name:         "redfish without Systems",
			redfish:      testserver.NewRedfish(t).RootService().GetSystem(systemNotFound.build()),
			user:         "admin",
			password:     "admin",
			systemURI:    "/redfish/v1/Systems/System-2",
			errorMessage: "system /redfish/v1/Systems/System-2 not found",
		},
		{
			name:         "redfish System without Managers",
			redfish:      testserver.NewRedfish(t).RootService().GetSystem(systemNoManagedBy.build()),
			user:         "admin",
			password:     "admin",
			systemURI:    "/redfish/v1/Systems/System-1",
			errorMessage: "system System-1 is not managed by any manager",
		},
		{
			name:         "redfish Manager without virtual media entry",
			redfish:      testserver.NewRedfish(t).RootService().GetSystem(systemManagedBy.build()).GetManager(managerNoVM.build()),
			user:         "admin",
			password:     "admin",
			systemURI:    "/redfish/v1/Systems/System-1",
			errorMessage: "no virtual media found for manager BMC-1",
		},
		{
			name: "redfish vmedia without CD",
			redfish: testserver.NewRedfish(t).RootService().Systems(1).GetSystem(systemManagedBy.build()).GetManager(managerVM.build()).VirtualMedia(
				[]testserver.VirtualMediaInfo{
					{
						Id:             "1",
						MediaTypes:     []string{`"USBStick"`},
						MissingActions: false,
					},
				}),
			user:         "admin",
			password:     "admin",
			systemURI:    "/redfish/v1/Systems/System-1",
			errorMessage: "virtual media 1 from manager BMC-1 does not contain CD MediaType",
		},
		{
			name: "redfish vmedia with missing actions",
			redfish: testserver.NewRedfish(t).RootService().Systems(1).GetSystem(systemManagedBy.build()).GetManager(managerVM.build()).VirtualMedia([]testserver.VirtualMediaInfo{
				{
					Id:             "1",
					MediaTypes:     []string{`"CD"`, `"DVD"`},
					MissingActions: true,
				},
			}),
			user:      "admin",
			password:  "admin",
			systemURI: "/redfish/v1/Systems/System-1",
			errorMessage: "virtual media 1 from manager BMC-1 does not support " +
				"at least one of following Actions: InsertMedia, EjectMedia",
		},
		{
			name: "virtual media is supported",
			redfish: testserver.NewRedfish(t).RootService().Systems(1).GetSystem(systemManagedBy.build()).GetManager(managerVM.build()).VirtualMedia(
				[]testserver.VirtualMediaInfo{
					{
						Id:             "1",
						MediaTypes:     []string{`"USBStick"`},
						MissingActions: false,
					},
					{
						Id:             "2",
						MediaTypes:     []string{`"CD"`, `"DVD"`},
						MissingActions: false,
					},
				}),
			user:         "admin",
			password:     "admin",
			systemURI:    "/redfish/v1/Systems/System-1",
			errorMessage: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.redfish != nil {
				tc.redfish.Start()
				defer tc.redfish.Stop()
			}
			ip := getRedfishAddress("redfish-virtualmedia", tc.redfish.Endpoint())
			validation := ValidateVirtualMedia(ip, tc.user, tc.password, tc.systemURI,
				true, tc.redfish.Client())

			if tc.errorMessage == "" {
				assert.NoError(t, validation)
				assert.Nil(t, validation)
			} else {
				assert.Error(t, validation)
				assert.Equal(t, tc.errorMessage, validation.Error())
			}
		})
	}
}
