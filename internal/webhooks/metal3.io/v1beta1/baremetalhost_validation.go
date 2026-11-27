/*
Copyright 2026 The Metal3 Authors.

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

package webhooks

import (
	"fmt"

	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1beta1"
)

// validateHost checks the rules that apply to v1beta1 only. Everything the two
// versions have in common is validated by the v1alpha1 webhook, which the API
// server also calls for v1beta1 requests (see the package documentation).
func validateHost(host *metal3api.BareMetalHost) []error {
	var errs []error

	if err := validateInspectAnnotation(host); err != nil {
		errs = append(errs, err)
	}

	return errs
}

// validateInspectAnnotation only accepts an empty inspect.metal3.io annotation,
// which requests a new inspection. The "disabled" value is replaced by
// Spec.InspectionMode in v1beta1, and v1alpha1 never accepted any other value.
func validateInspectAnnotation(host *metal3api.BareMetalHost) error {
	if host.Annotations[metal3api.InspectAnnotationPrefix] != "" {
		return fmt.Errorf("the %s annotation supports only an empty value, set spec.inspectionMode to disable inspection",
			metal3api.InspectAnnotationPrefix)
	}

	return nil
}
