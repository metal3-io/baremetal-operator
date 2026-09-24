package fuzz

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner/ironic/devicehints"
)

// FuzzMakeHintMap tests RootDeviceHints conversion to hint map with various
// device hint configurations to ensure correct mapping logic.
func FuzzMakeHintMap(f *testing.F) {
	// Add seed corpus with various RootDeviceHints configurations
	f.Add([]byte(`{"deviceName":"/dev/sda"}`))
	f.Add([]byte(`{"deviceName":"/dev/vda"}`))
	f.Add([]byte(`{"deviceName":"/dev/nvme0n1"}`))
	f.Add([]byte(`{"deviceName":"/dev/disk/by-path/pci-0000:00:1f.2-ata-1.0"}`))
	f.Add([]byte(`{"deviceName":"/dev/disk/by-path/pci-0000:01:00.0-scsi-0:2:0:0"}`))
	f.Add([]byte(`{"hctl":"0:0:0:0"}`))
	f.Add([]byte(`{"hctl":"1:0:0:0"}`))
	f.Add([]byte(`{"model":"INTEL SSDSC2BB800G4","vendor":"ATA"}`))
	f.Add([]byte(`{"model":"SAMSUNG MZ7LH960","vendor":"Samsung"}`))
	f.Add([]byte(`{"vendor":"Dell","minSizeGigabytes":480}`))
	f.Add([]byte(`{"serialNumber":"PHYS1234567890","minSizeGigabytes":100}`))
	f.Add([]byte(`{"serialNumber":"S3Z9NX0M12345","rotational":false}`))
	f.Add([]byte(`{"wwn":"0x5000c500a1b2c3d4","rotational":true}`))
	f.Add([]byte(`{"wwn":"0x60022480","wwnWithExtension":"0x600224801234567890"}`))
	f.Add([]byte(`{"wwnVendorExtension":"0x1234567890abcdef"}`))
	f.Add([]byte(`{"rotational":true,"minSizeGigabytes":2000}`))
	f.Add([]byte(`{"deviceName":"/dev/sda","hctl":"0:0:0:0","model":"MTFDDAK480TDN","vendor":"Micron","serialNumber":"22323F1234AB","minSizeGigabytes":480,"wwn":"0x500a07512345abcd","rotational":false}`))
	f.Add([]byte(`{}`))

	// Non-accepted seeds: malformed input that json.Unmarshal rejects.
	// The fuzz target must skip these without panicking.
	f.Add([]byte(``))
	f.Add([]byte(`not json`))
	f.Add([]byte(`{invalid}`))
	f.Add([]byte(`{"deviceName":`))
	f.Add([]byte(`{"minSizeGigabytes":"not-a-number"}`))
	f.Add([]byte(`{"rotational":"maybe"}`))
	f.Add([]byte("\xff\xfe\x00\x01"))

	f.Fuzz(func(t *testing.T, data []byte) {
		hints := &metal3api.RootDeviceHints{}

		// Attempt to unmarshal the fuzzed data
		if err := json.Unmarshal(data, hints); err != nil {
			t.Skip("invalid JSON input")
		}

		// Test MakeHintMap - should never panic
		hintMap := devicehints.MakeHintMap(hints)

		// Verify basic invariants
		if hintMap == nil {
			t.Fatal("MakeHintMap returned nil map")
		}

		// Verify device name mapping logic
		if hints.DeviceName != "" {
			if len(hintMap) == 0 {
				t.Error("expected hints when DeviceName is set")
			}
			// Check correct mapping based on prefix
			if strings.HasPrefix(hints.DeviceName, "/dev/disk/by-path/") {
				if got, want := hintMap["by_path"], "s== "+hints.DeviceName; got != want {
					t.Errorf("by_path hint = %q, want %q", got, want)
				}
			} else {
				if got, want := hintMap["name"], "s== "+hints.DeviceName; got != want {
					t.Errorf("name hint = %q, want %q", got, want)
				}
			}
		}

		// Verify size mapping
		if hints.MinSizeGigabytes != 0 {
			if got, want := hintMap["size"], fmt.Sprintf(">= %d", hints.MinSizeGigabytes); got != want {
				t.Errorf("size hint = %q, want %q", got, want)
			}
		}

		// Verify rotational mapping
		if hints.Rotational != nil {
			want := "false"
			if *hints.Rotational {
				want = "true"
			}
			if got := hintMap["rotational"]; got != want {
				t.Errorf("rotational hint = %q, want %q", got, want)
			}
		}
	})
}

// TestMakeHintMapNil tests that MakeHintMap handles nil input gracefully.
func TestMakeHintMapNil(t *testing.T) {
	hintMap := devicehints.MakeHintMap(nil)

	if hintMap == nil {
		t.Fatal("MakeHintMap returned nil for nil input")
	}

	if len(hintMap) != 0 {
		t.Errorf("expected empty map for nil hints, got %d entries", len(hintMap))
	}
}
