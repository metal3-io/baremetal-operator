package templates

import (
	"strings"
	"testing"
)

func compareStrings(t *testing.T, s1, s2 string) bool {
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
