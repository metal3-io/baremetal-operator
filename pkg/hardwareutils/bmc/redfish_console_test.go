package bmc

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"testing"
)

func TestDiscoverIPMISOL(t *testing.T) {
	fixture := serialConsoleFixture(t, "system.json")
	transport := roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/redfish/v1/Systems/1" {
			return nil, fmt.Errorf("unexpected path %q", r.URL.Path)
		}
		username, password, ok := r.BasicAuth()
		if !ok || username != "admin" || password != "password" {
			return nil, fmt.Errorf("unexpected credentials %q/%q", username, password)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(fixture)),
			Header:     make(http.Header),
			Request:    r,
		}, nil
	})
	access := redfishDetails(&url.URL{
		Scheme: "redfish+http",
		Host:   "bmc.example",
		Path:   "/redfish/v1/Systems/1",
	}, false)

	result, err := access.discoverIPMISOL(t.Context(), &http.Client{Transport: transport}, Credentials{
		Username: "admin",
		Password: "password",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result == nil || result.Address != "bmc.example" || result.Port != 623 {
		t.Fatalf("unexpected result: %#v", result)
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestDiscoverIPMISOLFromManagerAfterCollection(t *testing.T) {
	resources := map[string]string{
		"/redfish/v1/Systems":        `{"Members":[{"@odata.id":"/redfish/v1/Systems/1"}]}`,
		"/redfish/v1/Systems/1":      `{}`,
		"/redfish/v1/Managers":       `{"Members":[{"@odata.id":"/redfish/v1/Managers/other"},{"@odata.id":"/redfish/v1/Managers/1"}]}`,
		"/redfish/v1/Managers/other": `{"Links":{"ManagerForServers":[{"@odata.id":"/redfish/v1/Systems/other"}]},"SerialConsole":{"IPMI":{"Port":9999}}}`,
		"/redfish/v1/Managers/1":     `{"Links":{"ManagerForServers":[{"@odata.id":"/redfish/v1/Systems/1"}]},"SerialConsole":{"Oem":{"SushyTools":{"Host":"192.0.2.20","IPMITerminalPort":6233}}}}`,
	}
	transport := roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		body, ok := resources[r.URL.Path]
		if !ok {
			return nil, fmt.Errorf("unexpected resource %s", r.URL.Path)
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})
	access := redfishDetails(&url.URL{Scheme: "redfish+http", Host: "bmc.example"}, false)
	result, err := access.discoverIPMISOL(t.Context(), &http.Client{Transport: transport}, Credentials{})
	if err != nil {
		t.Fatal(err)
	}
	if result == nil || result.Address != "192.0.2.20" || result.Port != 6233 {
		t.Fatalf("unexpected SOL endpoint: %#v", result)
	}
}

func TestRedfishIPMISOLInvalidOrDisabled(t *testing.T) {
	for _, port := range []float64{0, -1, 65536, 623.5} {
		resource := map[string]interface{}{"SerialConsole": map[string]interface{}{"IPMI": map[string]interface{}{"Port": port}}}
		if _, _, err := redfishIPMISOL(resource); err == nil {
			t.Fatalf("accepted invalid port %v", port)
		}
	}
	resource := map[string]interface{}{"SerialConsole": map[string]interface{}{"ServiceEnabled": false, "IPMI": map[string]interface{}{"Port": float64(623)}}}
	info, found, err := redfishIPMISOL(resource)
	if info != nil || found || err != nil {
		t.Fatalf("accepted disabled console: %#v, %v, %v", info, found, err)
	}
}

func TestRedfishResourceLinksStayOnOrigin(t *testing.T) {
	origin, err := url.Parse("https://bmc.example")
	if err != nil {
		t.Fatal(err)
	}
	testCases := []struct {
		link    string
		allowed bool
	}{
		{link: "/redfish/v1/Systems/1", allowed: true},
		{link: "redfish/v1/Systems/1", allowed: true},
		{link: "https://bmc.example/redfish/v1/Systems/1", allowed: true},
		{link: "@other.example/redfish/v1/Systems/1", allowed: true},
		{link: "https://other.example/redfish/v1/Systems/1"},
		{link: "//other.example/redfish/v1/Systems/1"},
		{link: "http://bmc.example/redfish/v1/Systems/1"},
		{link: "https://user@bmc.example/redfish/v1/Systems/1"},
		{link: "/redfish/v1/Systems/1#fragment"},
		{link: ":invalid"},
	}
	for _, tc := range testCases {
		t.Run(tc.link, func(t *testing.T) {
			requests := 0
			client := &http.Client{Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
				requests++
				if r.URL.Host != origin.Host {
					t.Errorf("request escaped the BMC origin: %s", r.URL.Host)
				}
				username, password, authenticated := r.BasicAuth()
				if !authenticated || username != "admin" || password != "password" {
					t.Error("missing BMC authentication on an allowed request")
				}
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{}`)), Header: make(http.Header)}, nil
			})}
			_, requestErr := redfishGET(t.Context(), client, origin, tc.link, Credentials{Username: "admin", Password: "password"})
			if tc.allowed {
				if requestErr != nil || requests != 1 {
					t.Fatalf("allowed resource link failed: requests=%d error=%v", requests, requestErr)
				}
			} else if requestErr == nil || requests != 0 {
				t.Fatalf("unsafe resource link was not rejected before sending: requests=%d error=%v", requests, requestErr)
			}
		})
	}
}

func TestDiscoverIPMISOLDoesNotFollowRedirects(t *testing.T) {
	var forwarded atomic.Bool
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		forwarded.Store(true)
		w.WriteHeader(http.StatusOK)
	}))
	defer other.Close()
	bmcServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL, http.StatusFound)
	}))
	defer bmcServer.Close()
	parsed, err := url.Parse(bmcServer.URL)
	if err != nil {
		t.Fatal(err)
	}
	parsed.Scheme = "redfish+http"
	access, err := NewAccessDetails(parsed.String(), false)
	if err != nil {
		t.Fatal(err)
	}
	discoverer, ok := access.(IPMISOLDiscoverer)
	if !ok {
		t.Fatal("Redfish access does not support SOL discovery")
	}
	if _, err = discoverer.DiscoverIPMISOL(t.Context(), Credentials{Username: "admin", Password: "password"}); err == nil {
		t.Fatal("redirect should not be followed")
	}
	if forwarded.Load() {
		t.Fatal("forwarded an authenticated request to the redirect target")
	}
}

func serialConsoleFixture(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile("testdata/serialconsole/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func serialConsoleFixtureObject(t *testing.T, resource map[string]interface{}, key string) map[string]interface{} {
	t.Helper()
	object, ok := resource[key].(map[string]interface{})
	if !ok {
		t.Fatalf("fixture property %s is not an object", key)
	}
	return object
}

func TestDiscoverIPMISOLStandardRedfishManager(t *testing.T) {
	testCases := []struct {
		name               string
		mutate             func(*testing.T, map[string]interface{}, map[string]interface{})
		found              bool
		wantError          bool
		wantNetworkRequest bool
		systemDisabled     bool
	}{
		{name: "enabled", found: true, wantNetworkRequest: true},
		{name: "disabled-system", systemDisabled: true},
		{name: "disabled-manager", mutate: func(t *testing.T, manager, _ map[string]interface{}) {
			t.Helper()
			serialConsoleFixtureObject(t, manager, "SerialConsole")["ServiceEnabled"] = false
		}},
		{name: "SSH-only", mutate: func(t *testing.T, manager, _ map[string]interface{}) {
			t.Helper()
			serialConsoleFixtureObject(t, manager, "SerialConsole")["ConnectTypesSupported"] = []interface{}{"SSH"}
		}},
		{name: "shared-manager", mutate: func(t *testing.T, manager, _ map[string]interface{}) {
			t.Helper()
			serialConsoleFixtureObject(t, manager, "Links")["ManagerForServers"] = []interface{}{map[string]interface{}{"@odata.id": "/redfish/v1/Systems/1"}, map[string]interface{}{"@odata.id": "/redfish/v1/Systems/2"}}
		}},
		{name: "disabled-IPMI", wantNetworkRequest: true, mutate: func(t *testing.T, _, network map[string]interface{}) {
			t.Helper()
			serialConsoleFixtureObject(t, network, "IPMI")["ProtocolEnabled"] = false
		}},
		{name: "invalid-port", wantError: true, wantNetworkRequest: true, mutate: func(t *testing.T, _, network map[string]interface{}) {
			t.Helper()
			serialConsoleFixtureObject(t, network, "IPMI")["Port"] = 623.5
		}},
		{name: "foreign-network-link", wantError: true, mutate: func(t *testing.T, manager, _ map[string]interface{}) {
			t.Helper()
			serialConsoleFixtureObject(t, manager, "NetworkProtocol")["@odata.id"] = "https://other.example/NetworkProtocol"
		}},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var manager, network map[string]interface{}
			if err := json.Unmarshal([]byte(serialConsoleFixture(t, "manager.json")), &manager); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal([]byte(serialConsoleFixture(t, "network-protocol.json")), &network); err != nil {
				t.Fatal(err)
			}
			if tc.mutate != nil {
				tc.mutate(t, manager, network)
			}
			managerJSON, err := json.Marshal(manager)
			if err != nil {
				t.Fatal(err)
			}
			networkJSON, err := json.Marshal(network)
			if err != nil {
				t.Fatal(err)
			}
			networkRequested := false
			transport := roundTripperFunc(func(r *http.Request) (*http.Response, error) {
				if r.URL.Host != "bmc.example" {
					return nil, errors.New("request escaped the BMC origin")
				}
				username, password, authenticated := r.BasicAuth()
				if !authenticated || username != "admin" || password != "password" {
					return nil, errors.New("missing Redfish authentication")
				}
				var body string
				switch r.URL.Path {
				case "/redfish/v1/Systems/1":
					body = `{}`
					if tc.systemDisabled {
						body = `{"SerialConsole":{"IPMI":{"ServiceEnabled":false,"Port":623}}}`
					}
				case "/redfish/v1/Managers":
					body = `{"Members":[{"@odata.id":"/redfish/v1/Managers/1"}]}`
				case "/redfish/v1/Managers/1":
					body = string(managerJSON)
				case "/redfish/v1/Managers/1/NetworkProtocol":
					networkRequested = true
					body = string(networkJSON)
				default:
					return nil, fmt.Errorf("unexpected Redfish resource %s", r.URL.Path)
				}
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
			})
			access := redfishDetails(&url.URL{Scheme: "redfish+https", Host: "bmc.example", Path: "/redfish/v1/Systems/1"}, false)
			info, discoveryErr := access.discoverIPMISOL(t.Context(), &http.Client{Transport: transport}, Credentials{Username: "admin", Password: "password"})
			if (discoveryErr != nil) != tc.wantError {
				t.Fatalf("unexpected discovery error: %v", discoveryErr)
			}
			if (info != nil) != tc.found {
				t.Fatalf("unexpected discovery result: %+v", info)
			}
			if info != nil && (info.Address != "bmc.example" || info.Port != 623) {
				t.Fatalf("unexpected IPMI connection: %+v", info)
			}
			if networkRequested != tc.wantNetworkRequest {
				t.Fatalf("network protocol requested=%v, want %v", networkRequested, tc.wantNetworkRequest)
			}
		})
	}
}

func TestDiscoverIPMISOLTLSVerification(t *testing.T) {
	fixture := serialConsoleFixture(t, "system.json")
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, fixture) }))
	defer server.Close()
	parsed, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	base, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		t.Fatal("unexpected default HTTP transport")
	}
	t.Cleanup(func() { http.DefaultTransport = base })
	for _, tc := range []struct {
		name                           string
		skipVerify, trusted, wantError bool
	}{
		{name: "untrusted", wantError: true},
		{name: "explicit-insecure", skipVerify: true},
		{name: "trusted-CA", trusted: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			transport := base.Clone()
			if tc.trusted {
				roots := x509.NewCertPool()
				roots.AddCert(server.Certificate())
				transport.TLSClientConfig = &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}
			}
			http.DefaultTransport = transport
			defer transport.CloseIdleConnections()
			access := redfishDetails(&url.URL{Scheme: "redfish+https", Host: parsed.Host}, tc.skipVerify)
			info, discoveryErr := access.DiscoverIPMISOL(t.Context(), Credentials{})
			if (discoveryErr != nil) != tc.wantError {
				t.Fatalf("unexpected TLS discovery error: %v", discoveryErr)
			}
			if !tc.wantError && (info == nil || info.Port != 623) {
				t.Fatalf("unexpected discovery result: %+v", info)
			}
		})
	}
}
