// SPDX-License-Identifier: Apache-2.0

package redfish_test

import (
	"strings"
	"testing"

	"metal3.local/anaconda/internal/redfish"
)

// Deploying means inserting virtual media, so the address has to say so, and
// the vendor schemes select Ironic drivers this provisioner does not use.
func TestParseRedfishAddress(t *testing.T) {
	cases := map[string]struct {
		address      string
		wantEndpoint string
		wantSystemID string
		wantErr      string
	}{
		"virtualmedia with a system": {
			address:      "redfish-virtualmedia://bmc.example/redfish/v1/Systems/1",
			wantEndpoint: testEndpoint,
			wantSystemID: systemPath,
		},
		"virtualmedia over http": {
			address:      "redfish-virtualmedia+http://bmc.example:8000/redfish/v1/Systems/vm1",
			wantEndpoint: "http://bmc.example:8000",
			wantSystemID: "/redfish/v1/Systems/vm1",
		},
		"virtualmedia over https": {
			address:      "redfish-virtualmedia+https://bmc.example/redfish/v1/Systems/1",
			wantEndpoint: testEndpoint,
			wantSystemID: systemPath,
		},
		"no system in the path": {
			address:      "redfish-virtualmedia://bmc.example/redfish/v1",
			wantEndpoint: testEndpoint,
			wantSystemID: "",
		},
		"plain redfish cannot insert media": {
			address: "redfish://bmc.example/redfish/v1/Systems/1",
			wantErr: unsupportedErr,
		},
		"uefihttp cannot insert media": {
			address: "redfish-uefihttp://bmc.example/redfish/v1/Systems/1",
			wantErr: unsupportedErr,
		},
		"vendor virtualmedia": {
			address: "idrac-virtualmedia://bmc.example/redfish/v1/Systems/1",
			wantErr: unsupportedErr,
		},
		"ilo5 virtualmedia": {
			address: "ilo5-virtualmedia://bmc.example/redfish/v1/Systems/1",
			wantErr: unsupportedErr,
		},
		"ipmi": {
			address: "ipmi://bmc.example",
			wantErr: unsupportedErr,
		},
		// BMO parses a schemeless address as ipmi, so the rejection has to talk
		// about the missing scheme or it reads as nonsense.
		"bare host": {
			address: "192.168.1.1",
			wantErr: "has no scheme",
		},
		"empty": {
			address: "",
			wantErr: "is empty",
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := redfish.ParseRedfishAddress(tc.address)

			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("ParseRedfishAddress(%q) = %+v, want an error", tc.address, got)
				}

				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Errorf("error = %v, want it to mention %q", err, tc.wantErr)
				}

				if redfish.UsableBMCAddress(tc.address) {
					t.Error("UsableBMCAddress accepted an address ParseRedfishAddress rejected")
				}

				return
			}

			if err != nil {
				t.Fatalf("ParseRedfishAddress(%q): %v", tc.address, err)
			}

			if got.Endpoint != tc.wantEndpoint || got.SystemID != tc.wantSystemID {
				t.Errorf("= %+v, want endpoint %q system %q", got, tc.wantEndpoint, tc.wantSystemID)
			}
		})
	}
}
