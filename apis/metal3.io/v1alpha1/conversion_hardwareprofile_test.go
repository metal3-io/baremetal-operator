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

// Hardware profiles do not exist in v1beta1. These tests cover what happens to
// the names on the way to the hub and back.

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
	// Root device hints are part of both versions, so they convert as usual.
	g.Expect(restored.Spec.RootDeviceHints).To(gomega.Equal(&RootDeviceHints{DeviceName: "/dev/sdb"}))
	// ConvertFrom consumes the stashed v1alpha1 object and then stashes the
	// v1beta1 one in its place, so the annotation is still set afterwards.
	g.Expect(restored.Annotations).To(gomega.HaveKey(DataAnnotation))
}

func TestHostWithoutHardwareProfileGetsNone(t *testing.T) {
	g := gomega.NewWithT(t)

	// The whole spoke object is stashed regardless, but a host that never used
	// a profile must not be given one on the way back.
	spoke := &BareMetalHost{
		ObjectMeta: metav1.ObjectMeta{Name: "myhost", Namespace: "myns"},
		Spec:       BareMetalHostSpec{Online: true},
	}

	hub := &metal3apiv1beta1.BareMetalHost{}
	g.Expect(spoke.ConvertTo(hub)).To(gomega.Succeed())

	restored := &BareMetalHost{}
	g.Expect(restored.ConvertFrom(hub)).To(gomega.Succeed())
	g.Expect(restored.Spec.HardwareProfile).To(gomega.BeEmpty())
	g.Expect(restored.Status.HardwareProfile).To(gomega.BeEmpty())
}

func TestHubOnlyHostUsesEmptyProfile(t *testing.T) {
	g := gomega.NewWithT(t)

	// A host created through v1beta1 has nothing stashed to restore from, and
	// no notion of hardware profiles. The empty profile keeps the controller
	// from applying the root device hints of any other one.
	hub := &metal3apiv1beta1.BareMetalHost{
		ObjectMeta: metav1.ObjectMeta{Name: "myhost", Namespace: "myns"},
	}

	spoke := &BareMetalHost{}
	g.Expect(spoke.ConvertFrom(hub)).To(gomega.Succeed())

	g.Expect(spoke.Spec.HardwareProfile).To(gomega.Equal(EmptyHardwareProfile))
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
		Status: BareMetalHostStatus{HardwareProfile: "unknown"},
	}

	hub := &metal3apiv1beta1.BareMetalHost{}
	g.Expect(spoke.ConvertTo(hub)).To(gomega.Succeed())

	// Stashing the conversion data must not write to the object we converted.
	g.Expect(spoke.Annotations).To(gomega.Equal(map[string]string{"example.com/keep": "me"}))
	g.Expect(hub.Annotations).To(gomega.HaveKey("example.com/keep"))

	// ...and neither must restoring it.
	spokeAgain := &BareMetalHost{}
	g.Expect(spokeAgain.ConvertFrom(hub)).To(gomega.Succeed())
	g.Expect(hub.Annotations).To(gomega.HaveKey(DataAnnotation))
}
