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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// HostNetworkAttachmentRef references a HostNetworkAttachment for interface configuration.
type HostNetworkAttachmentRef struct {
	// Name of the HostNetworkAttachment resource
	// +kubebuilder:validation:MaxLength=253
	Name string `json:"name,omitempty"`

	// Namespace of the HostNetworkAttachment (defaults to BMH namespace)
	// +optional
	// +kubebuilder:validation:MaxLength=63
	Namespace string `json:"namespace,omitempty"`
}

// SwitchPortMode defines the switchport mode for network interfaces.
// +kubebuilder:validation:Enum=access;trunk;hybrid
type SwitchPortMode string

const (
	// SwitchportModeAccess sets the interface to access mode (single VLAN).
	SwitchportModeAccess SwitchPortMode = "access"
	// SwitchportModeTrunk sets the interface to trunk mode (multiple VLANs).
	SwitchportModeTrunk SwitchPortMode = "trunk"
	// SwitchportModeHybrid sets the interface to hybrid mode (access + trunk).
	SwitchportModeHybrid SwitchPortMode = "hybrid"
)

// HostNetworkAttachmentSpec defines the desired switchport configuration.
type HostNetworkAttachmentSpec struct {
	// Mode specifies the network attachment mode.
	// +kubebuilder:validation:Required
	// +kubebuilder:default=access
	Mode SwitchPortMode `json:"mode"`

	// NativeVLAN specifies the native VLAN ID for the network attachment.
	// This is the untagged VLAN used on the interface.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=4094
	NativeVLAN int `json:"nativeVLAN"`

	// AllowedVLANs specifies a list of VLAN IDs that are allowed on this network attachment.
	// This is typically used in trunk or hybrid mode to specify which tagged VLANs can be carried on the interface.
	// +optional
	// +kubebuilder:validation:items:Minimum=1
	// +kubebuilder:validation:items:Maximum=4094
	AllowedVLANs []int `json:"allowedVLANs,omitempty"`

	// MTU specifies the Maximum Transmission Unit size for the network attachment.
	// If not specified, the default MTU for the underlying network will be used.
	// +optional
	// +kubebuilder:validation:Minimum=68
	// +kubebuilder:validation:Maximum=9216
	MTU *int `json:"mtu,omitempty"`
}

// HostNetworkAttachment defines switchport configuration for BMH network interfaces.
// Spec fields are mutable when no BMH references the attachment, immutable when in use.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Mode",type="string",JSONPath=".spec.mode",description="Switchport mode"
// +kubebuilder:printcolumn:name="Native VLAN",type="integer",JSONPath=".spec.nativeVLAN",description="Native VLAN ID"
// +kubebuilder:printcolumn:name="MTU",type="integer",JSONPath=".spec.mtu",description="Maximum Transmission Unit"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp",description="Time duration since creation"
type HostNetworkAttachment struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec HostNetworkAttachmentSpec `json:"spec,omitempty"`
}

// +kubebuilder:object:root=true

// HostNetworkAttachmentList contains a list of HostNetworkAttachment.
type HostNetworkAttachmentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []HostNetworkAttachment `json:"items"`
}

func init() {
	SchemeBuilder.Register(&HostNetworkAttachment{}, &HostNetworkAttachmentList{})
}
