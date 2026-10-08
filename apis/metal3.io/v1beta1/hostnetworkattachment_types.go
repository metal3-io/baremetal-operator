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

package v1beta1

// HostNetworkAttachmentRef references a HostNetworkAttachment for interface configuration.
type HostNetworkAttachmentRef struct {
	// Name of the HostNetworkAttachment resource
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	Name string `json:"name"`

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

// v1beta1 only contains the reference type for HostNetworkAttachment and the
// switchport mode enumeration. It does not define the full HostNetworkAttachment
// resource or its spec. Check the v1alpha1 version for the complete definition.
