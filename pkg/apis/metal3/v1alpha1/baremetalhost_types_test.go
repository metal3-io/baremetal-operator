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
	hostWithError.SetErrorMessage(RegistrationError, "oops something went wrong")

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

	testCases := []struct {
		Scenario string
		Host     BareMetalHost
		Expected bool
	}{
		{
			Scenario: "no hardware details",
			Host: BareMetalHost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "myhost",
					Namespace: "myns",
				},
			},
			Expected: true,
		},

		{
			Scenario: "host with details",
			Host: BareMetalHost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "myhost",
					Namespace: "myns",
				},
				Status: BareMetalHostStatus{
					HardwareDetails: &HardwareDetails{},
				},
			},
			Expected: false,
		},

		{
			Scenario: "unprovisioned host with consumer",
			Host: BareMetalHost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "myhost",
					Namespace: "myns",
				},
				Spec: BareMetalHostSpec{
					ConsumerRef: &corev1.ObjectReference{},
				},
			},
			Expected: true,
		},

		{
			Scenario: "provisioned host",
			Host: BareMetalHost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "myhost",
					Namespace: "myns",
				},
				Status: BareMetalHostStatus{
					Provisioning: ProvisionStatus{
						Image: Image{
							URL: "not-empty",
						},
					},
				},
			},
			Expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Scenario, func(t *testing.T) {
			actual := tc.Host.NeedsHardwareInspection()
			if tc.Expected && !actual {
				t.Error("expected to need hardware inspection")
			}
			if !tc.Expected && actual {
				t.Error("did not expect to need hardware inspection")
			}
		})
	}
}

func TestHostNeedsProvisioning(t *testing.T) {
	testCases := []struct {
		Scenario string
		Host     BareMetalHost
		Expected bool
	}{

		{
			Scenario: "without image",
			Host: BareMetalHost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "myhost",
					Namespace: "myns",
				},
				Spec: BareMetalHostSpec{
					Online: true,
				},
			},
			Expected: false,
		},

		{
			Scenario: "without image url",
			Host: BareMetalHost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "myhost",
					Namespace: "myns",
				},
				Spec: BareMetalHostSpec{
					Image:  &Image{},
					Online: true,
				},
			},
			Expected: false,
		},

		{
			Scenario: "with image url, online",
			Host: BareMetalHost{
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
			},
			Expected: true,
		},

		{
			Scenario: "with image url, offline",
			Host: BareMetalHost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "myhost",
					Namespace: "myns",
				},
				Spec: BareMetalHostSpec{
					Image: &Image{
						URL: "not-empty",
					},
					Online: false,
				},
			},
			Expected: false,
		},

		{
			Scenario: "already provisioned",
			Host: BareMetalHost{
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
			},
			Expected: false,
		},

		{
			Scenario: "externally provisioned",
			Host: BareMetalHost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "myhost",
					Namespace: "myns",
				},
				Spec: BareMetalHostSpec{
					ExternallyProvisioned: true,
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
			},
			Expected: false,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.Scenario, func(t *testing.T) {
			actual := tc.Host.NeedsProvisioning()
			if tc.Expected && !actual {
				t.Error("expected to need provisioning")
			}
			if !tc.Expected && actual {
				t.Error("did not expect to need provisioning")
			}
		})
	}
}

func TestHostNeedsDeprovisioning(t *testing.T) {
	testCases := []struct {
		Scenario string
		Host     BareMetalHost
		Expected bool
	}{
		{
			Scenario: "with image url, unprovisioned",
			Host: BareMetalHost{
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
			},
			Expected: false,
		},

		{
			Scenario: "with image, unprovisioned",
			Host: BareMetalHost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "myhost",
					Namespace: "myns",
				},
				Spec: BareMetalHostSpec{
					Image:  &Image{},
					Online: true,
				},
			},
			Expected: true,
		},

		{
			Scenario: "without, unprovisioned",
			Host: BareMetalHost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "myhost",
					Namespace: "myns",
				},
				Spec: BareMetalHostSpec{
					Online: true,
				},
			},
			Expected: true,
		},

		{
			Scenario: "with image url, offline",
			Host: BareMetalHost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "myhost",
					Namespace: "myns",
				},
				Spec: BareMetalHostSpec{
					Image: &Image{
						URL: "not-empty",
					},
					Online: false,
				},
			},
			Expected: false,
		},

		{
			Scenario: "provisioned",
			Host: BareMetalHost{
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
			},
			Expected: false,
		},

		{
			Scenario: "removed image",
			Host: BareMetalHost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "myhost",
					Namespace: "myns",
				},
				Spec: BareMetalHostSpec{
					Online: true,
				},
				Status: BareMetalHostStatus{
					Provisioning: ProvisionStatus{
						Image: Image{
							URL: "same",
						},
					},
				},
			},
			Expected: true,
		},

		{
			Scenario: "changed image",
			Host: BareMetalHost{
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
			},
			Expected: true,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.Scenario, func(t *testing.T) {
			actual := tc.Host.NeedsDeprovisioning()
			if tc.Expected && !actual {
				t.Error("expected to need deprovisioning")
			}
			if !tc.Expected && actual {
				t.Error("did not expect to need deprovisioning")
			}
		})
	}
}

func TestCredentialStatusMatch(t *testing.T) {
	for _, tc := range []struct {
		Scenario   string
		CredStat   CredentialsStatus
		SecretName string
		Secret     corev1.Secret
		Expected   bool
	}{
		{
			Scenario: "not set",
			Secret:   corev1.Secret{},
			Expected: false,
		},

		{
			Scenario: "new name",
			CredStat: CredentialsStatus{
				Reference: &corev1.SecretReference{
					Name:      "old name",
					Namespace: "namespace",
				},
				Version: "1",
			},
			Secret: corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name: "new name",
				},
			},
			Expected: false,
		},

		{
			Scenario: "match",
			CredStat: CredentialsStatus{
				Reference: &corev1.SecretReference{
					Name:      "match",
					Namespace: "namespace",
				},
				Version: "1",
			},
			Secret: corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "match",
					Namespace:       "namespace",
					ResourceVersion: "1",
				},
			},
			Expected: true,
		},

		{
			Scenario: "wrong namespace",
			CredStat: CredentialsStatus{
				Reference: &corev1.SecretReference{
					Name:      "match",
					Namespace: "namespace",
				},
				Version: "1",
			},
			Secret: corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "match",
					Namespace:       "namespace2",
					ResourceVersion: "1",
				},
			},
			Expected: false,
		},

		{
			Scenario: "new version",
			CredStat: CredentialsStatus{
				Reference: &corev1.SecretReference{
					Name:      "new version",
					Namespace: "namespace",
				},
				Version: "1",
			},
			Secret: corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "new version",
					ResourceVersion: "2",
				},
			},
			Expected: false,
		},
	} {
		t.Run(tc.Scenario, func(t *testing.T) {
			actual := tc.CredStat.Match(tc.Secret)
			if actual != tc.Expected {
				t.Errorf("expected %v but got %v", tc.Expected, actual)
			}
		})
	}
}

func TestGetImageChecksum(t *testing.T) {
	for _, tc := range []struct {
		Scenario string
		Host     BareMetalHost
		Expected bool
	}{
		{
			Scenario: "both checksum value and type specified",
			Host: BareMetalHost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "myhost",
					Namespace: "myns",
				},
				Spec: BareMetalHostSpec{
					Image: &Image{
						Checksum:     "md5hash",
						ChecksumType: MD5,
					},
				},
			},
			Expected: true,
		},
		{
			Scenario: "checksum value specified but not type",
			Host: BareMetalHost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "myhost",
					Namespace: "myns",
				},
				Spec: BareMetalHostSpec{
					Image: &Image{
						Checksum: "md5hash",
					},
				},
			},
			Expected: true,
		},
		{
			Scenario: "sha256 checksum value and type specified",
			Host: BareMetalHost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "myhost",
					Namespace: "myns",
				},
				Spec: BareMetalHostSpec{
					Image: &Image{
						Checksum:     "sha256hash",
						ChecksumType: SHA256,
					},
				},
			},
			Expected: true,
		},
		{
			Scenario: "sha512 checksum value and type specified",
			Host: BareMetalHost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "myhost",
					Namespace: "myns",
				},
				Spec: BareMetalHostSpec{
					Image: &Image{
						Checksum:     "sha512hash",
						ChecksumType: SHA512,
					},
				},
			},
			Expected: true,
		},
		{
			Scenario: "checksum value not specified",
			Host: BareMetalHost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "myhost",
					Namespace: "myns",
				},
				Spec: BareMetalHostSpec{
					Image: &Image{
						ChecksumType: SHA512,
					},
				},
			},
			Expected: false,
		},
		{
			Scenario: "neither checksum value nor hash specified",
			Host: BareMetalHost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "myhost",
					Namespace: "myns",
				},
				Spec: BareMetalHostSpec{
					Image: &Image{
						URL: "someurl",
					},
				},
			},
			Expected: false,
		},
		{
			Scenario: "wrong checksum hash specified",
			Host: BareMetalHost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "myhost",
					Namespace: "myns",
				},
				Spec: BareMetalHostSpec{
					Image: &Image{
						Checksum:     "somehash",
						ChecksumType: "boondoggle",
					},
				},
			},
			Expected: false,
		},
	} {
		t.Run(tc.Scenario, func(t *testing.T) {
			_, _, actual := tc.Host.GetImageChecksum()
			if actual != tc.Expected {
				t.Errorf("expected %v but got %v", tc.Expected, actual)
			}
		})
	}
}
