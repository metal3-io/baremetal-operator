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

// v1beta1 does not accept the "vdi" and "vmdk" disk formats. Going up they are
// unset, which leaves the format to Ironic. Coming down they are restored from
// the stash, so existing hosts keep provisioning the way they always did.

func hostWithDiskFormat(format *string) *BareMetalHost {
	return &BareMetalHost{
		ObjectMeta: metav1.ObjectMeta{Name: "myhost", Namespace: "myns"},
		Spec: BareMetalHostSpec{
			Image: &Image{
				URL:        "http://example.com/image",
				Checksum:   "abcd",
				DiskFormat: format,
			},
		},
	}
}

func TestForbiddenDiskFormatsAreUnsetAndRestored(t *testing.T) {
	for _, format := range []string{"vdi", "vmdk"} {
		t.Run(format, func(t *testing.T) {
			g := gomega.NewWithT(t)

			spoke := hostWithDiskFormat(ptr.To(format))

			hub := &metal3apiv1beta1.BareMetalHost{}
			g.Expect(spoke.ConvertTo(hub)).To(gomega.Succeed())
			g.Expect(hub.Spec.Image.Format).To(gomega.BeNil())

			restored := &BareMetalHost{}
			g.Expect(restored.ConvertFrom(hub)).To(gomega.Succeed())
			g.Expect(restored.Spec.Image.DiskFormat).To(gomega.Equal(ptr.To(format)))
		})
	}
}

func TestSupportedDiskFormatsConvertUnchanged(t *testing.T) {
	g := gomega.NewWithT(t)

	for _, format := range []*string{nil, ptr.To("raw"), ptr.To("qcow2"), ptr.To("live-iso")} {
		spoke := hostWithDiskFormat(format)

		hub := &metal3apiv1beta1.BareMetalHost{}
		g.Expect(spoke.ConvertTo(hub)).To(gomega.Succeed())
		g.Expect(hub.Spec.Image.Format).To(gomega.Equal(format))

		restored := &BareMetalHost{}
		g.Expect(restored.ConvertFrom(hub)).To(gomega.Succeed())
		g.Expect(restored.Spec.Image.DiskFormat).To(gomega.Equal(format))
	}
}

func TestDiskFormatSetThroughHubWins(t *testing.T) {
	g := gomega.NewWithT(t)

	spoke := hostWithDiskFormat(ptr.To("vmdk"))

	hub := &metal3apiv1beta1.BareMetalHost{}
	g.Expect(spoke.ConvertTo(hub)).To(gomega.Succeed())

	// The user picks a format v1beta1 accepts, so the old one must not come back.
	hub.Spec.Image.Format = ptr.To("qcow2")

	restored := &BareMetalHost{}
	g.Expect(restored.ConvertFrom(hub)).To(gomega.Succeed())
	g.Expect(restored.Spec.Image.DiskFormat).To(gomega.Equal(ptr.To("qcow2")))
}

func TestForbiddenDiskFormatIsNotRestoredOnANewImage(t *testing.T) {
	g := gomega.NewWithT(t)

	spoke := hostWithDiskFormat(ptr.To("vdi"))

	hub := &metal3apiv1beta1.BareMetalHost{}
	g.Expect(spoke.ConvertTo(hub)).To(gomega.Succeed())

	// A different image has nothing to do with the stashed format.
	hub.Spec.Image.URL = "http://example.com/other-image"

	restored := &BareMetalHost{}
	g.Expect(restored.ConvertFrom(hub)).To(gomega.Succeed())
	g.Expect(restored.Spec.Image.DiskFormat).To(gomega.BeNil())
}

func TestForbiddenDiskFormatsAreRestoredInStatus(t *testing.T) {
	g := gomega.NewWithT(t)

	// The status carries images too, and the controller compares them against
	// the spec, so they have to survive as well.
	spoke := &BareMetalHost{
		ObjectMeta: metav1.ObjectMeta{Name: "myhost", Namespace: "myns"},
		Status: BareMetalHostStatus{
			Provisioning: ProvisionStatus{
				Image: Image{
					URL:        "http://example.com/image",
					DiskFormat: ptr.To("vdi"),
				},
			},
			LastAttemptedImage: &Image{
				URL:        "http://example.com/image",
				DiskFormat: ptr.To("vmdk"),
			},
		},
	}

	hub := &metal3apiv1beta1.BareMetalHost{}
	g.Expect(spoke.ConvertTo(hub)).To(gomega.Succeed())
	g.Expect(hub.Status.Provisioning.Image.Format).To(gomega.BeNil())
	g.Expect(hub.Status.LastAttemptedImage.Format).To(gomega.BeNil())

	restored := &BareMetalHost{}
	g.Expect(restored.ConvertFrom(hub)).To(gomega.Succeed())
	g.Expect(restored.Status.Provisioning.Image.DiskFormat).To(gomega.Equal(ptr.To("vdi")))
	g.Expect(restored.Status.LastAttemptedImage.DiskFormat).To(gomega.Equal(ptr.To("vmdk")))
}
