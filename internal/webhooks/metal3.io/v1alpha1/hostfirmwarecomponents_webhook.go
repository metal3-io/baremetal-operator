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

// hfclog is for logging in this package.
var hfclog = logf.Log.WithName("webhooks").WithName("HostFirmwareComponents")

// SetupWebhookWithManager registers the HostFirmwareComponents validation and defaulting webhooks with the manager.
func (webhook *HostFirmwareComponents) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &metal3api.HostFirmwareComponents{}).
		WithValidator(webhook).
		WithDefaulter(webhook).
		Complete()
}

//+kubebuilder:webhook:verbs=create;update,path=/validate-metal3-io-v1alpha1-hostfirmwarecomponents,mutating=false,failurePolicy=fail,sideEffects=none,admissionReviewVersions=v1,groups=metal3.io,resources=hostfirmwarecomponents,versions=v1alpha1,name=hostfirmwarecomponents.metal3.io

// HostFirmwareComponents implements a validation and defaulting webhook for HostFirmwareComponents.
type HostFirmwareComponents struct{}

var _ admission.Defaulter[*metal3api.HostFirmwareComponents] = &HostFirmwareComponents{}
var _ admission.Validator[*metal3api.HostFirmwareComponents] = &HostFirmwareComponents{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type.
func (webhook *HostFirmwareComponents) ValidateCreate(_ context.Context, hfc *metal3api.HostFirmwareComponents) (admission.Warnings, error) {
	if hfc == nil {
		hfclog.Error(errors.New("object is nil"), "validate create error")
		return nil, nil
	}

	hfclog.Info("validate create", "namespace", hfc.Namespace, "name", hfc.Name)
	return nil, kerrors.NewAggregate(webhook.validateHostFirmwareComponents(hfc))
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type.
func (webhook *HostFirmwareComponents) ValidateUpdate(_ context.Context, oldHfc, newHfc *metal3api.HostFirmwareComponents) (admission.Warnings, error) {
	if oldHfc == nil {
		hfclog.Error(errors.New("old object is nil"), "validate update error")
		return nil, nil
	}

	if newHfc == nil {
		hfclog.Error(fmt.Errorf("new object is nil for %s/%s", oldHfc.Namespace, oldHfc.Name), "validate update error")
		return nil, nil
	}
	hfclog.Info("validate update", "namespace", newHfc.Namespace, "name", newHfc.Name)

	return nil, kerrors.NewAggregate(webhook.validateHostFirmwareComponents(newHfc))
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type.
func (webhook *HostFirmwareComponents) ValidateDelete(_ context.Context, _ *metal3api.HostFirmwareComponents) (admission.Warnings, error) {
	return nil, nil
}

// Default implements webhook.Defaulter so a webhook will be registered for the type.
func (webhook *HostFirmwareComponents) Default(_ context.Context, _ *metal3api.HostFirmwareComponents) error {
	return nil
}
