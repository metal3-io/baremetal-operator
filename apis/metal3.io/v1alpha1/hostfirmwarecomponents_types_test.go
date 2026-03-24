package v1alpha1

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestValidateHostFirmwareComponents(t *testing.T) {
	objectMeta := metav1.ObjectMeta{
		Name:      "myhostfirmware",
		Namespace: "myns",
	}

	testCases := map[string]struct {
		hfc           *HostFirmwareComponents
		ExpectedError error
	}{
		"ValidBMCOnly": {
			hfc: &HostFirmwareComponents{
				ObjectMeta: objectMeta,
				Spec: HostFirmwareComponentsSpec{
					Updates: []FirmwareUpdate{
						{
							Component: "bmc",
							URL:       "https://example.com/bmcupdate",
						},
					},
				},
			},
			ExpectedError: nil,
		},
		"InvalidHostFirmwareComponents": {
			hfc: &HostFirmwareComponents{
				ObjectMeta: objectMeta,
				Spec: HostFirmwareComponentsSpec{
					Updates: []FirmwareUpdate{
						{
							Component: "something",
							URL:       "https://example.com/bmcupdate",
						},
					},
				},
			},
			ExpectedError: fmt.Errorf("'something' is not a valid component name, allowed: 'bmc', 'bios', 'nic', or names starting with 'nic:'"),
		},
		"ValidNICOnlyWithPrefix": {
			hfc: &HostFirmwareComponents{
				ObjectMeta: objectMeta,
				Spec: HostFirmwareComponentsSpec{
					Updates: []FirmwareUpdate{
						{
							Component: "nic:NIC.1",
							URL:       "https://example.com/nicupdate",
						},
						{
							Component: "nic:AD007",
							URL:       "https://example.com/nic2update",
						},
					},
				},
			},
			ExpectedError: nil,
		},
		"InvalidNICWithoutIdentifier": {
			hfc: &HostFirmwareComponents{
				ObjectMeta: objectMeta,
				Spec: HostFirmwareComponentsSpec{
					Updates: []FirmwareUpdate{
						{
							Component: "nic",
							URL:       "https://example.com/nicupdate",
						},
					},
				},
			},
			ExpectedError: fmt.Errorf("'nic' is not a valid component name, allowed: 'bmc', 'bios', 'nic', or names starting with 'nic:'"),
		},
		"InvalidNICEmptyIdentifier": {
			hfc: &HostFirmwareComponents{
				ObjectMeta: objectMeta,
				Spec: HostFirmwareComponentsSpec{
					Updates: []FirmwareUpdate{
						{
							Component: "nic:",
							URL:       "https://example.com/nicupdate",
						},
					},
				},
			},
			ExpectedError: nil,
		},
		"ValidBIOSOnly": {
			hfc: &HostFirmwareComponents{
				ObjectMeta: objectMeta,
				Spec: HostFirmwareComponentsSpec{
					Updates: []FirmwareUpdate{
						{
							Component: "bios",
							URL:       "https://example.com/biosupdate",
						},
					},
				},
			},
			ExpectedError: nil,
		},
		"ValidEmptyUpdatesList": {
			hfc: &HostFirmwareComponents{
				ObjectMeta: objectMeta,
				Spec: HostFirmwareComponentsSpec{
					Updates: []FirmwareUpdate{},
				},
			},
			ExpectedError: nil,
		},
		"InvalidEmptyComponent": {
			hfc: &HostFirmwareComponents{
				ObjectMeta: objectMeta,
				Spec: HostFirmwareComponentsSpec{
					Updates: []FirmwareUpdate{
						{
							Component: "",
							URL:       "https://example.com/update",
						},
					},
				},
			},
			ExpectedError: fmt.Errorf("'' is not a valid component name, allowed: 'bmc', 'bios', 'nic', or names starting with 'nic:'"),
		},
	}

	for scenario, tc := range testCases {
		t.Run(scenario, func(t *testing.T) {
			t.Parallel()
			err := tc.hfc.ValidateHostFirmwareComponents()
			assert.Equal(t, tc.ExpectedError, err)
		})
	}
}
