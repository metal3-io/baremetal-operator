// SPDX-License-Identifier: Apache-2.0

package httpapi_test

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"metal3.local/anaconda/internal/httpapi"
)

func TestAnacondaMACs(t *testing.T) {
	cases := []struct {
		name    string
		headers map[string]string
		want    []string
	}{
		{
			name: "one per interface, sorted and deduplicated",
			headers: map[string]string{
				httpapi.AnacondaMACHeaderPrefix + "0": "eth0 aa:bb:cc:dd:ee:02",
				httpapi.AnacondaMACHeaderPrefix + "1": "eth1 aa:bb:cc:dd:ee:01",
				httpapi.AnacondaMACHeaderPrefix + "2": "bond0 aa:bb:cc:dd:ee:01",
			},
			want: []string{testMAC, "aa:bb:cc:dd:ee:02"},
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
			headers: map[string]string{httpapi.AnacondaMACHeaderPrefix + "0": "eth0 not-a-mac"},
			want:    []string{},
		},
		{
			name:    "value with no interface prefix still parses",
			headers: map[string]string{httpapi.AnacondaMACHeaderPrefix + "0": testMAC},
			want:    []string{testMAC},
		},
		{
			name:    "hyphen notation normalizes to colons",
			headers: map[string]string{httpapi.AnacondaMACHeaderPrefix + "0": "eth0 AA-BB-CC-DD-EE-01"},
			want:    []string{testMAC},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/ks/kickstart", http.NoBody)
			for k, v := range tc.headers {
				req.Header.Set(k, v)
			}

			if got := httpapi.AnacondaMACs(req); !slices.Equal(got, tc.want) {
				t.Errorf("AnacondaMACs = %v, want %v", got, tc.want)
			}
		})
	}
}

// The fallback must never carry a storage command, that is the whole point of it.
func TestDefaultFallbackKickstartIsInert(t *testing.T) {
	ks := httpapi.DefaultFallbackKickstart

	for _, forbidden := range []string{"clearpart", "autopart", "part ", "zerombr", "ignoredisk", "bootloader"} {
		if strings.Contains(ks, forbidden) {
			t.Errorf("fallback kickstart contains %q, it must not touch storage", forbidden)
		}
	}

	if !strings.Contains(httpapi.DefaultFallbackKickstart, "poweroff") {
		t.Error("fallback kickstart does not power the machine off")
	}
}
