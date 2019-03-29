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

func TestParseIDRACURL(t *testing.T) {
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

func TestParseIDRACURLPath(t *testing.T) {
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

func TestParseIDRACURLIPv6(t *testing.T) {
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

func TestParseIDRACURLNoSep(t *testing.T) {
	T, H, P, A, err := getTypeHostPort("idrac:192.168.122.1")
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

func TestIDRACNeedsMAC(t *testing.T) {
	acc, err := NewAccessDetails("idrac://192.168.122.1")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if acc.NeedsMAC() {
		t.Fatal("expected to not need a MAC")
	}
}

func TestIDRACDriver(t *testing.T) {
	acc, err := NewAccessDetails("idrac://192.168.122.1")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	driver := acc.Driver()
	if driver != "idrac" {
		t.Fatal("unexpected driver for idrac")
	}
}

func TestIDRACDriverInfo(t *testing.T) {
	acc, err := NewAccessDetails("idrac://192.168.122.1")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	di := acc.DriverInfo(Credentials{})
	if di["drac_address"] != "192.168.122.1" {
		t.Fatalf("unexpected address: %v", di["drac_address"])
	}
	if _, present := di["drac_port"]; present {
		t.Fatalf("unexpected port: %v", di["drac_port"])
	}
	if _, present := di["drac_protocol"]; present {
		t.Fatalf("unexpected protocol: %v", di["drac_protocol"])
	}
	if _, present := di["drac_path"]; present {
		t.Fatalf("unexpected path: %v", di["drac_path"])
	}
}

func TestIDRACDriverInfoHTTP(t *testing.T) {
	acc, err := NewAccessDetails("idrac+http://192.168.122.1")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	di := acc.DriverInfo(Credentials{})
	if di["drac_address"] != "192.168.122.1" {
		t.Fatalf("unexpected address: %v", di["drac_address"])
	}
	if _, present := di["drac_port"]; present {
		t.Fatalf("unexpected port: %v", di["drac_port"])
	}
	if di["drac_protocol"] != "http" {
		t.Fatalf("unexpected protocol: %v", di["drac_protocol"])
	}
	if _, present := di["drac_path"]; present {
		t.Fatalf("unexpected path: %v", di["drac_path"])
	}
}

func TestIDRACDriverInfoHTTPS(t *testing.T) {
	acc, err := NewAccessDetails("idrac+https://192.168.122.1")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	di := acc.DriverInfo(Credentials{})
	if di["drac_address"] != "192.168.122.1" {
		t.Fatalf("unexpected address: %v", di["drac_address"])
	}
	if _, present := di["drac_port"]; present {
		t.Fatalf("unexpected port: %v", di["drac_port"])
	}
	if di["drac_protocol"] != "https" {
		t.Fatalf("unexpected protocol: %v", di["drac_protocol"])
	}
	if _, present := di["drac_path"]; present {
		t.Fatalf("unexpected path: %v", di["drac_path"])
	}
}

func TestIDRACDriverInfoPort(t *testing.T) {
	acc, err := NewAccessDetails("idrac://192.168.122.1:8080/foo")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	di := acc.DriverInfo(Credentials{})
	if di["drac_address"] != "192.168.122.1" {
		t.Fatalf("unexpected address: %v", di["drac_address"])
	}
	if di["drac_port"] != "8080" {
		t.Fatalf("unexpected port: %v", di["drac_port"])
	}
	if _, present := di["drac_protocol"]; present {
		t.Fatalf("unexpected protocol: %v", di["drac_protocol"])
	}
	if di["drac_path"] != "/foo" {
		t.Fatalf("unexpected path: %v", di["drac_path"])
	}
}

func TestIDRACDriverInfoIPv6(t *testing.T) {
	acc, err := NewAccessDetails("idrac://[fe80::fc33:62ff:fe83:8a76]/foo")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	di := acc.DriverInfo(Credentials{})
	if di["drac_address"] != "fe80::fc33:62ff:fe83:8a76" {
		t.Fatalf("unexpected address: %v", di["drac_address"])
	}
	if _, present := di["drac_port"]; present {
		t.Fatalf("unexpected port: %v", di["drac_port"])
	}
	if _, present := di["drac_protocol"]; present {
		t.Fatalf("unexpected protocol: %v", di["drac_protocol"])
	}
	if di["drac_path"] != "/foo" {
		t.Fatalf("unexpected path: %v", di["drac_path"])
	}
}

func TestIDRACDriverInfoIPv6Port(t *testing.T) {
	acc, err := NewAccessDetails("idrac://[fe80::fc33:62ff:fe83:8a76]:8080/foo")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	di := acc.DriverInfo(Credentials{})
	if di["drac_address"] != "fe80::fc33:62ff:fe83:8a76" {
		t.Fatalf("unexpected address: %v", di["drac_address"])
	}
	if di["drac_port"] != "8080" {
		t.Fatalf("unexpected port: %v", di["drac_port"])
	}
	if _, present := di["drac_protocol"]; present {
		t.Fatalf("unexpected protocol: %v", di["drac_protocol"])
	}
	if di["drac_path"] != "/foo" {
		t.Fatalf("unexpected path: %v", di["drac_path"])
	}
}

func TestUnknownType(t *testing.T) {
	acc, err := NewAccessDetails("foo://192.168.122.1")
	if err == nil || acc != nil {
		t.Fatal("unexpected parse success")
	}
}
