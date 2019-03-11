package v1alpha1

import (
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestHostAvailable(t *testing.T) {
	hostWithError := BareMetalHost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "myhost",
			Namespace: "myns",
		},
	}
	hostWithError.SetErrorMessage("oops something went wrong")

	testCases := []struct {
		Host        BareMetalHost
		Expected    bool
		FailMessage string
	}{
		{
			Host: BareMetalHost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "myhost",
					Namespace: "myns",
				},
			},
			Expected:    true,
			FailMessage: "available host returned not available",
		},
		{
			Host:        hostWithError,
			Expected:    false,
			FailMessage: "host with error returned as available",
		},
		{
			Host: BareMetalHost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "myhost",
					Namespace: "myns",
				},
				Spec: BareMetalHostSpec{
					MachineRef: &corev1.ObjectReference{
						Name:      "mymachine",
						Namespace: "myns",
					},
				},
			},
			Expected:    false,
			FailMessage: "host with machineref returned as available",
		},
		{
			Host: BareMetalHost{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "myhost",
					Namespace:         "myns",
					DeletionTimestamp: &metav1.Time{Time: time.Now()},
				},
			},
			Expected:    false,
			FailMessage: "deleted host returned as available",
		},
	}

	for _, tc := range testCases {
		if tc.Host.Available() != tc.Expected {
			t.Error(tc.FailMessage)
		}
	}
}
