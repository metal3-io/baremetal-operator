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
	"errors"

	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// HostSerialConsole validates requests against a same-namespace BareMetalHost.
// Consumers never supply backend URLs, credentials or Service configuration.
type HostSerialConsole struct{ Client client.Reader }

var _ admission.Validator[*metal3api.HostSerialConsole] = &HostSerialConsole{}

// SetupWebhookWithManager registers HostSerialConsole admission.
func (webhook *HostSerialConsole) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &metal3api.HostSerialConsole{}).WithValidator(webhook).Complete()
}

// +kubebuilder:webhook:verbs=create;update,path=/validate-metal3-io-v1alpha1-hostserialconsole,mutating=false,failurePolicy=fail,sideEffects=none,admissionReviewVersions=v1,groups=metal3.io,resources=hostserialconsoles,versions=v1alpha1,name=hostserialconsole.metal3.io

// ValidateCreate verifies that the request refers to an existing host.
func (webhook *HostSerialConsole) ValidateCreate(ctx context.Context, console *metal3api.HostSerialConsole) (admission.Warnings, error) {
	if console == nil {
		return nil, errors.New("HostSerialConsole must not be nil")
	}
	if err := validateBMHName(console.Name); err != nil {
		return nil, err
	}
	host := &metal3api.BareMetalHost{}
	if err := webhook.Client.Get(ctx, client.ObjectKeyFromObject(console), host); err != nil {
		if !k8serrors.IsNotFound(err) {
			return nil, err
		}
		return nil, k8serrors.NewInvalid(schema.GroupKind{Group: metal3api.GroupVersion.Group, Kind: "HostSerialConsole"}, console.Name,
			field.ErrorList{field.Invalid(field.NewPath("metadata", "name"), console.Name, "must match an existing BareMetalHost in the same namespace")})
	}
	if !host.DeletionTimestamp.IsZero() {
		return nil, errors.New("cannot request a console for a deleting BareMetalHost")
	}
	if _, detached := host.Annotations[metal3api.DetachedAnnotation]; detached {
		return nil, errors.New("cannot request a console for a detached BareMetalHost")
	}
	return nil, nil
}

// ValidateUpdate allows metadata/finalizer updates even after the host is gone.
func (webhook *HostSerialConsole) ValidateUpdate(_ context.Context, oldConsole, newConsole *metal3api.HostSerialConsole) (admission.Warnings, error) {
	if oldConsole == nil || newConsole == nil {
		return nil, errors.New("HostSerialConsole must not be nil")
	}
	return nil, nil
}

// ValidateDelete always allows closing access, including for missing hosts.
func (webhook *HostSerialConsole) ValidateDelete(_ context.Context, _ *metal3api.HostSerialConsole) (admission.Warnings, error) {
	return nil, nil
}
