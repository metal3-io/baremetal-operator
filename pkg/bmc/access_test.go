package bmc

import (
	"testing"

	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

func init() {
	logf.SetLogger(logf.ZapLogger(true))
}

func TestParseLibvirtURL(t *testing.T) {
	T, H, P, A, err := getTypeHostPort("libvirt://192.168.122.1:6233/")
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
	if A != "/" {
		t.Fatalf("unexpected path: %q", A)
	}
}

func TestParseIPMIDefaultSchemeAndPort(t *testing.T) {
	T, H, P, A, err := getTypeHostPort("192.168.122.1")
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
	if A != "" {
		t.Fatalf("unexpected path: %q", A)
	}
}

func TestParseIPMIDefaultSchemeAndPortHostname(t *testing.T) {
	T, H, P, A, err := getTypeHostPort("my.favoritebmc.com")
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
	if A != "" {
		t.Fatalf("unexpected path: %q", A)
	}
}

func TestParseHostPort(t *testing.T) {
	T, H, P, A, err := getTypeHostPort("192.168.122.1:6233")
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
	if A != "" {
		t.Fatalf("unexpected path: %q", A)
	}
}

func TestParseHostPortIPv6(t *testing.T) {
	T, H, P, A, err := getTypeHostPort("[fe80::fc33:62ff:fe83:8a76]:6233")
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
	if A != "" {
		t.Fatalf("unexpected path: %q", A)
	}
}

func TestParseHostNoPortIPv6(t *testing.T) {
	// They either have to give us a port or a URL scheme with IPv6.
	_, _, _, _, err := getTypeHostPort("[fe80::fc33:62ff:fe83:8a76]")
	if err == nil {
		t.Fatal("expected parse error")
	}
}

func TestParseIPMIURL(t *testing.T) {
	T, H, P, A, err := getTypeHostPort("ipmi://192.168.122.1:6233")
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
	if A != "" {
		t.Fatalf("unexpected path: %q", A)
	}
}

func TestParseIPMIURLNoSep(t *testing.T) {
	T, H, P, A, err := getTypeHostPort("ipmi:192.168.122.1")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if T != "ipmi" {
		t.Fatalf("unexpected type: %q", T)
	}
	if H != "192.168.122.1" {
		t.Fatalf("unexpected hostname: %q", H)
	}
	if P != "" {
		t.Fatalf("unexpected port: %q", P)
	}
	if A != "" {
		t.Fatalf("unexpected path: %q", A)
	}
}

func TestParseIPMIURLNoSepPort(t *testing.T) {
	T, H, P, A, err := getTypeHostPort("ipmi:192.168.122.1:6233")
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
	if A != "" {
		t.Fatalf("unexpected path: %q", A)
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

func TestIPMIDriver(t *testing.T) {
	acc, err := NewAccessDetails("ipmi://192.168.122.1:6233")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if acc.Driver() != "ipmi" {
		t.Fatal("unexpected driver for ipmi")
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

func TestLibvirtDriver(t *testing.T) {
	acc, err := NewAccessDetails("libvirt://192.168.122.1:6233/")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	driver := acc.Driver()
	if driver != "ipmi" {
		t.Fatal("unexpected driver for libvirt")
	}
}

func TestParseiDRACURL(t *testing.T) {
	T, H, P, A, err := getTypeHostPort("idrac://192.168.122.1")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if T != "idrac" {
		t.Fatalf("unexpected type: %q", T)
	}
	if H != "192.168.122.1" {
		t.Fatalf("unexpected hostname: %q", H)
	}
	if P != "" {
		t.Fatalf("unexpected port: %q", P)
	}
	if A != "" {
		t.Fatalf("unexpected path: %q", A)
	}
}

func TestParseiDRACURLPath(t *testing.T) {
	T, H, P, A, err := getTypeHostPort("idrac://192.168.122.1:6233/foo")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if T != "idrac" {
		t.Fatalf("unexpected type: %q", T)
	}
	if H != "192.168.122.1" {
		t.Fatalf("unexpected hostname: %q", H)
	}
	if P != "6233" {
		t.Fatalf("unexpected port: %q", P)
	}
	if A != "/foo" {
		t.Fatalf("unexpected path: %q", A)
	}
}

func TestParseiDRACURLIPv6(t *testing.T) {
	T, H, P, A, err := getTypeHostPort("idrac://[fe80::fc33:62ff:fe83:8a76]")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if T != "idrac" {
		t.Fatalf("unexpected type: %q", T)
	}
	if H != "fe80::fc33:62ff:fe83:8a76" {
		t.Fatalf("unexpected hostname: %q", H)
	}
	if P != "" {
		t.Fatalf("unexpected port: %q", P)
	}
	if A != "" {
		t.Fatalf("unexpected path: %q", A)
	}
}

func TestiDRACNeedsMAC(t *testing.T) {
	acc, err := NewAccessDetails("idrac://192.168.122.1/")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if acc.NeedsMAC() {
		t.Fatal("expected to not need a MAC")
	}
}

func TestiDRACDriver(t *testing.T) {
	acc, err := NewAccessDetails("idrac://192.168.122.1/")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	driver := acc.Driver()
	if driver != "idrac" {
		t.Fatal("unexpected driver for idrac")
	}
}

func TestiDRACDriverInfo(t *testing.T) {
	acc, err := NewAccessDetails("idrac://192.168.122.1/")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	di := acc.DriverInfo(Credentials{})
	if di["drac_address"] != "http://192.168.122.1/" {
		t.Fatalf("unexpected port: %v", di["ipmi_port"])
	}
}

func TestiDRACDriverInfoHTTP(t *testing.T) {
	acc, err := NewAccessDetails("idrac+http://192.168.122.1/")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	di := acc.DriverInfo(Credentials{})
	if di["drac_address"] != "http://192.168.122.1/" {
		t.Fatalf("unexpected port: %v", di["ipmi_port"])
	}
}

func TestiDRACDriverInfoHTTPS(t *testing.T) {
	acc, err := NewAccessDetails("idrac+https://192.168.122.1/")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	di := acc.DriverInfo(Credentials{})
	if di["drac_address"] != "https://192.168.122.1/" {
		t.Fatalf("unexpected port: %v", di["ipmi_port"])
	}
}

func TestiDRACDriverInfoPort(t *testing.T) {
	acc, err := NewAccessDetails("idrac://192.168.122.1:8080/foo")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	di := acc.DriverInfo(Credentials{})
	if di["drac_address"] != "http://192.168.122.1:8080/foo" {
		t.Fatalf("unexpected port: %v", di["ipmi_port"])
	}
}

func TestiDRACDriverInfoIPv6(t *testing.T) {
	acc, err := NewAccessDetails("idrac://[fe80::fc33:62ff:fe83:8a76]/foo")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	di := acc.DriverInfo(Credentials{})
	if di["drac_address"] != "http://[fe80::fc33:62ff:fe83:8a76]/foo" {
		t.Fatalf("unexpected port: %v", di["ipmi_port"])
	}
}

func TestiDRACDriverInfoIPv6Port(t *testing.T) {
	acc, err := NewAccessDetails("idrac://[fe80::fc33:62ff:fe83:8a76]:8080/foo")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	di := acc.DriverInfo(Credentials{})
	if di["drac_address"] != "http://[fe80::fc33:62ff:fe83:8a76]:8080/foo" {
		t.Fatalf("unexpected port: %v", di["ipmi_port"])
	}
}

func TestUnknownType(t *testing.T) {
	acc, err := NewAccessDetails("foo://192.168.122.1/")
	if err == nil || acc != nil {
		t.Fatal("unexpected parse success")
	}
}
