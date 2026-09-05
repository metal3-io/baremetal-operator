// SPDX-License-Identifier: Apache-2.0

package anaconda_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"metal3.local/anaconda/internal/anaconda"
)

// twoSystemService serves a BMC fronting two machines, like sushy-tools does,
// recording which system each reset was aimed at.
func twoSystemService(t *testing.T, resets map[string]int) *httptest.Server {
	t.Helper()

	var mu sync.Mutex

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.URL.Path == rootPath:
			_, _ = fmt.Fprint(w, `{"@odata.id":"/redfish/v1/","Systems":{"@odata.id":"/redfish/v1/Systems"}}`)

		case r.URL.Path == systemsPath:
			_, _ = fmt.Fprint(w, `{"Members@odata.count":2,"Members":[
				{"@odata.id":"/redfish/v1/Systems/1"},{"@odata.id":"/redfish/v1/Systems/2"}]}`)

		case strings.HasPrefix(r.URL.Path, "/redfish/v1/Systems/") && strings.HasSuffix(r.URL.Path, "/Actions/ComputerSystem.Reset"):
			id := strings.Split(r.URL.Path, "/")[4]
			resets[id]++

			w.WriteHeader(http.StatusNoContent)

		case strings.HasPrefix(r.URL.Path, "/redfish/v1/Systems/"):
			id := strings.TrimPrefix(r.URL.Path, "/redfish/v1/Systems/")
			_, _ = fmt.Fprintf(w, `{"@odata.id":%q,"Id":%q,"PowerState":"Off","Status":{"Health":"OK"},
				"Actions":{"#ComputerSystem.Reset":{"target":"%s/Actions/ComputerSystem.Reset"}}}`,
				r.URL.Path, id, r.URL.Path)

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)

	return srv
}

// addressing points a provisioner at one system of a multi system BMC, which is
// the only thing that decides which machine a power action reaches.
func addressing(srv *httptest.Server, systemID string) func(*anaconda.Provisioner) {
	return func(p *anaconda.Provisioner) {
		p.HostData.BMCAddress = "redfish-virtualmedia+http://" + strings.TrimPrefix(srv.URL, "http://") + systemID
	}
}

// A BMC fronting several machines lists them all, so the system named in the
// address is all that separates this host from somebody else's machine.
func TestPowerOnTargetsTheSystemInTheAddress(t *testing.T) {
	resets := map[string]int{}
	srv := twoSystemService(t, resets)

	p := testProvisioner(t, srv, &fakeStore{}, addressing(srv, "/redfish/v1/Systems/2"))

	if _, err := p.PowerOn(t.Context(), false); err != nil {
		t.Fatalf("PowerOn: %v", err)
	}

	if resets["2"] != 1 || resets["1"] != 0 {
		t.Errorf("resets = %v, want exactly one against system 2 and none against system 1", resets)
	}
}

// Without a system in the address a multi system BMC is ambiguous, and guessing
// is how the wrong machine gets powered off.
func TestAmbiguousMultiSystemBMCIsRefused(t *testing.T) {
	srv := twoSystemService(t, map[string]int{})

	p := testProvisioner(t, srv, &fakeStore{}, addressing(srv, "/redfish/v1"))

	_, err := p.PowerOn(t.Context(), false)
	if err == nil {
		t.Fatal("PowerOn picked a system from an ambiguous BMC")
	}

	if !strings.Contains(err.Error(), "name one in the BMC address") {
		t.Errorf("error = %v, want it to say the address must name a system", err)
	}
}
