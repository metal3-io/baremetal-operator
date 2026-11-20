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
	"k8s.io/utils/ptr"

	metal3apiv1beta1 "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1beta1"
)

// v1beta1 renames three root device hints: deviceName -> devicePath and
// model/vendor -> modelContains/vendorContains. The values are unchanged, so
// these tests check that each renamed field lands in the right place, including
// where the hints are nested in other types.

func allRootDeviceHints() RootDeviceHints {
	return RootDeviceHints{
		DeviceName:         "/dev/disk/by-path/pci-0000:01:00.0-scsi-0:2:0:0",
		HCTL:               "0:2:0:0",
		Model:              "QEMU HARDDISK",
		Vendor:             "QEMU",
		SerialNumber:       "drive-scsi0-0-0-0",
		MinSizeGigabytes:   50,
		WWN:                "0x5000c500a0b1c2d3",
		WWNWithExtension:   "0x5000c500a0b1c2d3-ext",
		WWNVendorExtension: "ext",
		Rotational:         ptr.To(true),
	}
}

func TestRootDeviceHintsRenamesConvertToHub(t *testing.T) {
	g := gomega.NewWithT(t)

	hints := allRootDeviceHints()
	spoke := &BareMetalHost{
		ObjectMeta: metav1.ObjectMeta{Name: "myhost", Namespace: "myns"},
		Spec:       BareMetalHostSpec{RootDeviceHints: hints.DeepCopy()},
	}

	hub := &metal3apiv1beta1.BareMetalHost{}
	g.Expect(spoke.ConvertTo(hub)).To(gomega.Succeed())

	g.Expect(hub.Spec.RootDeviceHints).To(gomega.Equal(&metal3apiv1beta1.RootDeviceHints{
		DevicePath:         "/dev/disk/by-path/pci-0000:01:00.0-scsi-0:2:0:0",
		HCTL:               "0:2:0:0",
		ModelContains:      "QEMU HARDDISK",
		VendorContains:     "QEMU",
		SerialNumber:       "drive-scsi0-0-0-0",
		MinSizeGigabytes:   50,
		WWN:                "0x5000c500a0b1c2d3",
		WWNWithExtension:   "0x5000c500a0b1c2d3-ext",
		WWNVendorExtension: "ext",
		Rotational:         ptr.To(true),
	}))
}

func TestRootDeviceHintsRenamesConvertFromHub(t *testing.T) {
	g := gomega.NewWithT(t)

	hub := &metal3apiv1beta1.BareMetalHost{
		ObjectMeta: metav1.ObjectMeta{Name: "myhost", Namespace: "myns"},
		Spec: metal3apiv1beta1.BareMetalHostSpec{
			RootDeviceHints: &metal3apiv1beta1.RootDeviceHints{
				DevicePath:     "/dev/sda",
				ModelContains:  "HARDDISK",
				VendorContains: "QEMU",
			},
		},
	}

	spoke := &BareMetalHost{}
	g.Expect(spoke.ConvertFrom(hub)).To(gomega.Succeed())

	g.Expect(spoke.Spec.RootDeviceHints).To(gomega.Equal(&RootDeviceHints{
		DeviceName: "/dev/sda",
		Model:      "HARDDISK",
		Vendor:     "QEMU",
	}))
}

func TestRootDeviceHintsSurviveRoundTrip(t *testing.T) {
	g := gomega.NewWithT(t)

	hints := allRootDeviceHints()
	spoke := &BareMetalHost{
		ObjectMeta: metav1.ObjectMeta{Name: "myhost", Namespace: "myns"},
		Spec:       BareMetalHostSpec{RootDeviceHints: hints.DeepCopy()},
		Status: BareMetalHostStatus{
			Provisioning: ProvisionStatus{RootDeviceHints: hints.DeepCopy()},
		},
	}

	hub := &metal3apiv1beta1.BareMetalHost{}
	g.Expect(spoke.ConvertTo(hub)).To(gomega.Succeed())

	restored := &BareMetalHost{}
	g.Expect(restored.ConvertFrom(hub)).To(gomega.Succeed())

	g.Expect(restored.Spec.RootDeviceHints).To(gomega.Equal(&hints))
	g.Expect(restored.Status.Provisioning.RootDeviceHints).To(gomega.Equal(&hints))
}

func TestRootDeviceHintsInSoftwareRAIDConvert(t *testing.T) {
	g := gomega.NewWithT(t)

	// Software RAID volumes carry a list of hints, so the renames have to be
	// applied there as well.
	spoke := &BareMetalHost{
		ObjectMeta: metav1.ObjectMeta{Name: "myhost", Namespace: "myns"},
		Spec: BareMetalHostSpec{
			RAID: &RAIDConfig{
				SoftwareRAIDVolumes: []SoftwareRAIDVolume{{
					Level: "1",
					PhysicalDisks: []RootDeviceHints{
						{DeviceName: "/dev/sda"},
						{DeviceName: "/dev/sdb"},
					},
				}},
			},
		},
	}

	hub := &metal3apiv1beta1.BareMetalHost{}
	g.Expect(spoke.ConvertTo(hub)).To(gomega.Succeed())

	disks := hub.Spec.RAID.SoftwareRAIDVolumes[0].PhysicalDisks
	g.Expect(disks).To(gomega.HaveLen(2))
	g.Expect(disks[0].DevicePath).To(gomega.Equal("/dev/sda"))
	g.Expect(disks[1].DevicePath).To(gomega.Equal("/dev/sdb"))

	restored := &BareMetalHost{}
	g.Expect(restored.ConvertFrom(hub)).To(gomega.Succeed())
	g.Expect(restored.Spec.RAID).To(gomega.Equal(spoke.Spec.RAID))
}
