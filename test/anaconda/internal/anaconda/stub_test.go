// SPDX-License-Identifier: Apache-2.0

package anaconda_test

import (
	"cmp"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

const (
	testHost = "node-1"
	testUID  = "bmh-uid"
	testMAC  = "aa:bb:cc:dd:ee:01"
	testDisk = "vda"
	testISO  = "http://boot.example/ks.iso"

	testFailure = "disk sda not found"

	stubUser = "admin"
	stubPass = "s3cret"
)

// Redfish paths the stub answers on. The stub builds every route from these, so
// a test addressing a different system reaches nothing.
const (
	rootPath    = "/redfish/v1/"
	systemsPath = "/redfish/v1/Systems"
	systemPath  = "/redfish/v1/Systems/1"
)

type liveISOState struct {
	Boot  map[string]any
	Image string
	// RewriteImage stands for a BMC that fetches the image itself and reports
	// back its own URL rather than the one it was handed.
	RewriteImage string
	Resets       []string
	Ejects       int
	mu           sync.Mutex
	// DropInsert and DropBoot accept the write and change nothing, which is how
	// an emulator that never reaches the hypervisor behaves.
	DropInsert bool
	DropBoot   bool
	PowerOn    bool
	Inserted   bool
}

// bootJSON renders the Boot object a compliant BMC echoes back, empty when the
// stub is pretending to have ignored the override.
func (s *liveISOState) bootJSON() string {
	target, _ := s.Boot["BootSourceOverrideTarget"].(string)
	if s.DropBoot {
		target = ""
	}

	return fmt.Sprintf(`"Boot":{"BootSourceOverrideTarget":%q},`, target)
}

func powerStateName(on bool) string {
	if on {
		return "On"
	}

	return "Off"
}

// liveISOService serves a Redfish tree whose virtual media, power state and
// boot override mutate, which is what makes the provision sequence testable.
func liveISOService(t *testing.T, state *liveISOState) *httptest.Server {
	t.Helper()

	const (
		mediaPath  = systemPath + "/VirtualMedia/CD"
		resetPath  = systemPath + "/Actions/ComputerSystem.Reset"
		insertPath = mediaPath + "/Actions/VirtualMedia.InsertMedia"
		ejectPath  = mediaPath + "/Actions/VirtualMedia.EjectMedia"
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != rootPath {
			user, pass, ok := r.BasicAuth()
			if !ok || user != stubUser || pass != stubPass {
				w.WriteHeader(http.StatusUnauthorized)

				return
			}
		}

		state.mu.Lock()
		defer state.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")

		switch r.Method + " " + r.URL.Path {
		case "GET /redfish/v1/":
			// No Managers link, so the system's own collection is all there is,
			// which is the arm Conn.VirtualMedia tries first.
			_, _ = io.WriteString(w, `{"@odata.id":"/redfish/v1/","Id":"RootService",
				"RedfishVersion":"1.6.0","Systems":{"@odata.id":"/redfish/v1/Systems"}}`)

		case "GET /redfish/v1/Systems":
			_, _ = fmt.Fprintf(w, `{"Members@odata.count":1,"Members":[{"@odata.id":%q}]}`, systemPath)

		case "GET " + systemPath:
			_, _ = fmt.Fprintf(w, `{"@odata.id":%q,"Id":"1","Name":"System","PowerState":%q,
				"Status":{"Health":"OK"},"VirtualMedia":{"@odata.id":%q},%s
				"Actions":{"#ComputerSystem.Reset":{"target":%q}}}`,
				systemPath, powerStateName(state.PowerOn), systemPath+"/VirtualMedia",
				state.bootJSON(), resetPath)

		case "PATCH " + systemPath:
			var payload struct {
				Boot map[string]any
			}

			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				w.WriteHeader(http.StatusBadRequest)

				return
			}

			state.Boot = payload.Boot

			w.WriteHeader(http.StatusNoContent)

		case "POST " + resetPath:
			var payload struct {
				ResetType string
			}

			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				w.WriteHeader(http.StatusBadRequest)

				return
			}

			state.Resets = append(state.Resets, payload.ResetType)
			state.PowerOn = payload.ResetType == "On"

			w.WriteHeader(http.StatusNoContent)

		case "GET " + systemPath + "/VirtualMedia":
			_, _ = fmt.Fprintf(w, `{"Members@odata.count":1,"Members":[{"@odata.id":%q}]}`, mediaPath)

		case "GET " + mediaPath:
			_, _ = fmt.Fprintf(w, `{"@odata.id":%q,"Id":"CD","MediaTypes":["CD","DVD"],
				"Inserted":%t,"Image":%q,
				"Actions":{"#VirtualMedia.InsertMedia":{"target":%q},
					"#VirtualMedia.EjectMedia":{"target":%q}}}`,
				mediaPath, state.Inserted, state.Image, insertPath, ejectPath)

		case "POST " + insertPath:
			var payload struct {
				Image string
			}

			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				w.WriteHeader(http.StatusBadRequest)

				return
			}

			// Accepted either way, the drive only changes when the stub is not
			// standing in for a BMC that never reaches its hypervisor.
			if !state.DropInsert {
				state.Inserted = true
				state.Image = cmp.Or(state.RewriteImage, payload.Image)
			}

			w.WriteHeader(http.StatusNoContent)

		case "POST " + ejectPath:
			state.Inserted = false
			state.Image = ""
			state.Ejects++

			w.WriteHeader(http.StatusNoContent)

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)

	return srv
}
