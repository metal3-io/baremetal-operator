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
	"strconv"
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
		fuzzerFuncs: []fuzzer.FuzzerFuncs{checksumTypeFuzzerFuncs},
	}))
	t.Run("for HardwareData", fuzzTestFunc(fuzzTestFuncInput{
		hub:   &metal3apiv1beta1.HardwareData{},
		spoke: &HardwareData{},
	}))
}

// checksumTypeFuzzerFuncs makes the fuzzer produce the known checksum types,
// in particular the v1alpha1-only "md5", which random strings never hit.
func checksumTypeFuzzerFuncs(_ runtimeserializer.CodecFactory) []interface{} {
	return []interface{}{
		func(in *ChecksumType, c randfill.Continue) {
			types := []ChecksumType{"", MD5, SHA256, SHA512, AutoChecksum}
			*in = types[c.Intn(len(types))]
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

// popConversionData checks that obj carries the conversion-data annotation,
// unmarshals it into stash, and removes it from obj (see
// normalizeConversionAnnotations), so that obj can be compared with an object
// that was never converted.
func popConversionData(g gomega.Gomega, obj metav1.Object, stash interface{}) {
	hasData, err := UnmarshalData(obj, stash)
	g.Expect(err).NotTo(gomega.HaveOccurred())
	g.Expect(hasData).To(gomega.BeTrue(), "expected the %s annotation", DataAnnotation)
	normalizeConversionAnnotations(obj)
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

// TestPoweredOnConversion checks that the v1alpha1 online field is converted
// to the v1beta1 poweredOn field and back.
func TestPoweredOnConversion(t *testing.T) {
	for _, value := range []bool{true, false} {
		t.Run(strconv.FormatBool(value), func(t *testing.T) {
			g := gomega.NewWithT(t)

			spoke := &BareMetalHost{Spec: BareMetalHostSpec{Online: value}}
			hub := &metal3apiv1beta1.BareMetalHost{}
			g.Expect(spoke.ConvertTo(hub)).To(gomega.Succeed())
			g.Expect(hub.Spec.PoweredOn).To(gomega.Equal(value))

			// Change the value through v1beta1 to check that ConvertFrom
			// uses poweredOn rather than the stashed v1alpha1 object.
			hub.Spec.PoweredOn = !value
			restored := &BareMetalHost{}
			g.Expect(restored.ConvertFrom(hub)).To(gomega.Succeed())
			g.Expect(restored.Spec.Online).To(gomega.Equal(!value))
		})
	}
}

// TestMD5ChecksumConversion checks that the "md5" checksum type, which does not
// exist in v1beta1, is unset and restored on the way back.
func TestMD5ChecksumConversion(t *testing.T) {
	md5Image := func() *Image {
		return &Image{URL: "http://example.com/image.qcow2", Checksum: "md5hash", ChecksumType: MD5}
	}
	newSpoke := func() *BareMetalHost {
		return &BareMetalHost{
			ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{"foo": "bar"}},
			Spec:       BareMetalHostSpec{Image: md5Image()},
			Status: BareMetalHostStatus{
				LastAttemptedImage: md5Image(),
				Provisioning:       ProvisionStatus{Image: *md5Image()},
			},
		}
	}

	t.Run("md5 is unset", func(t *testing.T) {
		g := gomega.NewWithT(t)
		hub := &metal3apiv1beta1.BareMetalHost{}
		g.Expect(newSpoke().ConvertTo(hub)).To(gomega.Succeed())
		g.Expect(hub.Spec.Image.ChecksumType).To(gomega.BeEmpty())
		g.Expect(hub.Status.LastAttemptedImage.ChecksumType).To(gomega.BeEmpty())
		g.Expect(hub.Status.Provisioning.Image.ChecksumType).To(gomega.BeEmpty())
		g.Expect(hub.Spec.Image.Checksum).To(gomega.Equal("md5hash"))
		g.Expect(hub.Annotations).To(gomega.HaveKeyWithValue("foo", "bar"))
		g.Expect(hub.Annotations).To(gomega.HaveKey(DataAnnotation))
	})

	t.Run("spoke-hub-spoke preserves md5", func(t *testing.T) {
		g := gomega.NewWithT(t)
		spoke := newSpoke()
		hub := &metal3apiv1beta1.BareMetalHost{}
		g.Expect(spoke.ConvertTo(hub)).To(gomega.Succeed())
		// The source object must not be modified.
		g.Expect(spoke).To(gomega.Equal(newSpoke()))

		hubBefore := hub.DeepCopy()

		restored := &BareMetalHost{}
		g.Expect(restored.ConvertFrom(hub)).To(gomega.Succeed())
		// The source hub must not be modified either.
		g.Expect(hub).To(gomega.Equal(hubBefore))

		// ConvertFrom replaces the stashed v1alpha1 object with a stash of
		// the hub, in which the md5 checksum type is unset.
		stashedHub := &metal3apiv1beta1.BareMetalHost{}
		popConversionData(g, restored, stashedHub)
		g.Expect(stashedHub.Spec).To(gomega.Equal(hub.Spec))
		g.Expect(stashedHub.Status).To(gomega.Equal(hub.Status))
		g.Expect(stashedHub.Spec.Image.ChecksumType).To(gomega.BeEmpty())

		g.Expect(restored).To(gomega.Equal(newSpoke()))
	})

	t.Run("annotation is set without md5", func(t *testing.T) {
		g := gomega.NewWithT(t)
		spoke := &BareMetalHost{Spec: BareMetalHostSpec{Image: &Image{
			URL: "http://example.com/image.qcow2", Checksum: "shahash", ChecksumType: SHA256,
		}}}
		hub := &metal3apiv1beta1.BareMetalHost{}
		g.Expect(spoke.ConvertTo(hub)).To(gomega.Succeed())
		g.Expect(hub.Spec.Image.ChecksumType).To(gomega.Equal(metal3apiv1beta1.SHA256))

		// The whole v1alpha1 object is stashed, even if v1beta1 can represent
		// it, and it is the only annotation.
		stashedSpoke := &BareMetalHost{}
		popConversionData(g, hub, stashedSpoke)
		g.Expect(stashedSpoke.Spec).To(gomega.Equal(spoke.Spec))
		g.Expect(hub.Annotations).To(gomega.BeNil())
	})

	t.Run("md5 is not restored for a changed image", func(t *testing.T) {
		g := gomega.NewWithT(t)
		hub := &metal3apiv1beta1.BareMetalHost{}
		g.Expect(newSpoke().ConvertTo(hub)).To(gomega.Succeed())
		hub.Spec.Image.Checksum = "newhash"
		hub.Status.LastAttemptedImage.URL = "http://example.com/other.qcow2"
		hub.Status.Provisioning.Image.ChecksumType = metal3apiv1beta1.SHA512

		restored := &BareMetalHost{}
		g.Expect(restored.ConvertFrom(hub)).To(gomega.Succeed())
		g.Expect(restored.Spec.Image.ChecksumType).To(gomega.BeEmpty())
		g.Expect(restored.Status.LastAttemptedImage.ChecksumType).To(gomega.BeEmpty())
		g.Expect(restored.Status.Provisioning.Image.ChecksumType).To(gomega.Equal(SHA512))

		// The stash of the hub carries the changed images.
		stashedHub := &metal3apiv1beta1.BareMetalHost{}
		popConversionData(g, restored, stashedHub)
		g.Expect(stashedHub.Spec.Image).To(gomega.Equal(hub.Spec.Image))
		g.Expect(stashedHub.Status.LastAttemptedImage).To(gomega.Equal(hub.Status.LastAttemptedImage))
		g.Expect(stashedHub.Status.Provisioning.Image).To(gomega.Equal(hub.Status.Provisioning.Image))
		g.Expect(restored.Annotations).To(gomega.Equal(map[string]string{"foo": "bar"}))
	})

	t.Run("md5 is not restored for a removed image", func(t *testing.T) {
		g := gomega.NewWithT(t)
		hub := &metal3apiv1beta1.BareMetalHost{}
		g.Expect(newSpoke().ConvertTo(hub)).To(gomega.Succeed())
		hub.Spec.Image = nil

		restored := &BareMetalHost{}
		g.Expect(restored.ConvertFrom(hub)).To(gomega.Succeed())
		g.Expect(restored.Spec.Image).To(gomega.BeNil())
		g.Expect(restored.Status.LastAttemptedImage.ChecksumType).To(gomega.Equal(MD5))
	})

	for _, checksumType := range []metal3apiv1beta1.ChecksumType{"", metal3apiv1beta1.AutoChecksum} {
		t.Run("hub-spoke-hub keeps type \""+string(checksumType)+"\"", func(t *testing.T) {
			g := gomega.NewWithT(t)
			hub := &metal3apiv1beta1.BareMetalHost{
				Spec: metal3apiv1beta1.BareMetalHostSpec{Image: &metal3apiv1beta1.Image{
					URL: "http://example.com/image.qcow2", Checksum: "md5hash", ChecksumType: checksumType,
				}},
			}
			spoke := &BareMetalHost{}
			g.Expect(spoke.ConvertFrom(hub.DeepCopy())).To(gomega.Succeed())
			g.Expect(spoke.Spec.Image.ChecksumType).To(gomega.Equal(ChecksumType(checksumType)))

			// ConvertFrom stashes the hub in the spoke.
			g.Expect(spoke.Annotations).To(gomega.HaveKey(DataAnnotation))

			hubAfter := &metal3apiv1beta1.BareMetalHost{}
			g.Expect(spoke.ConvertTo(hubAfter)).To(gomega.Succeed())

			// ConvertTo replaces the stashed hub with a stash of the spoke,
			// so the annotation is carried back to the hub as well.
			stashedSpoke := &BareMetalHost{}
			popConversionData(g, hubAfter, stashedSpoke)
			g.Expect(stashedSpoke.Spec.Image.ChecksumType).To(gomega.Equal(ChecksumType(checksumType)))
			g.Expect(hubAfter).To(gomega.Equal(hub))
		})
	}

	t.Run("md5 is not restored when auto is set on the hub", func(t *testing.T) {
		g := gomega.NewWithT(t)
		hub := &metal3apiv1beta1.BareMetalHost{}
		g.Expect(newSpoke().ConvertTo(hub)).To(gomega.Succeed())
		hub.Spec.Image.ChecksumType = metal3apiv1beta1.AutoChecksum

		restored := &BareMetalHost{}
		g.Expect(restored.ConvertFrom(hub)).To(gomega.Succeed())
		g.Expect(restored.Spec.Image.ChecksumType).To(gomega.Equal(AutoChecksum))
		g.Expect(restored.Status.LastAttemptedImage.ChecksumType).To(gomega.Equal(MD5))
	})
}

// TestFirmwareConversion checks that the firmware configuration, which does not
// exist in v1beta1, is dropped and restored from the stash on the way back,
// even if other fields are changed through v1beta1.
func TestFirmwareConversion(t *testing.T) {
	g := gomega.NewWithT(t)

	newFirmware := func() *FirmwareConfig {
		return &FirmwareConfig{
			VirtualizationEnabled:             ptr.To(true),
			SimultaneousMultithreadingEnabled: ptr.To(false),
		}
	}
	spoke := &BareMetalHost{
		Spec: BareMetalHostSpec{Firmware: newFirmware()},
		Status: BareMetalHostStatus{
			Provisioning: ProvisionStatus{Firmware: newFirmware()},
		},
	}
	hub := &metal3apiv1beta1.BareMetalHost{}
	g.Expect(spoke.ConvertTo(hub)).To(gomega.Succeed())
	hub.Spec.Description = "changed in v1beta1"

	restored := &BareMetalHost{}
	g.Expect(restored.ConvertFrom(hub)).To(gomega.Succeed())
	g.Expect(restored.Spec.Firmware).To(gomega.Equal(newFirmware()))
	g.Expect(restored.Status.Provisioning.Firmware).To(gomega.Equal(newFirmware()))
	g.Expect(restored.Spec.Description).To(gomega.Equal("changed in v1beta1"))

	// Without a stash, e.g. for an object created through v1beta1, there is
	// no firmware configuration.
	fromHub := &BareMetalHost{}
	g.Expect(fromHub.ConvertFrom(&metal3apiv1beta1.BareMetalHost{})).To(gomega.Succeed())
	g.Expect(fromHub.Spec.Firmware).To(gomega.BeNil())
	g.Expect(fromHub.Status.Provisioning.Firmware).To(gomega.BeNil())
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
			name:     "name and other namespace",
			ref:      &corev1.SecretReference{Name: "secret", Namespace: "other-ns"},
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

// TestDataNameConversionRestore checks that the Secret references stashed in
// the conversion-data annotation are restored only as long as the Secret
// name has not been changed through v1beta1.
func TestDataNameConversionRestore(t *testing.T) {
	testCases := []struct {
		name     string
		ref      *corev1.SecretReference
		modify   func(*metal3apiv1beta1.BareMetalHostSpec)
		expected *corev1.SecretReference
	}{
		{
			name:     "empty namespace is preserved",
			ref:      &corev1.SecretReference{Name: "secret"},
			expected: &corev1.SecretReference{Name: "secret"},
		},
		{
			name:     "host namespace is preserved",
			ref:      &corev1.SecretReference{Name: "secret", Namespace: "host-ns"},
			expected: &corev1.SecretReference{Name: "secret", Namespace: "host-ns"},
		},
		{
			name:     "other namespace is preserved",
			ref:      &corev1.SecretReference{Name: "secret", Namespace: "other-ns"},
			expected: &corev1.SecretReference{Name: "secret", Namespace: "other-ns"},
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
			name:     "nil reference is preserved",
			ref:      nil,
			expected: nil,
		},
		{
			name: "name changed in v1beta1",
			ref:  &corev1.SecretReference{Name: "secret"},
			modify: func(spec *metal3apiv1beta1.BareMetalHostSpec) {
				spec.UserDataName = "new-secret"
				spec.NetworkDataName = "new-secret"
				spec.MetaDataName = "new-secret"
			},
			expected: &corev1.SecretReference{Name: "new-secret", Namespace: "host-ns"},
		},
		{
			name: "name changed in v1beta1 for reference in other namespace",
			ref:  &corev1.SecretReference{Name: "secret", Namespace: "other-ns"},
			modify: func(spec *metal3apiv1beta1.BareMetalHostSpec) {
				spec.UserDataName = "new-secret"
				spec.NetworkDataName = "new-secret"
				spec.MetaDataName = "new-secret"
			},
			expected: &corev1.SecretReference{Name: "new-secret", Namespace: "host-ns"},
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
			name: "name removed in v1beta1",
			ref:  &corev1.SecretReference{Name: "secret", Namespace: "host-ns"},
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

// TestBareMetalHostConversionDataAnnotation checks the conversion-data
// annotation handling: ConvertTo stashes the v1alpha1 object in the v1beta1
// hub, ConvertFrom replaces it with a stash of the v1beta1 object, and neither
// modifies the annotations of the object being converted.
func TestBareMetalHostConversionDataAnnotation(t *testing.T) {
	g := gomega.NewWithT(t)

	spoke := &BareMetalHost{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "host",
			Namespace:   "host-ns",
			Annotations: map[string]string{"example.com/annotation": "value"},
		},
		Spec: BareMetalHostSpec{
			UserData: &corev1.SecretReference{Name: "secret", Namespace: "other-ns"},
		},
	}
	hub := &metal3apiv1beta1.BareMetalHost{}
	g.Expect(spoke.ConvertTo(hub)).To(gomega.Succeed())

	// ConvertTo does not modify the v1alpha1 source, and stashes it in the hub.
	g.Expect(spoke.Annotations).To(gomega.Equal(map[string]string{"example.com/annotation": "value"}))
	g.Expect(hub.Annotations).To(gomega.HaveKeyWithValue("example.com/annotation", "value"))
	g.Expect(hub.Annotations).To(gomega.HaveKey(DataAnnotation))
	stashedSpoke := &BareMetalHost{}
	g.Expect(UnmarshalData(hub.DeepCopy(), stashedSpoke)).To(gomega.BeTrue())
	g.Expect(stashedSpoke.Spec).To(gomega.Equal(spoke.Spec))
	hubAnnotations := maps.Clone(hub.Annotations)

	restored := &BareMetalHost{}
	g.Expect(restored.ConvertFrom(hub)).To(gomega.Succeed())

	// ConvertFrom does not modify the v1beta1 source, and replaces the
	// stashed v1alpha1 object with a stash of the hub.
	g.Expect(hub.Annotations).To(gomega.Equal(hubAnnotations))
	g.Expect(restored.Annotations).To(gomega.HaveKeyWithValue("example.com/annotation", "value"))
	g.Expect(restored.Annotations).To(gomega.HaveLen(2))
	stashedHub := &metal3apiv1beta1.BareMetalHost{}
	g.Expect(UnmarshalData(restored.DeepCopy(), stashedHub)).To(gomega.BeTrue())
	g.Expect(stashedHub.Spec).To(gomega.Equal(hub.Spec))
	g.Expect(restored.Spec).To(gomega.Equal(spoke.Spec))
}
