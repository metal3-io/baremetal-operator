/*
Copyright 2025 The Metal3 Authors.

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
	"errors"
	"fmt"

	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// log is for logging in this package.
var baremetalhostlog = logf.Log.WithName("webhooks").WithName("BareMetalHost")

// SetupWebhookWithManager registers the BareMetalHost validation and defaulting webhooks with the manager.
func (webhook *BareMetalHost) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &metal3api.BareMetalHost{}).
		WithValidator(webhook).
		WithDefaulter(webhook).
		Complete()
}

//+kubebuilder:webhook:verbs=create;update,path=/validate-metal3-io-v1alpha1-baremetalhost,mutating=false,failurePolicy=fail,sideEffects=none,admissionReviewVersions=v1;v1beta,groups=metal3.io,resources=baremetalhosts,versions=v1alpha1,name=baremetalhost.metal3.io

// BareMetalHost implements a validation and defaulting webhook for BareMetalHost.
type BareMetalHost struct{}

var _ admission.Defaulter[*metal3api.BareMetalHost] = &BareMetalHost{}
var _ admission.Validator[*metal3api.BareMetalHost] = &BareMetalHost{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type.
func (webhook *BareMetalHost) ValidateCreate(_ context.Context, bmh *metal3api.BareMetalHost) (admission.Warnings, error) {
	baremetalhostlog.Info("validate create", "namespace", bmh.Namespace, "name", bmh.Name)
	return nil, kerrors.NewAggregate(webhook.validateHost(bmh))
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type.
func (webhook *BareMetalHost) ValidateUpdate(_ context.Context, oldBmh, newBmh *metal3api.BareMetalHost) (admission.Warnings, error) {
	if oldBmh == nil {
		baremetalhostlog.Error(errors.New("old object is nil"), "validate update error")
		return nil, nil
	}

	if newBmh == nil {
		baremetalhostlog.Error(fmt.Errorf("new object is nil for %s/%s", oldBmh.Namespace, oldBmh.Name), "validate update error")
		return nil, nil
	}
	baremetalhostlog.Info("validate update", "namespace", newBmh.Namespace, "name", newBmh.Name)
	return nil, kerrors.NewAggregate(webhook.validateChanges(oldBmh, newBmh))
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type.
func (webhook *BareMetalHost) ValidateDelete(_ context.Context, _ *metal3api.BareMetalHost) (admission.Warnings, error) {
	return nil, nil
}

// Default implements webhook.Defaulter so a webhook will be registered for the type.
func (webhook *BareMetalHost) Default(_ context.Context, _ *metal3api.BareMetalHost) error {
	return nil
}
