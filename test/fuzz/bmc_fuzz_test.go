package fuzz

import (
	"testing"

	"github.com/metal3-io/baremetal-operator/pkg/hardwareutils/bmc"
)

func FuzzGetParsedURL(f *testing.F) {
	// Add seed corpus
	f.Add("")
	f.Add("192.168.122.1")
	f.Add("192.168.122.1:6233")
	f.Add("ipmi://192.168.122.1:6233")
	f.Add("redfish://192.168.122.1")
	f.Add("libvirt://192.168.122.1:6233/?abc=def")
	f.Add("[fe80::fc33:62ff:fe83:8a76]:6233")
	f.Add("my.favoritebmc.com")

	f.Fuzz(func(t *testing.T, address string) {
		parsedURL, err := bmc.GetParsedURL(address)
		if err != nil {
			// Error is valid, skip
			return
		}

		// If no error, URL must be non-nil
		if parsedURL == nil {
			t.Fatalf("GetParsedURL(%q) returned nil URL without error", address)
		}

		// Attempt round-trip parsing for well-formed URLs
		// Skip if hostname or scheme is empty
		if parsedURL.Hostname() != "" && parsedURL.Scheme != "" {
			_, err := bmc.GetParsedURL(parsedURL.String())
			// Round-trip errors are acceptable for some edge cases
			// We just verify it doesn't panic
			_ = err
		}
	})
}
