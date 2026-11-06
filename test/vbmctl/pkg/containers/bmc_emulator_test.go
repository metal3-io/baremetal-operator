//go:build vbmctl
// +build vbmctl

package containers

import (
	"context"
	"errors"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	vbmctlapi "github.com/metal3-io/baremetal-operator/test/vbmctl/pkg/api"
	"github.com/metal3-io/baremetal-operator/test/vbmctl/pkg/config"
)

func TestParseSushyEmulatorConfigFile(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "sushy.conf")

	content := `# Listen on the local IP address 192.168.222.1
SUSHY_EMULATOR_LISTEN_IP = u'127.0.0.1'

# Bind to TCP port 8000
SUSHY_EMULATOR_LISTEN_PORT = 9000 # This is a comment

# The libvirt URI to use. This option enables libvirt driver.
SUSHY_EMULATOR_LIBVIRT_URI = u'qemu:///system'
SUSHY_EMULATOR_STORAGE_POOL = 'baremetal-e2e'
`

	if err := os.WriteFile(configPath, []byte(content), 0o600); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	address, port, err := parseSushyEmulatorConfigFile(configPath)
	if err != nil {
		t.Fatalf("unexpected error parsing config file: %v", err)
	}

	if address != "127.0.0.1" {
		t.Fatalf("expected address %q, got %q", "127.0.0.1", address)
	}

	if port != 9000 {
		t.Fatalf("expected port %d, got %d", 9000, port)
	}
}

func TestParseSushyEmulatorConfigFileWithQuotes(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "sushy.conf")

	content := `SUSHY_EMULATOR_LISTEN_IP = "192.168.222.2"
SUSHY_EMULATOR_LISTEN_PORT = '8001' # This is a comment
`

	if err := os.WriteFile(configPath, []byte(content), 0o600); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	address, port, err := parseSushyEmulatorConfigFile(configPath)
	if err != nil {
		t.Fatalf("unexpected error parsing config file: %v", err)
	}

	if address != "192.168.222.2" {
		t.Fatalf("expected address %q, got %q", "192.168.222.2", address)
	}

	if port != 8001 {
		t.Fatalf("expected port %d, got %d", 8001, port)
	}
}

func TestParseSushyEmulatorConfigFileMissingValues(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "sushy.conf")

	content := `# Listen address missing
# SUSHY_EMULATOR_LISTEN_IP = u'127.0.0.1'

# Listen port missing
# SUSHY_EMULATOR_LISTEN_PORT = 9000
`

	if err := os.WriteFile(configPath, []byte(content), 0o600); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	address, port, err := parseSushyEmulatorConfigFile(configPath)
	if err != nil {
		t.Fatalf("unexpected error parsing config file: %v", err)
	}

	if address != "" {
		t.Fatalf("expected empty address, got %q", address)
	}

	if port != 0 {
		t.Fatalf("expected port 0, got %d", port)
	}
}

func TestWaitForBMCEmulatorReadiness_Exhaustion(t *testing.T) {
	// Start a controlled HTTP server on an OS-assigned port. The handler will
	// return a 503 Service Unavailable status.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen on loopback: %v", err)
	}
	defer ln.Close()

	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	})}
	go srv.Serve(ln)
	defer srv.Close()

	//nolint:forcetypeassert
	addr := ln.Addr().(*net.TCPAddr)
	cfg := &vbmctlapi.BMCEmulatorConfig{
		Type:          config.BMCEmulatorTypeSushyTools,
		ListenAddress: "127.0.0.1",
		ListenPort:    uint16(addr.Port),
	}

	// Use a very small retry policy to make the test fast.
	err = WaitForBMCEmulatorReadinessWithPolicy(context.Background(), cfg, 3, 50*time.Millisecond, 20*time.Millisecond)
	if err == nil {
		t.Fatalf("expected error when emulator is unreachable, got nil")
	}
}

func TestWaitForBMCEmulatorReadiness_Cancellation(t *testing.T) {
	// Start a controlled HTTP server on an OS-assigned port. The handler will
	// block until the request context is done so that client-side cancellation
	// during the request is deterministic.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen on loopback: %v", err)
	}
	defer ln.Close()

	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	})}
	go srv.Serve(ln)
	defer srv.Close()

	//nolint:forcetypeassert
	addr := ln.Addr().(*net.TCPAddr)
	cfg := &vbmctlapi.BMCEmulatorConfig{
		Type:          config.BMCEmulatorTypeSushyTools,
		ListenAddress: "127.0.0.1",
		ListenPort:    uint16(addr.Port),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 75*time.Millisecond)
	defer cancel()

	err = WaitForBMCEmulatorReadinessWithPolicy(ctx, cfg, 10, 500*time.Millisecond, 20*time.Millisecond)
	if err == nil {
		t.Fatalf("expected error due to context cancellation or timeout, got nil")
	}
	if !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context cancellation error, got: %v", err)
	}
}
