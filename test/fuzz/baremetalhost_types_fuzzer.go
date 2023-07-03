package fuzzing_test

import (
	"fmt"

	"github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"

	fuzz "github.com/AdaLogics/go-fuzz-headers"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type TestCaseBareMetalHost struct {
	Scenario string
	Host     v1alpha1.BareMetalHost
	Expected bool
}

func FuzzTestHostNeedsHardwareInspection(data []byte) int {
	f := fuzz.NewConsumer(data)
	tc := &TestCaseBareMetalHost{}
	err := f.GenerateStruct(tc)
	if err != nil {
		return 0
	}
	actual := tc.Host.NeedsHardwareInspection()
	fmt.Printf("before %+v\n", tc)
	fmt.Printf("actual %+v\n", actual)

	if tc.Expected && !actual {
		return 0
	}
	if !tc.Expected && actual {
		return 0
	}
	return 1
}

func FuzzTestHostNeedsProvisioning(data []byte) int {
	f := fuzz.NewConsumer(data)
	tc := &TestCaseBareMetalHost{}
	err := f.GenerateStruct(tc)
	if err != nil {
		return 0
	}
	actual := tc.Host.NeedsProvisioning()
	fmt.Printf("before %+v\n", tc)
	fmt.Printf("actual %+v\n", actual)

	if tc.Expected && !actual {
		return 0
	}
	if !tc.Expected && actual {
		return 0
	}
	return 1
}

type TestCaseBareMetalCredentials struct {
	Scenario   string
	CredStat   v1alpha1.CredentialsStatus
	SecretName string
	Secret     corev1.Secret
	Expected   bool
}

func FuzzTestCredentialStatusMatch(data []byte) int {
	f := fuzz.NewConsumer(data)
	tc := &TestCaseBareMetalCredentials{}
	err := f.GenerateStruct(tc)
	if err != nil {
		return 0
	}
	actual := tc.CredStat.Match(tc.Secret)
	if actual != tc.Expected {
		return 0
	}
	return 1
}

func FuzzTestGetImageChecksum(data []byte) int {
	f := fuzz.NewConsumer(data)
	tc := &TestCaseBareMetalHost{}
	err := f.GenerateStruct(tc)
	if err != nil {
		return 0
	}
	_, _, actual := tc.Host.Spec.Image.GetChecksum()
	if actual != tc.Expected {
		return 0
	}
	return 1
}

type TestCaseBootMode struct {
	Scenario  string
	HostValue v1alpha1.BootMode
	Expected  v1alpha1.BootMode
}

func FuzzTestBootMode(data []byte) int {
	f := fuzz.NewConsumer(data)
	tc := &TestCaseBootMode{}
	err := f.GenerateStruct(tc)
	if err != nil {
		return 0
	}
	host := &v1alpha1.BareMetalHost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "myhost",
			Namespace: "myns",
		},
		Spec: v1alpha1.BareMetalHostSpec{
			BootMode: tc.HostValue,
		},
	}
	actual := host.BootMode()
	if actual != tc.Expected {
		return 0
	}
	return 1
}
