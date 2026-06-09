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

// dataimagelog is for logging in this webhook.
var dataimagelog = logf.Log.WithName("webhooks").WithName("DataImage")

// SetupWebhookWithManager registers the BMCEventSubscription validation and defaulting webhooks with the manager.
func (webhook *DataImage) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &metal3api.DataImage{}).
		WithValidator(webhook).
		WithDefaulter(webhook).
		Complete()
}

//+kubebuilder:webhook:verbs=create;update,path=/validate-metal3-io-v1alpha1-dataimage,mutating=false,failurePolicy=fail,sideEffects=none,admissionReviewVersions=v1,groups=metal3.io,resources=dataimages,versions=v1alpha1,name=dataimage.metal3.io

// DataImage implements a validation and defaulting webhook for DataImage.
type DataImage struct{}

var _ admission.Defaulter[*metal3api.DataImage] = &DataImage{}
var _ admission.Validator[*metal3api.DataImage] = &DataImage{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type.
func (webhook *DataImage) ValidateCreate(_ context.Context, dataimg *metal3api.DataImage) (admission.Warnings, error) {
	if dataimg == nil {
		dataimagelog.Error(errors.New("object is nil"), "validate create error")
		return nil, nil
	}

	dataimagelog.Info("validate create", "namespace", dataimg.Namespace, "name", dataimg.Name)
	return nil, kerrors.NewAggregate(webhook.validateDataimage(dataimg))
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type.
func (webhook *DataImage) ValidateUpdate(_ context.Context, oldImg, newImg *metal3api.DataImage) (admission.Warnings, error) {
	if oldImg == nil {
		dataimagelog.Error(errors.New("old object is nil"), "validate update error")
		return nil, nil
	}

	if newImg == nil {
		dataimagelog.Error(fmt.Errorf("new object is nil for %s/%s", oldImg.Namespace, oldImg.Name), "validate update error")
		return nil, nil
	}
	dataimagelog.Info("validate update", "namespace", newImg.Namespace, "name", newImg.Name)

	return nil, kerrors.NewAggregate(webhook.validateDataimage(newImg))
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type.
func (webhook *DataImage) ValidateDelete(_ context.Context, _ *metal3api.DataImage) (admission.Warnings, error) {
	return nil, nil
}

// Default implements webhook.Defaulter so a webhook will be registered for the type.
func (webhook *DataImage) Default(_ context.Context, _ *metal3api.DataImage) error {
	return nil
}
