package validation

import (
	"testing"

	"github.com/stretchr/testify/assert"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/client-go/kubernetes/scheme"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"

	metal3apis "github.com/metal3-io/baremetal-operator/pkg/apis"
	metal3v1alpha1 "github.com/metal3-io/baremetal-operator/pkg/apis/metal3/v1alpha1"
)

const (
	namespace                 string = "test-namespace"
	defaultHostName           string = "default-host"
	defaultBMCCredentialsName string = "bmc-creds-valid"
	defaultBootMACAddress     string = "00:00:00:00:00"
	defaultBMCAddress         string = "ipmi://192.168.122.1:6233"
)

func init() {
	logf.SetLogger(logf.ZapLogger(true))
	// Register our package types with the global scheme
	metal3apis.AddToScheme(scheme.Scheme)
}

func newHost(name, bmcAddress, bmcCredentialsName, bootMACAddress string) metal3v1alpha1.BareMetalHost {
	return metal3v1alpha1.BareMetalHost{
		TypeMeta: metav1.TypeMeta{
			Kind:       "BareMetalHost",
			APIVersion: "metal3.io/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: metal3v1alpha1.BareMetalHostSpec{
			BMC: metal3v1alpha1.BMCDetails{
				Address:         bmcAddress,
				CredentialsName: bmcCredentialsName,
			},
			BootMACAddress: bootMACAddress,
		},
	}
}

func newDefaultHost() metal3v1alpha1.BareMetalHost {
	return newHost(defaultHostName, defaultBMCAddress, defaultBMCCredentialsName, defaultBootMACAddress)
}

func asList(hosts ...metal3v1alpha1.BareMetalHost) []metal3v1alpha1.BareMetalHost {
	return hosts
}

func TestCanBeAdmitted(t *testing.T) {
	testCases := []struct {
		Scenario string
		Host     metal3v1alpha1.BareMetalHost
		Expected error
	}{
		{
			Scenario: "BMC address is not unique",
			Host:     newHost(defaultHostName, defaultBMCAddress, "", ""),
			Expected: apierrors.NewInvalid(
				metal3v1alpha1.SchemeGroupVersion.WithKind("BareMetalHost").GroupKind(),
				defaultHostName,
				field.ErrorList{
					field.Invalid(
						field.NewPath("spec", "bmc", "address"),
						defaultBMCAddress,
						"is not unique",
					),
				}),
		},
		{
			Scenario: "BMC address is unique",
			Host:     newHost(defaultHostName, "unique", "", ""),
			Expected: nil,
		},
		{
			Scenario: "BootMACAddress is not unique",
			Host:     newHost(defaultHostName, "", "", defaultBootMACAddress),
			Expected: apierrors.NewInvalid(
				metal3v1alpha1.SchemeGroupVersion.WithKind("BareMetalHost").GroupKind(),
				defaultHostName,
				field.ErrorList{
					field.Invalid(
						field.NewPath("spec", "bootMACAddress"),
						defaultBootMACAddress,
						"is not unique",
					),
				}),
		},
		{
			Scenario: "BootMACAddress is unique",
			Host:     newHost(defaultHostName, "", "", "unique"),
			Expected: nil,
		},
		{
			Scenario: "BMC credentials name is not unique",
			Host:     newHost(defaultHostName, "", defaultBMCCredentialsName, ""),
			Expected: apierrors.NewInvalid(
				metal3v1alpha1.SchemeGroupVersion.WithKind("BareMetalHost").GroupKind(),
				defaultHostName,
				field.ErrorList{
					field.Invalid(
						field.NewPath("spec", "bmc", "credentialsName"),
						defaultBMCCredentialsName,
						"is not unique",
					),
				}),
		},
		{
			Scenario: "BMC credentials name is unique",
			Host:     newHost(defaultHostName, "", "unique", ""),
			Expected: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Scenario, func(t *testing.T) {
			existingHosts := asList(newDefaultHost())
			err := ValidateHostForAdmission(existingHosts, tc.Host, nil)
			assert.Equal(t, tc.Expected, err)
		})
	}
}

func TestCanBeAdmittedMultiple(t *testing.T) {
	h1 := newHost("h1", "", "", "")
	h2 := newHost("h2", "", "", "")
	h3 := newHost("h3", "", "", "")
	existingHosts := asList(h1, h2, h3)

	err := ValidateHostForAdmission(existingHosts, newDefaultHost(), nil)
	assert.Equal(t, nil, err)

	err = ValidateHostForAdmission(existingHosts, newHost(defaultHostName, "", "", ""), nil)
	assert.Equal(t, nil, err)
}

func TestCanBeAdmittedUpdate(t *testing.T) {
	h1 := newHost("h1", "1", "", "")
	h2 := newHost("h2", "2", "", "")
	h3 := newHost("h3", "3", "", "")
	existingHosts := asList(h1, h2, h3)

	h1Updated := h1.DeepCopy()
	h1Updated.Spec.HardwareProfile = "test"
	err := ValidateHostForAdmission(existingHosts, *h1Updated, &h1)
	assert.Equal(t, nil, err)

	h1Updated.Spec.BMC.Address = "updated"
	err = ValidateHostForAdmission(existingHosts, *h1Updated, &h1)
	assert.Equal(t, nil, err)

	h1Updated.Spec.BMC.Address = "2"
	err = ValidateHostForAdmission(existingHosts, *h1Updated, &h1)
	expected := apierrors.NewInvalid(
		metal3v1alpha1.SchemeGroupVersion.WithKind("BareMetalHost").GroupKind(),
		"h1",
		field.ErrorList{
			field.Invalid(
				field.NewPath("spec", "bmc", "address"),
				"2",
				"is not unique",
			),
		})
	assert.Equal(t, expected, err)

}
