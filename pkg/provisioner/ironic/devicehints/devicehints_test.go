package devicehints

import (
	"testing"

	"github.com/stretchr/testify/assert"

	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
)

func TestMakeHintMap(t *testing.T) {
	addressableTrue := true
	addressableFalse := false

	for _, tc := range []struct {
		Scenario string
		Hints    metal3api.RootDeviceHints
		Expected map[string]string
	}{
		{
			Scenario: "device-name",
			Hints: metal3api.RootDeviceHints{
				DeviceName: "userd_devicename",
			},
			Expected: map[string]string{
				"name": "s== userd_devicename",
			},
		},
		{
			Scenario: "hctl",
			Hints: metal3api.RootDeviceHints{
				HCTL: "1:2:3:4",
			},
			Expected: map[string]string{
				"hctl": "s== 1:2:3:4",
			},
		},
		{
			Scenario: "model",
			Hints: metal3api.RootDeviceHints{
				Model: "userd_model",
			},
			Expected: map[string]string{
				"model": "<in> userd_model",
			},
		},
		{
			Scenario: "vendor",
			Hints: metal3api.RootDeviceHints{
				Vendor: "userd_vendor",
			},
			Expected: map[string]string{
				"vendor": "<in> userd_vendor",
			},
		},
		{
			Scenario: "serial-number",
			Hints: metal3api.RootDeviceHints{
				SerialNumber: "userd_serial",
			},
			Expected: map[string]string{
				"serial": "s== userd_serial",
			},
		},
		{
			Scenario: "min-size",
			Hints: metal3api.RootDeviceHints{
				MinSizeGigabytes: 40,
			},
			Expected: map[string]string{
				"size": ">= 40",
			},
		},
		{
			Scenario: "wwn",
			Hints: metal3api.RootDeviceHints{
				WWN: "userd_wwn",
			},
			Expected: map[string]string{
				"wwn": "s== userd_wwn",
			},
		},
		{
			Scenario: "wwn-with-extension",
			Hints: metal3api.RootDeviceHints{
				WWNWithExtension: "userd_with_extension",
			},
			Expected: map[string]string{
				"wwn_with_extension": "s== userd_with_extension",
			},
		},
		{
			Scenario: "wwn-extension",
			Hints: metal3api.RootDeviceHints{
				WWNVendorExtension: "userd_vendor_extension",
			},
			Expected: map[string]string{
				"wwn_vendor_extension": "s== userd_vendor_extension",
			},
		},
		{
			Scenario: "rotational-true",
			Hints: metal3api.RootDeviceHints{
				Rotational: &addressableTrue,
			},
			Expected: map[string]string{
				"rotational": "true",
			},
		},
		{
			Scenario: "rotational-false",
			Hints: metal3api.RootDeviceHints{
				Rotational: &addressableFalse,
			},
			Expected: map[string]string{
				"rotational": "false",
			},
		},
		{
			Scenario: "everything-bagel",
			Hints: metal3api.RootDeviceHints{
				DeviceName:         "userd_devicename",
				HCTL:               "1:2:3:4",
				Model:              "userd_model",
				Vendor:             "userd_vendor",
				SerialNumber:       "userd_serial",
				MinSizeGigabytes:   40,
				WWN:                "userd_wwn",
				WWNWithExtension:   "userd_with_extension",
				WWNVendorExtension: "userd_vendor_extension",
				Rotational:         &addressableTrue,
			},
			Expected: map[string]string{
				"name":                 "s== userd_devicename",
				"hctl":                 "s== 1:2:3:4",
				"model":                "<in> userd_model",
				"vendor":               "<in> userd_vendor",
				"serial":               "s== userd_serial",
				"size":                 ">= 40",
				"wwn":                  "s== userd_wwn",
				"wwn_with_extension":   "s== userd_with_extension",
				"wwn_vendor_extension": "s== userd_vendor_extension",
				"rotational":           "true",
			},
		},
		{
			Scenario: "empty",
			Hints:    metal3api.RootDeviceHints{},
			Expected: map[string]string{},
		},
	} {
		t.Run(tc.Scenario, func(t *testing.T) {
			hints := tc.Hints
			actual := MakeHintMap(&hints)
			assert.Equal(t, tc.Expected, actual, "hint map does not match")
		})
	}
}
