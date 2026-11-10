/*


Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	"testing"

	"github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	metal3apiv1beta1 "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1beta1"
)

// The hardware profile does not exist in v1beta1. These tests cover what
// happens to it, and to the root device hints it implies, on the way to the
// hub and back.

func TestHardwareProfileIsRestoredFromConversionData(t *testing.T) {
	g := gomega.NewWithT(t)

	spoke := &BareMetalHost{
		ObjectMeta: metav1.ObjectMeta{Name: "myhost", Namespace: "myns"},
		Spec: BareMetalHostSpec{
			HardwareProfile: "dell",
			RootDeviceHints: &RootDeviceHints{DeviceName: "/dev/sdb"},
		},
		Status: BareMetalHostStatus{HardwareProfile: "dell-raid"},
	}

	hub := &metal3apiv1beta1.BareMetalHost{}
	g.Expect(spoke.ConvertTo(hub)).To(gomega.Succeed())
	g.Expect(hub.Annotations).To(gomega.HaveKey(DataAnnotation))

	restored := &BareMetalHost{}
	g.Expect(restored.ConvertFrom(hub)).To(gomega.Succeed())

	g.Expect(restored.Spec.HardwareProfile).To(gomega.Equal("dell"))
	g.Expect(restored.Status.HardwareProfile).To(gomega.Equal("dell-raid"))
	// Explicit hints are not touched in either direction.
	g.Expect(restored.Spec.RootDeviceHints).To(gomega.Equal(&RootDeviceHints{DeviceName: "/dev/sdb"}))
	g.Expect(restored.Annotations).NotTo(gomega.HaveKey(DataAnnotation))
}

func TestConversionDataHoldsOnlyWhatItNeeds(t *testing.T) {
	g := gomega.NewWithT(t)

	// Annotations count towards the object size limit, so the conversion data
	// must stay a handful of values rather than a snapshot of the whole host.
	spoke := &BareMetalHost{
		ObjectMeta: metav1.ObjectMeta{Name: "myhost", Namespace: "myns"},
		Spec: BareMetalHostSpec{
			HardwareProfile: "dell",
			BMC:             BMCDetails{Address: "redfish://192.168.0.1/redfish/v1/Systems/1"},
			Image:           &Image{URL: "http://example.com/image.qcow2"},
		},
		Status: BareMetalHostStatus{
			HardwareProfile: "dell-raid",
			HardwareDetails: &HardwareDetails{
				NIC: []NIC{{Name: "eth0", MAC: "00:11:22:33:44:55"}},
			},
		},
	}

	hub := &metal3apiv1beta1.BareMetalHost{}
	g.Expect(spoke.ConvertTo(hub)).To(gomega.Succeed())

	stashed := hub.Annotations[DataAnnotation]
	g.Expect(stashed).NotTo(gomega.ContainSubstring("redfish://"))
	g.Expect(stashed).NotTo(gomega.ContainSubstring("image.qcow2"))
	g.Expect(stashed).NotTo(gomega.ContainSubstring("00:11:22:33:44:55"))

	restored := bareMetalHostConversionData{}
	cached, err := UnmarshalData(hub, &restored)
	g.Expect(err).NotTo(gomega.HaveOccurred())
	g.Expect(cached).To(gomega.BeTrue())
	g.Expect(restored).To(gomega.Equal(bareMetalHostConversionData{
		HardwareProfile: &hardwareProfileConversionData{
			Name:       "dell",
			StatusName: "dell-raid",
			// The host has no explicit hints, so ConvertTo resolved them from
			// the dell-raid profile and recorded that it did.
			RootDeviceHints: true,
		},
	}))
}

func TestNoConversionDataWithoutHardwareProfile(t *testing.T) {
	g := gomega.NewWithT(t)

	// Nothing to preserve means no annotation at all, so hosts that never used
	// a profile are not given one.
	spoke := &BareMetalHost{
		ObjectMeta: metav1.ObjectMeta{Name: "myhost", Namespace: "myns"},
		Spec:       BareMetalHostSpec{Online: true},
	}

	hub := &metal3apiv1beta1.BareMetalHost{}
	g.Expect(spoke.ConvertTo(hub)).To(gomega.Succeed())
	g.Expect(hub.Annotations).NotTo(gomega.HaveKey(DataAnnotation))

	restored := &BareMetalHost{}
	g.Expect(restored.ConvertFrom(hub)).To(gomega.Succeed())
	g.Expect(restored.Spec.HardwareProfile).To(gomega.BeEmpty())
	g.Expect(restored.Status.HardwareProfile).To(gomega.BeEmpty())
	g.Expect(restored.Spec.RootDeviceHints).To(gomega.BeNil())
}

func TestHardwareProfileResolvesRootDeviceHints(t *testing.T) {
	for _, tc := range []struct {
		Scenario      string
		SpecProfile   string
		StatusProfile string
		Expected      *metal3apiv1beta1.RootDeviceHints
	}{
		{
			Scenario:      "status profile wins over spec profile",
			SpecProfile:   "libvirt",
			StatusProfile: DefaultProfileName,
			Expected:      &metal3apiv1beta1.RootDeviceHints{DeviceName: "/dev/sda"},
		},
		{
			Scenario:    "requested profile is used until the status is set",
			SpecProfile: "libvirt",
			Expected:    &metal3apiv1beta1.RootDeviceHints{DeviceName: "/dev/vda"},
		},
		{
			Scenario:      "profile with hctl hints",
			StatusProfile: "dell",
			Expected:      &metal3apiv1beta1.RootDeviceHints{HCTL: "0:0:0:0"},
		},
		{
			Scenario:      "empty profile implies no hints",
			StatusProfile: EmptyProfileName,
			Expected:      nil,
		},
		{
			Scenario:      "unknown profile name is ignored",
			StatusProfile: "not-a-profile",
			Expected:      nil,
		},
		{
			Scenario: "no profile at all",
			Expected: nil,
		},
	} {
		t.Run(tc.Scenario, func(t *testing.T) {
			g := gomega.NewWithT(t)

			spoke := &BareMetalHost{
				ObjectMeta: metav1.ObjectMeta{Name: "myhost", Namespace: "myns"},
				Spec:       BareMetalHostSpec{HardwareProfile: tc.SpecProfile},
				Status:     BareMetalHostStatus{HardwareProfile: tc.StatusProfile},
			}

			hub := &metal3apiv1beta1.BareMetalHost{}
			g.Expect(spoke.ConvertTo(hub)).To(gomega.Succeed())
			g.Expect(hub.Spec.RootDeviceHints).To(gomega.Equal(tc.Expected))

			// The resolved hints are an artefact of the conversion, so they
			// must not leak back into the stored object.
			restored := &BareMetalHost{}
			g.Expect(restored.ConvertFrom(hub)).To(gomega.Succeed())
			g.Expect(restored.Spec.RootDeviceHints).To(gomega.BeNil())
			g.Expect(restored.Spec.HardwareProfile).To(gomega.Equal(tc.SpecProfile))
			g.Expect(restored.Status.HardwareProfile).To(gomega.Equal(tc.StatusProfile))
		})
	}
}

func TestRootDeviceHintsChangedThroughHubAreKept(t *testing.T) {
	g := gomega.NewWithT(t)

	spoke := &BareMetalHost{
		ObjectMeta: metav1.ObjectMeta{Name: "myhost", Namespace: "myns"},
		Status:     BareMetalHostStatus{HardwareProfile: DefaultProfileName},
	}

	hub := &metal3apiv1beta1.BareMetalHost{}
	g.Expect(spoke.ConvertTo(hub)).To(gomega.Succeed())
	g.Expect(hub.Spec.RootDeviceHints).To(gomega.Equal(&metal3apiv1beta1.RootDeviceHints{DeviceName: "/dev/sda"}))

	// A v1beta1 client overrides the hints it was served.
	hub.Spec.RootDeviceHints = &metal3apiv1beta1.RootDeviceHints{DeviceName: "/dev/nvme0n1"}

	restored := &BareMetalHost{}
	g.Expect(restored.ConvertFrom(hub)).To(gomega.Succeed())
	g.Expect(restored.Spec.RootDeviceHints).To(gomega.Equal(&RootDeviceHints{DeviceName: "/dev/nvme0n1"}))
	g.Expect(restored.Status.HardwareProfile).To(gomega.Equal(DefaultProfileName))
}

func TestHubOnlyHostHasNoHardwareProfile(t *testing.T) {
	g := gomega.NewWithT(t)

	// A host created through v1beta1 has no conversion data to restore from and
	// no notion of hardware profiles, so it must not inherit the hints of any
	// profile either.
	hub := &metal3apiv1beta1.BareMetalHost{
		ObjectMeta: metav1.ObjectMeta{Name: "myhost", Namespace: "myns"},
	}

	spoke := &BareMetalHost{}
	g.Expect(spoke.ConvertFrom(hub)).To(gomega.Succeed())

	g.Expect(spoke.Spec.HardwareProfile).To(gomega.BeEmpty())
	g.Expect(spoke.Status.HardwareProfile).To(gomega.BeEmpty())
	g.Expect(spoke.Spec.RootDeviceHints).To(gomega.BeNil())
}

func TestConvertToDoesNotModifySource(t *testing.T) {
	g := gomega.NewWithT(t)

	spoke := &BareMetalHost{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "myhost",
			Namespace:   "myns",
			Annotations: map[string]string{"example.com/keep": "me"},
		},
		Status: BareMetalHostStatus{HardwareProfile: DefaultProfileName},
	}

	hub := &metal3apiv1beta1.BareMetalHost{}
	g.Expect(spoke.ConvertTo(hub)).To(gomega.Succeed())

	// Stashing the conversion data must not write to the object we converted.
	g.Expect(spoke.Annotations).To(gomega.Equal(map[string]string{"example.com/keep": "me"}))
	g.Expect(spoke.Spec.RootDeviceHints).To(gomega.BeNil())
	g.Expect(hub.Annotations).To(gomega.HaveKey("example.com/keep"))

	// ...and neither must restoring it.
	spokeAgain := &BareMetalHost{}
	g.Expect(spokeAgain.ConvertFrom(hub)).To(gomega.Succeed())
	g.Expect(hub.Annotations).To(gomega.HaveKey(DataAnnotation))
}
