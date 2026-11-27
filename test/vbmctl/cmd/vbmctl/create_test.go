//go:build vbmctl
// +build vbmctl

package main

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	vbmctlapi "github.com/metal3-io/baremetal-operator/test/vbmctl/pkg/api"
	"github.com/metal3-io/baremetal-operator/test/vbmctl/pkg/config"
	"github.com/metal3-io/baremetal-operator/test/vbmctl/pkg/containers"
)

func TestBuildBMCEmulatorHealthCheckURL(t *testing.T) {
	t.Parallel()

	cfg := &vbmctlapi.BMCEmulatorConfig{
		Type:          config.BMCEmulatorTypeSushyTools,
		ListenAddress: "127.0.0.1",
		ListenPort:    8000,
	}

	got := containers.BuildBMCEmulatorHealthCheckURL(cfg)
	want := "http://127.0.0.1:8000/redfish/v1/"
	if got != want {
		t.Fatalf("BuildBMCEmulatorHealthCheckURL() = %q, want %q", got, want)
	}
}

func TestBuildBMCEmulatorHealthCheckURLDefaults(t *testing.T) {
	t.Parallel()

	cfg := &vbmctlapi.BMCEmulatorConfig{Type: config.BMCEmulatorTypeSushyTools}

	got := containers.BuildBMCEmulatorHealthCheckURL(cfg)
	want := "http://" + config.DefaultNetworkAddress + ":" + strconv.Itoa(config.DefaultBMCEmulatorSushyToolsListenPort) + "/redfish/v1/"
	if got != want {
		t.Fatalf("BuildBMCEmulatorHealthCheckURL() = %q, want %q", got, want)
	}
}

func TestWaitForBMCEmulatorReadinessSucceedsWhenEndpointIsReachable(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	host, port, err := net.SplitHostPort(server.Listener.Addr().String())
	if err != nil {
		t.Fatalf("SplitHostPort() failed: %v", err)
	}
	if host == "" || host == "::" || host == "[::]" {
		host = "127.0.0.1"
	}

	cfg := &vbmctlapi.BMCEmulatorConfig{
		Type:          config.BMCEmulatorTypeSushyTools,
		ListenAddress: host,
		ListenPort:    uint16(mustParsePort(t, port)),
	}

	if err := containers.WaitForBMCEmulatorReadiness(context.Background(), cfg); err != nil {
		t.Fatalf("WaitForBMCEmulatorReadiness() returned error: %v", err)
	}
}

func mustParsePort(t *testing.T, value string) int {
	t.Helper()

	parsed, err := strconv.Atoi(value)
	if err != nil {
		t.Fatalf("failed to parse port %q: %v", value, err)
	}

	return parsed
}
