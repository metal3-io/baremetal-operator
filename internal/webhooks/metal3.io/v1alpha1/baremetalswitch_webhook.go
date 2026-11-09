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
	"fmt"

	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	"github.com/metal3-io/baremetal-operator/pkg/features"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

//+kubebuilder:webhook:verbs=create,path=/validate-metal3-io-v1alpha1-baremetalswitch,mutating=false,failurePolicy=fail,sideEffects=none,admissionReviewVersions=v1,groups=metal3.io,resources=baremetalswitches,versions=v1alpha1,name=baremetalswitch.metal3.io

// BareMetalSwitch implements a validation webhook for BareMetalSwitch.
type BareMetalSwitch struct{}

var _ admission.Validator[*metal3api.BareMetalSwitch] = &BareMetalSwitch{}

func (webhook *BareMetalSwitch) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &metal3api.BareMetalSwitch{}).
		WithValidator(webhook).
		Complete()
}

// ValidateCreate prevents creating BareMetalSwitch resources while IronicNetworking is disabled.
func (webhook *BareMetalSwitch) ValidateCreate(_ context.Context, _ *metal3api.BareMetalSwitch) (admission.Warnings, error) {
	if !features.CurrentFeatureGate.Enabled(features.FeatureIronicNetworking) {
		return nil, fmt.Errorf("%s feature gate must be enabled to create BareMetalSwitch resources", features.FeatureIronicNetworking)
	}
	return nil, nil
}

// ValidateUpdate implements webhook.Validator.
func (webhook *BareMetalSwitch) ValidateUpdate(_ context.Context, _, _ *metal3api.BareMetalSwitch) (admission.Warnings, error) {
	return nil, nil
}

// ValidateDelete implements webhook.Validator.
func (webhook *BareMetalSwitch) ValidateDelete(_ context.Context, _ *metal3api.BareMetalSwitch) (admission.Warnings, error) {
	return nil, nil
}
