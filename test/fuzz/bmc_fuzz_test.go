package fuzz

import (
	"testing"

	"github.com/metal3-io/baremetal-operator/pkg/hardwareutils/bmc"
)

func FuzzGetParsedURL(f *testing.F) {
	// Add seed corpus
	f.Add("ipmi://192.168.122.1:6233")
	f.Add("redfish://192.168.122.1")
	f.Add("idrac-virtualmedia://192.168.122.1")
	f.Add("idrac-virtualmedia://192.168.122.1:443")
	f.Add("ilo5-redfish://192.168.122.1")
	f.Add("ilo5-redfish://ilo.example.com")
	f.Add("idrac-redfish://192.168.122.1")
	f.Add("idrac-redfish://idrac.example.com:443")
	f.Add("redfish+http://192.168.122.1")
	f.Add("idrac-virtualmedia+http://192.168.122.1")
	f.Add("idrac-virtualmedia+http://192.168.122.1:443")
	f.Add("ilo5-redfish+http://192.168.122.1")
	f.Add("ilo5-redfish+http://ilo.example.com")
	f.Add("idrac-redfish+http://192.168.122.1")
	f.Add("idrac-redfish+http://idrac.example.com:443")
	f.Add("redfish+https://192.168.122.1")
	f.Add("redfish+https://redfish.example.com:8443")
	f.Add("redfish-virtualmedia://192.168.122.1")
	f.Add("redfish-virtualmedia://192.168.122.1:443/redfish/v1")
	f.Add("redfish-virtualmedia+http://192.168.122.1")
	f.Add("redfish-virtualmedia+http://192.168.122.1:443/redfish/v1")
	f.Add("ilo5-virtualmedia://192.168.122.1")
	f.Add("ilo5-virtualmedia://ilo.example.com:443")
	f.Add("ilo5-virtualmedia+http://192.168.122.1")
	f.Add("ilo5-virtualmedia+http://ilo.example.com:443")
	f.Add("redfish-uefi+http://192.168.122.1")
	f.Add("redfish-uefi+http://192.168.122.1:8000/boot")
	f.Add("redfish-uefi+https://192.168.122.1")
	f.Add("redfish-uefi+https://192.168.122.1:8000/boot")
	f.Add("ilo5-virtualmedia+https://192.168.122.1")
	f.Add("ilo5-virtualmedia+https://ilo.example.com:443")
	f.Add("redfish-virtualmedia+https://192.168.122.1")
	f.Add("redfish-virtualmedia+https://192.168.122.1:443/redfish/v1")
	f.Add("redfish+https://192.168.122.1")
	f.Add("idrac-virtualmedia+https://192.168.122.1")
	f.Add("idrac-virtualmedia+https://192.168.122.1:443")
	f.Add("ilo5-redfish+https://192.168.122.1")
	f.Add("ilo5-redfish+https://ilo.example.com")
	f.Add("idrac-redfish+https://192.168.122.1")
	f.Add("idrac-redfish+https://idrac.example.com:443")

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

		// Verify we can safely access URL properties without panicking
		_ = parsedURL.Scheme
		_ = parsedURL.Host
		_ = parsedURL.Hostname()
		_ = parsedURL.Port()
		_ = parsedURL.Path
		_ = parsedURL.RawQuery
		_ = parsedURL.String()
	})
}
