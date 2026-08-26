/*

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package starlark

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
)

func TestNormalizeMAC(t *testing.T) {
	cases := map[string]string{
		"aa:bb:cc:dd:ee:01":     "aa:bb:cc:dd:ee:01",
		"AA:BB:CC:DD:EE:01":     "aa:bb:cc:dd:ee:01",
		"aa-bb-cc-dd-ee-01":     "aa:bb:cc:dd:ee:01",
		"aabb.ccdd.ee01":        "aa:bb:cc:dd:ee:01",
		"  aa:bb:cc:dd:ee:01  ": "aa:bb:cc:dd:ee:01",
		"":                      "",
		"not-a-mac":             "",
		"aa:bb:cc":              "",
		"eth0":                  "",
	}

	for in, want := range cases {
		if got := NormalizeMAC(in); got != want {
			t.Errorf("NormalizeMAC(%q) = %q, want %q", in, got, want)
		}
	}
}

// Go canonicalizes the wire name to X-Rhn-Provisioning-Mac-0, so a case sensitive
// prefix compare finds nothing and every host silently gets the fallback.
func TestAnacondaMACsSurvivesHeaderCanonicalization(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/ks/kickstart", http.NoBody)
	req.Header.Set("X-RHN-Provisioning-MAC-0", "eth0 aa:bb:cc:dd:ee:01")

	stored := req.Header.Get("X-Rhn-Provisioning-Mac-0")
	if stored == "" {
		t.Fatal("header did not canonicalize as expected, the test premise is wrong")
	}

	got := AnacondaMACs(req)
	if !slices.Equal(got, []string{"aa:bb:cc:dd:ee:01"}) {
		t.Errorf("AnacondaMACs = %v, want the single MAC", got)
	}
}

func TestAnacondaMACs(t *testing.T) {
	cases := []struct {
		name    string
		headers map[string]string
		want    []string
	}{
		{
			name: "one per interface, sorted and deduplicated",
			headers: map[string]string{
				"X-RHN-Provisioning-MAC-0": "eth0 aa:bb:cc:dd:ee:02",
				"X-RHN-Provisioning-MAC-1": "eth1 aa:bb:cc:dd:ee:01",
				"X-RHN-Provisioning-MAC-2": "bond0 aa:bb:cc:dd:ee:01",
			},
			want: []string{"aa:bb:cc:dd:ee:01", "aa:bb:cc:dd:ee:02"},
		},
		{
			name:    "no headers at all",
			headers: map[string]string{},
			want:    []string{},
		},
		{
			name:    "unrelated headers are ignored",
			headers: map[string]string{"User-Agent": "curl/8.0", "X-System-Serial-Number": "1234"},
			want:    []string{},
		},
		{
			name:    "malformed value is dropped",
			headers: map[string]string{"X-RHN-Provisioning-MAC-0": "eth0 not-a-mac"},
			want:    []string{},
		},
		{
			name:    "value with no interface prefix still parses",
			headers: map[string]string{"X-RHN-Provisioning-MAC-0": "aa:bb:cc:dd:ee:01"},
			want:    []string{"aa:bb:cc:dd:ee:01"},
		},
		{
			name:    "hyphen notation normalizes to colons",
			headers: map[string]string{"X-RHN-Provisioning-MAC-0": "eth0 AA-BB-CC-DD-EE-01"},
			want:    []string{"aa:bb:cc:dd:ee:01"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/ks/kickstart", http.NoBody)
			for k, v := range tc.headers {
				req.Header.Set(k, v)
			}

			if got := AnacondaMACs(req); !slices.Equal(got, tc.want) {
				t.Errorf("AnacondaMACs = %v, want %v", got, tc.want)
			}
		})
	}
}

// The fallback must never carry a storage command, that is the whole point of it.
func TestDefaultFallbackKickstartIsInert(t *testing.T) {
	for _, forbidden := range []string{"clearpart", "autopart", "part ", "zerombr", "ignoredisk", "bootloader"} {
		if strings.Contains(DefaultFallbackKickstart, forbidden) {
			t.Errorf("fallback kickstart contains %q, it must not touch storage", forbidden)
		}
	}

	if !strings.Contains(DefaultFallbackKickstart, "poweroff") {
		t.Error("fallback kickstart does not power the machine off")
	}
}
