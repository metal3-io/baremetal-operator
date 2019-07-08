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
					ConsumerRef: &corev1.ObjectReference{
						Name:      "mymachine",
						Namespace: "myns",
					},
				},
			},
			Expected:    false,
			FailMessage: "host with consumerref returned as available",
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

func TestHostNeedsHardwareInspection(t *testing.T) {
	hostYes := BareMetalHost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "myhost",
			Namespace: "myns",
		},
	}
	if !hostYes.NeedsHardwareInspection() {
		t.Error("expected to need hardware inspection")
	}

	hostWithDetails := BareMetalHost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "myhost",
			Namespace: "myns",
		},
		Status: BareMetalHostStatus{
			HardwareDetails: &HardwareDetails{},
		},
	}
	if hostWithDetails.NeedsHardwareInspection() {
		t.Error("expected to not need hardware inspection")
	}

	hostWithConsumer := BareMetalHost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "myhost",
			Namespace: "myns",
		},
		Spec: BareMetalHostSpec{
			ConsumerRef: &corev1.ObjectReference{},
		},
	}
	if hostWithConsumer.NeedsHardwareInspection() {
		t.Error("expected to not need hardware inspection")
	}
}

func TestHostNeedsProvisioning(t *testing.T) {
	hostYes := BareMetalHost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "myhost",
			Namespace: "myns",
		},
		Spec: BareMetalHostSpec{
			Image: &Image{
				URL: "not-empty",
			},
			Online: true,
		},
	}
	if !hostYes.NeedsProvisioning() {
		t.Error("expected to need provisioning")
	}

	hostNoURL := BareMetalHost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "myhost",
			Namespace: "myns",
		},
		Spec: BareMetalHostSpec{
			Image:  &Image{},
			Online: true,
		},
	}
	if hostNoURL.NeedsProvisioning() {
		t.Error("expected to not need provisioning")
	}

	hostNoImage := BareMetalHost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "myhost",
			Namespace: "myns",
		},
		Spec: BareMetalHostSpec{
			Online: true,
		},
	}
	if hostNoImage.NeedsProvisioning() {
		t.Error("expected to not need provisioning")
	}

	hostOffline := BareMetalHost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "myhost",
			Namespace: "myns",
		},
		Spec: BareMetalHostSpec{
			Image: &Image{
				URL: "not-empty",
			},
		},
	}
	if hostOffline.NeedsProvisioning() {
		t.Error("expected to not need provisioning")
	}

	hostAlreadyProvisioned := BareMetalHost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "myhost",
			Namespace: "myns",
		},
		Spec: BareMetalHostSpec{
			Image: &Image{
				URL: "not-empty",
			},
			Online: true,
		},
		Status: BareMetalHostStatus{
			Provisioning: ProvisionStatus{
				Image: Image{
					URL: "also-not-empty",
				},
			},
		},
	}
	if hostAlreadyProvisioned.NeedsProvisioning() {
		t.Error("expected to not need provisioning")
	}
}

func TestHostNeedsDeprovisioning(t *testing.T) {
	hostYes := BareMetalHost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "myhost",
			Namespace: "myns",
		},
		Spec: BareMetalHostSpec{
			Image: &Image{
				URL: "not-empty",
			},
			Online: true,
		},
	}
	if hostYes.NeedsDeprovisioning() {
		t.Error("expected to not need deprovisioning")
	}

	hostNoURL := BareMetalHost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "myhost",
			Namespace: "myns",
		},
		Spec: BareMetalHostSpec{
			Image:  &Image{},
			Online: true,
		},
	}
	if hostNoURL.NeedsDeprovisioning() {
		t.Error("expected to not need deprovisioning")
	}

	hostNoImage := BareMetalHost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "myhost",
			Namespace: "myns",
		},
		Spec: BareMetalHostSpec{
			Online: true,
		},
	}
	if hostNoImage.NeedsDeprovisioning() {
		t.Error("expected to not need deprovisioning")
	}

	hostOffline := BareMetalHost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "myhost",
			Namespace: "myns",
		},
		Spec: BareMetalHostSpec{
			Image: &Image{
				URL: "not-empty",
			},
		},
	}
	if hostOffline.NeedsDeprovisioning() {
		t.Error("expected to not need deprovisioning")
	}

	hostAlreadyProvisioned := BareMetalHost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "myhost",
			Namespace: "myns",
		},
		Spec: BareMetalHostSpec{
			Image: &Image{
				URL: "same",
			},
			Online: true,
		},
		Status: BareMetalHostStatus{
			Provisioning: ProvisionStatus{
				Image: Image{
					URL: "same",
				},
			},
		},
	}
	if hostAlreadyProvisioned.NeedsDeprovisioning() {
		t.Error("expected to not need deprovisioning")
	}

	hostChangedImage := BareMetalHost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "myhost",
			Namespace: "myns",
		},
		Spec: BareMetalHostSpec{
			Image: &Image{
				URL: "not-empty",
			},
			Online: true,
		},
		Status: BareMetalHostStatus{
			Provisioning: ProvisionStatus{
				Image: Image{
					URL: "also-not-empty",
				},
			},
		},
	}
	if !hostChangedImage.NeedsDeprovisioning() {
		t.Error("expected to need deprovisioning")
	}
}

func TestHostWasExternallyProvisioned(t *testing.T) {

	for _, tc := range []struct {
		Scenario string
		Host     BareMetalHost
		Expected bool
	}{

		{
			Scenario: "with image",
			Host: BareMetalHost{
				Spec: BareMetalHostSpec{
					Image: &Image{
						URL: "with-image",
					},
				},
			},
			Expected: false,
		},

		{
			Scenario: "with image and consumer",
			Host: BareMetalHost{
				Spec: BareMetalHostSpec{
					Image: &Image{
						URL: "with-image",
					},
					ConsumerRef: &corev1.ObjectReference{},
				},
			},
			Expected: false,
		},

		{
			Scenario: "without image or consumer",
			Host: BareMetalHost{
				Spec: BareMetalHostSpec{},
			},
			Expected: false,
		},

		{
			Scenario: "without image with consumer",
			Host: BareMetalHost{
				Spec: BareMetalHostSpec{
					ConsumerRef: &corev1.ObjectReference{},
				},
			},
			Expected: true,
		},

		{
			Scenario: "provisioned state",
			Host: BareMetalHost{
				Status: BareMetalHostStatus{
					Provisioning: ProvisionStatus{
						State: StateProvisioned,
					},
				},
			},
			Expected: false,
		},

		{
			Scenario: "provisioning state",
			Host: BareMetalHost{
				Status: BareMetalHostStatus{
					Provisioning: ProvisionStatus{
						State: StateProvisioning,
					},
				},
			},
			Expected: false,
		},

		{
			Scenario: "deprovisioning state",
			Host: BareMetalHost{
				Status: BareMetalHostStatus{
					Provisioning: ProvisionStatus{
						State: StateDeprovisioning,
					},
				},
			},
			Expected: false,
		},
	} {
		t.Run(tc.Scenario, func(t *testing.T) {
			if tc.Expected && !tc.Host.WasExternallyProvisioned() {
				t.Error("expected to find externally provisioned host")
			}
			if !tc.Expected && tc.Host.WasExternallyProvisioned() {
				t.Error("did not expect to find externally provisioned host")
			}
		})
	}
}
