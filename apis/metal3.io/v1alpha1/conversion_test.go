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
	"maps"
	"math/rand"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/apitesting/fuzzer"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	metafuzzer "k8s.io/apimachinery/pkg/apis/meta/fuzzer"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	runtimeserializer "k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/conversion"
	"sigs.k8s.io/randfill"

	metal3apiv1beta1 "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1beta1"
)

// v1alpha1 is a spoke version and v1beta1 is the hub (storage version).
// The harness below (getFuzzer / fuzzTestFunc) is a local copy of
// sigs.k8s.io/cluster-api/util/conversion, trimmed to what BMO needs. The
// lightweight apis module deliberately avoids depending on the whole
// cluster-api module (see conversion_data.go), so the harness is vendored here
// rather than imported. It keeps the same two-direction, fuzz-based approach as
// the upstream cluster-api projects (CAPM3, CAPO).

// TestFuzzyConversion verifies that conversion between the v1alpha1 spoke and
// the v1beta1 hub is lossless in both directions for every convertible kind.
func TestFuzzyConversion(t *testing.T) {
	t.Run("for BareMetalHost", fuzzTestFunc(fuzzTestFuncInput{
		hub:   &metal3apiv1beta1.BareMetalHost{},
		spoke: &BareMetalHost{},
	}))
	t.Run("for BareMetalSwitch", fuzzTestFunc(fuzzTestFuncInput{
		hub:   &metal3apiv1beta1.BareMetalSwitch{},
		spoke: &BareMetalSwitch{},
	}))
	t.Run("for BMCEventSubscription", fuzzTestFunc(fuzzTestFuncInput{
		hub:   &metal3apiv1beta1.BMCEventSubscription{},
		spoke: &BMCEventSubscription{},
	}))
	t.Run("for DataImage", fuzzTestFunc(fuzzTestFuncInput{
		hub:   &metal3apiv1beta1.DataImage{},
		spoke: &DataImage{},
	}))
	t.Run("for FirmwareSchema", fuzzTestFunc(fuzzTestFuncInput{
		hub:   &metal3apiv1beta1.FirmwareSchema{},
		spoke: &FirmwareSchema{},
	}))
	t.Run("for HardwareData", fuzzTestFunc(fuzzTestFuncInput{
		hub:   &metal3apiv1beta1.HardwareData{},
		spoke: &HardwareData{},
	}))
	t.Run("for HostClaim", fuzzTestFunc(fuzzTestFuncInput{
		hub:   &metal3apiv1beta1.HostClaim{},
		spoke: &HostClaim{},
	}))
	t.Run("for HostDeployPolicy", fuzzTestFunc(fuzzTestFuncInput{
		hub:   &metal3apiv1beta1.HostDeployPolicy{},
		spoke: &HostDeployPolicy{},
	}))
	t.Run("for HostFirmwareComponents", fuzzTestFunc(fuzzTestFuncInput{
		hub:   &metal3apiv1beta1.HostFirmwareComponents{},
		spoke: &HostFirmwareComponents{},
	}))
	t.Run("for HostFirmwareSettings", fuzzTestFunc(fuzzTestFuncInput{
		hub:   &metal3apiv1beta1.HostFirmwareSettings{},
		spoke: &HostFirmwareSettings{},
	}))
	t.Run("for HostNetworkAttachment", fuzzTestFunc(fuzzTestFuncInput{
		hub:   &metal3apiv1beta1.HostNetworkAttachment{},
		spoke: &HostNetworkAttachment{},
	}))
	t.Run("for HostUpdatePolicy", fuzzTestFunc(fuzzTestFuncInput{
		hub:   &metal3apiv1beta1.HostUpdatePolicy{},
		spoke: &HostUpdatePolicy{},
	}))
	t.Run("for PreprovisioningImage", fuzzTestFunc(fuzzTestFuncInput{
		hub:   &metal3apiv1beta1.PreprovisioningImage{},
		spoke: &PreprovisioningImage{},
	}))
}

// --- Local copy of the cluster-api conversion fuzz harness ---
//
// See the doc comment above for why this is vendored rather than imported. The
// logic mirrors sigs.k8s.io/cluster-api/util/conversion.

// fuzzTestFuncInput contains input parameters for fuzzTestFunc.
type fuzzTestFuncInput struct {
	hub   conversion.Hub
	spoke conversion.Convertible

	// fuzzerFuncs allows a kind to register custom fuzzers for its special
	// field types. metav1.Time and intstr.IntOrString are always registered.
	fuzzerFuncs []fuzzer.FuzzerFuncs
}

// conversionTestScheme registers both API versions so the fuzzer can build a
// codec factory.
func conversionTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := AddToScheme(s); err != nil {
		t.Fatalf("failed to add v1alpha1 to scheme: %v", err)
	}
	if err := metal3apiv1beta1.AddToScheme(s); err != nil {
		t.Fatalf("failed to add v1beta1 to scheme: %v", err)
	}
	return s
}

// normalizeConversionAnnotations removes the conversion-data annotation and, if
// that leaves the annotations map empty, resets it to nil. The
// MarshalData/UnmarshalData helpers allocate the annotations map to store the
// conversion data; once that entry is removed an empty (but non-nil) map would
// otherwise show up as a spurious difference against a freshly fuzzed object.
func normalizeConversionAnnotations(obj metav1.Object) {
	annotations := obj.GetAnnotations()
	delete(annotations, DataAnnotation)
	if len(annotations) == 0 {
		obj.SetAnnotations(nil)
	}
}

// getFuzzer returns a randfill.Filler seeded with the standard meta fuzzers
// plus custom fuzzers for metav1.Time and intstr.IntOrString, which otherwise
// fuzz to nil / are not fuzzed at all. This matches cluster-api's GetFuzzer.
func getFuzzer(s *runtime.Scheme, funcs ...fuzzer.FuzzerFuncs) *randfill.Filler {
	funcs = append([]fuzzer.FuzzerFuncs{
		metafuzzer.Funcs,
		func(_ runtimeserializer.CodecFactory) []interface{} {
			return []interface{}{
				// Custom fuzzer for metav1.Time pointers which otherwise always
				// resulted in nil values.
				//
				// Note: a pointer to the *zero* time is deliberately never
				// produced. metav1.Time marshals the zero value to JSON null,
				// so a &metav1.Time{} stashed in the conversion-data annotation
				// comes back as nil and cannot be distinguished from nil on a
				// round-trip. Only nil or a non-zero time round-trip losslessly.
				func(input **metav1.Time, c randfill.Continue) {
					if c.Bool() {
						// Leave the Time nil sometimes for coverage.
						return
					}
					var sec, nsec uint32
					c.Fill(&sec)
					c.Fill(&nsec)
					fuzzed := metav1.Unix(int64(sec), int64(nsec)).Rfc3339Copy()
					*input = &metav1.Time{Time: fuzzed.Time}
				},
				// Custom fuzzer for metav1.Time values (non-pointer). JSON
				// serialization truncates to seconds, so normalize to an
				// RFC3339 copy to keep the round-trip lossless.
				func(input *metav1.Time, c randfill.Continue) {
					var sec, nsec uint32
					c.Fill(&sec)
					c.Fill(&nsec)
					*input = metav1.Unix(int64(sec), int64(nsec)).Rfc3339Copy()
				},
				// Custom fuzzer for intstr.IntOrString which does not get
				// fuzzed otherwise.
				func(in **intstr.IntOrString, c randfill.Continue) {
					if c.Bool() {
						return
					}
					if c.Bool() {
						*in = &intstr.IntOrString{}
						return
					}
					*in = ptr.To(intstr.FromInt32(c.Int31n(50)))
				},
				func(in *intstr.IntOrString, c randfill.Continue) {
					*in = intstr.FromInt32(c.Int31n(50))
				},
			}
		},
	}, funcs...)
	return fuzzer.FuzzerFor(
		fuzzer.MergeFuzzerFuncs(funcs...),
		rand.NewSource(rand.Int63()), //nolint:gosec // weak RNG is fine for test fuzzing
		runtimeserializer.NewCodecFactory(s),
	)
}

// fuzzTestFunc returns a test that runs the spoke->hub->spoke and
// hub->spoke->hub round-trips and asserts neither loses data.
func fuzzTestFunc(input fuzzTestFuncInput) func(*testing.T) {
	return func(t *testing.T) {
		t.Helper()
		s := conversionTestScheme(t)

		t.Run("spoke-hub-spoke", func(t *testing.T) {
			g := gomega.NewWithT(t)
			f := getFuzzer(s, input.fuzzerFuncs...)

			for range 10000 {
				// Fuzz a spoke object.
				spokeBefore := input.spoke.DeepCopyObject().(conversion.Convertible)
				f.Fill(spokeBefore)

				// spoke -> hub
				hubCopy := input.hub.DeepCopyObject().(conversion.Hub)
				g.Expect(spokeBefore.ConvertTo(hubCopy)).To(gomega.Succeed())

				// hub -> spoke
				spokeAfter := input.spoke.DeepCopyObject().(conversion.Convertible)
				g.Expect(spokeAfter.ConvertFrom(hubCopy)).To(gomega.Succeed())

				// Drop the data annotation added by ConvertFrom so it does not
				// count as a difference, then normalize an emptied annotations
				// map back to nil (MarshalData/UnmarshalData allocate the map
				// even when there is nothing else in it).
				normalizeConversionAnnotations(spokeAfter.(metav1.Object))

				if !apiequality.Semantic.DeepEqual(spokeBefore, spokeAfter) {
					g.Expect(false).To(gomega.BeTrue(), cmp.Diff(spokeBefore, spokeAfter))
				}
			}
		})

		t.Run("hub-spoke-hub", func(t *testing.T) {
			g := gomega.NewWithT(t)
			f := getFuzzer(s, input.fuzzerFuncs...)

			for range 10000 {
				// Fuzz a hub object.
				hubBefore := input.hub.DeepCopyObject().(conversion.Hub)
				f.Fill(hubBefore)

				// hub -> spoke (deep copy hubBefore so MarshalData mutations do
				// not affect the comparison below).
				dstCopy := input.spoke.DeepCopyObject().(conversion.Convertible)
				g.Expect(dstCopy.ConvertFrom(hubBefore.DeepCopyObject().(conversion.Hub))).To(gomega.Succeed())

				// The apiserver sometimes sends objects without a spec (in the
				// context of managed-field conversion). Verify ConvertTo does
				// not error or panic on an object that carries only metadata.
				// Run this before the ConvertTo below, which clears the restore
				// annotation from dstCopy.
				if dstMeta, ok := dstCopy.(metav1.Object); ok {
					noSpec := input.spoke.DeepCopyObject().(conversion.Convertible)
					noSpecMeta := noSpec.(metav1.Object)
					noSpecMeta.SetLabels(maps.Clone(dstMeta.GetLabels()))
					noSpecMeta.SetAnnotations(maps.Clone(dstMeta.GetAnnotations()))
					g.Expect(noSpec.ConvertTo(input.hub.DeepCopyObject().(conversion.Hub))).To(gomega.Succeed())
				}

				// spoke -> hub
				hubAfter := input.hub.DeepCopyObject().(conversion.Hub)
				g.Expect(dstCopy.ConvertTo(hubAfter)).To(gomega.Succeed())

				// The conversion-data annotation is round-trip machinery, not
				// real hub data: ConvertTo (spoke->hub) always stamps it on the
				// hub. Normalize it away on both sides before comparing.
				normalizeConversionAnnotations(hubBefore.(metav1.Object))
				normalizeConversionAnnotations(hubAfter.(metav1.Object))

				if !apiequality.Semantic.DeepEqual(hubBefore, hubAfter) {
					g.Expect(false).To(gomega.BeTrue(), cmp.Diff(hubBefore, hubAfter))
				}
			}
		})
	}
}
