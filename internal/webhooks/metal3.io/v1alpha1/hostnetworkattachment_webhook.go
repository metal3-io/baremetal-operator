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

package webhooks

import (
	"context"
	"fmt"

	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	"github.com/metal3-io/baremetal-operator/pkg/features"
	"k8s.io/apimachinery/pkg/api/equality"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// log is for logging in this package.
var hostnetworkattachmentlog = logf.Log.WithName("webhooks").WithName("HostNetworkAttachment")

func (webhook *HostNetworkAttachment) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &metal3api.HostNetworkAttachment{}).
		WithValidator(webhook).
		Complete()
}

//+kubebuilder:webhook:verbs=create;update,path=/validate-metal3-io-v1alpha1-hostnetworkattachment,mutating=false,failurePolicy=fail,sideEffects=none,admissionReviewVersions=v1,groups=metal3.io,resources=hostnetworkattachments,versions=v1alpha1,name=hostnetworkattachment.metal3.io

// HostNetworkAttachment implements a validation webhook for HostNetworkAttachment.
type HostNetworkAttachment struct{}

var _ admission.Validator[*metal3api.HostNetworkAttachment] = &HostNetworkAttachment{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type.
func (webhook *HostNetworkAttachment) ValidateCreate(_ context.Context, attachment *metal3api.HostNetworkAttachment) (admission.Warnings, error) {
	hostnetworkattachmentlog.Info("validate create", "namespace", attachment.Namespace, "name", attachment.Name)
	if !features.CurrentFeatureGate.Enabled(features.FeatureIronicNetworking) {
		return nil, fmt.Errorf("%s feature gate must be enabled to create HostNetworkAttachment resources", features.FeatureIronicNetworking)
	}
	return nil, kerrors.NewAggregate(webhook.validateAttachment(attachment))
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type.
func (webhook *HostNetworkAttachment) ValidateUpdate(_ context.Context, oldAttachment, newAttachment *metal3api.HostNetworkAttachment) (admission.Warnings, error) {
	hostnetworkattachmentlog.Info("validate update", "namespace", newAttachment.Namespace, "name", newAttachment.Name)
	if equality.Semantic.DeepEqual(oldAttachment.Spec, newAttachment.Spec) {
		return nil, nil
	}

	return nil, kerrors.NewAggregate(webhook.validateAttachment(newAttachment))
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type.
func (webhook *HostNetworkAttachment) ValidateDelete(_ context.Context, attachment *metal3api.HostNetworkAttachment) (admission.Warnings, error) {
	hostnetworkattachmentlog.Info("validate delete", "namespace", attachment.Namespace, "name", attachment.Name)
	return nil, nil
}
