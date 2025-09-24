package bmc

import (
	"reflect"
	"testing"
)

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
				"abc": {"def"},
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
			Scenario: "valid dns hostname",
			Address:  "my.examplehost.com",
			Type:     "ipmi",
			Port:     "",
			Host:     "my.examplehost.com",
			Hostname: "my.examplehost.com",
			Path:     "",
		},

		{
			Scenario:    "invalid dns hostname",
			Address:     "my-.examplehost.com",
			Type:        "ipmi",
			Port:        "",
			Host:        "my-.examplehost.com",
			Hostname:    "my-.examplehost.com",
			Path:        "",
			ExpectError: true,
		},

		{
			Scenario:    "invalid ipv6 host address",
			Address:     "[fe80::fc33:62ff:fe33:8xff]:6223",
			Type:        "ipmi",
			Port:        "6223",
			Host:        "fe80::fc33:62ff:fe33.8xff",
			Hostname:    "[fe80::fc33:62ff:fe33:8xff]:6223",
			Path:        "",
			ExpectError: true,
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

		{
			Scenario: "ilo4 url",
			Address:  "ilo4://192.168.122.1",
			Type:     "ilo4",
			Port:     "",
			Host:     "192.168.122.1",
			Hostname: "192.168.122.1",
			Path:     "",
		},

		{
			Scenario: "ilo4 url, ipv6",
			Address:  "ilo4://[fe80::fc33:62ff:fe83:8a76]",
			Type:     "ilo4",
			Port:     "",
			Host:     "fe80::fc33:62ff:fe83:8a76",
			Hostname: "[fe80::fc33:62ff:fe83:8a76]",
			Path:     "",
		},

		{
			Scenario: "ilo4 url, no sep",
			Address:  "ilo4:192.168.122.1",
			Type:     "ilo4",
			Port:     "",
			Host:     "192.168.122.1",
			Hostname: "192.168.122.1",
			Path:     "",
		},

		{
			Scenario: "ilo5 url",
			Address:  "ilo5://192.168.122.1",
			Type:     "ilo5",
			Port:     "",
			Host:     "192.168.122.1",
			Hostname: "192.168.122.1",
			Path:     "",
		},

		{
			Scenario: "ilo5 url, ipv6",
			Address:  "ilo5://[fe80::fc33:62ff:fe83:8a76]",
			Type:     "ilo5",
			Port:     "",
			Host:     "fe80::fc33:62ff:fe83:8a76",
			Hostname: "[fe80::fc33:62ff:fe83:8a76]",
			Path:     "",
		},

		{
			Scenario: "ilo5 url, no sep",
			Address:  "ilo5:192.168.122.1",
			Type:     "ilo5",
			Port:     "",
			Host:     "192.168.122.1",
			Hostname: "192.168.122.1",
			Path:     "",
		},
	} {
		t.Run(tc.Scenario, func(t *testing.T) {
			url, err := GetParsedURL(tc.Address)

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
		Scenario   string
		input      string
		needsMac   bool
		driver     string
		bios       string
		boot       string
		firmware   string
		management string
		power      string
		vendor     string
	}{
		{
			Scenario:   "ipmi",
			input:      "ipmi://192.168.122.1:6233",
			needsMac:   false,
			driver:     "ipmi",
			bios:       "",
			boot:       "ipxe",
			firmware:   "",
			management: "",
			power:      "",
		},

		{
			Scenario:   "libvirt",
			input:      "libvirt://192.168.122.1",
			needsMac:   true,
			driver:     "ipmi",
			bios:       "",
			boot:       "ipxe",
			firmware:   "",
			management: "",
			power:      "",
		},

		{
			Scenario:   "irmc",
			input:      "irmc://192.168.122.1",
			needsMac:   true,
			driver:     "irmc",
			bios:       "",
			boot:       "ipxe",
			firmware:   "",
			management: "",
			power:      "",
		},

		{
			Scenario:   "redfish",
			input:      "redfish://192.168.122.1",
			needsMac:   true,
			driver:     "redfish",
			bios:       "",
			boot:       "ipxe",
			firmware:   "redfish",
			management: "",
			power:      "",
		},

		{
			Scenario:   "redfish virtual media",
			input:      "redfish-virtualmedia://192.168.122.1",
			needsMac:   true,
			driver:     "redfish",
			bios:       "",
			boot:       "redfish-virtual-media",
			firmware:   "redfish",
			management: "",
			power:      "",
		},

		{
			Scenario:   "redfish virtual media HTTP",
			input:      "redfish-virtualmedia+http://192.168.122.1",
			needsMac:   true,
			driver:     "redfish",
			bios:       "",
			boot:       "redfish-virtual-media",
			firmware:   "redfish",
			management: "",
			power:      "",
		},

		{
			Scenario:   "redfish virtual media HTTPS",
			input:      "redfish-virtualmedia+https://192.168.122.1",
			needsMac:   true,
			driver:     "redfish",
			bios:       "",
			boot:       "redfish-virtual-media",
			firmware:   "redfish",
			management: "",
			power:      "",
		},

		{
			Scenario:   "redfish uefi http boot",
			input:      "redfish-uefihttp+https://192.168.122.1",
			needsMac:   true,
			driver:     "redfish",
			bios:       "",
			boot:       "redfish-https",
			firmware:   "redfish",
			management: "",
			power:      "",
		},

		{
			Scenario:   "idrac redfish",
			input:      "idrac-redfish://192.168.122.1",
			needsMac:   true,
			driver:     "idrac",
			bios:       "idrac-redfish",
			boot:       "ipxe",
			firmware:   "redfish",
			management: "idrac-redfish",
			power:      "idrac-redfish",
			vendor:     "idrac-redfish",
		},

		{
			Scenario: "ilo5 virtual media",
			input:    "ilo5-virtualmedia://192.168.122.1",
			needsMac: true,
			driver:   "redfish",
			bios:     "",
			boot:     "redfish-virtual-media",
			firmware: "redfish",
		},

		{
			Scenario: "ilo5 virtual media HTTP",
			input:    "ilo5-virtualmedia+http://192.168.122.1",
			needsMac: true,
			driver:   "redfish",
			bios:     "",
			boot:     "redfish-virtual-media",
			firmware: "redfish",
		},

		{
			Scenario: "ilo5 virtual media HTTPS",
			input:    "ilo5-virtualmedia+https://192.168.122.1",
			needsMac: true,
			driver:   "redfish",
			bios:     "",
			boot:     "redfish-virtual-media",
			firmware: "redfish",
		},

		{
			Scenario:   "idrac virtual media",
			input:      "idrac-virtualmedia://192.168.122.1",
			needsMac:   true,
			driver:     "idrac",
			bios:       "idrac-redfish",
			boot:       "idrac-redfish-virtual-media",
			firmware:   "redfish",
			management: "idrac-redfish",
			power:      "idrac-redfish",
			vendor:     "idrac-redfish",
		},

		{
			Scenario:   "idrac virtual media HTTP",
			input:      "idrac-virtualmedia+http://192.168.122.1",
			needsMac:   true,
			driver:     "idrac",
			bios:       "idrac-redfish",
			boot:       "idrac-redfish-virtual-media",
			firmware:   "redfish",
			management: "idrac-redfish",
			power:      "idrac-redfish",
			vendor:     "idrac-redfish",
		},

		{
			Scenario:   "idrac virtual media HTTPS",
			input:      "idrac-virtualmedia+https://192.168.122.1",
			needsMac:   true,
			driver:     "idrac",
			bios:       "idrac-redfish",
			boot:       "idrac-redfish-virtual-media",
			firmware:   "redfish",
			management: "idrac-redfish",
			power:      "idrac-redfish",
			vendor:     "idrac-redfish",
		},

		{
			Scenario:   "ilo4",
			input:      "ilo4://192.168.122.1",
			needsMac:   true,
			driver:     "ilo",
			bios:       "",
			boot:       "ilo-ipxe",
			firmware:   "",
			management: "",
			power:      "",
		},

		{
			Scenario:   "ilo4 virtual media",
			input:      "ilo4-virtualmedia://192.168.122.1",
			needsMac:   true,
			driver:     "ilo",
			bios:       "",
			boot:       "ilo-virtual-media",
			firmware:   "",
			management: "",
			power:      "",
		},

		{
			Scenario:   "ilo5",
			input:      "ilo5://192.168.122.1",
			needsMac:   true,
			driver:     "ilo5",
			bios:       "",
			boot:       "ilo-ipxe",
			firmware:   "",
			management: "",
			power:      "",
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
			if acc.BIOSInterface() != tc.bios {
				t.Fatalf("Unexpected bios interface %q, expected %q",
					acc.BIOSInterface(), tc.bios)
			}
			if acc.FirmwareInterface() != tc.firmware {
				t.Fatalf("Unexpected firmware interface %q, expected %q",
					acc.FirmwareInterface(), tc.firmware)
			}
			if acc.VendorInterface() != tc.vendor {
				t.Fatalf("Unexpected vendor interface %q, expected %q",
					acc.VendorInterface(), tc.vendor)
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
				"ipmi_port":            ipmiDefaultPort,
				"ipmi_password":        "",
				"ipmi_username":        "",
				"ipmi_address":         "192.168.122.1",
				"ipmi_verify_ca":       false,
				"ipmi_priv_level":      "ADMINISTRATOR",
				"kernel_append_params": "",
			},
		},

		{
			Scenario: "ipmi setting privilege level",
			input:    "ipmi://192.168.122.1?privilegelevel=OPERATOR",
			expects: map[string]interface{}{
				"ipmi_port":            ipmiDefaultPort,
				"ipmi_password":        "",
				"ipmi_username":        "",
				"ipmi_address":         "192.168.122.1",
				"ipmi_verify_ca":       false,
				"ipmi_priv_level":      "OPERATOR",
				"kernel_append_params": "",
			},
		},

		{
			Scenario: "irmc",
			input:    "irmc://192.168.122.1",
			expects: map[string]interface{}{
				"irmc_address":         "192.168.122.1",
				"irmc_password":        "",
				"irmc_username":        "",
				"ipmi_address":         "192.168.122.1",
				"ipmi_password":        "",
				"ipmi_username":        "",
				"irmc_verify_ca":       false,
				"kernel_append_params": "",
			},
		},

		{
			Scenario: "irmc port",
			input:    "irmc://192.168.122.1:8080",
			expects: map[string]interface{}{
				"irmc_address":         "192.168.122.1",
				"irmc_port":            "8080",
				"irmc_password":        "",
				"irmc_username":        "",
				"ipmi_address":         "192.168.122.1",
				"ipmi_password":        "",
				"ipmi_username":        "",
				"irmc_verify_ca":       false,
				"kernel_append_params": "",
			},
		},

		{
			Scenario: "irmc ipv6",
			input:    "irmc://[fe80::fc33:62ff:fe83:8a76]",
			expects: map[string]interface{}{
				"irmc_address":         "fe80::fc33:62ff:fe83:8a76",
				"irmc_password":        "",
				"irmc_username":        "",
				"ipmi_address":         "fe80::fc33:62ff:fe83:8a76",
				"ipmi_password":        "",
				"ipmi_username":        "",
				"irmc_verify_ca":       false,
				"kernel_append_params": "",
			},
		},

		{
			Scenario: "irmc ipv6 port",
			input:    "irmc://[fe80::fc33:62ff:fe83:8a76]:8080",
			expects: map[string]interface{}{
				"irmc_address":         "fe80::fc33:62ff:fe83:8a76",
				"irmc_port":            "8080",
				"irmc_password":        "",
				"irmc_username":        "",
				"ipmi_address":         "fe80::fc33:62ff:fe83:8a76",
				"ipmi_password":        "",
				"ipmi_username":        "",
				"irmc_verify_ca":       false,
				"kernel_append_params": "",
			},
		},

		{
			Scenario: "Redfish",
			input:    "redfish://192.168.122.1/redfish/v1/foo/bar",
			expects: map[string]interface{}{
				"redfish_address":      "https://192.168.122.1",
				"redfish_system_id":    "/redfish/v1/foo/bar",
				"redfish_password":     "",
				"redfish_username":     "",
				"redfish_verify_ca":    false,
				"kernel_append_params": "",
			},
		},

		{
			Scenario: "Redfish http",
			input:    "redfish+http://192.168.122.1/foo/bar",
			expects: map[string]interface{}{
				"redfish_address":      "http://192.168.122.1",
				"redfish_system_id":    "/foo/bar",
				"redfish_password":     "",
				"redfish_username":     "",
				"redfish_verify_ca":    false,
				"kernel_append_params": "",
			},
		},

		{
			Scenario: "Redfish https",
			input:    "redfish+https://192.168.122.1/foo/bar",
			expects: map[string]interface{}{
				"redfish_address":      "https://192.168.122.1",
				"redfish_system_id":    "/foo/bar",
				"redfish_password":     "",
				"redfish_username":     "",
				"redfish_verify_ca":    false,
				"kernel_append_params": "",
			},
		},

		{
			Scenario: "Redfish port",
			input:    "redfish://192.168.122.1:8080/foo/bar",
			expects: map[string]interface{}{
				"redfish_address":      "https://192.168.122.1:8080",
				"redfish_system_id":    "/foo/bar",
				"redfish_password":     "",
				"redfish_username":     "",
				"redfish_verify_ca":    false,
				"kernel_append_params": "",
			},
		},

		{
			Scenario: "Redfish ipv6",
			input:    "redfish://[fe80::fc33:62ff:fe83:8a76]/foo/bar",
			expects: map[string]interface{}{
				"redfish_address":      "https://[fe80::fc33:62ff:fe83:8a76]",
				"redfish_system_id":    "/foo/bar",
				"redfish_password":     "",
				"redfish_username":     "",
				"redfish_verify_ca":    false,
				"kernel_append_params": "",
			},
		},

		{
			Scenario: "Redfish ipv6 port",
			input:    "redfish://[fe80::fc33:62ff:fe83:8a76]:8080/foo",
			expects: map[string]interface{}{
				"redfish_address":      "https://[fe80::fc33:62ff:fe83:8a76]:8080",
				"redfish_system_id":    "/foo",
				"redfish_password":     "",
				"redfish_username":     "",
				"redfish_verify_ca":    false,
				"kernel_append_params": "",
			},
		},

		{
			Scenario: "Redfish no system ID",
			input:    "redfish+https://192.168.122.1/",
			expects: map[string]interface{}{
				"redfish_address":      "https://192.168.122.1",
				"redfish_password":     "",
				"redfish_username":     "",
				"redfish_verify_ca":    false,
				"kernel_append_params": "",
			},
		},

		{
			Scenario: "Redfish wrong system ID",
			input:    "redfish+https://192.168.122.1/redfish/v1/",
			expects: map[string]interface{}{
				"redfish_address":      "https://192.168.122.1",
				"redfish_password":     "",
				"redfish_username":     "",
				"redfish_verify_ca":    false,
				"kernel_append_params": "",
			},
		},

		{
			Scenario: "Redfish virtual media",
			input:    "redfish-virtualmedia://192.168.122.1/redfish/v1/foo/bar",
			expects: map[string]interface{}{
				"redfish_address":      "https://192.168.122.1",
				"redfish_system_id":    "/redfish/v1/foo/bar",
				"redfish_password":     "",
				"redfish_username":     "",
				"redfish_verify_ca":    false,
				"kernel_append_params": "",
			},
		},

		{
			Scenario: "Redfish virtual media wrong system ID",
			input:    "redfish-virtualmedia://192.168.122.1/redfish/v1/",
			expects: map[string]interface{}{
				"redfish_address":      "https://192.168.122.1",
				"redfish_password":     "",
				"redfish_username":     "",
				"redfish_verify_ca":    false,
				"kernel_append_params": "",
			},
		},

		{
			Scenario: "ilo5 virtual media",
			input:    "ilo5-virtualmedia://192.168.122.1/foo/bar",
			expects: map[string]interface{}{
				"redfish_address":      "https://192.168.122.1",
				"redfish_system_id":    "/foo/bar",
				"redfish_password":     "",
				"redfish_username":     "",
				"redfish_verify_ca":    false,
				"kernel_append_params": "",
			},
		},

		{
			Scenario: "idrac redfish",
			input:    "idrac-redfish://192.168.122.1/foo/bar",
			expects: map[string]interface{}{
				"redfish_address":      "https://192.168.122.1",
				"redfish_system_id":    "/foo/bar",
				"redfish_password":     "",
				"redfish_username":     "",
				"redfish_verify_ca":    false,
				"kernel_append_params": "",
			},
		},

		{
			Scenario: "idrac virtual media",
			input:    "idrac-virtualmedia://192.168.122.1/redfish/v1/foo/bar",
			expects: map[string]interface{}{
				"redfish_address":      "https://192.168.122.1",
				"redfish_system_id":    "/redfish/v1/foo/bar",
				"redfish_password":     "",
				"redfish_username":     "",
				"redfish_verify_ca":    false,
				"kernel_append_params": "",
			},
		},

		{
			Scenario: "idrac virtual media wrong system ID",
			input:    "idrac-virtualmedia://192.168.122.1/redfish/v1/",
			expects: map[string]interface{}{
				"redfish_address":      "https://192.168.122.1",
				"redfish_password":     "",
				"redfish_username":     "",
				"redfish_verify_ca":    false,
				"kernel_append_params": "",
			},
		},

		{
			Scenario: "ilo4",
			input:    "ilo4://192.168.122.1",
			expects: map[string]interface{}{
				"ilo_address":          "192.168.122.1",
				"ilo_password":         "",
				"ilo_username":         "",
				"ilo_verify_ca":        false,
				"kernel_append_params": "",
			},
		},

		{
			Scenario: "ilo4 port",
			input:    "ilo4://192.168.122.1:8080",
			expects: map[string]interface{}{
				"ilo_address":          "192.168.122.1",
				"client_port":          "8080",
				"ilo_password":         "",
				"ilo_username":         "",
				"ilo_verify_ca":        false,
				"kernel_append_params": "",
			},
		},

		{
			Scenario: "ilo4 ipv6",
			input:    "ilo4://[fe80::fc33:62ff:fe83:8a76]",
			expects: map[string]interface{}{
				"ilo_address":          "fe80::fc33:62ff:fe83:8a76",
				"ilo_password":         "",
				"ilo_username":         "",
				"ilo_verify_ca":        false,
				"kernel_append_params": "",
			},
		},

		{
			Scenario: "ilo4 ipv6 port",
			input:    "ilo4://[fe80::fc33:62ff:fe83:8a76]:8080",
			expects: map[string]interface{}{
				"ilo_address":          "fe80::fc33:62ff:fe83:8a76",
				"client_port":          "8080",
				"ilo_password":         "",
				"ilo_username":         "",
				"ilo_verify_ca":        false,
				"kernel_append_params": "",
			},
		},

		{
			Scenario: "ilo4 virtual media",
			input:    "ilo4-virtualmedia://192.168.122.1",
			expects: map[string]interface{}{
				"ilo_address":          "192.168.122.1",
				"ilo_password":         "",
				"ilo_username":         "",
				"ilo_verify_ca":        false,
				"kernel_append_params": "",
			},
		},

		{
			Scenario: "ilo5",
			input:    "ilo5://192.168.122.1",
			expects: map[string]interface{}{
				"ilo_address":          "192.168.122.1",
				"ilo_password":         "",
				"ilo_username":         "",
				"ilo_verify_ca":        false,
				"kernel_append_params": "",
			},
		},

		{
			Scenario: "ilo5 port",
			input:    "ilo5://192.168.122.1:8080",
			expects: map[string]interface{}{
				"ilo_address":          "192.168.122.1",
				"client_port":          "8080",
				"ilo_password":         "",
				"ilo_username":         "",
				"ilo_verify_ca":        false,
				"kernel_append_params": "",
			},
		},

		{
			Scenario: "ilo5 ipv6",
			input:    "ilo5://[fe80::fc33:62ff:fe83:8a76]",
			expects: map[string]interface{}{
				"ilo_address":          "fe80::fc33:62ff:fe83:8a76",
				"ilo_password":         "",
				"ilo_username":         "",
				"ilo_verify_ca":        false,
				"kernel_append_params": "",
			},
		},

		{
			Scenario: "ilo5 ipv6 port",
			input:    "ilo5://[fe80::fc33:62ff:fe83:8a76]:8080",
			expects: map[string]interface{}{
				"ilo_address":          "fe80::fc33:62ff:fe83:8a76",
				"client_port":          "8080",
				"ilo_password":         "",
				"ilo_username":         "",
				"ilo_verify_ca":        false,
				"kernel_append_params": "",
			},
		},
	} {
		t.Run(tc.Scenario, func(t *testing.T) {
			acc, err := NewAccessDetails(tc.input, true)
			if err != nil {
				t.Fatalf("unexpected parse error: %v", err)
			}
			di := acc.DriverInfo(Credentials{}, "")
			// If a key is present when it should not, this will catch it
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

func TestBuildBIOSCleanSteps(t *testing.T) {
	var True = true
	var False = false

	cases := []struct {
		name          string
		address       string
		firmware      *FirmwareConfig
		expected      []map[string]string
		expectedError bool
	}{
		// ilo4
		{
			name:    "ilo4",
			address: "ilo4://192.168.122.1",
			firmware: &FirmwareConfig{
				VirtualizationEnabled:             &True,
				SimultaneousMultithreadingEnabled: &False,
			},
			expected: []map[string]string{
				{
					"name":  "ProcVirtualization",
					"value": "Enabled",
				},
				{
					"name":  "ProcHyperthreading",
					"value": "Disabled",
				},
			},
		},
		{
			name:     "ilo4, firmware is nil",
			address:  "ilo4://192.168.122.1",
			firmware: nil,
			expected: nil,
		},
		{
			name:     "ilo4, firmware is empty",
			address:  "ilo4://192.168.122.1",
			firmware: &FirmwareConfig{},
			expected: nil,
		},
		// ilo5
		{
			name:    "ilo5",
			address: "ilo5://192.168.122.1",
			firmware: &FirmwareConfig{
				VirtualizationEnabled:             &True,
				SimultaneousMultithreadingEnabled: &False,
			},
			expected: []map[string]string{
				{
					"name":  "ProcVirtualization",
					"value": "Enabled",
				},
				{
					"name":  "ProcHyperthreading",
					"value": "Disabled",
				},
			},
		},
		{
			name:     "ilo5, firmware is nil",
			address:  "ilo5://192.168.122.1",
			firmware: nil,
			expected: nil,
		},
		{
			name:     "ilo5, firmware is empty",
			address:  "ilo5://192.168.122.1",
			firmware: &FirmwareConfig{},
			expected: nil,
		},
		// irmc
		{
			name:    "irmc",
			address: "irmc://192.168.122.1",
			firmware: &FirmwareConfig{
				VirtualizationEnabled:             &True,
				SimultaneousMultithreadingEnabled: &False,
			},
			expected: []map[string]string{
				{
					"name":  "cpu_vt_enabled",
					"value": "True",
				},
				{
					"name":  "hyper_threading_enabled",
					"value": "False",
				},
			},
		},
		{
			name:     "irmc, firmware is nil",
			address:  "irmc://192.168.122.1",
			firmware: nil,
			expected: nil,
		},
		{
			name:     "irmc, firmware is empty",
			address:  "irmc://192.168.122.1",
			firmware: &FirmwareConfig{},
			expected: nil,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			acc, err := NewAccessDetails(c.address, false)
			if err != nil {
				t.Fatalf("new AccessDetails failed: %v", err)
			}

			settings, err := acc.BuildBIOSSettings(c.firmware)
			if (err != nil) != c.expectedError {
				t.Fatalf("got unexpected error: %v", err)
			}

			if !reflect.DeepEqual(c.expected, settings) {
				t.Errorf("expected settings: %v, got: %v", c.expected, settings)
			}
		})
	}
}
