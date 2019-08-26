package bmc

import (
	"testing"

	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

func init() {
	logf.SetLogger(logf.ZapLogger(true))
}

func TestParseLibvirtURL(t *testing.T) {
	url, err := getParsedURL("libvirt://192.168.122.1:6233/?abc=def")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if url.Scheme != "libvirt" {
		t.Fatalf("unexpected type: %q", url.Scheme)
	}
	if url.Port() != "6233" {
		t.Fatalf("unexpected port: %q", url.Port())
	}
	if url.Hostname() != "192.168.122.1" {
		t.Fatalf("unexpected hostname: %q", url.Hostname())
	}
	if url.Path != "/" {
		t.Fatalf("unexpected path: %q", url.Path)
	}
	if len(url.Query()) != 1 {
		t.Fatalf("unexpected query length: %q", len(url.Query()))
	}
	if len(url.Query()["abc"]) != 1 {
		t.Fatalf("unexpected query length: %q", len(url.Query()["abc"]))
	}
	if url.Query()["abc"][0] != "def" {
		t.Fatalf("unexpected query: %q=%q", "abc", url.Query()["abc"])
	}
}

func TestParseIPMIDefaultSchemeAndPort(t *testing.T) {
	url, err := getParsedURL("192.168.122.1")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if url.Scheme != "ipmi" {
		t.Fatalf("unexpected type: %q", url.Scheme)
	}
	if url.Port() != "" { // default is set in DriverInfo() method
		t.Fatalf("unexpected port: %q", url.Port())
	}
	if url.Hostname() != "192.168.122.1" {
		t.Fatalf("unexpected hostname: %q", url.Hostname())
	}
	if url.Path != "" {
		t.Fatalf("unexpected path: %q", url.Path)
	}
	if len(url.Query()) != 0 {
		t.Fatalf("unexpected query length: %q", len(url.Query()))
	}
}

func TestParseIPMIDefaultSchemeAndPortHostname(t *testing.T) {
	url, err := getParsedURL("my.favoritebmc.com")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if url.Scheme != "ipmi" {
		t.Fatalf("unexpected type: %q", url.Scheme)
	}
	if url.Port() != "" { // default is set in DriverInfo() method
		t.Fatalf("unexpected port: %q", url.Port())
	}
	if url.Hostname() != "my.favoritebmc.com" {
		t.Fatalf("unexpected hostname: %q", url.Hostname())
	}
	if url.Path != "" {
		t.Fatalf("unexpected path: %q", url.Path)
	}
	if len(url.Query()) != 0 {
		t.Fatalf("unexpected query length: %q", len(url.Query()))
	}
}

func TestParseHostPort(t *testing.T) {
	url, err := getParsedURL("192.168.122.1:6233")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if url.Scheme != "ipmi" {
		t.Fatalf("unexpected type: %q", url.Scheme)
	}
	if url.Hostname() != "192.168.122.1" {
		t.Fatalf("unexpected hostname: %q", url.Hostname())
	}
	if url.Port() != "6233" {
		t.Fatalf("unexpected port: %q", url.Port())
	}
	if url.Path != "" {
		t.Fatalf("unexpected path: %q", url.Path)
	}
	if len(url.Query()) != 0 {
		t.Fatalf("unexpected query length: %q", len(url.Query()))
	}
}

func TestParseHostPortIPv6(t *testing.T) {
	url, err := getParsedURL("[fe80::fc33:62ff:fe83:8a76]:6233")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if url.Scheme != "ipmi" {
		t.Fatalf("unexpected type: %q", url.Scheme)
	}
	if url.Hostname() != "fe80::fc33:62ff:fe83:8a76" {
		t.Fatalf("unexpected hostname: %q", url.Hostname())
	}
	if url.Port() != "6233" {
		t.Fatalf("unexpected port: %q", url.Port())
	}
	if url.Path != "" {
		t.Fatalf("unexpected path: %q", url.Path)
	}
	if len(url.Query()) != 0 {
		t.Fatalf("unexpected query length: %q", len(url.Query()))
	}
}

func TestParseHostNoPortIPv6(t *testing.T) {
	// TBy default we fallback to IPMI, also with IPv6.
	_, err := getParsedURL("[fe80::fc33:62ff:fe83:8a76]")
	if err == nil {
		t.Fatal("expected parse error")
	}
}

func TestParseIPMIURL(t *testing.T) {
	url, err := getParsedURL("ipmi://192.168.122.1:6233")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if url.Scheme != "ipmi" {
		t.Fatalf("unexpected type: %q", url.Scheme)
	}
	if url.Hostname() != "192.168.122.1" {
		t.Fatalf("unexpected hostname: %q", url.Hostname())
	}
	if url.Port() != "6233" {
		t.Fatalf("unexpected port: %q", url.Port())
	}
	if url.Path != "" {
		t.Fatalf("unexpected path: %q", url.Path)
	}
	if len(url.Query()) != 0 {
		t.Fatalf("unexpected query length: %q", len(url.Query()))
	}
}

func TestParseIPMIURLNoSep(t *testing.T) {
	url, err := getParsedURL("ipmi:192.168.122.1")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if url.Scheme != "ipmi" {
		t.Fatalf("unexpected type: %q", url.Scheme)
	}
	if url.Hostname() != "192.168.122.1" {
		t.Fatalf("unexpected hostname: %q", url.Hostname())
	}
	if url.Port() != "" {
		t.Fatalf("unexpected port: %q", url.Port())
	}
	if url.Path != "" {
		t.Fatalf("unexpected path: %q", url.Path)
	}
	if len(url.Query()) != 0 {
		t.Fatalf("unexpected query length: %q", len(url.Query()))
	}
}

func TestParseIPMIURLNoSepPort(t *testing.T) {
	url, err := getParsedURL("ipmi:192.168.122.1:6233")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if url.Scheme != "ipmi" {
		t.Fatalf("unexpected type: %q", url.Scheme)
	}
	if url.Hostname() != "192.168.122.1" {
		t.Fatalf("unexpected hostname: %q", url.Hostname())
	}
	if url.Port() != "6233" {
		t.Fatalf("unexpected port: %q", url.Port())
	}
	if url.Path != "" {
		t.Fatalf("unexpected path: %q", url.Path)
	}
	if len(url.Query()) != 0 {
		t.Fatalf("unexpected query length: %q", len(url.Query()))
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

func TestIPMIBootInterface(t *testing.T) {
	acc, err := NewAccessDetails("ipmi://192.168.122.1:6233")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if acc.BootInterface() != "ipxe" {
		t.Fatal("expected boot interface to be ipxe")
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

func TestLibvirtBootInterface(t *testing.T) {
	acc, err := NewAccessDetails("libvirt://192.168.122.1:6233/")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if acc.BootInterface() != "ipxe" {
		t.Fatal("expected boot interface to be ipxe")
	}
}

func TestParseIDRACURL(t *testing.T) {
	url, err := getParsedURL("idrac://192.168.122.1")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if url.Scheme != "idrac" {
		t.Fatalf("unexpected type: %q", url.Scheme)
	}
	if url.Hostname() != "192.168.122.1" {
		t.Fatalf("unexpected hostname: %q", url.Hostname())
	}
	if url.Port() != "" {
		t.Fatalf("unexpected port: %q", url.Port())
	}
	if url.Path != "" {
		t.Fatalf("unexpected path: %q", url.Path)
	}
	if len(url.Query()) != 0 {
		t.Fatalf("unexpected query length: %q", len(url.Query()))
	}
}

func TestParseIDRACURLPath(t *testing.T) {
	url, err := getParsedURL("idrac://192.168.122.1:6233/foo")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if url.Scheme != "idrac" {
		t.Fatalf("unexpected type: %q", url.Scheme)
	}
	if url.Hostname() != "192.168.122.1" {
		t.Fatalf("unexpected hostname: %q", url.Hostname())
	}
	if url.Port() != "6233" {
		t.Fatalf("unexpected port: %q", url.Port())
	}
	if url.Path != "/foo" {
		t.Fatalf("unexpected path: %q", url.Path)
	}
	if len(url.Query()) != 0 {
		t.Fatalf("unexpected query length: %q", len(url.Query()))
	}
}

func TestParseIDRACURLIPv6(t *testing.T) {
	url, err := getParsedURL("idrac://[fe80::fc33:62ff:fe83:8a76]")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if url.Scheme != "idrac" {
		t.Fatalf("unexpected type: %q", url.Scheme)
	}
	if url.Hostname() != "fe80::fc33:62ff:fe83:8a76" {
		t.Fatalf("unexpected hostname: %q", url.Hostname())
	}
	if url.Port() != "" {
		t.Fatalf("unexpected port: %q", url.Port())
	}
	if url.Path != "" {
		t.Fatalf("unexpected path: %q", url.Path)
	}
	if len(url.Query()) != 0 {
		t.Fatalf("unexpected query length: %q", len(url.Query()))
	}
}

func TestParseIDRACURLNoSep(t *testing.T) {
	url, err := getParsedURL("idrac:192.168.122.1")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if url.Scheme != "idrac" {
		t.Fatalf("unexpected type: %q", url.Scheme)
	}
	if url.Hostname() != "192.168.122.1" {
		t.Fatalf("unexpected hostname: %q", url.Hostname())
	}
	if url.Port() != "" {
		t.Fatalf("unexpected port: %q", url.Port())
	}
	if url.Path != "" {
		t.Fatalf("unexpected path: %q", url.Path)
	}
	if len(url.Query()) != 0 {
		t.Fatalf("unexpected query length: %q", len(url.Query()))
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

func TestIDRACBootInterface(t *testing.T) {
	acc, err := NewAccessDetails("idrac://192.168.122.1")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if acc.BootInterface() != "ipxe" {
		t.Fatal("expected boot interface to be ipxe")
	}
}

func TestParseIRMCURL(t *testing.T) {
	url, err := getParsedURL("irmc://192.168.122.1")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if url.Scheme != "irmc" {
		t.Fatalf("unexpected type: %q", url.Scheme)
	}
	if url.Hostname() != "192.168.122.1" {
		t.Fatalf("unexpected hostname: %q", url.Hostname())
	}
	if url.Port() != "" {
		t.Fatalf("unexpected port: %q", url.Port())
	}
	if url.Path != "" {
		t.Fatalf("unexpected path: %q", url.Path)
	}
	if len(url.Query()) != 0 {
		t.Fatalf("unexpected query length: %q", len(url.Query()))
	}
}

func TestParseIRMCURLIPv6(t *testing.T) {
	url, err := getParsedURL("irmc://[fe80::fc33:62ff:fe83:8a76]")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if url.Scheme != "irmc" {
		t.Fatalf("unexpected type: %q", url.Scheme)
	}
	if url.Hostname() != "fe80::fc33:62ff:fe83:8a76" {
		t.Fatalf("unexpected hostname: %q", url.Hostname())
	}
	if url.Port() != "" {
		t.Fatalf("unexpected port: %q", url.Port())
	}
	if url.Path != "" {
		t.Fatalf("unexpected path: %q", url.Path)
	}
	if len(url.Query()) != 0 {
		t.Fatalf("unexpected query length: %q", len(url.Query()))
	}
}

func TestParseIRMCURLNoSep(t *testing.T) {
	url, err := getParsedURL("irmc:192.168.122.1")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if url.Scheme != "irmc" {
		t.Fatalf("unexpected type: %q", url.Scheme)
	}
	if url.Hostname() != "192.168.122.1" {
		t.Fatalf("unexpected hostname: %q", url.Hostname())
	}
	if url.Port() != "" {
		t.Fatalf("unexpected port: %q", url.Port())
	}
	if url.Path != "" {
		t.Fatalf("unexpected path: %q", url.Path)
	}
	if len(url.Query()) != 0 {
		t.Fatalf("unexpected query length: %q", len(url.Query()))
	}
}

func TestIRMCNeedsMAC(t *testing.T) {
	acc, err := NewAccessDetails("irmc://192.168.122.1")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if acc.NeedsMAC() {
		t.Fatal("expected to not need a MAC")
	}
}

func TestIRMCDriver(t *testing.T) {
	acc, err := NewAccessDetails("irmc://192.168.122.1")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	driver := acc.Driver()
	if driver != "irmc" {
		t.Fatal("unexpected driver for irmc")
	}
}

func TestIRMCDriverInfo(t *testing.T) {
	acc, err := NewAccessDetails("irmc://192.168.122.1")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	di := acc.DriverInfo(Credentials{})
	if di["irmc_address"] != "192.168.122.1" {
		t.Fatalf("unexpected address: %v", di["irmc_address"])
	}
	if _, present := di["irmc_port"]; present {
		t.Fatalf("unexpected port: %v", di["irmc_port"])
	}
}

func TestIRMCDriverInfoPort(t *testing.T) {
	acc, err := NewAccessDetails("irmc://192.168.122.1:8080")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	di := acc.DriverInfo(Credentials{})
	if di["irmc_address"] != "192.168.122.1" {
		t.Fatalf("unexpected address: %v", di["irmc_address"])
	}
	if di["irmc_port"] != "8080" {
		t.Fatalf("unexpected port: %v", di["irmc_port"])
	}
}

func TestIRMCDriverInfoIPv6(t *testing.T) {
	acc, err := NewAccessDetails("irmc://[fe80::fc33:62ff:fe83:8a76]")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	di := acc.DriverInfo(Credentials{})
	if di["irmc_address"] != "fe80::fc33:62ff:fe83:8a76" {
		t.Fatalf("unexpected address: %v", di["irmc_address"])
	}
	if _, present := di["irmc_port"]; present {
		t.Fatalf("unexpected port: %v", di["irmc_port"])
	}
}

func TestIRMCDriverInfoIPv6Port(t *testing.T) {
	acc, err := NewAccessDetails("irmc://[fe80::fc33:62ff:fe83:8a76]:8080")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	di := acc.DriverInfo(Credentials{})
	if di["irmc_address"] != "fe80::fc33:62ff:fe83:8a76" {
		t.Fatalf("unexpected address: %v", di["irmc_address"])
	}
	if di["irmc_port"] != "8080" {
		t.Fatalf("unexpected port: %v", di["irmc_port"])
	}
}

func TestIRMCBootInterface(t *testing.T) {
	acc, err := NewAccessDetails("irmc://192.168.122.1")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if acc.BootInterface() != "pxe" {
		t.Fatal("expected boot interface to be pxe")
	}
}

func TestParserRedfishURL(t *testing.T) {
	url, err := getParsedURL("redfish://192.168.122.1")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if url.Scheme != "redfish" {
		t.Fatalf("unexpected type: %q", url.Scheme)
	}
	if url.Hostname() != "192.168.122.1" {
		t.Fatalf("unexpected hostname: %q", url.Hostname())
	}
	if url.Port() != "" {
		t.Fatalf("unexpected port: %q", url.Port())
	}
	if url.Path != "" {
		t.Fatalf("unexpected path: %q", url.Path)
	}
	if len(url.Query()) != 0 {
		t.Fatalf("unexpected query length: %q", len(url.Query()))
	}
}

func TestParseRedfishURLPath(t *testing.T) {
	url, err := getParsedURL("redfish://192.168.122.1:6233/foo?abc=def&ghi=jkl")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if url.Scheme != "redfish" {
		t.Fatalf("unexpected type: %q", url.Scheme)
	}
	if url.Hostname() != "192.168.122.1" {
		t.Fatalf("unexpected hostname: %q", url.Hostname())
	}
	if url.Port() != "6233" {
		t.Fatalf("unexpected port: %q", url.Port())
	}
	if url.Path != "/foo" {
		t.Fatalf("unexpected path: %q", url.Path)
	}
	if len(url.Query()) != 2 {
		t.Fatalf("unexpected query length: %q", len(url.Query()))
	}
	if len(url.Query()["abc"]) != 1 {
		t.Fatalf("unexpected query length: %q", len(url.Query()["abc"]))
	}
	if url.Query()["abc"][0] != "def" {
		t.Fatalf("unexpected query: %q=%q", "abc", url.Query()["abc"])
	}
	if len(url.Query()["ghi"]) != 1 {
		t.Fatalf("unexpected query length: %q", len(url.Query()["ghi"]))
	}
	if url.Query()["ghi"][0] != "jkl" {
		t.Fatalf("unexpected query: %q=%q", "ghi", url.Query()["jkl"])
	}
}

func TestParseRedfishURLIPv6(t *testing.T) {
	url, err := getParsedURL("redfish://[fe80::fc33:62ff:fe83:8a76]")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if url.Scheme != "redfish" {
		t.Fatalf("unexpected type: %q", url.Scheme)
	}
	if url.Hostname() != "fe80::fc33:62ff:fe83:8a76" {
		t.Fatalf("unexpected hostname: %q", url.Hostname())
	}
	if url.Port() != "" {
		t.Fatalf("unexpected port: %q", url.Port())
	}
	if url.Path != "" {
		t.Fatalf("unexpected path: %q", url.Path)
	}
	if len(url.Query()) != 0 {
		t.Fatalf("unexpected query length: %q", len(url.Query()))
	}
}

func TestParseRedfishURLNoSep(t *testing.T) {
	url, err := getParsedURL("redfish:192.168.122.1")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if url.Scheme != "redfish" {
		t.Fatalf("unexpected type: %q", url.Scheme)
	}
	if url.Hostname() != "192.168.122.1" {
		t.Fatalf("unexpected hostname: %q", url.Hostname())
	}
	if url.Port() != "" {
		t.Fatalf("unexpected port: %q", url.Port())
	}
	if url.Path != "" {
		t.Fatalf("unexpected path: %q", url.Path)
	}
	if len(url.Query()) != 0 {
		t.Fatalf("unexpected query length: %q", len(url.Query()))
	}
}

func TestRedfishNeedsMAC(t *testing.T) {
	acc, err := NewAccessDetails("redfish://192.168.122.1")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if ! acc.NeedsMAC() {
		t.Fatal("expected to need a MAC")
	}
}

func TestRedfishDriver(t *testing.T) {
	acc, err := NewAccessDetails("redfish://192.168.122.1")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	driver := acc.Driver()
	if driver != "redfish" {
		t.Fatal("unexpected driver for redfish")
	}
}

func TestRedfishDriverInfo(t *testing.T) {
	acc, err := NewAccessDetails("redfish://192.168.122.1/foo/bar?abc=def&ghi=jkl")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	di := acc.DriverInfo(Credentials{})
	if di["redfish_address"] != "https://192.168.122.1" {
		t.Fatalf("unexpected address: %v", di["redfish_address"])
	}
	if di["redfish_system_id"] != "/foo/bar" {
		t.Fatalf("unexpected system ID: %v", di["redfish_system_id"])
	}
	if di["abc"] != "def" {
		t.Fatalf("unexpected query parameter: %v=%v", "abc", di["abc"])
	}
	if di["ghi"] != "jkl" {
		t.Fatalf("unexpected query parameter: %v=%v", "ghi", di["jkl"])
	}
}

func TestRedfishDriverInfoHTTP(t *testing.T) {
	acc, err := NewAccessDetails("redfish+http://192.168.122.1/foo/bar?abc=def&ghi=jkl")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	di := acc.DriverInfo(Credentials{})
	if di["redfish_address"] != "http://192.168.122.1" {
		t.Fatalf("unexpected address: %v", di["redfish_address"])
	}
	if di["redfish_system_id"] != "/foo/bar" {
		t.Fatalf("unexpected system ID: %v", di["redfish_system_id"])
	}
	if di["abc"] != "def" {
		t.Fatalf("unexpected query parameter: %v=%v", "abc", di["abc"])
	}
	if di["ghi"] != "jkl" {
		t.Fatalf("unexpected query parameter: %v=%v", "ghi", di["jkl"])
	}
}

func TestRedfishDriverInfoHTTPS(t *testing.T) {
	acc, err := NewAccessDetails("redfish+https://192.168.122.1/foo/bar?abc=def&ghi=jkl")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	di := acc.DriverInfo(Credentials{})
	if di["redfish_address"] != "https://192.168.122.1" {
		t.Fatalf("unexpected address: %v", di["redfish_address"])
	}
	if di["redfish_system_id"] != "/foo/bar" {
		t.Fatalf("unexpected system ID: %v", di["redfish_system_id"])
	}
	if di["abc"] != "def" {
		t.Fatalf("unexpected query parameter: %v=%v", "abc", di["abc"])
	}
	if di["ghi"] != "jkl" {
		t.Fatalf("unexpected query parameter: %v=%v", "ghi", di["jkl"])
	}
}

func TestRedfishDriverInfoPort(t *testing.T) {
	acc, err := NewAccessDetails("redfish://192.168.122.1:8080/foo/bar?abc=def&ghi=jkl")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	di := acc.DriverInfo(Credentials{})
	if di["redfish_address"] != "https://192.168.122.1:8080" {
		t.Fatalf("unexpected address: %v", di["redfish_address"])
	}
	if di["redfish_system_id"] != "/foo/bar" {
		t.Fatalf("unexpected system ID: %v", di["redfish_system_id"])
	}
	if di["abc"] != "def" {
		t.Fatalf("unexpected query parameter: %v=%v", "abc", di["abc"])
	}
	if di["ghi"] != "jkl" {
		t.Fatalf("unexpected query parameter: %v=%v", "ghi", di["jkl"])
	}
}

func TestRedfishDriverInfoIPv6(t *testing.T) {
	acc, err := NewAccessDetails("redfish://[fe80::fc33:62ff:fe83:8a76]/foo/bar?abc=def&ghi=jkl")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	di := acc.DriverInfo(Credentials{})
	if di["redfish_address"] != "https://[fe80::fc33:62ff:fe83:8a76]" {
		t.Fatalf("unexpected address: %v", di["redfish_address"])
	}
	if di["redfish_system_id"] != "/foo/bar" {
		t.Fatalf("unexpected system ID: %v", di["redfish_system_id"])
	}
	if di["abc"] != "def" {
		t.Fatalf("unexpected query parameter: %v=%v", "abc", di["abc"])
	}
	if di["ghi"] != "jkl" {
		t.Fatalf("unexpected query parameter: %v=%v", "ghi", di["jkl"])
	}
}

func TestRedfishDriverInfoIPv6Port(t *testing.T) {
	acc, err := NewAccessDetails("redfish://[fe80::fc33:62ff:fe83:8a76]:8080/foo?abc=def&ghi=jkl")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	di := acc.DriverInfo(Credentials{})
	if di["redfish_address"] != "https://[fe80::fc33:62ff:fe83:8a76]:8080" {
		t.Fatalf("unexpected address: %v", di["redfish_address"])
	}
	if di["redfish_system_id"] != "/foo" {
		t.Fatalf("unexpected system ID: %v", di["redfish_system_id"])
	}
	if di["abc"] != "def" {
		t.Fatalf("unexpected query parameter: %v=%v", "abc", di["abc"])
	}
	if di["ghi"] != "jkl" {
		t.Fatalf("unexpected query parameter: %v=%v", "ghi", di["jkl"])
	}
}

func TestRedfishBootInterface(t *testing.T) {
	acc, err := NewAccessDetails("redfish://192.168.122.1")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	// TODO: Change to ipxe
	if acc.BootInterface() != "pxe" {
		t.Fatal("expected boot interface to be pxe")
	}
}

func TestUnknownType(t *testing.T) {
	acc, err := NewAccessDetails("foo://192.168.122.1")
	if err == nil || acc != nil {
		t.Fatal("unexpected parse success")
	}
}
