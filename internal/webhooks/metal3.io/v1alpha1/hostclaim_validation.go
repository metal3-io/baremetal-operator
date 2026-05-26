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
	"strings"

	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/apimachinery/pkg/util/validation"
)

// validateHostClaim validates a HostClaim resource.
func (webhook *HostClaimWebhook) validateHostClaim(hostclaim *metal3api.HostClaim) []error {
	var errs []error
	for lblKey, lblValue := range hostclaim.Spec.HostSelector.MatchLabels {
		for _, err := range validation.IsQualifiedName(lblKey) {
			errs = append(errs, fmt.Errorf("%s=%s: %s", lblKey, lblValue, err))
		}
		for _, err := range validation.IsValidLabelValue(lblValue) {
			errs = append(errs, fmt.Errorf("%s=%s: %s", lblKey, lblValue, err))
		}
	}
	for i, hsr := range hostclaim.Spec.HostSelector.MatchExpressions {
		for _, err := range validation.IsQualifiedName(hsr.Key) {
			errs = append(errs, fmt.Errorf("matchExpr %d: %s", i+1, err))
		}
		switch hsr.Operator {
		case selection.Equals, selection.DoubleEquals, selection.NotEquals:
			if len(hsr.Values) != 1 {
				errs = append(errs, fmt.Errorf(
					"matchExpr %d: exactly one value for operator %s in match expression on label key %s",
					i+1, hsr.Operator, hsr.Key))
			} else {
				for _, err := range validation.IsValidLabelValue(hsr.Values[0]) {
					errs = append(errs, fmt.Errorf("matchExpr %d (value %q): %s", i+1, hsr.Values[0], err))
				}
			}
		case selection.In, selection.NotIn:
			if len(hsr.Values) == 0 {
				errs = append(errs, fmt.Errorf(
					"matchExpr %d: At least one value in list for operator %s in match expression on label key %s",
					i+1, hsr.Operator, hsr.Key))
			}
			for _, v := range hsr.Values {
				for _, err := range validation.IsValidLabelValue(v) {
					errs = append(errs, fmt.Errorf("matchExpr %d (value %q): %s", i+1, v, err))
				}
			}
		case selection.Exists, selection.DoesNotExist:
			if len(hsr.Values) != 0 {
				errs = append(errs, fmt.Errorf(
					"matchExpr %d: values not authorized for operator %s in match expression on label key %s",
					i+1, hsr.Operator, hsr.Key))
			}
		default:
			errs = append(errs, fmt.Errorf(
				"matchExpr %d: invalid operation %s in match expression on label key %s", i+1, hsr.Operator, hsr.Key))
		}
	}

	annotationErrs := validateHostclaimAnnotations(hostclaim)
	errs = append(errs, annotationErrs...)

	if hostclaim.Spec.Image != nil {
		imageErrs := validateImage(hostclaim.Spec.Image)
		errs = append(errs, imageErrs...)
	}

	return errs
}

func validateHostclaimAnnotations(hostclaim *metal3api.HostClaim) []error {
	var errs []error
	var err error

	for annotation, value := range hostclaim.Annotations {
		switch {
		case strings.HasPrefix(annotation, metal3api.RebootAnnotationPrefix+"/") || annotation == metal3api.RebootAnnotationPrefix:
			err = validateRebootAnnotation(value)
		// TODO: should we check Detached annotation.
		default:
			err = nil
		}
		if err != nil {
			errs = append(errs, err)
		}
	}

	return errs
}
