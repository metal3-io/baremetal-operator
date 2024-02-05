package templates

import (
	"strings"
	"testing"
)

func compareStrings(t *testing.T, s1, s2 string) bool {
	t.Helper()
	if s1 == s2 {
		return true
	}

	s1Lines := strings.Split(s1, "\n")
	s2Lines := strings.Split(s2, "\n")

	max := len(s1Lines)
	if len(s2Lines) < max {
		max = len(s2Lines)
	}

	for i := 0; i < max; i++ {
		if s1Lines[i] != s2Lines[i] {
			t.Logf("line %d differ: %q != %q", i, s1Lines[i], s2Lines[i])
		}
	}
	return false
}

func TestWithHardwareProfile(t *testing.T) {
	template := Template{
		Name:            "hostname",
		BMCAddress:      "bmcAddress",
		Username:        "username",
		Password:        "password",
		HardwareProfile: "hardware profile",
	}
	actual, _ := template.Render()
	expected := `---
apiVersion: v1
kind: Secret
metadata:
  name: hostname-bmc-secret
type: Opaque
data:
  username: dXNlcm5hbWU=
  password: cGFzc3dvcmQ=

---
apiVersion: metal3.io/v1alpha1
kind: BareMetalHost
metadata:
  name: hostname
spec:
  online: true
  hardwareProfile: hardware profile
  bmc:
    address: bmcAddress
    credentialsName: hostname-bmc-secret
`
	if !compareStrings(t, expected, actual) {
		t.Fail()
	}
}

func TestWithoutHardwareProfile(t *testing.T) {
	template := Template{
		Name:       "hostname",
		BMCAddress: "bmcAddress",
		Username:   "username",
		Password:   "password",
	}
	actual, _ := template.Render()
	expected := `---
apiVersion: v1
kind: Secret
metadata:
  name: hostname-bmc-secret
type: Opaque
data:
  username: dXNlcm5hbWU=
  password: cGFzc3dvcmQ=

---
apiVersion: metal3.io/v1alpha1
kind: BareMetalHost
metadata:
  name: hostname
spec:
  online: true
  bmc:
    address: bmcAddress
    credentialsName: hostname-bmc-secret
`
	if !compareStrings(t, expected, actual) {
		t.Fail()
	}
}

func TestWithConsumer(t *testing.T) {
	template := Template{
		Name:              "hostname",
		BMCAddress:        "bmcAddress",
		Username:          "username",
		Password:          "password",
		Consumer:          "consumer",
		ConsumerNamespace: "consumerNamespace",
	}
	actual, _ := template.Render()
	expected := `---
apiVersion: v1
kind: Secret
metadata:
  name: hostname-bmc-secret
type: Opaque
data:
  username: dXNlcm5hbWU=
  password: cGFzc3dvcmQ=

---
apiVersion: metal3.io/v1alpha1
kind: BareMetalHost
metadata:
  name: hostname
spec:
  online: true
  bmc:
    address: bmcAddress
    credentialsName: hostname-bmc-secret
  consumerRef:
    name: consumer
    namespace: consumerNamespace
`
	if !compareStrings(t, expected, actual) {
		t.Fail()
	}
}

func TestWithAutomatedCleaningMode(t *testing.T) {
	template := Template{
		Name:                  "hostname",
		BMCAddress:            "bmcAddress",
		Username:              "username",
		Password:              "password",
		AutomatedCleaningMode: "metadata",
	}
	actual, _ := template.Render()
	expected := `---
apiVersion: v1
kind: Secret
metadata:
  name: hostname-bmc-secret
type: Opaque
data:
  username: dXNlcm5hbWU=
  password: cGFzc3dvcmQ=

---
apiVersion: metal3.io/v1alpha1
kind: BareMetalHost
metadata:
  name: hostname
spec:
  automatedCleaningMode: metadata
  online: true
  bmc:
    address: bmcAddress
    credentialsName: hostname-bmc-secret
`
	if !compareStrings(t, expected, actual) {
		t.Fail()
	}
}

func TestWithImage(t *testing.T) {
	template := Template{
		Name:              "hostname",
		BMCAddress:        "bmcAddress",
		Username:          "username",
		Password:          "password",
		ImageURL:          "imageURL",
		ImageChecksum:     "imageChecksum",
		ImageChecksumType: "md5",
		ImageFormat:       "raw",
	}
	actual, _ := template.Render()
	expected := `---
apiVersion: v1
kind: Secret
metadata:
  name: hostname-bmc-secret
type: Opaque
data:
  username: dXNlcm5hbWU=
  password: cGFzc3dvcmQ=

---
apiVersion: metal3.io/v1alpha1
kind: BareMetalHost
metadata:
  name: hostname
spec:
  online: true
  bmc:
    address: bmcAddress
    credentialsName: hostname-bmc-secret
  image:
    checksum: imageChecksum
    checksumType: md5
    format: raw
    url: imageURL
`
	if !compareStrings(t, expected, actual) {
		t.Fail()
	}
}

func TestWithDisableCertificateVerification(t *testing.T) {
	template := Template{
		Name:                           "hostname",
		BMCAddress:                     "bmcAddress",
		Username:                       "username",
		Password:                       "password",
		DisableCertificateVerification: true,
	}
	actual, _ := template.Render()
	expected := `---
apiVersion: v1
kind: Secret
metadata:
  name: hostname-bmc-secret
type: Opaque
data:
  username: dXNlcm5hbWU=
  password: cGFzc3dvcmQ=

---
apiVersion: metal3.io/v1alpha1
kind: BareMetalHost
metadata:
  name: hostname
spec:
  online: true
  bmc:
    address: bmcAddress
    credentialsName: hostname-bmc-secret
    disableCertificateVerification: true
`
	if !compareStrings(t, expected, actual) {
		t.Fail()
	}
}

func TestWithBootMacAddress(t *testing.T) {
	template := Template{
		Name:           "hostname",
		BMCAddress:     "bmcAddress",
		Username:       "username",
		Password:       "password",
		BootMacAddress: "boot-mac",
	}
	actual, _ := template.Render()
	expected := `---
apiVersion: v1
kind: Secret
metadata:
  name: hostname-bmc-secret
type: Opaque
data:
  username: dXNlcm5hbWU=
  password: cGFzc3dvcmQ=

---
apiVersion: metal3.io/v1alpha1
kind: BareMetalHost
metadata:
  name: hostname
spec:
  online: true
  bootMACAddress: boot-mac
  bmc:
    address: bmcAddress
    credentialsName: hostname-bmc-secret
`
	if !compareStrings(t, expected, actual) {
		t.Fail()
	}
}

func TestWithBootMode(t *testing.T) {
	template := Template{
		Name:       "hostname",
		BMCAddress: "bmcAddress",
		Username:   "username",
		Password:   "password",
		BootMode:   "UEFI",
	}
	actual, _ := template.Render()
	expected := `---
apiVersion: v1
kind: Secret
metadata:
  name: hostname-bmc-secret
type: Opaque
data:
  username: dXNlcm5hbWU=
  password: cGFzc3dvcmQ=

---
apiVersion: metal3.io/v1alpha1
kind: BareMetalHost
metadata:
  name: hostname
spec:
  online: true
  bootMode: UEFI
  bmc:
    address: bmcAddress
    credentialsName: hostname-bmc-secret
`
	if !compareStrings(t, expected, actual) {
		t.Fail()
	}
}
