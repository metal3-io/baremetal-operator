package bmc

import (
	"testing"

	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

func init() {
	logf.SetLogger(logf.ZapLogger(true))
}

func TestParse(t *testing.T) {
	for _, tc := range []struct {
		Scenario    string
		Address     string
		Type        string
		Host        string
		Port        string
		Path        string
		ExpectError bool
	}{
		{
			Scenario: "libvirt url",
			Address:  "libvirt://192.168.122.1:6233/",
			Type:     "libvirt",
			Port:     "6233",
			Host:     "192.168.122.1",
			Path:     "/",
		},

		{
			Scenario: "ipmi default scheme and port",
			Address:  "192.168.122.1",
			Type:     "ipmi",
			Port:     "",
			Host:     "192.168.122.1",
			Path:     "",
		},

		{
			Scenario: "ipmi default scheme and port, hostname",
			Address:  "my.favoritebmc.com",
			Type:     "ipmi",
			Port:     "",
			Host:     "my.favoritebmc.com",
			Path:     "",
		},

		{
			Scenario: "host and port",
			Address:  "192.168.122.1:6233",
			Type:     "ipmi",
			Port:     "6233",
			Host:     "192.168.122.1",
			Path:     "",
		},

		{
			Scenario: "host and port, ipv6",
			Address:  "[fe80::fc33:62ff:fe83:8a76]:6233",
			Type:     "ipmi",
			Port:     "6233",
			Host:     "fe80::fc33:62ff:fe83:8a76",
			Path:     "",
		},

		{
			Scenario:    "host and no port, ipv6",
			Address:     "[fe80::fc33:62ff:fe83:8a76]",
			ExpectError: true,
		},

		{
			Scenario: "ipmi full url",
			Address:  "ipmi://192.168.122.1:6233",
			Type:     "ipmi",
			Port:     "6233",
			Host:     "192.168.122.1",
			Path:     "",
		},

		{
			Scenario: "ipmi url, no sep",
			Address:  "ipmi:192.168.122.1",
			Type:     "ipmi",
			Port:     "",
			Host:     "192.168.122.1",
			Path:     "",
		},

		{
			Scenario: "ipmi url with port, no sep",
			Address:  "ipmi:192.168.122.1:6233",
			Type:     "ipmi",
			Port:     "6233",
			Host:     "192.168.122.1",
			Path:     "",
		},

		{
			Scenario: "idrac url",
			Address:  "idrac://192.168.122.1",
			Type:     "idrac",
			Port:     "",
			Host:     "192.168.122.1",
			Path:     "",
		},

		{
			Scenario: "idrac url with path",
			Address:  "idrac://192.168.122.1:6233/foo",
			Type:     "idrac",
			Port:     "6233",
			Host:     "192.168.122.1",
			Path:     "/foo",
		},

		{
			Scenario: "idrac url ipv6",
			Address:  "idrac://[fe80::fc33:62ff:fe83:8a76]",
			Type:     "idrac",
			Port:     "",
			Host:     "fe80::fc33:62ff:fe83:8a76",
			Path:     "",
		},

		{
			Scenario: "idrac url, no sep",
			Address:  "idrac:192.168.122.1",
			Type:     "idrac",
			Port:     "",
			Host:     "192.168.122.1",
			Path:     "",
		},

		{
			Scenario: "irmc url",
			Address:  "irmc://192.168.122.1",
			Type:     "irmc",
			Port:     "",
			Host:     "192.168.122.1",
			Path:     "",
		},

		{
			Scenario: "irmc url, ipv6",
			Address:  "irmc://[fe80::fc33:62ff:fe83:8a76]",
			Type:     "irmc",
			Port:     "",
			Host:     "fe80::fc33:62ff:fe83:8a76",
			Path:     "",
		},

		{
			Scenario: "irmc url, no sep",
			Address:  "irmc:192.168.122.1",
			Type:     "irmc",
			Port:     "",
			Host:     "192.168.122.1",
			Path:     "",
		},
	} {
		t.Run(tc.Scenario, func(t *testing.T) {
			T, H, P, A, err := getTypeHostPort(tc.Address)

			if tc.ExpectError {
				if err == nil {
					t.Fatal("Expected error, did not get one")
				}
				// Expected an error and did get one, so no need to
				// test anything else.
				return
			}
			if !tc.ExpectError && err != nil {
				t.Fatalf("unexpected parse error: %v", err)
			}

			if T != tc.Type {
				t.Fatalf("expected type %q but got %q", tc.Type, T)
			}

			if P != tc.Port {
				t.Fatalf("expected port %q but got %q", tc.Port, P)
			}

			if H != tc.Host {
				t.Fatalf("expected host %q but got %q", tc.Host, H)
			}

			if A != tc.Path {
				t.Fatalf("expected path %q but got %q", tc.Path, A)
			}
		})
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

func TestUnknownType(t *testing.T) {
	acc, err := NewAccessDetails("foo://192.168.122.1")
	if err == nil || acc != nil {
		t.Fatal("unexpected parse success")
	}
}
