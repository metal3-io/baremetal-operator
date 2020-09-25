package baremetalhost

import (
	"testing"

	metal3v1alpha1 "github.com/metal3-io/baremetal-operator/pkg/apis/metal3/v1alpha1"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestUpdateBootModeStatus(t *testing.T) {
	testCases := []struct {
		Scenario       string
		SpecValue      metal3v1alpha1.BootMode
		StatusValue    metal3v1alpha1.BootMode
		ExpectedValue  metal3v1alpha1.BootMode
		ExpectedChange bool
	}{
		{
			Scenario:       "default",
			SpecValue:      "",
			StatusValue:    "",
			ExpectedValue:  metal3v1alpha1.DefaultBootMode,
			ExpectedChange: true,
		},

		{
			Scenario:       "set UEFI",
			SpecValue:      metal3v1alpha1.UEFI,
			StatusValue:    "",
			ExpectedValue:  metal3v1alpha1.UEFI,
			ExpectedChange: true,
		},

		{
			Scenario:       "already UEFI",
			SpecValue:      metal3v1alpha1.UEFI,
			StatusValue:    metal3v1alpha1.UEFI,
			ExpectedValue:  metal3v1alpha1.UEFI,
			ExpectedChange: false,
		},

		{
			Scenario:       "set Legacy",
			SpecValue:      metal3v1alpha1.Legacy,
			StatusValue:    "",
			ExpectedValue:  metal3v1alpha1.Legacy,
			ExpectedChange: true,
		},

		{
			Scenario:       "already Legacy",
			SpecValue:      metal3v1alpha1.Legacy,
			StatusValue:    metal3v1alpha1.Legacy,
			ExpectedValue:  metal3v1alpha1.Legacy,
			ExpectedChange: false,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.Scenario, func(t *testing.T) {
			host := metal3v1alpha1.BareMetalHost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "myhost",
					Namespace: "myns",
				},
				Spec: metal3v1alpha1.BareMetalHostSpec{
					Image: &metal3v1alpha1.Image{
						URL: "not-empty",
					},
					Online:   true,
					BootMode: tc.SpecValue,
				},
				Status: metal3v1alpha1.BareMetalHostStatus{
					Provisioning: metal3v1alpha1.ProvisionStatus{
						Image: metal3v1alpha1.Image{
							URL: "also-not-empty",
						},
						BootMode: tc.StatusValue,
					},
				},
			}
			changed := updateBootModeStatus(&host)
			assert.Equal(t, tc.ExpectedChange, changed, "unexpected change response")
			assert.Equal(t, tc.ExpectedValue, host.Status.Provisioning.BootMode)
		})
	}
}
