package bmc

import (
	"testing"

	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

func init() {
	logf.SetLogger(logf.ZapLogger(true))
}

func TestParseLibvirtURL(t *testing.T) {
	acc, err := NewAccessDetails("libvirt://192.168.122.1:6233/")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if acc.Type != "libvirt" {
		t.Fatalf("unexpected type: %q", acc.Type)
	}
	if acc.portNum != "6233" {
		t.Fatalf("unexpected port: %q", acc.portNum)
	}
	if acc.hostname != "192.168.122.1" {
		t.Fatalf("unexpected hostname: %q", acc.hostname)
	}
}

func TestParseLibvirtNeedsMAC(t *testing.T) {
	acc, err := NewAccessDetails("libvirt://192.168.122.1:6233/")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if !acc.NeedsMAC() {
		t.Fatal("expected to need a MAC")
	}
}

func TestParseIPMIDefaultSchemeAndPort(t *testing.T) {
	acc, err := NewAccessDetails("192.168.122.1")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if acc.Type != "ipmi" {
		t.Fatalf("unexpected type: %q", acc.Type)
	}
	if acc.hostname != "192.168.122.1" {
		t.Fatalf("unexpected hostname: %q", acc.hostname)
	}
	if acc.portNum != ipmiDefaultPort {
		t.Fatalf("unexpected port: %q", acc.portNum)
	}
}

func TestParseIPMIDefaultSchemeAndPortHostname(t *testing.T) {
	acc, err := NewAccessDetails("my.favoritebmc.com")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if acc.Type != "ipmi" {
		t.Fatalf("unexpected type: %q", acc.Type)
	}
	if acc.hostname != "my.favoritebmc.com" {
		t.Fatalf("unexpected hostname: %q", acc.hostname)
	}
	if acc.portNum != ipmiDefaultPort {
		t.Fatalf("unexpected port: %q", acc.portNum)
	}
}

func TestParseHostPort(t *testing.T) {
	acc, err := NewAccessDetails("192.168.122.1:6233")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if acc.Type != "ipmi" {
		t.Fatalf("unexpected type: %q", acc.Type)
	}
	if acc.hostname != "192.168.122.1" {
		t.Fatalf("unexpected hostname: %q", acc.hostname)
	}
	if acc.portNum != "6233" {
		t.Fatalf("unexpected port: %q", acc.portNum)
	}
}

func TestParseHostPortIPv6(t *testing.T) {
	acc, err := NewAccessDetails("[fe80::fc33:62ff:fe83:8a76]:6233")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if acc.Type != "ipmi" {
		t.Fatalf("unexpected type: %q", acc.Type)
	}
	if acc.hostname != "fe80::fc33:62ff:fe83:8a76" {
		t.Fatalf("unexpected hostname: %q", acc.hostname)
	}
	if acc.portNum != "6233" {
		t.Fatalf("unexpected port: %q", acc.portNum)
	}
}

func TestParseHostNoPortIPv6(t *testing.T) {
	// They either have to give us a port or a URL scheme with IPv6.
	_, err := NewAccessDetails("[fe80::fc33:62ff:fe83:8a76]")
	if err == nil {
		t.Fatal("expected parse error")
	}
}

func TestParseIPMIURL(t *testing.T) {
	acc, err := NewAccessDetails("ipmi://192.168.122.1:6233")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if acc.Type != "ipmi" {
		t.Fatalf("unexpected type: %q", acc.Type)
	}
	if acc.hostname != "192.168.122.1" {
		t.Fatalf("unexpected hostname: %q", acc.hostname)
	}
	if acc.portNum != "6233" {
		t.Fatalf("unexpected port: %q", acc.portNum)
	}
}

func TestParseIPMINeedsMAC(t *testing.T) {
	acc, err := NewAccessDetails("ipmi://192.168.122.1:6233")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if acc.NeedsMAC() {
		t.Fatal("expected to not need a MAC")
	}
}
