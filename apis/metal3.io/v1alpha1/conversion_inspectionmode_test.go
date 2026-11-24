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

// v1beta1 does not accept the "disabled" value of the inspect.metal3.io
// annotation; Spec.InspectionMode replaces it. Going up, the annotation becomes
// the field. Coming down, both are restored from the stashed object, so a host
// that used the annotation keeps using it and one that used the field keeps
// using that.

func hostWithInspectAnnotation(value string) *BareMetalHost {
	return &BareMetalHost{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "myhost",
			Namespace:   "myns",
			Annotations: map[string]string{InspectAnnotationPrefix: value},
		},
	}
}

func TestDisabledInspectAnnotationSurvivesRoundTrip(t *testing.T) {
	g := gomega.NewWithT(t)

	spoke := hostWithInspectAnnotation(InspectAnnotationValueDisabled)

	hub := &metal3apiv1beta1.BareMetalHost{}
	g.Expect(spoke.ConvertTo(hub)).To(gomega.Succeed())

	// v1beta1 sees the field, not the annotation.
	g.Expect(hub.Annotations).NotTo(gomega.HaveKey(metal3apiv1beta1.InspectAnnotationPrefix))
	g.Expect(hub.Spec.InspectionMode).To(gomega.Equal(metal3apiv1beta1.InspectionModeDisabled))

	// Coming back, the host is exactly as it was: annotation set, field unset.
	restored := &BareMetalHost{}
	g.Expect(restored.ConvertFrom(hub)).To(gomega.Succeed())
	g.Expect(restored.Annotations).To(gomega.HaveKeyWithValue(InspectAnnotationPrefix, InspectAnnotationValueDisabled))
	g.Expect(restored.Spec.InspectionMode).To(gomega.BeEmpty())
}

func TestInspectionModeInSpecSurvivesRoundTrip(t *testing.T) {
	g := gomega.NewWithT(t)

	// A host that uses the field keeps using it, and gains no annotation.
	spoke := &BareMetalHost{
		ObjectMeta: metav1.ObjectMeta{Name: "myhost", Namespace: "myns"},
		Spec:       BareMetalHostSpec{InspectionMode: InspectionModeDisabled},
	}

	hub := &metal3apiv1beta1.BareMetalHost{}
	g.Expect(spoke.ConvertTo(hub)).To(gomega.Succeed())
	g.Expect(hub.Spec.InspectionMode).To(gomega.Equal(metal3apiv1beta1.InspectionModeDisabled))

	restored := &BareMetalHost{}
	g.Expect(restored.ConvertFrom(hub)).To(gomega.Succeed())
	g.Expect(restored.Spec.InspectionMode).To(gomega.Equal(InspectionModeDisabled))
	g.Expect(restored.Annotations).NotTo(gomega.HaveKey(InspectAnnotationPrefix))
}

func TestHubOnlyHostGetsNoInspectAnnotation(t *testing.T) {
	g := gomega.NewWithT(t)

	// A host created through v1beta1 has nothing stashed, so it must not be
	// given the deprecated annotation.
	hub := &metal3apiv1beta1.BareMetalHost{
		ObjectMeta: metav1.ObjectMeta{Name: "myhost", Namespace: "myns"},
		Spec:       metal3apiv1beta1.BareMetalHostSpec{InspectionMode: metal3apiv1beta1.InspectionModeDisabled},
	}

	spoke := &BareMetalHost{}
	g.Expect(spoke.ConvertFrom(hub)).To(gomega.Succeed())
	g.Expect(spoke.Spec.InspectionMode).To(gomega.Equal(InspectionModeDisabled))
	g.Expect(spoke.Annotations).NotTo(gomega.HaveKey(InspectAnnotationPrefix))
}

func TestInspectionModeChangedThroughHubWins(t *testing.T) {
	g := gomega.NewWithT(t)

	spoke := hostWithInspectAnnotation(InspectAnnotationValueDisabled)

	hub := &metal3apiv1beta1.BareMetalHost{}
	g.Expect(spoke.ConvertTo(hub)).To(gomega.Succeed())

	// The owner enables inspection again through v1beta1.
	hub.Spec.InspectionMode = metal3apiv1beta1.InspectionModeAgent

	// The annotation must not come back: it would contradict the field, which
	// the v1alpha1 webhook rejects.
	restored := &BareMetalHost{}
	g.Expect(restored.ConvertFrom(hub)).To(gomega.Succeed())
	g.Expect(restored.Spec.InspectionMode).To(gomega.Equal(InspectionModeAgent))
	g.Expect(restored.Annotations).NotTo(gomega.HaveKey(InspectAnnotationPrefix))
}

func TestInspectionRequestSurvivesRoundTrip(t *testing.T) {
	g := gomega.NewWithT(t)

	// An empty value asks for a new inspection. It is not the "disabled"
	// value, so it passes through untouched in both directions.
	spoke := hostWithInspectAnnotation("")

	hub := &metal3apiv1beta1.BareMetalHost{}
	g.Expect(spoke.ConvertTo(hub)).To(gomega.Succeed())
	g.Expect(hub.Annotations).To(gomega.HaveKeyWithValue(metal3apiv1beta1.InspectAnnotationPrefix, ""))
	g.Expect(hub.Spec.InspectionMode).To(gomega.BeEmpty())

	restored := &BareMetalHost{}
	g.Expect(restored.ConvertFrom(hub)).To(gomega.Succeed())
	g.Expect(restored.Annotations).To(gomega.HaveKeyWithValue(InspectAnnotationPrefix, ""))
	g.Expect(restored.Spec.InspectionMode).To(gomega.BeEmpty())
}

func TestInspectionRequestIsNotOverwritten(t *testing.T) {
	g := gomega.NewWithT(t)

	spoke := hostWithInspectAnnotation(InspectAnnotationValueDisabled)

	hub := &metal3apiv1beta1.BareMetalHost{}
	g.Expect(spoke.ConvertTo(hub)).To(gomega.Succeed())

	// A re-inspection is requested while the stash still says disabled. The
	// request must stand, and the derived mode must not keep inspection off.
	hub.Annotations[metal3apiv1beta1.InspectAnnotationPrefix] = ""

	restored := &BareMetalHost{}
	g.Expect(restored.ConvertFrom(hub)).To(gomega.Succeed())
	g.Expect(restored.Annotations).To(gomega.HaveKeyWithValue(InspectAnnotationPrefix, ""))
	g.Expect(restored.Spec.InspectionMode).To(gomega.BeEmpty())
}

func TestConvertToDoesNotModifyInspectAnnotationOnSource(t *testing.T) {
	g := gomega.NewWithT(t)

	spoke := hostWithInspectAnnotation(InspectAnnotationValueDisabled)

	hub := &metal3apiv1beta1.BareMetalHost{}
	g.Expect(spoke.ConvertTo(hub)).To(gomega.Succeed())

	// Removing the annotation must happen on the copy only.
	g.Expect(spoke.Annotations).To(gomega.HaveKeyWithValue(InspectAnnotationPrefix, InspectAnnotationValueDisabled))
}

func TestStashKeepsOnlyTheInspectAnnotation(t *testing.T) {
	g := gomega.NewWithT(t)

	// Annotations can be large, so the stash holds only the one the
	// conversion actually needs.
	spoke := &BareMetalHost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "myhost",
			Namespace: "myns",
			Annotations: map[string]string{
				InspectAnnotationPrefix: InspectAnnotationValueDisabled,
				StatusAnnotation:        `{"operationalStatus":"OK"}`,
				"example.com/other":     "value",
			},
		},
	}

	hub := &metal3apiv1beta1.BareMetalHost{}
	g.Expect(spoke.ConvertTo(hub)).To(gomega.Succeed())

	restored := &BareMetalHost{}
	cached, err := UnmarshalData(hub, restored)
	g.Expect(err).NotTo(gomega.HaveOccurred())
	g.Expect(cached).To(gomega.BeTrue())

	g.Expect(restored.Annotations).To(gomega.Equal(map[string]string{
		InspectAnnotationPrefix: InspectAnnotationValueDisabled,
	}))
}
