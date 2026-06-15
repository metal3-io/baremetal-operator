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
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// log is for logging in this package.
var hostnetworkattachmentlog = logf.Log.WithName("webhooks").WithName("HostNetworkAttachment")

// bmhNetworkAttachmentIndexField is the field index name for BMH -> HostNetworkAttachment references.
const bmhNetworkAttachmentIndexField = ".spec.networkInterfaces.hostNetworkAttachment.name"

func (webhook *HostNetworkAttachment) SetupWebhookWithManager(ctx context.Context, mgr ctrl.Manager) error {
	webhook.Client = mgr.GetClient()

	// Register field indexer for efficient BMH reference lookups
	// This allows us to quickly find all BMHs that reference a specific HostNetworkAttachment
	if err := mgr.GetFieldIndexer().IndexField(
		ctx,
		&metal3api.BareMetalHost{},
		bmhNetworkAttachmentIndexField,
		func(obj client.Object) []string {
			bmh, ok := obj.(*metal3api.BareMetalHost)
			if !ok {
				return nil
			}
			var attachments []string
			for _, iface := range bmh.Spec.NetworkInterfaces {
				if iface.HostNetworkAttachment.Name != "" {
					// Include namespace in index key for cross-namespace support
					ns := iface.HostNetworkAttachment.Namespace
					if ns == "" {
						ns = bmh.Namespace
					}
					key := fmt.Sprintf("%s/%s", ns, iface.HostNetworkAttachment.Name)
					attachments = append(attachments, key)
				}
			}
			return attachments
		},
	); err != nil {
		return err
	}

	return ctrl.NewWebhookManagedBy(mgr, &metal3api.HostNetworkAttachment{}).
		WithValidator(webhook).
		Complete()
}

//+kubebuilder:webhook:verbs=create;update;delete,path=/validate-metal3-io-v1alpha1-hostnetworkattachment,mutating=false,failurePolicy=fail,sideEffects=none,admissionReviewVersions=v1,groups=metal3.io,resources=hostnetworkattachments,versions=v1alpha1,name=hostnetworkattachment.metal3.io

// HostNetworkAttachment implements a validation webhook for HostNetworkAttachment.
type HostNetworkAttachment struct {
	Client client.Client
}

var _ admission.Validator[*metal3api.HostNetworkAttachment] = &HostNetworkAttachment{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type.
func (webhook *HostNetworkAttachment) ValidateCreate(_ context.Context, attachment *metal3api.HostNetworkAttachment) (admission.Warnings, error) {
	hostnetworkattachmentlog.Info("validate create", "namespace", attachment.Namespace, "name", attachment.Name)
	return nil, kerrors.NewAggregate(webhook.validateAttachment(attachment))
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type.
func (webhook *HostNetworkAttachment) ValidateUpdate(ctx context.Context, oldAttachment, newAttachment *metal3api.HostNetworkAttachment) (admission.Warnings, error) {
	hostnetworkattachmentlog.Info("validate update", "namespace", newAttachment.Namespace, "name", newAttachment.Name)
	return webhook.validateUpdate(ctx, oldAttachment, newAttachment)
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type.
func (webhook *HostNetworkAttachment) ValidateDelete(ctx context.Context, attachment *metal3api.HostNetworkAttachment) (admission.Warnings, error) {
	hostnetworkattachmentlog.Info("validate delete", "namespace", attachment.Namespace, "name", attachment.Name)
	return webhook.validateDelete(ctx, attachment)
}
