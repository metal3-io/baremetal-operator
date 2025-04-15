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
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// bmcsubscriptionlog is for logging in this package.
var bmcsubscriptionlog = logf.Log.WithName("webhooks").WithName("BMCEventSubscription")

func (webhook *BMCEventSubscription) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&metal3api.BMCEventSubscription{}).
		WithValidator(webhook).
		Complete()
}

//+kubebuilder:webhook:verbs=create;update,path=/validate-metal3-io-v1alpha1-bmceventsubscription,mutating=false,failurePolicy=fail,sideEffects=none,admissionReviewVersions=v1;v1beta,groups=metal3.io,resources=bmceventsubscriptions,versions=v1alpha1,name=bmceventsubscription.metal3.io

// BMCEventSubscription implements a validation and defaulting webhook for BMCEventSubscription.
type BMCEventSubscription struct{}

var _ webhook.CustomValidator = &BMCEventSubscription{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type.
func (webhook *BMCEventSubscription) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	bmces, ok := obj.(*metal3api.BMCEventSubscription)
	if !ok {
		return nil, k8serrors.NewBadRequest(fmt.Sprintf("expected a BMCEventSubscription but got a %T", obj))
	}

	bmcsubscriptionlog.Info("validate create", "namespace", bmces.Namespace, "name", bmces.Name)
	return nil, kerrors.NewAggregate(webhook.validateSubscription(bmces))
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type.
func (webhook *BMCEventSubscription) ValidateUpdate(_ context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	newBMCES, ok := newObj.(*metal3api.BMCEventSubscription)
	if !ok {
		return nil, k8serrors.NewBadRequest(fmt.Sprintf("expected a BMCEventSubscription but got a %T", newObj))
	}
	bmcsubscriptionlog.Info("validate update", "namespace", newBMCES.Namespace, "name", newBMCES.Name)

	oldBMCES, casted := oldObj.(*metal3api.BMCEventSubscription)
	if !casted {
		bmcsubscriptionlog.Error(fmt.Errorf("old object conversion error for %s/%s", oldBMCES.Namespace, oldBMCES.Name), "validate update error")
		return nil, nil
	}

	if newBMCES.Spec != oldBMCES.Spec {
		return nil, errors.New("subscriptions cannot be updated, please recreate it")
	}

	return nil, nil
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type.
func (webhook *BMCEventSubscription) ValidateDelete(_ context.Context, _ runtime.Object) (admission.Warnings, error) {
	return nil, nil
}
