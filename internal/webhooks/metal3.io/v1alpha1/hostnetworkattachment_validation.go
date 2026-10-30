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
	"errors"
	"fmt"
	"strconv"
	"strings"

	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
)

// validateAttachment validates HostNetworkAttachment resource for creation.
func (webhook *HostNetworkAttachment) validateAttachment(attachment *metal3api.HostNetworkAttachment) []error {
	var errs []error

	// Validate switchport mode and VLAN configuration
	if err := validateSwitchportConfiguration(attachment); err != nil {
		errs = append(errs, err)
	}

	// Validate VLAN IDs and ranges
	errs = append(errs, validateAllowedVLANs(attachment)...)

	return errs
}

// validateSwitchportConfiguration validates mode-specific switchport constraints
// that cannot be expressed as CRD schema markers (cross-field validation).
func validateSwitchportConfiguration(attachment *metal3api.HostNetworkAttachment) error {
	if attachment.Spec.Mode == metal3api.SwitchportModeAccess && len(attachment.Spec.AllowedVLANs) > 0 {
		return errors.New("allowedVLANs cannot be specified for access mode")
	}
	return nil
}

// validateAllowedVLANs validates each entry in AllowedVLANs is a valid VLAN ID
// or VLAN range.  Each entry must be either a single integer (1-4094) or a
// "start-end" range where start < end and both are in 1-4094.
func validateAllowedVLANs(attachment *metal3api.HostNetworkAttachment) []error {
	var errs []error
	for i, entry := range attachment.Spec.AllowedVLANs {
		if err := parseVLANEntry(entry); err != nil {
			errs = append(errs, fmt.Errorf("allowedVLANs[%d] %q: %w", i, entry, err))
		}
	}
	return errs
}

func parseVLANEntry(entry string) error {
	before, after, hasRange := strings.Cut(entry, "-")
	start, err := strconv.Atoi(before)
	if err != nil {
		return fmt.Errorf("invalid VLAN ID: %w", err)
	}
	if start < 1 || start > 4094 {
		return fmt.Errorf("VLAN ID %d out of range 1-4094", start)
	}

	if !hasRange {
		return nil
	}

	end, err := strconv.Atoi(after)
	if err != nil {
		return fmt.Errorf("invalid range end: %w", err)
	}
	if end < 1 || end > 4094 {
		return fmt.Errorf("VLAN ID %d out of range 1-4094", end)
	}
	if start >= end {
		return fmt.Errorf("range start (%d) must be less than end (%d)", start, end)
	}

	return nil
}
