// SPDX-License-Identifier: Apache-2.0

// The connection tests, certificate handling and the system scoping that keeps
// this host off another machine's drive.

package redfish_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"metal3.local/anaconda/internal/redfish"
)

const (
	testEndpoint = "https://bmc.example"
	rootPath     = "/redfish/v1/"
	systemsPath  = "/redfish/v1/Systems"
	systemPath   = "/redfish/v1/Systems/1"
)

const unsupportedErr = "does not support"

// BMC certificates are self signed, so verification has to be off, which gofish
// only does by reaching into a non nil transport, so this pins the arrangement.
func TestBMCCertificateIsNotVerified(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch r.URL.Path {
		case rootPath:
			_, _ = fmt.Fprint(w, `{"@odata.id":"/redfish/v1/","Systems":{"@odata.id":"/redfish/v1/Systems"}}`)
		case systemsPath:
			_, _ = fmt.Fprint(w, `{"Members":[{"@odata.id":"/redfish/v1/Systems/1"}]}`)
		case systemPath:
			_, _ = fmt.Fprint(w, `{"@odata.id":"/redfish/v1/Systems/1","Id":"1","PowerState":"On"}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)

	// httptest signs with its own throwaway CA, which nothing trusts.
	conn := redfish.Conn{Endpoint: srv.URL, SystemID: systemPath}

	state, err := conn.PowerState(t.Context())
	if err != nil {
		if strings.Contains(err.Error(), "certificate") {
			t.Fatalf("certificate was verified, so a self signed BMC would be unreachable: %v", err)
		}

		t.Fatalf("PowerState: %v", err)
	}

	if string(state) != "On" {
		t.Errorf("PowerState = %q, want On", state)
	}
}

// An emulator fronting several machines exposes one manager per machine, so
// walking every manager hands back a drive belonging to somebody else.
func TestVirtualMediaIsScopedToTheAddressedSystem(t *testing.T) {
	const (
		otherSystem = "/redfish/v1/Systems/other"
		otherMedia  = otherSystem + "/VirtualMedia/Cd"
		ourMedia    = systemPath + "/VirtualMedia/Cd"
	)

	var touched []string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		touched = append(touched, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")

		switch r.URL.Path {
		case rootPath:
			_, _ = fmt.Fprintf(w, `{"@odata.id":%q,"Systems":{"@odata.id":"/redfish/v1/Systems"},
				"Managers":{"@odata.id":"/redfish/v1/Managers"}}`, rootPath)

		// Ours links only to its own manager, the other machine is a sibling.
		case systemPath:
			_, _ = fmt.Fprintf(w, `{"@odata.id":%q,"Id":"1","VirtualMedia":{"@odata.id":%q},
				"Links":{"ManagedBy":[{"@odata.id":"/redfish/v1/Managers/ours"}]}}`,
				systemPath, systemPath+"/VirtualMedia")

		case systemPath + "/VirtualMedia":
			_, _ = fmt.Fprintf(w, `{"Members":[{"@odata.id":%q}]}`, ourMedia)

		case ourMedia:
			_, _ = fmt.Fprintf(w, `{"@odata.id":%q,"Id":"Cd","MediaTypes":["CD"],"Inserted":false,
				"Actions":{"#VirtualMedia.InsertMedia":{"target":%q}}}`, ourMedia, ourMedia+"/Actions/insert")

		// The other machine's drive already holds an image. Collecting it would
		// report this host as loaded while its own drive is empty.
		case otherMedia:
			_, _ = fmt.Fprintf(w, `{"@odata.id":%q,"Id":"Cd","MediaTypes":["CD"],
				"Inserted":true,"Image":"http://boot.example/someone-elses.iso",
				"Actions":{"#VirtualMedia.InsertMedia":{"target":%q}}}`, otherMedia, otherMedia+"/Actions/insert")

		case ourMedia + "/Actions/insert", otherMedia + "/Actions/insert":
			w.WriteHeader(http.StatusNoContent)

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)

	conn := redfish.Conn{Endpoint: srv.URL, SystemID: systemPath}

	// The read path. A sibling's loaded drive must not be reported as ours.
	status, err := conn.MediaStatus(t.Context())
	if err != nil {
		t.Fatalf("MediaStatus: %v", err)
	}

	if status.Inserted {
		t.Errorf("status = %+v, want this host's own empty drive", status)
	}

	// The write path, which is what actually loaded a live ISO into somebody
	// else's machine.
	if err := conn.InsertMedia(t.Context(), "http://boot.example/ks.iso"); err != nil {
		t.Fatalf("InsertMedia: %v", err)
	}

	if !slices.Contains(touched, ourMedia+"/Actions/insert") {
		t.Errorf("insert never reached this system's drive, paths touched %v", touched)
	}

	for _, path := range touched {
		if strings.HasPrefix(path, otherSystem) || path == "/redfish/v1/Managers" {
			t.Errorf("touched %q, want only this system's own media", path)
		}
	}
}

// Real BMCs hang virtual media off the manager rather than the system, so the
// fallback has to work, and it must follow ManagedBy rather than every manager.
func TestVirtualMediaFallsBackToTheSystemsOwnManager(t *testing.T) {
	const (
		ourManager   = "/redfish/v1/Managers/ours"
		ourMedia     = ourManager + "/VirtualMedia/Cd"
		otherManager = "/redfish/v1/Managers/theirs"
	)

	var touched []string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		touched = append(touched, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")

		switch r.URL.Path {
		case rootPath:
			_, _ = fmt.Fprintf(w, `{"@odata.id":%q,"Systems":{"@odata.id":"/redfish/v1/Systems"},
				"Managers":{"@odata.id":"/redfish/v1/Managers"}}`, rootPath)

		// The system exposes no media of its own, only the manager that owns it.
		case systemPath:
			_, _ = fmt.Fprintf(w, `{"@odata.id":%q,"Id":"1",
				"Links":{"ManagedBy":[{"@odata.id":%q}]}}`, systemPath, ourManager)

		case ourManager:
			_, _ = fmt.Fprintf(w, `{"@odata.id":%q,"Id":"ours","VirtualMedia":{"@odata.id":%q}}`,
				ourManager, ourManager+"/VirtualMedia")

		case ourManager + "/VirtualMedia":
			_, _ = fmt.Fprintf(w, `{"Members":[{"@odata.id":%q}]}`, ourMedia)

		case ourMedia:
			_, _ = fmt.Fprintf(w, `{"@odata.id":%q,"Id":"Cd","MediaTypes":["CD"],"Inserted":true,
				"Image":"http://boot.example/ours.iso"}`, ourMedia)

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)

	conn := redfish.Conn{Endpoint: srv.URL, SystemID: systemPath}

	status, err := conn.MediaStatus(t.Context())
	if err != nil {
		t.Fatalf("MediaStatus: %v", err)
	}

	if !status.Inserted || status.Image != "http://boot.example/ours.iso" {
		t.Errorf("status = %+v, want the media found on this system's own manager", status)
	}

	// Walking every manager is what once loaded an ISO into a neighbor.
	for _, path := range touched {
		if strings.HasPrefix(path, otherManager) {
			t.Errorf("touched %q, want only the manager this system is managed by", path)
		}
	}
}

// Nothing scrubs output, so the one struct carrying the BMC password must not
// render it, through a format verb or a structured log value.
func TestConnDoesNotRenderThePassword(t *testing.T) {
	conn := redfish.Conn{Endpoint: testEndpoint, Username: "admin", Password: "p@ssw0rd"}

	// zap resolves a Stringer before it reflects over fields, so the same method
	// covers a Conn passed as a log value.
	rendered := []string{
		conn.String(),
		fmt.Sprintf("%v", conn), //nolint:gocritic // calling String directly would not test the Stringer
		fmt.Sprintf("%+v", conn),
		fmt.Sprint(conn), //nolint:gocritic // calling String directly would not test the Stringer
	}

	for _, got := range rendered {
		if strings.Contains(got, conn.Password) {
			t.Errorf("rendered = %q, want no password in it", got)
		}
	}

	if conn.String() != conn.Endpoint {
		t.Errorf("String = %q, want the endpoint so a log line still says which BMC", conn.String())
	}
}

// Availability is the only question, and the Redfish service root answers it
// without credentials, so a rejected password cannot fake an unhealthy BMC.
func TestHealthyProbesTheServiceRootWithoutCredentials(t *testing.T) {
	var paths, auths []string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		auths = append(auths, r.Header.Get("Authorization"))

		// Everything but the root refuses, the way a BMC with bad credentials does.
		if r.URL.Path != rootPath {
			w.WriteHeader(http.StatusUnauthorized)

			return
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"@odata.id":"/redfish/v1/","Id":"RootService"}`)
	}))
	t.Cleanup(srv.Close)

	conn := redfish.Conn{Endpoint: srv.URL, SystemID: systemPath, Username: "wrong", Password: "wrong"}

	healthy, err := conn.Healthy(t.Context())
	if err != nil {
		t.Fatalf("Healthy: %v", err)
	}

	if !healthy {
		t.Error("healthy = false, want a BMC that answers its root to count as reachable")
	}

	if len(paths) != 1 || paths[0] != rootPath {
		t.Errorf("requests = %v, want exactly one to %s", paths, rootPath)
	}

	// An Authorization header would put this back on the credentials, which is
	// what made a reachable BMC report unhealthy once per reconcile.
	if auths[0] != "" {
		t.Errorf("Authorization = %q, want the probe to send none", auths[0])
	}
}

// A BMC that does not answer at all is the one case worth reporting.
func TestHealthyReportsAnUnreachableBMC(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {}))
	srv.Close()

	conn := redfish.Conn{Endpoint: srv.URL}

	if healthy, err := conn.Healthy(t.Context()); err == nil || healthy {
		t.Errorf("Healthy = (%v, %v), want an error for a BMC that never answered", healthy, err)
	}
}
