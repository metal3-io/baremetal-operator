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
}

func TestHardwareDetailsStayUnsetWhenAbsent(t *testing.T) {
	g := gomega.NewWithT(t)

	// A host that was never inspected must not gain hardware details on the
	// way back.
	spoke := &BareMetalHost{
		ObjectMeta: metav1.ObjectMeta{Name: "myhost", Namespace: "myns"},
		Status:     BareMetalHostStatus{OperationalStatus: OperationalStatusDiscovered},
	}

	hub := &metal3apiv1beta1.BareMetalHost{}
	g.Expect(spoke.ConvertTo(hub)).To(gomega.Succeed())

	restored := &BareMetalHost{}
	g.Expect(restored.ConvertFrom(hub)).To(gomega.Succeed())
	g.Expect(restored.Status.HardwareDetails).To(gomega.BeNil())
}

func TestHubOnlyHostHasNoHardwareDetails(t *testing.T) {
	g := gomega.NewWithT(t)

	// A host created through v1beta1 has no stashed object to restore from.
	hub := &metal3apiv1beta1.BareMetalHost{
		ObjectMeta: metav1.ObjectMeta{Name: "myhost", Namespace: "myns"},
	}

	spoke := &BareMetalHost{}
	g.Expect(spoke.ConvertFrom(hub)).To(gomega.Succeed())
	g.Expect(spoke.Status.HardwareDetails).To(gomega.BeNil())
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

func TestNICModelConversion(t *testing.T) {
	testCases := []struct {
		name     string
		model    string
		vendor   string
		deviceID string
	}{
		{name: "vendor and device", model: "0x8086 0x1572", vendor: "0x8086", deviceID: "0x1572"},
		{name: "device only", model: "0x1572", deviceID: "0x1572"},
		{name: "vendor only", model: "0x8086 ", vendor: "0x8086"},
		{name: "empty"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := gomega.NewWithT(t)

			// Start from a used destination so stale fields would show up.
			hub := &metal3apiv1beta1.NIC{Vendor: "stale", DeviceID: "stale"}
			g.Expect(Convert_v1alpha1_NIC_To_v1beta1_NIC(&NIC{Model: tc.model}, hub, nil)).To(gomega.Succeed())
			g.Expect(hub.Vendor).To(gomega.Equal(tc.vendor))
			g.Expect(hub.DeviceID).To(gomega.Equal(tc.deviceID))

			spoke := &NIC{}
			g.Expect(Convert_v1beta1_NIC_To_v1alpha1_NIC(hub, spoke, nil)).To(gomega.Succeed())
			g.Expect(spoke.Model).To(gomega.Equal(tc.model))
		})
	}
}
