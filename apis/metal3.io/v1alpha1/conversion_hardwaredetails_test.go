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

// v1beta1 has no Status.HardwareDetails: the same data is published through the
// HardwareData resource. The v1alpha1 status still carries it and the
// controller still reads it, so the conversion has to preserve it.

func hostWithHardwareDetails() *BareMetalHost {
	return &BareMetalHost{
		ObjectMeta: metav1.ObjectMeta{Name: "myhost", Namespace: "myns"},
		Status: BareMetalHostStatus{
			HardwareDetails: &HardwareDetails{
				Hostname:     "myhost",
				RAMMebibytes: 65536,
				CPU: CPU{
					Arch:           "x86_64",
					Model:          "Awesome CPU",
					ClockMegahertz: 3000,
					Count:          32,
				},
				NIC: []NIC{{
					Name: "eth0",
					MAC:  "00:11:22:33:44:55",
					IP:   "192.168.0.10",
					PXE:  true,
				}},
				Storage: []Storage{{
					Name:      "/dev/sda",
					SizeBytes: 500 * GibiByte,
					Type:      SSD,
				}},
				SystemVendor: HardwareSystemVendor{
					Manufacturer: "Awesome Inc.",
					ProductName:  "Awesome Server",
					SerialNumber: "1234",
				},
			},
		},
	}
}

func TestHardwareDetailsSurviveRoundTrip(t *testing.T) {
	g := gomega.NewWithT(t)

	spoke := hostWithHardwareDetails()

	hub := &metal3apiv1beta1.BareMetalHost{}
	g.Expect(spoke.ConvertTo(hub)).To(gomega.Succeed())
	g.Expect(hub.Annotations).To(gomega.HaveKey(DataAnnotation))

	restored := &BareMetalHost{}
	g.Expect(restored.ConvertFrom(hub)).To(gomega.Succeed())
	g.Expect(restored.Status.HardwareDetails).To(gomega.Equal(spoke.Status.HardwareDetails))
	g.Expect(restored.Annotations).NotTo(gomega.HaveKey(DataAnnotation))
}

func TestHardwareDetailsAreNotStoredWhenUnset(t *testing.T) {
	g := gomega.NewWithT(t)

	// A host that was never inspected has nothing to preserve, so it should
	// not get a conversion-data annotation at all.
	spoke := &BareMetalHost{
		ObjectMeta: metav1.ObjectMeta{Name: "myhost", Namespace: "myns"},
		Status:     BareMetalHostStatus{OperationalStatus: OperationalStatusDiscovered},
	}

	hub := &metal3apiv1beta1.BareMetalHost{}
	g.Expect(spoke.ConvertTo(hub)).To(gomega.Succeed())
	g.Expect(hub.Annotations).NotTo(gomega.HaveKey(DataAnnotation))

	restored := &BareMetalHost{}
	g.Expect(restored.ConvertFrom(hub)).To(gomega.Succeed())
	g.Expect(restored.Status.HardwareDetails).To(gomega.BeNil())
}

func TestHardwareDetailsAreStoredWithoutAHardwareProfile(t *testing.T) {
	g := gomega.NewWithT(t)

	// The details alone are enough to need the annotation, even when there is
	// no hardware profile to preserve.
	spoke := hostWithHardwareDetails()

	hub := &metal3apiv1beta1.BareMetalHost{}
	g.Expect(spoke.ConvertTo(hub)).To(gomega.Succeed())

	stored := bareMetalHostConversionData{}
	cached, err := UnmarshalData(hub, &stored)
	g.Expect(err).NotTo(gomega.HaveOccurred())
	g.Expect(cached).To(gomega.BeTrue())
	g.Expect(stored.HardwareProfile).To(gomega.BeNil())
	g.Expect(stored.HardwareDetails).To(gomega.Equal(spoke.Status.HardwareDetails))
}

func TestHardwareDataKeepsItsHardwareDetails(t *testing.T) {
	g := gomega.NewWithT(t)

	// The HardwareData resource is where the details live from v1beta1 on, so
	// its own copy must convert as usual.
	spoke := &HardwareData{
		ObjectMeta: metav1.ObjectMeta{Name: "myhost", Namespace: "myns"},
		Spec: HardwareDataSpec{
			HardwareDetails: hostWithHardwareDetails().Status.HardwareDetails,
		},
	}

	hub := &metal3apiv1beta1.HardwareData{}
	g.Expect(spoke.ConvertTo(hub)).To(gomega.Succeed())
	g.Expect(hub.Spec.HardwareDetails).NotTo(gomega.BeNil())
	g.Expect(hub.Spec.HardwareDetails.Hostname).To(gomega.Equal("myhost"))

	restored := &HardwareData{}
	g.Expect(restored.ConvertFrom(hub)).To(gomega.Succeed())
	g.Expect(restored.Spec.HardwareDetails).To(gomega.Equal(spoke.Spec.HardwareDetails))
}
