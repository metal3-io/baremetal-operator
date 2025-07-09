package v1alpha1

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

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
		Scenario     string
		Host         BareMetalHost
		Expected     bool
		ExpectedType string
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
			Expected:     true,
			ExpectedType: "md5",
		},
		{
			Scenario: "checksum value specified and type not specified, default to auto type",
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
			Expected:     true,
			ExpectedType: "",
		},
		{
			Scenario: "checksum value specified, auto type",
			Host: BareMetalHost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "myhost",
					Namespace: "myns",
				},
				Spec: BareMetalHostSpec{
					Image: &Image{
						Checksum:     "md5hash",
						ChecksumType: AutoChecksum,
					},
				},
			},
			Expected:     true,
			ExpectedType: "",
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
			Expected:     true,
			ExpectedType: "sha256",
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
			Expected:     true,
			ExpectedType: "sha512",
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
		{
			Scenario: "no image",
			Host: BareMetalHost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "myhost",
					Namespace: "myns",
				},
				Spec: BareMetalHostSpec{},
			},
			Expected: false,
		},
	} {
		t.Run(tc.Scenario, func(t *testing.T) {
			_, checksumType, actual := tc.Host.Spec.Image.GetChecksum()
			assert.Equal(t, tc.Expected, actual == nil)
			if tc.Expected {
				assert.Equal(t, tc.ExpectedType, checksumType)
			}
		})
	}
}

func TestBootMode(t *testing.T) {
	for _, tc := range []struct {
		Scenario  string
		HostValue BootMode
		Expected  BootMode
	}{
		{
			Scenario:  "default",
			HostValue: "",
			Expected:  UEFI,
		},
		{
			Scenario:  "UEFI",
			HostValue: UEFI,
			Expected:  UEFI,
		},
		{
			Scenario:  "legacy",
			HostValue: Legacy,
			Expected:  Legacy,
		},
	} {
		t.Run(tc.Scenario, func(t *testing.T) {
			host := &BareMetalHost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "myhost",
					Namespace: "myns",
				},
				Spec: BareMetalHostSpec{
					BootMode: tc.HostValue,
				},
			}
			actual := host.BootMode()
			assert.Equal(t, tc.Expected, actual)
		})
	}
}

func TestSetOperationalStatus(t *testing.T) {
	for _, tc := range []struct {
		Scenario  string
		Host      BareMetalHost
		StatusArg OperationalStatus
		Expected  bool
	}{
		{
			Scenario: "Host is partially configured and operationalStatus is ok",
			Host: BareMetalHost{
				Status: BareMetalHostStatus{
					OperationalStatus: "OK",
				},
			},
			StatusArg: "discovered",
			Expected:  true,
		},
		{
			Scenario: "Host is detached and operationalStatus is also detached",
			Host: BareMetalHost{
				Status: BareMetalHostStatus{
					OperationalStatus: "detached",
				},
			},
			StatusArg: "detached",
			Expected:  false,
		},
		{
			Scenario: "Host has an error and operationalStatus also has an error",
			Host: BareMetalHost{
				Status: BareMetalHostStatus{
					OperationalStatus: "error",
				},
			},
			StatusArg: "error",
			Expected:  false,
		},
		{
			Scenario: "Host is delayed and operationalStatus is not defined",
			Host: BareMetalHost{
				Status: BareMetalHostStatus{},
			},
			StatusArg: "delayed",
			Expected:  true,
		},
	} {
		t.Run(tc.Scenario, func(t *testing.T) {
			actual := tc.Host.SetOperationalStatus(tc.StatusArg)
			if tc.Expected && !actual {
				t.Error("Expected a change in operational status")
			}
			if !tc.Expected && actual {
				t.Error("Did not expect a change in operational status")
			}
		})
	}
}

func TestSetHardwareProfile(t *testing.T) {
	for _, tc := range []struct {
		Scenario string
		Host     BareMetalHost
		name     string
		Expected bool
	}{
		{
			Scenario: "Host hardware profile and name is same",
			Host: BareMetalHost{
				Status: BareMetalHostStatus{
					HardwareProfile: "test",
				},
			},
			name:     "test",
			Expected: false,
		},
		{
			Scenario: "Host hardware profile and name are different",
			Host: BareMetalHost{
				Status: BareMetalHostStatus{
					HardwareProfile: "test",
				},
			},
			name:     "newtest",
			Expected: true,
		},
		{
			Scenario: "Host hardware profile is not defined",
			Host: BareMetalHost{
				Status: BareMetalHostStatus{},
			},
			name:     "test",
			Expected: true,
		},
	} {
		t.Run(tc.Scenario, func(t *testing.T) {
			actual := tc.Host.SetHardwareProfile(tc.name)
			if tc.Expected && !actual {
				t.Error("Expected a change in hardware profile")
			}
			if !tc.Expected && actual {
				t.Error("Did not expect a change in hardware profile")
			}
		})
	}
}

func TestHasBMCDetails(t *testing.T) {
	for _, tc := range []struct {
		Scenario string
		Host     BareMetalHost
		Expected bool
	}{
		{
			Scenario: "Only host bmc address is set",
			Host: BareMetalHost{
				Spec: BareMetalHostSpec{
					BMC: BMCDetails{
						Address: "http://127.0.0.1/",
					},
				},
			},
			Expected: true,
		},
		{
			Scenario: "Only host credentialsName is set",
			Host: BareMetalHost{
				Spec: BareMetalHostSpec{
					BMC: BMCDetails{
						CredentialsName: "test",
					},
				},
			},
			Expected: true,
		},
		{
			Scenario: "Neither host credentialsName nor address is set",
			Host: BareMetalHost{
				Spec: BareMetalHostSpec{
					BMC: BMCDetails{},
				},
			},
			Expected: false,
		},
	} {
		t.Run(tc.Scenario, func(t *testing.T) {
			actual := tc.Host.HasBMCDetails()
			if tc.Expected && !actual {
				t.Error("Expected BMC details exist")
			}
			if !tc.Expected && actual {
				t.Error("Did not expect that BMC details exist")
			}
		})
	}
}
