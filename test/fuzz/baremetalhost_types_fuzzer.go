package fuzzing_test

import (
	// Needed for building the fuzzers
	_ "github.com/dvyukov/go-fuzz/go-fuzz-dep"

	"github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"

	fuzz "github.com/AdaLogics/go-fuzz-headers"
	corev1 "k8s.io/api/core/v1"
)

func FuzzTestHostNeedsHardwareInspection(data []byte) int {
	f := fuzz.NewConsumer(data)
	bmh := &v1alpha1.BareMetalHost{}
	err := f.GenerateStruct(bmh)
	if err != nil {
		return 0
	}
	bmh.NeedsHardwareInspection()

	return 1
}

func FuzzTestHostNeedsProvisioning(data []byte) int {
	f := fuzz.NewConsumer(data)
	bmh := &v1alpha1.BareMetalHost{}
	err := f.GenerateStruct(bmh)
	if err != nil {
		return 0
	}
	bmh.NeedsProvisioning()

	return 1
}

type TestCaseBareMetalCredentials struct {
	CredStat v1alpha1.CredentialsStatus
	Secret   corev1.Secret
}

func FuzzTestCredentialStatusMatch(data []byte) int {
	f := fuzz.NewConsumer(data)
	tc := &TestCaseBareMetalCredentials{}
	err := f.GenerateStruct(tc)
	if err != nil {
		return 0
	}
	tc.CredStat.Match(tc.Secret)

	return 1
}

func FuzzTestGetImageChecksum(data []byte) int {
	f := fuzz.NewConsumer(data)
	image := &v1alpha1.Image{}
	err := f.GenerateStruct(image)
	if err != nil {
		return 0
	}
	image.GetChecksum()

	return 1
}

func FuzzTestBootMode(data []byte) int {
	f := fuzz.NewConsumer(data)
	var bootMode v1alpha1.BootMode
	str, err := f.GetString()
	if err != nil {
		return 0
	}
	bootMode = v1alpha1.BootMode(str)
	host := &v1alpha1.BareMetalHost{
		Spec: v1alpha1.BareMetalHostSpec{
			BootMode: bootMode,
		},
	}
	host.BootMode()

	return 1
}
