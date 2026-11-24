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
	"context"
	"testing"

	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1beta1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func hostWithAnnotations(annotations map[string]string) *metal3api.BareMetalHost {
	return &metal3api.BareMetalHost{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "myhost",
			Namespace:   "myns",
			Annotations: annotations,
		},
	}
}

func TestValidateInspectAnnotation(t *testing.T) {
	for _, tc := range []struct {
		Scenario    string
		Annotations map[string]string
		Allowed     bool
	}{
		{
			Scenario: "no annotations",
			Allowed:  true,
		},
		{
			Scenario: "an empty value requests inspection",
			Annotations: map[string]string{
				metal3api.InspectAnnotationPrefix: "",
			},
			Allowed: true,
		},
		{
			Scenario: "disabled is rejected",
			Annotations: map[string]string{
				metal3api.InspectAnnotationPrefix: "disabled",
			},
			Allowed: false,
		},
		{
			Scenario: "any other value is rejected",
			Annotations: map[string]string{
				metal3api.InspectAnnotationPrefix: "now",
			},
			Allowed: false,
		},
		{
			Scenario: "the hardwaredetails annotation is unaffected",
			Annotations: map[string]string{
				metal3api.HardwareDetailsAnnotation: "{}",
			},
			Allowed: true,
		},
	} {
		t.Run(tc.Scenario, func(t *testing.T) {
			webhook := &BareMetalHost{}
			host := hostWithAnnotations(tc.Annotations)

			_, err := webhook.ValidateCreate(context.Background(), host)
			if tc.Allowed {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), metal3api.InspectAnnotationPrefix)
			}

			// The same rule applies to updates.
			_, err = webhook.ValidateUpdate(context.Background(), hostWithAnnotations(nil), host)
			if tc.Allowed {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}

func TestInspectionModeDisabledIsAllowed(t *testing.T) {
	// The field is the supported way to disable inspection in v1beta1.
	host := hostWithAnnotations(nil)
	host.Spec.InspectionMode = metal3api.InspectionModeDisabled

	webhook := &BareMetalHost{}
	_, err := webhook.ValidateCreate(context.Background(), host)
	require.NoError(t, err)
}

func TestValidateNilObjects(t *testing.T) {
	webhook := &BareMetalHost{}

	_, err := webhook.ValidateCreate(context.Background(), nil)
	require.NoError(t, err)

	_, err = webhook.ValidateUpdate(context.Background(), nil, hostWithAnnotations(nil))
	require.NoError(t, err)

	_, err = webhook.ValidateUpdate(context.Background(), hostWithAnnotations(nil), nil)
	require.NoError(t, err)

	_, err = webhook.ValidateDelete(context.Background(), hostWithAnnotations(nil))
	require.NoError(t, err)
}
