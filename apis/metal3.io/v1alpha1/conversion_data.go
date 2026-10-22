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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/json"
)

// The helpers in this file implement the same conversion-data annotation
// round-trip that cluster-api's sigs.k8s.io/cluster-api/util/conversion package
// provides (MarshalData/UnmarshalData). They are copied here, rather than
// imported, so the lightweight apis module does not need to depend on the whole
// cluster-api module. The behaviour and annotation semantics match the upstream
// implementation so the conversions stay compatible with the wider metal3
// ecosystem. CAPM3 has adopted the same approach for handling conversion data.
// It uses the CAPI MarshalData/UnmarshalData helpers to manage conversion data.
// These helpers can be removed once the v1alpha1 API is no longer used, that is
// why the helpers are included directly in this package.

// DataAnnotation is the annotation that conversion webhooks
// use to retain the data in case of down-conversion from the hub.
const DataAnnotation = "metal3.io/conversion-data"

// MarshalData stores the source object as json data in the destination object annotations map.
// It ignores the metadata of the source object.
func MarshalData(src metav1.Object, dst metav1.Object) error {
	u, err := runtime.DefaultUnstructuredConverter.ToUnstructured(src)
	if err != nil {
		return err
	}
	delete(u, "metadata")

	data, err := json.Marshal(u)
	if err != nil {
		return err
	}
	annotations := dst.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}
	annotations[DataAnnotation] = string(data)
	dst.SetAnnotations(annotations)
	return nil
}

// UnmarshalData retrieves the data from the annotation set by MarshalData and
// unmarshals it into the object passed as input. It reports whether the
// annotation was present, and removes the annotation from the source object so
// it does not leak into the converted result.
func UnmarshalData(from metav1.Object, to interface{}) (bool, error) {
	annotations := from.GetAnnotations()
	data, ok := annotations[DataAnnotation]
	if !ok {
		return false, nil
	}
	if err := json.Unmarshal([]byte(data), to); err != nil {
		return false, err
	}
	delete(annotations, DataAnnotation)
	from.SetAnnotations(annotations)
	return true, nil
}
