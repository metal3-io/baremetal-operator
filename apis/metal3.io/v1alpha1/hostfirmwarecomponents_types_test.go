package v1alpha1

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestValidateHostFirmwareComponents(t *testing.T) {
	for _, tc := range []struct {
		Scenario      string
		Components    []FirmwareComponentStatus
		Updates       []FirmwareUpdate
		LastUpdated   *metav1.Time
		Conditions    []metav1.Condition
		ExpectedError string
	}{
		{
			Scenario: "ValidHostFirmwareComponents",
			Components: []FirmwareComponentStatus{
				{
					Component:      "bios",
					InitialVersion: "1.0",
					CurrentVersion: "1.0",
				},
				{
					Component:          "bmc",
					InitialVersion:     "1.0",
					CurrentVersion:     "2.0",
					LastVersionFlashed: "2.0",
					UpdatedAt:          metav1.NewTime(time.Now()),
				},
			},
			Updates: []FirmwareUpdate{
				{
					Component: "bmc",
					URL:       "https://example.com/bmcupdate",
				},
			},
			Conditions: []metav1.Condition{
				{
					Type:               string(HostFirmwareComponentsChangeDetected),
					Status:             metav1.ConditionTrue,
					LastTransitionTime: metav1.NewTime(time.Now()),
				},
			},
			ExpectedError: "",
		},
		{
			Scenario:   "InvalidHostFirmwareComponents",
			Components: []FirmwareComponentStatus{},
			Updates: []FirmwareUpdate{
				{
					Component: "something",
					URL:       "https://example.com/bmcupdate",
				},
			},
			Conditions: []metav1.Condition{
				{
					Type:               string(HostFirmwareComponentsValid),
					Status:             metav1.ConditionFalse,
					LastTransitionTime: metav1.NewTime(time.Now()),
				},
			},
			ExpectedError: "component something is invalid, only 'bmc', 'bios', or names starting with 'nic:' are allowed as update names",
		},
		{
			Scenario: "ValidNicPrefixComponent",
			Components: []FirmwareComponentStatus{
				{
					Component:      "nic:NIC.1",
					InitialVersion: "1.0",
					CurrentVersion: "1.0",
				},
				{
					Component:      "nic:AD007",
					InitialVersion: "2.0",
					CurrentVersion: "2.1",
				},
			},
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
			Conditions: []metav1.Condition{
				{
					Type:               string(HostFirmwareComponentsChangeDetected),
					Status:             metav1.ConditionTrue,
					LastTransitionTime: metav1.NewTime(time.Now()),
				},
			},
			ExpectedError: "",
		},
	} {
		t.Run(tc.Scenario, func(t *testing.T) {
			hostFirmwareComponents := &HostFirmwareComponents{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "myhostfirmware",
					Namespace: "myns",
				},
				Spec: HostFirmwareComponentsSpec{
					Updates: tc.Updates,
				},
				Status: HostFirmwareComponentsStatus{
					Components:  tc.Components,
					LastUpdated: tc.LastUpdated,
					Conditions:  tc.Conditions,
				},
			}

			err := hostFirmwareComponents.ValidateHostFirmwareComponents()
			if err == nil {
				assert.Empty(t, tc.ExpectedError)
			} else {
				assert.Equal(t, tc.ExpectedError, err.Error())
			}
		})
	}
}
