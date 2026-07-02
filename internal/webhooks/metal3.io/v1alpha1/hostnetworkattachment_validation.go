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
	"errors"
	"fmt"
	"reflect"
	"strings"

	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime/schema"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// validateAttachment validates HostNetworkAttachment resource for creation.
func (webhook *HostNetworkAttachment) validateAttachment(attachment *metal3api.HostNetworkAttachment) []error {
	var errs []error

	// Validate switchport mode and VLAN configuration
	if err := validateSwitchportConfiguration(attachment); err != nil {
		errs = append(errs, err)
	}

	return errs
}

// validateUpdate handles update validation including conditional immutability.
func (webhook *HostNetworkAttachment) validateUpdate(ctx context.Context, oldAttachment, newAttachment *metal3api.HostNetworkAttachment) (admission.Warnings, error) {
	var warnings admission.Warnings
	var errs []error

	// First validate the new attachment configuration
	if validationErrs := webhook.validateAttachment(newAttachment); len(validationErrs) > 0 {
		errs = append(errs, validationErrs...)
	}

	// Check if spec has changed
	if reflect.DeepEqual(oldAttachment.Spec, newAttachment.Spec) {
		// No spec changes, allow the update (probably just metadata or status)
		return warnings, nil
	}

	// Spec has changed - check if any BMH references this attachment
	// Fail-closed: if we cannot verify references, reject the update
	references, err := webhook.findBMHReferences(ctx, oldAttachment)
	if err != nil {
		// Unable to verify if attachment is in use - reject update for safety
		return warnings, fmt.Errorf("failed to check BMH references, cannot safely allow update: %w", err)
	}

	if len(references) > 0 {
		// Only enforce immutability if there are active references
		warnings = append(warnings, fmt.Sprintf("Cannot modify attachment while in use by: %s. Remove references first or create a new attachment.",
			strings.Join(references, ", ")))
		errs = append(errs, fmt.Errorf("HostNetworkAttachment spec is immutable while referenced by BMH interfaces: %s",
			strings.Join(references, ", ")))
	} else {
		hostnetworkattachmentlog.V(1).Info("no BMH references found, allowing update",
			"namespace", newAttachment.Namespace, "name", newAttachment.Name)
	}

	if len(errs) > 0 {
		return warnings, kerrors.NewAggregate(errs)
	}

	return warnings, nil
}

// validateDelete handles delete validation.
func (webhook *HostNetworkAttachment) validateDelete(ctx context.Context, attachment *metal3api.HostNetworkAttachment) (admission.Warnings, error) {
	var warnings admission.Warnings

	// Check if any BMH still references this attachment
	references, err := webhook.findBMHReferences(ctx, attachment)
	if err != nil {
		return warnings, fmt.Errorf("failed to check BMH references: %w", err)
	}

	if len(references) > 0 {
		warnings = append(warnings, "This attachment is referenced by: "+strings.Join(references, ", "))
		return warnings, k8serrors.NewForbidden(
			schema.GroupResource{Group: "metal3.io", Resource: "hostnetworkattachments"},
			attachment.Name,
			fmt.Errorf("cannot delete attachment while referenced by BMH interfaces: %s",
				strings.Join(references, ", ")))
	}

	return warnings, nil
}

// findBMHReferences finds all BMH instances that reference this attachment.
// Uses a field indexer for efficient lookups (O(k) vs O(n) where k << n).
func (webhook *HostNetworkAttachment) findBMHReferences(ctx context.Context, attachment *metal3api.HostNetworkAttachment) ([]string, error) {
	bmhList := &metal3api.BareMetalHostList{}

	// Use indexed field lookup for efficient querying
	// The index key format is "namespace/name" to support cross-namespace references
	indexKey := fmt.Sprintf("%s/%s", attachment.Namespace, attachment.Name)

	listOpts := &client.ListOptions{
		FieldSelector: fields.OneTermEqualSelector(bmhNetworkAttachmentIndexField, indexKey),
	}

	if err := webhook.Client.List(ctx, bmhList, listOpts); err != nil {
		return nil, fmt.Errorf("failed to list BMHs using index: %w", err)
	}

	var references []string
	for _, bmh := range bmhList.Items {
		for _, netIf := range bmh.Spec.NetworkInterfaces {
			refNS := netIf.HostNetworkAttachment.Namespace
			if refNS == "" {
				refNS = bmh.Namespace
			}
			if netIf.HostNetworkAttachment.Name == attachment.Name && refNS == attachment.Namespace {
				ifID := netIf.Name
				if ifID == "" {
					ifID = netIf.MACAddress
				}
				references = append(references, fmt.Sprintf("%s/%s[%s]", bmh.Namespace, bmh.Name, ifID))
			}
		}
	}

	return references, nil
}

// validateSwitchportConfiguration validates mode-specific switchport constraints
// that cannot be expressed as CRD schema markers (cross-field validation).
// VLAN range and mode enum validation are handled by CRD markers on the type.
func validateSwitchportConfiguration(attachment *metal3api.HostNetworkAttachment) error {
	if attachment.Spec.Mode == metal3api.SwitchportModeAccess && len(attachment.Spec.AllowedVLANs) > 0 {
		return errors.New("allowedVLANs cannot be specified for access mode")
	}
	return nil
}
