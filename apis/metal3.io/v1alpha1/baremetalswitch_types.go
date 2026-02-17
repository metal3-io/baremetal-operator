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

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// BareMetalSwitchSpec defines the desired state of BareMetalSwitch.
type BareMetalSwitchSpec struct {
	// Address is the network address of the switch (IP address or hostname).
	// +kubebuilder:validation:Required
	Address string `json:"address"`

	// MACAddress is the MAC address of the switch management interface.
	// Used to correlate LLDP information from nodes to identify which switch they're connected to.
	// +kubebuilder:validation:Pattern=`^[0-9a-fA-F]{2}(:[0-9a-fA-F]{2}){5}$`
	// +kubebuilder:validation:Required
	MACAddress string `json:"macAddress"`

	// Driver specifies the driver to use. Currently only "generic-switch" is supported.
	// +kubebuilder:validation:Enum=generic-switch
	// +kubebuilder:default=generic-switch
	// +optional
	Driver string `json:"driver,omitempty"`

	// DeviceType specifies the device type for the generic-switch driver.
	// Must be one of the device types supported by networking-generic-switch.
	// Examples: netmiko_cisco_ios, netmiko_dell_force10, netmiko_dell_os10,
	//   netmiko_juniper_junos, netmiko_arista_eos
	// See https://docs.openstack.org/networking-generic-switch/latest/configuration.html
	// +kubebuilder:validation:Required
	DeviceType string `json:"deviceType"`

	// The secret containing the switch credentials (requires key "username"
	// and either "password" or "ssh-privatekey"). An optional "admin-password"
	// key can be provided for privilege escalation.
	Credentials *corev1.SecretReference `json:"credentials,omitempty"`

	// Port specifies the management port to connect to (e.g., SSH port 22, HTTPS port 443).
	// If not specified, the driver will use its default port based on the device type.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	// +optional
	Port *int32 `json:"port,omitempty"`

	// DisableCertificateVerification disables TLS certificate verification when using
	// HTTPS to connect to the switch. This is required for self-signed certificates,
	// but is insecure as it allows man-in-the-middle attacks.
	// +optional
	DisableCertificateVerification bool `json:"disableCertificateVerification,omitempty"`
}

// SwitchConditionType defines the condition types for BareMetalSwitch.
type SwitchConditionType string

const (
	// SwitchConditionValid indicates whether the switch has been
	// successfully reconciled into the switch config secret.
	SwitchConditionValid SwitchConditionType = "Valid"
)

// BareMetalSwitchStatus defines the observed state of BareMetalSwitch.
type BareMetalSwitchStatus struct {
	// Conditions describes the state of the BareMetalSwitch resource.
	// +patchMergeKey=type
	// +patchStrategy=merge
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Namespaced,shortName=bms
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Driver",type="string",JSONPath=".spec.driver",description="Switch driver type"
// +kubebuilder:printcolumn:name="Device Type",type="string",JSONPath=".spec.deviceType",description="Switch device type"
// +kubebuilder:printcolumn:name="Address",type="string",JSONPath=".spec.address",description="Switch address"
// +kubebuilder:printcolumn:name="Valid",type="string",JSONPath=".status.conditions[?(@.type==\"Valid\")].status",description="Valid"

// BareMetalSwitch represents a Top-of-Rack switch managed by Ironic Networking.
type BareMetalSwitch struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BareMetalSwitchSpec   `json:"spec,omitempty"`
	Status BareMetalSwitchStatus `json:"status,omitempty"`
}

// GetConditions returns the set of conditions for this object.
func (s *BareMetalSwitch) GetConditions() []metav1.Condition {
	return s.Status.Conditions
}

// SetConditions sets conditions for this object.
func (s *BareMetalSwitch) SetConditions(conditions []metav1.Condition) {
	s.Status.Conditions = conditions
}

// +kubebuilder:object:root=true

// BareMetalSwitchList contains a list of BareMetalSwitch.
type BareMetalSwitchList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BareMetalSwitch `json:"items"`
}

func init() {
	SchemeBuilder.Register(&BareMetalSwitch{}, &BareMetalSwitchList{})
}
