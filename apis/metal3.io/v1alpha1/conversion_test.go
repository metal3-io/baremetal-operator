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
	"errors"
	"maps"
	"math/rand"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/apitesting/fuzzer"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	metafuzzer "k8s.io/apimachinery/pkg/apis/meta/fuzzer"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	runtimeserializer "k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/conversion"
	"sigs.k8s.io/randfill"

	metal3apiv1beta1 "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1beta1"
)

// v1alpha1 is the storage version and v1beta1 is the conversion hub.
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
		hub:         &metal3apiv1beta1.BareMetalHost{},
		spoke:       &BareMetalHost{},
		fuzzerFuncs: []fuzzer.FuzzerFuncs{bareMetalHostFuzzerFuncs},
	}))
	t.Run("for HardwareData", fuzzTestFunc(fuzzTestFuncInput{
		hub:   &metal3apiv1beta1.HardwareData{},
		spoke: &HardwareData{},
	}))
}

// bareMetalHostFuzzerFuncs restricts the namespace of the fuzzed v1alpha1
// userData, networkData and metaData Secret references to either the empty
// string or the namespace of the host. ConvertTo rejects cross-namespace
// references, so other values cannot be converted. The names are left fuzzed,
// including empty names.
func bareMetalHostFuzzerFuncs(_ runtimeserializer.CodecFactory) []interface{} {
	return []interface{}{
		func(in *BareMetalHost, c randfill.Continue) {
			c.FillNoCustom(in)

			for _, ref := range []*corev1.SecretReference{in.Spec.UserData, in.Spec.NetworkData, in.Spec.MetaData} {
				if ref == nil {
					continue
				}
				if c.Bool() {
					ref.Namespace = ""
				} else {
					ref.Namespace = in.Namespace
				}
			}
		},
	}
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
					switch c.Int31n(4) {
					case 0:
						return // 25% of time leave it nil
					case 1:
						*in = &intstr.IntOrString{} // 25% of time leave it empty
					default:
						*in = ptr.To(fuzzIntOrString(c)) // 50% of time populate with a real int or string (where int/string is 50/50)
					}
				},
				func(in *intstr.IntOrString, c randfill.Continue) {
					*in = fuzzIntOrString(c)
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

// fuzzIntOrString returns a fuzzed intstr.IntOrString, generating both the Int
// and String representations so the conversion round-trip exercises StringVal
// as well as IntVal.
func fuzzIntOrString(c randfill.Continue) intstr.IntOrString {
	if c.Bool() {
		return intstr.FromInt32(c.Int31n(50))
	}
	// Prefix with a letter to guarantee the value is non-empty and cannot be
	// parsed as an integer.
	return intstr.FromString("s" + c.String(0))
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

// TestSecretAccessErrorConversion checks that a host whose status carries the
// secret access error keeps it when converted to the v1beta1 hub and back.
func TestSecretAccessErrorConversion(t *testing.T) {
	g := gomega.NewWithT(t)

	spoke := &BareMetalHost{Status: BareMetalHostStatus{ErrorType: SecretAccessError}}
	hub := &metal3apiv1beta1.BareMetalHost{}
	g.Expect(spoke.ConvertTo(hub)).To(gomega.Succeed())
	g.Expect(hub.Status.ErrorType).To(gomega.Equal(metal3apiv1beta1.SecretAccessError))

	restored := &BareMetalHost{}
	g.Expect(restored.ConvertFrom(hub)).To(gomega.Succeed())
	g.Expect(restored.Status.ErrorType).To(gomega.Equal(SecretAccessError))
}

// TestDataNameConversionToHub checks that the userData, networkData and
// metaData Secret references are converted to the v1beta1 *DataName fields.
func TestDataNameConversionToHub(t *testing.T) {
	testCases := []struct {
		name     string
		ref      *corev1.SecretReference
		expected string
	}{
		{
			name:     "nil reference",
			ref:      nil,
			expected: "",
		},
		{
			name:     "name only",
			ref:      &corev1.SecretReference{Name: "secret"},
			expected: "secret",
		},
		{
			name:     "name and host namespace",
			ref:      &corev1.SecretReference{Name: "secret", Namespace: "host-ns"},
			expected: "secret",
		},
		{
			name:     "namespace without name",
			ref:      &corev1.SecretReference{Namespace: "host-ns"},
			expected: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := gomega.NewWithT(t)

			spoke := &BareMetalHost{
				ObjectMeta: metav1.ObjectMeta{Name: "host", Namespace: "host-ns"},
				Spec: BareMetalHostSpec{
					UserData:    tc.ref.DeepCopy(),
					NetworkData: tc.ref.DeepCopy(),
					MetaData:    tc.ref.DeepCopy(),
				},
			}
			hub := &metal3apiv1beta1.BareMetalHost{}
			g.Expect(spoke.ConvertTo(hub)).To(gomega.Succeed())

			g.Expect(hub.Spec.UserDataName).To(gomega.Equal(tc.expected))
			g.Expect(hub.Spec.NetworkDataName).To(gomega.Equal(tc.expected))
			g.Expect(hub.Spec.MetaDataName).To(gomega.Equal(tc.expected))
		})
	}
}

// TestDataNameConversionFromHub checks that the v1beta1 *DataName fields are
// converted to Secret references in the namespace of the host when there is
// no conversion data to restore from.
func TestDataNameConversionFromHub(t *testing.T) {
	testCases := []struct {
		name     string
		dataName string
		expected *corev1.SecretReference
	}{
		{
			name:     "empty name",
			dataName: "",
			expected: nil,
		},
		{
			name:     "name",
			dataName: "secret",
			expected: &corev1.SecretReference{Name: "secret", Namespace: "host-ns"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := gomega.NewWithT(t)

			hub := &metal3apiv1beta1.BareMetalHost{
				ObjectMeta: metav1.ObjectMeta{Name: "host", Namespace: "host-ns"},
				Spec: metal3apiv1beta1.BareMetalHostSpec{
					UserDataName:    tc.dataName,
					NetworkDataName: tc.dataName,
					MetaDataName:    tc.dataName,
				},
			}
			spoke := &BareMetalHost{}
			g.Expect(spoke.ConvertFrom(hub)).To(gomega.Succeed())

			g.Expect(spoke.Spec.UserData).To(gomega.Equal(tc.expected))
			g.Expect(spoke.Spec.NetworkData).To(gomega.Equal(tc.expected))
			g.Expect(spoke.Spec.MetaData).To(gomega.Equal(tc.expected))
		})
	}
}

// TestDataNameConversionCrossNamespace checks that ConvertTo rejects
// cross-namespace userData, networkData and metaData Secret references, since
// v1beta1 always uses the namespace of the host.
func TestDataNameConversionCrossNamespace(t *testing.T) {
	crossNamespaceRef := &corev1.SecretReference{Name: "secret", Namespace: "other-ns"}

	testCases := []struct {
		name          string
		spec          BareMetalHostSpec
		expectedField string
	}{
		{
			name:          "userData",
			spec:          BareMetalHostSpec{UserData: crossNamespaceRef.DeepCopy()},
			expectedField: "spec.userData.namespace",
		},
		{
			name:          "networkData",
			spec:          BareMetalHostSpec{NetworkData: crossNamespaceRef.DeepCopy()},
			expectedField: "spec.networkData.namespace",
		},
		{
			name:          "metaData",
			spec:          BareMetalHostSpec{MetaData: crossNamespaceRef.DeepCopy()},
			expectedField: "spec.metaData.namespace",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := gomega.NewWithT(t)

			spoke := &BareMetalHost{
				ObjectMeta: metav1.ObjectMeta{Name: "host", Namespace: "host-ns"},
				Spec:       tc.spec,
			}
			hub := &metal3apiv1beta1.BareMetalHost{}
			err := spoke.ConvertTo(hub)

			var fieldErr *field.Error
			g.Expect(errors.As(err, &fieldErr)).To(gomega.BeTrue(), "expected a field error, got %v", err)
			g.Expect(fieldErr.Type).To(gomega.Equal(field.ErrorTypeForbidden))
			g.Expect(fieldErr.Field).To(gomega.Equal(tc.expectedField))
		})
	}
}

// TestDataNameConversionRestore checks the v1alpha1 -> v1beta1 -> v1alpha1
// round-trip of the userData, networkData and metaData Secret references,
// including changes made through v1beta1 in between.
func TestDataNameConversionRestore(t *testing.T) {
	testCases := []struct {
		name     string
		ref      *corev1.SecretReference
		modify   func(*metal3apiv1beta1.BareMetalHostSpec)
		expected *corev1.SecretReference
	}{
		{
			name:     "nil reference is preserved",
			ref:      nil,
			expected: nil,
		},
		{
			name:     "host namespace is preserved",
			ref:      &corev1.SecretReference{Name: "secret", Namespace: "host-ns"},
			expected: &corev1.SecretReference{Name: "secret", Namespace: "host-ns"},
		},
		{
			name:     "empty namespace is preserved",
			ref:      &corev1.SecretReference{Name: "secret"},
			expected: &corev1.SecretReference{Name: "secret"},
		},
		{
			name:     "reference without name is preserved",
			ref:      &corev1.SecretReference{Namespace: "host-ns"},
			expected: &corev1.SecretReference{Namespace: "host-ns"},
		},
		{
			name:     "empty reference is preserved",
			ref:      &corev1.SecretReference{},
			expected: &corev1.SecretReference{},
		},
		{
			name: "name changed in v1beta1",
			ref:  &corev1.SecretReference{Name: "secret", Namespace: "host-ns"},
			modify: func(spec *metal3apiv1beta1.BareMetalHostSpec) {
				spec.UserDataName = "new-secret"
				spec.NetworkDataName = "new-secret"
				spec.MetaDataName = "new-secret"
			},
			expected: &corev1.SecretReference{Name: "new-secret", Namespace: "host-ns"},
		},
		{
			name: "name changed in v1beta1 keeps empty namespace",
			ref:  &corev1.SecretReference{Name: "secret"},
			modify: func(spec *metal3apiv1beta1.BareMetalHostSpec) {
				spec.UserDataName = "new-secret"
				spec.NetworkDataName = "new-secret"
				spec.MetaDataName = "new-secret"
			},
			expected: &corev1.SecretReference{Name: "new-secret"},
		},
		{
			name: "name set in v1beta1",
			ref:  nil,
			modify: func(spec *metal3apiv1beta1.BareMetalHostSpec) {
				spec.UserDataName = "new-secret"
				spec.NetworkDataName = "new-secret"
				spec.MetaDataName = "new-secret"
			},
			expected: &corev1.SecretReference{Name: "new-secret", Namespace: "host-ns"},
		},
		{
			name: "name set in v1beta1 for reference without name",
			ref:  &corev1.SecretReference{Namespace: "host-ns"},
			modify: func(spec *metal3apiv1beta1.BareMetalHostSpec) {
				spec.UserDataName = "new-secret"
				spec.NetworkDataName = "new-secret"
				spec.MetaDataName = "new-secret"
			},
			expected: &corev1.SecretReference{Name: "new-secret", Namespace: "host-ns"},
		},
		{
			name: "name removed in v1beta1",
			ref:  &corev1.SecretReference{Name: "secret", Namespace: "host-ns"},
			modify: func(spec *metal3apiv1beta1.BareMetalHostSpec) {
				spec.UserDataName = ""
				spec.NetworkDataName = ""
				spec.MetaDataName = ""
			},
			expected: nil,
		},
		{
			name: "name removed in v1beta1 for reference with empty namespace",
			ref:  &corev1.SecretReference{Name: "secret"},
			modify: func(spec *metal3apiv1beta1.BareMetalHostSpec) {
				spec.UserDataName = ""
				spec.NetworkDataName = ""
				spec.MetaDataName = ""
			},
			expected: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := gomega.NewWithT(t)

			spoke := &BareMetalHost{
				ObjectMeta: metav1.ObjectMeta{Name: "host", Namespace: "host-ns"},
				Spec: BareMetalHostSpec{
					UserData:    tc.ref.DeepCopy(),
					NetworkData: tc.ref.DeepCopy(),
					MetaData:    tc.ref.DeepCopy(),
				},
			}
			hub := &metal3apiv1beta1.BareMetalHost{}
			g.Expect(spoke.ConvertTo(hub)).To(gomega.Succeed())

			if tc.modify != nil {
				tc.modify(&hub.Spec)
			}

			restored := &BareMetalHost{}
			g.Expect(restored.ConvertFrom(hub)).To(gomega.Succeed())

			g.Expect(restored.Spec.UserData).To(gomega.Equal(tc.expected))
			g.Expect(restored.Spec.NetworkData).To(gomega.Equal(tc.expected))
			g.Expect(restored.Spec.MetaData).To(gomega.Equal(tc.expected))
		})
	}
}

// TestBareMetalHostConversionDataContent checks that the conversion-data
// annotation only records whether a Secret reference had an empty name or an
// empty namespace, and is not set at all when there is nothing to record.
func TestBareMetalHostConversionDataContent(t *testing.T) {
	testCases := []struct {
		name     string
		spec     BareMetalHostSpec
		expected string // expected annotation value, empty if not set
	}{
		{
			name:     "no references",
			spec:     BareMetalHostSpec{},
			expected: "",
		},
		{
			name: "references in the host namespace",
			spec: BareMetalHostSpec{
				UserData:    &corev1.SecretReference{Name: "user-data", Namespace: "host-ns"},
				NetworkData: &corev1.SecretReference{Name: "network-data", Namespace: "host-ns"},
				MetaData:    &corev1.SecretReference{Name: "meta-data", Namespace: "host-ns"},
			},
			expected: "",
		},
		{
			name: "reference with empty namespace",
			spec: BareMetalHostSpec{
				UserData:    &corev1.SecretReference{Name: "user-data", Namespace: "host-ns"},
				NetworkData: &corev1.SecretReference{Name: "network-data"},
			},
			expected: `{"networkData":{"emptyNamespace":true}}`,
		},
		{
			name: "reference without name",
			spec: BareMetalHostSpec{
				MetaData: &corev1.SecretReference{Namespace: "host-ns"},
			},
			expected: `{"metaData":{"emptyName":true}}`,
		},
		{
			name: "empty reference",
			spec: BareMetalHostSpec{
				UserData: &corev1.SecretReference{},
			},
			expected: `{"userData":{"emptyName":true,"emptyNamespace":true}}`,
		},
		{
			name: "all references with empty namespace",
			spec: BareMetalHostSpec{
				UserData:    &corev1.SecretReference{Name: "user-data"},
				NetworkData: &corev1.SecretReference{Name: "network-data"},
				MetaData:    &corev1.SecretReference{Name: "meta-data"},
			},
			expected: `{"userData":{"emptyNamespace":true},"networkData":{"emptyNamespace":true},"metaData":{"emptyNamespace":true}}`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := gomega.NewWithT(t)

			spoke := &BareMetalHost{
				ObjectMeta: metav1.ObjectMeta{Name: "host", Namespace: "host-ns"},
				Spec:       tc.spec,
				// The status must never end up in the annotation.
				Status: BareMetalHostStatus{
					HardwareDetails: &HardwareDetails{Hostname: "hostname"},
				},
			}
			hub := &metal3apiv1beta1.BareMetalHost{}
			g.Expect(spoke.ConvertTo(hub)).To(gomega.Succeed())

			if tc.expected == "" {
				g.Expect(hub.Annotations).NotTo(gomega.HaveKey(DataAnnotation))
			} else {
				g.Expect(hub.Annotations).To(gomega.HaveKeyWithValue(DataAnnotation, tc.expected))
			}

			restored := &BareMetalHost{}
			g.Expect(restored.ConvertFrom(hub)).To(gomega.Succeed())
			g.Expect(restored.Spec).To(gomega.Equal(tc.spec))
			g.Expect(restored.Annotations).NotTo(gomega.HaveKey(DataAnnotation))
		})
	}
}

// TestBareMetalHostConversionDoesNotModifyInput checks that neither ConvertTo
// nor ConvertFrom modify the annotations of the object being converted.
func TestBareMetalHostConversionDoesNotModifyInput(t *testing.T) {
	g := gomega.NewWithT(t)

	spoke := &BareMetalHost{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "host",
			Namespace:   "host-ns",
			Annotations: map[string]string{"example.com/annotation": "value"},
		},
		Spec: BareMetalHostSpec{
			UserData: &corev1.SecretReference{Name: "secret"},
		},
	}
	hub := &metal3apiv1beta1.BareMetalHost{}
	g.Expect(spoke.ConvertTo(hub)).To(gomega.Succeed())

	g.Expect(spoke.Annotations).To(gomega.Equal(map[string]string{"example.com/annotation": "value"}))
	g.Expect(hub.Annotations).To(gomega.HaveKeyWithValue("example.com/annotation", "value"))
	g.Expect(hub.Annotations).To(gomega.HaveKey(DataAnnotation))
	hubAnnotations := maps.Clone(hub.Annotations)

	restored := &BareMetalHost{}
	g.Expect(restored.ConvertFrom(hub)).To(gomega.Succeed())

	g.Expect(hub.Annotations).To(gomega.Equal(hubAnnotations))
	g.Expect(restored.Annotations).To(gomega.Equal(map[string]string{"example.com/annotation": "value"}))
}
