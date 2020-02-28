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
		Hostname    string
		Port        string
		Path        string
		ExpectError bool
		Query       map[string][]string
	}{
		{
			Scenario: "libvirt url",
			Address:  "libvirt://192.168.122.1:6233/?abc=def",
			Type:     "libvirt",
			Port:     "6233",
			Host:     "192.168.122.1",
			Hostname: "192.168.122.1:6233",
			Path:     "/",
			Query: map[string][]string{
				"abc": []string{"def"},
			},
		},

		{
			Scenario: "ipmi default scheme and port",
			Address:  "192.168.122.1",
			Type:     "ipmi",
			Port:     "",
			Host:     "192.168.122.1",
			Hostname: "192.168.122.1",
			Path:     "",
		},

		{
			Scenario: "ipmi default scheme and port, hostname",
			Address:  "my.favoritebmc.com",
			Type:     "ipmi",
			Port:     "",
			Host:     "my.favoritebmc.com",
			Hostname: "my.favoritebmc.com",
			Path:     "",
		},

		{
			Scenario: "host and port",
			Address:  "192.168.122.1:6233",
			Type:     "ipmi",
			Port:     "6233",
			Host:     "192.168.122.1",
			Hostname: "192.168.122.1:6233",
			Path:     "",
		},

		{
			Scenario: "host and port, ipv6",
			Address:  "[fe80::fc33:62ff:fe83:8a76]:6233",
			Type:     "ipmi",
			Port:     "6233",
			Host:     "fe80::fc33:62ff:fe83:8a76",
			Path:     "",
			Hostname: "[fe80::fc33:62ff:fe83:8a76]:6233",
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
			Hostname: "192.168.122.1:6233",
		},

		{
			Scenario: "ipmi url, no sep",
			Address:  "ipmi:192.168.122.1",
			Type:     "ipmi",
			Port:     "",
			Host:     "192.168.122.1",
			Hostname: "192.168.122.1",
			Path:     "",
		},

		{
			Scenario: "ipmi url with port, no sep",
			Address:  "ipmi:192.168.122.1:6233",
			Type:     "ipmi",
			Port:     "6233",
			Host:     "192.168.122.1",
			Hostname: "192.168.122.1:6233",
			Path:     "",
		},

		{
			Scenario: "idrac url",
			Address:  "idrac://192.168.122.1",
			Type:     "idrac",
			Port:     "",
			Host:     "192.168.122.1",
			Hostname: "192.168.122.1",
			Path:     "",
		},

		{
			Scenario: "idrac url with path",
			Address:  "idrac://192.168.122.1:6233/foo",
			Type:     "idrac",
			Port:     "6233",
			Host:     "192.168.122.1",
			Hostname: "192.168.122.1:6233",
			Path:     "/foo",
		},

		{
			Scenario: "idrac url ipv6",
			Address:  "idrac://[fe80::fc33:62ff:fe83:8a76]",
			Type:     "idrac",
			Port:     "",
			Host:     "fe80::fc33:62ff:fe83:8a76",
			Hostname: "[fe80::fc33:62ff:fe83:8a76]",
			Path:     "",
		},

		{
			Scenario: "idrac url, no sep",
			Address:  "idrac:192.168.122.1",
			Type:     "idrac",
			Port:     "",
			Host:     "192.168.122.1",
			Hostname: "192.168.122.1",
			Path:     "",
		},

		{
			Scenario: "irmc url",
			Address:  "irmc://192.168.122.1",
			Type:     "irmc",
			Port:     "",
			Host:     "192.168.122.1",
			Hostname: "192.168.122.1",
			Path:     "",
		},

		{
			Scenario: "irmc url, ipv6",
			Address:  "irmc://[fe80::fc33:62ff:fe83:8a76]",
			Type:     "irmc",
			Port:     "",
			Host:     "fe80::fc33:62ff:fe83:8a76",
			Hostname: "[fe80::fc33:62ff:fe83:8a76]",
			Path:     "",
		},

		{
			Scenario: "irmc url, no sep",
			Address:  "irmc:192.168.122.1",
			Type:     "irmc",
			Port:     "",
			Host:     "192.168.122.1",
			Hostname: "192.168.122.1",
			Path:     "",
		},

		{
			Scenario: "redfish url",
			Address:  "redfish://192.168.122.1",
			Type:     "redfish",
			Port:     "",
			Host:     "192.168.122.1",
			Hostname: "192.168.122.1",
			Path:     "",
		},

		{
			Scenario: "redfish url path",
			Address:  "redfish://192.168.122.1:6233/foo",
			Type:     "redfish",
			Port:     "6233",
			Host:     "192.168.122.1",
			Hostname: "192.168.122.1:6233",
			Path:     "/foo",
		},

		{
			Scenario: "redfish url ipv6",
			Address:  "redfish://[fe80::fc33:62ff:fe83:8a76]",
			Type:     "redfish",
			Port:     "",
			Host:     "fe80::fc33:62ff:fe83:8a76",
			Hostname: "[fe80::fc33:62ff:fe83:8a76]",
			Path:     "",
		},

		{
			Scenario: "redfish url path ipv6",
			Address:  "redfish://[fe80::fc33:62ff:fe83:8a76]:6233/foo",
			Type:     "redfish",
			Port:     "6233",
			Host:     "fe80::fc33:62ff:fe83:8a76",
			Hostname: "[fe80::fc33:62ff:fe83:8a76]:6233",
			Path:     "/foo",
		},

		{
			Scenario: "redfish url no sep",
			Address:  "redfish:192.168.122.1",
			Type:     "redfish",
			Port:     "",
			Host:     "192.168.122.1",
			Hostname: "192.168.122.1",
			Path:     "",
		},
	} {
		t.Run(tc.Scenario, func(t *testing.T) {
			url, err := getParsedURL(tc.Address)

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

			if url.Scheme != tc.Type {
				t.Fatalf("expected type %q but got %q", tc.Type, url.Scheme)
			}

			if url.Port() != tc.Port {
				t.Fatalf("expected port %q but got %q", tc.Port, url.Port())
			}

			if url.Hostname() != tc.Host {
				t.Fatalf("expected host %q but got %q", tc.Host, url.Hostname())
			}

			if url.Host != tc.Hostname {
				t.Fatalf("expected hostname %q but got %q", tc.Hostname, url.Host)
			}

			if url.Path != tc.Path {
				t.Fatalf("expected path %q but got %q", tc.Path, url.Path)
			}

			if len(url.Query()) != len(tc.Query) {
				t.Fatalf("unexpected query length: %q , expected %q",
					len(url.Query()), len(tc.Query))
			}

			for queryKey, queryArg := range tc.Query {
				if len(url.Query()[queryKey]) != len(queryArg) {
					t.Fatalf("unexpected query length: %q , expected %q",
						len(url.Query()[queryKey]), len(queryArg))
				}

				for queryArgPos := range queryArg {
					if url.Query()[queryKey][queryArgPos] != queryArg[queryArgPos] {
						t.Fatalf("unexpected query: %q=%q", queryArg[queryArgPos],
							url.Query()[queryKey][queryArgPos])
					}
				}
			}
		})
	}
}

func TestStaticDriverInfo(t *testing.T) {
	for _, tc := range []struct {
		Scenario string
		input    string
		needsMac bool
		driver   string
		boot     string
		management string
		power string
		raid string
		vendor string
	}{
		{
			Scenario: "ipmi",
			input:    "ipmi://192.168.122.1:6233",
			needsMac: false,
			driver:   "ipmi",
			boot:     "ipxe",
			management: "",
			power: "",
			raid: "",
			vendor: "",
		},

		{
			Scenario: "libvirt",
			input:    "libvirt://192.168.122.1",
			needsMac: true,
			driver:   "ipmi",
			boot:     "ipxe",
			management: "",
			power: "",
			raid: "",
			vendor: "",
		},

		{
			Scenario: "idrac",
			input:    "idrac://192.168.122.1",
			needsMac: false,
			driver:   "idrac",
			boot:     "ipxe",
			management: "",
			power: "",
			raid: "",
			vendor: "",
		},

		{
			Scenario: "irmc",
			input:    "irmc://192.168.122.1",
			needsMac: false,
			driver:   "irmc",
			boot:     "pxe",
			management: "",
			power: "",
			raid: "irmc",
			vendor: "",
		},

		{
			Scenario: "redfish",
			input:    "redfish://192.168.122.1",
			needsMac: true,
			driver:   "redfish",
			boot:     "ipxe",
			management: "",
			power: "",
			raid: "",
			vendor: "",
		},

		{
			Scenario: "redfish virtual media",
			input:    "redfish-virtualmedia://192.168.122.1",
			needsMac: true,
			driver:   "redfish",
			boot:     "redfish-virtual-media",
			management: "",
			power: "",
			raid: "",
			vendor: "",
		},

		{
			Scenario: "ilo5 virtual media",
			input:    "ilo5-virtualmedia://192.168.122.1",
			needsMac: true,
			driver:   "redfish",
			boot:     "redfish-virtual-media",
		},

		{
			Scenario: "idrac virtual media",
			input:    "idrac-virtualmedia://192.168.122.1",
			needsMac: true,
			driver:   "idrac",
			boot:     "idrac-redfish-virtual-media",
			management: "idrac-redfish",
			power: "idrac-redfish",
			raid: "no-raid",
			vendor: "no-vendor",
		},
	} {
		t.Run(tc.Scenario, func(t *testing.T) {
			acc, err := NewAccessDetails(tc.input, false)
			if err != nil {
				t.Fatalf("unexpected parse error: %v", err)
			}
			if acc.NeedsMAC() != tc.needsMac {
				t.Fatalf("MAC needed: %v , expected %v", acc.NeedsMAC(), tc.needsMac)
			}
			if acc.Driver() != tc.driver {
				t.Fatalf("Unexpected driver %q, expected %q", acc.Driver(), tc.driver)
			}
			if acc.BootInterface() != tc.boot {
				t.Fatalf("Unexpected boot interface %q, expected %q",
					acc.BootInterface(), tc.boot)
			}
		})
	}
}

func TestDriverInfo(t *testing.T) {
	for _, tc := range []struct {
		Scenario string
		input    string
		expects  map[string]interface{}
	}{
		{
			Scenario: "ipmi default port",
			input:    "ipmi://192.168.122.1",
			expects: map[string]interface{}{
				"ipmi_port":      ipmiDefaultPort,
				"ipmi_password":  "",
				"ipmi_username":  "",
				"ipmi_address":   "192.168.122.1",
				"ipmi_verify_ca": false,
			},
		},

		{
			Scenario: "idrac",
			input:    "idrac://192.168.122.1",
			expects: map[string]interface{}{
				"drac_address":   "192.168.122.1",
				"drac_password":  "",
				"drac_username":  "",
				"drac_verify_ca": false,
			},
		},

		{
			Scenario: "idrac http",
			input:    "idrac+http://192.168.122.1",
			expects: map[string]interface{}{
				"drac_address":   "192.168.122.1",
				"drac_protocol":  "http",
				"drac_password":  "",
				"drac_username":  "",
				"drac_verify_ca": false,
			},
		},

		{
			Scenario: "idrac https",
			input:    "idrac+https://192.168.122.1",
			expects: map[string]interface{}{
				"drac_address":   "192.168.122.1",
				"drac_protocol":  "https",
				"drac_password":  "",
				"drac_username":  "",
				"drac_verify_ca": false,
			},
		},

		{
			Scenario: "idrac port and path http",
			input:    "idrac://192.168.122.1:8080/foo",
			expects: map[string]interface{}{
				"drac_address":   "192.168.122.1",
				"drac_port":      "8080",
				"drac_path":      "/foo",
				"drac_password":  "",
				"drac_username":  "",
				"drac_verify_ca": false,
			},
		},

		{
			Scenario: "idrac ipv6",
			input:    "idrac://[fe80::fc33:62ff:fe83:8a76]/foo",
			expects: map[string]interface{}{
				"drac_address":   "fe80::fc33:62ff:fe83:8a76",
				"drac_path":      "/foo",
				"drac_password":  "",
				"drac_username":  "",
				"drac_verify_ca": false,
			},
		},

		{
			Scenario: "idrac ipv6 port and path",
			input:    "idrac://[fe80::fc33:62ff:fe83:8a76]:8080/foo",
			expects: map[string]interface{}{
				"drac_address":   "fe80::fc33:62ff:fe83:8a76",
				"drac_port":      "8080",
				"drac_path":      "/foo",
				"drac_password":  "",
				"drac_username":  "",
				"drac_verify_ca": false,
			},
		},

		{
			Scenario: "irmc",
			input:    "irmc://192.168.122.1",
			expects: map[string]interface{}{
				"irmc_address":   "192.168.122.1",
				"irmc_password":  "",
				"irmc_username":  "",
				"irmc_verify_ca": false,
			},
		},

		{
			Scenario: "irmc port",
			input:    "irmc://192.168.122.1:8080",
			expects: map[string]interface{}{
				"irmc_address":   "192.168.122.1",
				"irmc_port":      "8080",
				"irmc_password":  "",
				"irmc_username":  "",
				"irmc_verify_ca": false,
			},
		},

		{
			Scenario: "irmc ipv6",
			input:    "irmc://[fe80::fc33:62ff:fe83:8a76]",
			expects: map[string]interface{}{
				"irmc_address":   "fe80::fc33:62ff:fe83:8a76",
				"irmc_password":  "",
				"irmc_username":  "",
				"irmc_verify_ca": false,
			},
		},

		{
			Scenario: "irmc ipv6 port",
			input:    "irmc://[fe80::fc33:62ff:fe83:8a76]:8080",
			expects: map[string]interface{}{
				"irmc_address":   "fe80::fc33:62ff:fe83:8a76",
				"irmc_port":      "8080",
				"irmc_password":  "",
				"irmc_username":  "",
				"irmc_verify_ca": false,
			},
		},

		{
			Scenario: "Redfish",
			input:    "redfish://192.168.122.1/foo/bar",
			expects: map[string]interface{}{
				"redfish_address":   "https://192.168.122.1",
				"redfish_system_id": "/foo/bar",
				"redfish_password":  "",
				"redfish_username":  "",
				"redfish_verify_ca": false,
			},
		},

		{
			Scenario: "Redfish http",
			input:    "redfish+http://192.168.122.1/foo/bar",
			expects: map[string]interface{}{
				"redfish_address":   "http://192.168.122.1",
				"redfish_system_id": "/foo/bar",
				"redfish_password":  "",
				"redfish_username":  "",
				"redfish_verify_ca": false,
			},
		},

		{
			Scenario: "Redfish https",
			input:    "redfish+https://192.168.122.1/foo/bar",
			expects: map[string]interface{}{
				"redfish_address":   "https://192.168.122.1",
				"redfish_system_id": "/foo/bar",
				"redfish_password":  "",
				"redfish_username":  "",
				"redfish_verify_ca": false,
			},
		},

		{
			Scenario: "Redfish port",
			input:    "redfish://192.168.122.1:8080/foo/bar",
			expects: map[string]interface{}{
				"redfish_address":   "https://192.168.122.1:8080",
				"redfish_system_id": "/foo/bar",
				"redfish_password":  "",
				"redfish_username":  "",
				"redfish_verify_ca": false,
			},
		},

		{
			Scenario: "Redfish ipv6",
			input:    "redfish://[fe80::fc33:62ff:fe83:8a76]/foo/bar",
			expects: map[string]interface{}{
				"redfish_address":   "https://[fe80::fc33:62ff:fe83:8a76]",
				"redfish_system_id": "/foo/bar",
				"redfish_password":  "",
				"redfish_username":  "",
				"redfish_verify_ca": false,
			},
		},

		{
			Scenario: "Redfish ipv6 port",
			input:    "redfish://[fe80::fc33:62ff:fe83:8a76]:8080/foo",
			expects: map[string]interface{}{
				"redfish_address":   "https://[fe80::fc33:62ff:fe83:8a76]:8080",
				"redfish_system_id": "/foo",
				"redfish_password":  "",
				"redfish_username":  "",
				"redfish_verify_ca": false,
			},
		},

		{
			Scenario: "Redfish virtual media",
			input:    "redfish-virtualmedia://192.168.122.1/foo/bar",
			expects: map[string]interface{}{
				"redfish_address":   "https://192.168.122.1",
				"redfish_system_id": "/foo/bar",
				"redfish_password":  "",
				"redfish_username":  "",
				"redfish_verify_ca": false,
			},
		},

		{
			Scenario: "ilo5 virtual media",
			input:    "ilo5-virtualmedia://192.168.122.1/foo/bar",
			expects: map[string]interface{}{
				"redfish_address":   "https://192.168.122.1",
				"redfish_system_id": "/foo/bar",
				"redfish_password":  "",
				"redfish_username":  "",
				"redfish_verify_ca": false,
			},
		},

		{
			Scenario: "idrac virtual media",
			input:    "idrac-virtualmedia://192.168.122.1/foo/bar",
			expects: map[string]interface{}{
				"redfish_address":   "https://192.168.122.1",
				"redfish_system_id": "/foo/bar",
				"redfish_password":  "",
				"redfish_username":  "",
				"redfish_verify_ca": false,
			},
		},
	} {
		t.Run(tc.Scenario, func(t *testing.T) {
			acc, err := NewAccessDetails(tc.input, true)
			if err != nil {
				t.Fatalf("unexpected parse error: %v", err)
			}
			di := acc.DriverInfo(Credentials{})
			//If a key is present when it should not, this will catch it
			if len(di) != len(tc.expects) {
				t.Fatalf("Number of items do not match: %v and %v, %#v", len(di),
					len(tc.expects), di)
			}
			for expectKey, expectArg := range tc.expects {
				value, ok := di[expectKey]
				if value != expectArg && ok {
					t.Fatalf("unexpected value for %v (key present: %v): %v, expected %v",
						ok, expectKey, value, expectArg)
				}
			}
		})
	}
}

func TestUnknownType(t *testing.T) {
	acc, err := NewAccessDetails("foo://192.168.122.1", false)
	if err == nil || acc != nil {
		t.Fatalf("unexpected parse success")
	}
}
