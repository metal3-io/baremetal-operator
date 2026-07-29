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

package controllers

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// hardwareDetailsFromInfo extracts HardwareDetails, preferring the HardwareData
// resource and falling back to host.Status.HardwareDetails for hosts where
// details were supplied via annotation (inspection disabled / external inspection).
func hardwareDetailsFromInfo(info *reconcileInfo) *metal3api.HardwareDetails {
	if info.hardwareData != nil && info.hardwareData.Spec.HardwareDetails != nil {
		return info.hardwareData.Spec.HardwareDetails
	}
	return info.host.Status.HardwareDetails
}

// nicsFromInfo extracts the NIC list from HardwareData in reconcileInfo.
func nicsFromInfo(info *reconcileInfo) []metal3api.NIC {
	if hwDetails := hardwareDetailsFromInfo(info); hwDetails != nil {
		return hwDetails.NIC
	}
	return nil
}

// isHNANamespaceAllowed checks whether the given namespace is allowed for
// HostNetworkAttachment references. When AllowedHNANamespaces is empty, all
// namespaces are permitted.
func (r *BareMetalHostReconciler) isHNANamespaceAllowed(namespace string) bool {
	if len(r.AllowedHNANamespaces) == 0 {
		return true
	}
	for _, ns := range r.AllowedHNANamespaces {
		if ns == namespace {
			return true
		}
	}
	return false
}

// validateNetworkInterfaces validates that networkInterfaces correspond to actual NICs
// and that referenced HostNetworkAttachments exist. The hwDetails parameter
// provides the inspected NIC list from HardwareData.
func (r *BareMetalHostReconciler) validateNetworkInterfaces(ctx context.Context, host *metal3api.BareMetalHost, hwDetails *metal3api.HardwareDetails) (bool, error) {
	// Skip validation if no network interfaces specified
	if len(host.Spec.NetworkInterfaces) == 0 {
		return r.clearNetworkInterfaceValidation(host)
	}

	// Skip validation if hardware inspection not complete
	if hwDetails == nil {
		return r.setNetworkInterfaceValidation(host, metav1.ConditionFalse,
			"HardwareInspectionIncomplete",
			"Waiting for hardware inspection to complete before validating network interfaces")
	}

	// Now we can safely validate since hardware details are available
	return r.performNetworkInterfaceValidation(ctx, host, hwDetails.NIC)
}

// performNetworkInterfaceValidation validates network interfaces against discovered hardware
// and checks that referenced HostNetworkAttachments exist.
func (r *BareMetalHostReconciler) performNetworkInterfaceValidation(ctx context.Context, host *metal3api.BareMetalHost, nics []metal3api.NIC) (bool, error) {
	// Build map of available NIC identifiers from hardware details
	availableNICs := make(map[string]bool)
	for _, nic := range nics {
		if nic.Name != "" {
			availableNICs[nic.Name] = true
		}
		if nic.MAC != "" {
			availableNICs[strings.ToLower(nic.MAC)] = true
		}
	}

	// Validate each specified network interface
	var invalidInterfaces []string
	var missingAttachments []string
	var deniedNamespaces []string

	for _, netIf := range host.Spec.NetworkInterfaces {
		key := netIf.GetKey()
		if !availableNICs[key] {
			invalidInterfaces = append(invalidInterfaces, key)
		}

		// Check that referenced HostNetworkAttachment exists
		if netIf.HostNetworkAttachment.Name != "" {
			attachmentNS := netIf.HostNetworkAttachment.Namespace
			if attachmentNS == "" {
				attachmentNS = host.Namespace
			}

			if !r.isHNANamespaceAllowed(attachmentNS) {
				deniedNamespaces = append(deniedNamespaces,
					fmt.Sprintf("%s/%s (interface %s)", attachmentNS, netIf.HostNetworkAttachment.Name, key))
				continue
			}

			attachment := &metal3api.HostNetworkAttachment{}
			err := r.Get(ctx, types.NamespacedName{
				Name:      netIf.HostNetworkAttachment.Name,
				Namespace: attachmentNS,
			}, attachment)
			if err != nil {
				if k8serrors.IsNotFound(err) {
					missingAttachments = append(missingAttachments,
						fmt.Sprintf("%s/%s (interface %s)", attachmentNS, netIf.HostNetworkAttachment.Name, key))
				} else {
					return false, fmt.Errorf("failed to check attachment %s/%s: %w",
						attachmentNS, netIf.HostNetworkAttachment.Name, err)
				}
			}
		}
	}

	// Update validation status based on results
	if len(invalidInterfaces) > 0 {
		reason := "InvalidInterfaceNames"
		availableNames := r.getAvailableNICNames(nics)
		var message string
		if len(availableNames) == 0 {
			message = fmt.Sprintf("Invalid interface names: %s. No such network interfaces.",
				strings.Join(invalidInterfaces, ", "))
		} else {
			message = fmt.Sprintf("Invalid interface names: %s. Available interfaces: %s",
				strings.Join(invalidInterfaces, ", "),
				strings.Join(availableNames, ", "))
		}
		return r.setNetworkInterfaceValidation(host, metav1.ConditionFalse, reason, message)
	}

	if len(deniedNamespaces) > 0 {
		reason := "AttachmentNamespaceDenied"
		message := "HostNetworkAttachment namespace not allowed: " +
			strings.Join(deniedNamespaces, ", ")
		return r.setNetworkInterfaceValidation(host, metav1.ConditionFalse, reason, message)
	}

	if len(missingAttachments) > 0 {
		reason := "AttachmentNotFound"
		message := "HostNetworkAttachment not found: " +
			strings.Join(missingAttachments, ", ")
		return r.setNetworkInterfaceValidation(host, metav1.ConditionFalse, reason, message)
	}

	reason := "AllInterfacesValid"
	message := "All network interfaces and attachments are valid"
	return r.setNetworkInterfaceValidation(host, metav1.ConditionTrue, reason, message)
}

// buildAppliedPortConfigs builds the applied status from the resolved
// configs map and the host's network interfaces.
func buildAppliedPortConfigs(info *reconcileInfo) []metal3api.AppliedPortConfig {
	macToName := buildNICMacToNameMap(nicsFromInfo(info))
	applied := make([]metal3api.AppliedPortConfig, 0, len(info.portConfigs))

	for mac, config := range info.portConfigs {
		name := macToName[mac]
		if name == "" {
			name = mac
		}
		appliedPortConfig := metal3api.AppliedPortConfig{
			Name: name,
			SwitchPortConfig: metal3api.SwitchPortConfig{
				Mode:         config.SwitchPortConfig.Mode,
				NativeVLAN:   config.SwitchPortConfig.NativeVLAN,
				AllowedVLANs: config.SwitchPortConfig.AllowedVLANs,
				MTU:          config.SwitchPortConfig.MTU,
			},
		}
		if config.SwitchPortIdentifier != nil {
			appliedPortConfig.LocalLinkConnection = config.SwitchPortIdentifier
		}
		applied = append(applied, appliedPortConfig)
	}

	if len(applied) == 0 {
		return nil
	}

	sort.Slice(applied, func(i, j int) bool {
		return applied[i].Name < applied[j].Name
	})

	return applied
}

// setNetworkInterfaceValidation updates the network interface validation condition.
func (r *BareMetalHostReconciler) setNetworkInterfaceValidation(host *metal3api.BareMetalHost, status metav1.ConditionStatus, reason, message string) (bool, error) {
	existing := meta.FindStatusCondition(host.Status.Conditions, metal3api.NetworkInterfacesValidCondition)
	if existing != nil && existing.Status == status && existing.Reason == reason && existing.Message == message {
		return false, nil
	}

	meta.SetStatusCondition(&host.Status.Conditions, metav1.Condition{
		Type:    metal3api.NetworkInterfacesValidCondition,
		Status:  status,
		Reason:  reason,
		Message: message,
	})
	return true, nil
}

// clearNetworkInterfaceValidation removes the network interface validation condition.
func (r *BareMetalHostReconciler) clearNetworkInterfaceValidation(host *metal3api.BareMetalHost) (bool, error) {
	if meta.FindStatusCondition(host.Status.Conditions, metal3api.NetworkInterfacesValidCondition) != nil {
		meta.RemoveStatusCondition(&host.Status.Conditions, metal3api.NetworkInterfacesValidCondition)
		return true, nil
	}
	return false, nil
}

// getAvailableNICNames returns a sorted list of available NIC names.
func (r *BareMetalHostReconciler) getAvailableNICNames(nics []metal3api.NIC) []string {
	names := make([]string, 0)
	for _, nic := range nics {
		if nic.Name != "" {
			names = append(names, nic.Name)
		}
	}
	sort.Strings(names)
	return names
}

// buildNICNameToMACMap builds a map from NIC name to MAC address.
func buildNICNameToMACMap(nics []metal3api.NIC) map[string]string {
	nameToMAC := make(map[string]string)
	for _, nic := range nics {
		if nic.Name != "" && nic.MAC != "" {
			nameToMAC[nic.Name] = strings.ToLower(nic.MAC)
		}
	}
	return nameToMAC
}

// buildNICMacToNameMap builds a map from NIC MAC address to name.
func buildNICMacToNameMap(nics []metal3api.NIC) map[string]string {
	macToName := make(map[string]string)
	for _, nic := range nics {
		if nic.Name != "" && nic.MAC != "" {
			macToName[strings.ToLower(nic.MAC)] = nic.Name
		}
	}
	return macToName
}

// expandVLANRanges converts a list of VLAN entries (single IDs like "100" or
// ranges like "100-200") into a flat sorted list of individual VLAN IDs.
// Validation has already been performed by the webhook, so parsing errors here
// indicate a bug.
func expandVLANRanges(entries []string) ([]int, error) {
	if len(entries) == 0 {
		return nil, nil
	}
	var result []int
	for _, entry := range entries {
		before, after, hasRange := strings.Cut(entry, "-")
		start, err := strconv.Atoi(before)
		if err != nil {
			return nil, fmt.Errorf("invalid VLAN entry %q: %w", entry, err)
		}
		if !hasRange {
			if start < 1 || start > 4094 {
				return nil, fmt.Errorf("invalid VLAN ID %q: must be within 1-4094", entry)
			}
			result = append(result, start)
			continue
		}
		end, err := strconv.Atoi(after)
		if err != nil {
			return nil, fmt.Errorf("invalid VLAN range %q: %w", entry, err)
		}
		if start < 1 || end > 4094 || end < start {
			return nil, fmt.Errorf("invalid VLAN range %q: must be within 1-4094 and ascending", entry)
		}
		for v := start; v <= end; v++ {
			result = append(result, v)
		}
	}
	sort.Ints(result)
	return result, nil
}

func (r *BareMetalHostReconciler) resolveSwitchPortConfig(ctx context.Context, namespace string, netIf *metal3api.NetworkInterface, config *metal3api.SwitchPortConfig) (bool, error) {
	if netIf.HostNetworkAttachment.Name == "" {
		// This should get caught earlier in validation so should never get here
		return true, nil
	}

	attachmentNS := netIf.HostNetworkAttachment.Namespace
	if attachmentNS == "" {
		attachmentNS = namespace
	}

	if !r.isHNANamespaceAllowed(attachmentNS) {
		r.Log.Info("network attachment namespace not allowed, skipping interface",
			"interface", netIf.GetKey(),
			"namespace", attachmentNS)
		return true, nil
	}

	attachment := &metal3api.HostNetworkAttachment{}
	err := r.Get(ctx, types.NamespacedName{
		Name:      netIf.HostNetworkAttachment.Name,
		Namespace: attachmentNS,
	}, attachment)

	if err != nil {
		if k8serrors.IsNotFound(err) {
			r.Log.Info("network attachment not found, skipping interface",
				"interface", netIf.GetKey(),
				"attachment", fmt.Sprintf("%s/%s", attachmentNS, netIf.HostNetworkAttachment.Name))
			return true, nil
		}

		return false, fmt.Errorf("failed to get network attachment %s/%s: %w",
			attachmentNS, netIf.HostNetworkAttachment.Name, err)
	}

	config.Mode = attachment.Spec.Mode
	config.NativeVLAN = attachment.Spec.NativeVLAN
	config.MTU = attachment.Spec.MTU

	allowedVLANs, err := expandVLANRanges(attachment.Spec.AllowedVLANs)
	if err != nil {
		return false, fmt.Errorf("failed to expand allowed VLANs for attachment %s/%s: %w",
			attachmentNS, netIf.HostNetworkAttachment.Name, err)
	}
	config.AllowedVLANs = allowedVLANs

	return false, nil
}

func (r *BareMetalHostReconciler) resolveLocalLinkConnectionConfig(netIf *metal3api.NetworkInterface, config *provisioner.PortConfig) {
	if netIf.SwitchPort != nil {
		config.SwitchPortIdentifier = netIf.SwitchPort.DeepCopy()
	}
}

// resolvePortConfigs resolves network attachments for the given host into a set
// of port configurations keyed by MAC address.
func (r *BareMetalHostReconciler) resolvePortConfigs(ctx context.Context, host *metal3api.BareMetalHost, nics []metal3api.NIC) (map[string]*provisioner.PortConfig, error) {
	configs := make(map[string]*provisioner.PortConfig)
	nameToMAC := buildNICNameToMACMap(nics)

	for _, netIf := range host.Spec.NetworkInterfaces {
		// Resolve MAC address key for this interface
		var macKey string
		if netIf.MACAddress != "" {
			macKey = strings.ToLower(netIf.MACAddress)
		} else if mac, ok := nameToMAC[netIf.Name]; ok {
			macKey = mac
		} else {
			r.Log.Info("cannot resolve interface name to MAC address, skipping",
				"interface", netIf.Name)
			continue
		}

		config := &provisioner.PortConfig{}
		if skip, err := r.resolveSwitchPortConfig(ctx, host.Namespace, &netIf, &config.SwitchPortConfig); err != nil {
			return nil, err
		} else if skip {
			continue
		}

		// Populate manual switch port identity if provided.
		// This overrides LLDP-discovered data in the provisioner.
		r.resolveLocalLinkConnectionConfig(&netIf, config)

		configs[macKey] = config
	}

	return configs, nil
}
