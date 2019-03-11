package bmc

import (
	"testing"

	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

func init() {
	logf.SetLogger(logf.ZapLogger(true))
}

func TestParseLibvirtURL(t *testing.T) {
	T, H, P, err := getTypeHostPort("libvirt://192.168.122.1:6233/")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if T != "libvirt" {
		t.Fatalf("unexpected type: %q", T)
	}
	if P != "6233" {
		t.Fatalf("unexpected port: %q", P)
	}
	if H != "192.168.122.1" {
		t.Fatalf("unexpected hostname: %q", H)
	}
}

func TestParseIPMIDefaultSchemeAndPort(t *testing.T) {
	T, H, P, err := getTypeHostPort("192.168.122.1")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if T != "ipmi" {
		t.Fatalf("unexpected type: %q", T)
	}
	if P != "" { // default is set in DriverInfo() method
		t.Fatalf("unexpected port: %q", P)
	}
	if H != "192.168.122.1" {
		t.Fatalf("unexpected hostname: %q", H)
	}
}

func TestParseIPMIDefaultSchemeAndPortHostname(t *testing.T) {
	T, H, P, err := getTypeHostPort("my.favoritebmc.com")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if T != "ipmi" {
		t.Fatalf("unexpected type: %q", T)
	}
	if P != "" { // default is set in DriverInfo() method
		t.Fatalf("unexpected port: %q", P)
	}
	if H != "my.favoritebmc.com" {
		t.Fatalf("unexpected hostname: %q", H)
	}
}

func TestParseHostPort(t *testing.T) {
	T, H, P, err := getTypeHostPort("192.168.122.1:6233")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if T != "ipmi" {
		t.Fatalf("unexpected type: %q", T)
	}
	if H != "192.168.122.1" {
		t.Fatalf("unexpected hostname: %q", H)
	}
	if P != "6233" {
		t.Fatalf("unexpected port: %q", P)
	}
}

func TestParseHostPortIPv6(t *testing.T) {
	T, H, P, err := getTypeHostPort("[fe80::fc33:62ff:fe83:8a76]:6233")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if T != "ipmi" {
		t.Fatalf("unexpected type: %q", T)
	}
	if H != "fe80::fc33:62ff:fe83:8a76" {
		t.Fatalf("unexpected hostname: %q", H)
	}
	if P != "6233" {
		t.Fatalf("unexpected port: %q", P)
	}
}

func TestParseHostNoPortIPv6(t *testing.T) {
	// They either have to give us a port or a URL scheme with IPv6.
	_, _, _, err := getTypeHostPort("[fe80::fc33:62ff:fe83:8a76]")
	if err == nil {
		t.Fatal("expected parse error")
	}
}

func TestParseIPMIURL(t *testing.T) {
	T, H, P, err := getTypeHostPort("ipmi://192.168.122.1:6233")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if T != "ipmi" {
		t.Fatalf("unexpected type: %q", T)
	}
	if H != "192.168.122.1" {
		t.Fatalf("unexpected hostname: %q", H)
	}
	if P != "6233" {
		t.Fatalf("unexpected port: %q", P)
	}
}

func TestIPMINeedsMAC(t *testing.T) {
	acc, err := NewAccessDetails("ipmi://192.168.122.1:6233")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if acc.NeedsMAC() {
		t.Fatal("expected to not need a MAC")
	}
}

func TestIPMIDriverInfoDefaultPort(t *testing.T) {
	acc, err := NewAccessDetails("ipmi://192.168.122.1")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	di := acc.DriverInfo(Credentials{})
	if di["ipmi_port"] != ipmiDefaultPort {
		t.Fatalf("unexpected port: %v", di["ipmi_port"])
	}
}

func TestLibvirtNeedsMAC(t *testing.T) {
	acc, err := NewAccessDetails("libvirt://192.168.122.1:6233/")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if !acc.NeedsMAC() {
		t.Fatal("expected to need a MAC")
	}
}
