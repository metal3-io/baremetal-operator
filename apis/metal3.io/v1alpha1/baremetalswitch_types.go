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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SwitchCredentialType defines the type of credential used for switch authentication.
type SwitchCredentialType string

const (
	// SwitchCredentialTypePassword indicates password-based authentication
	SwitchCredentialTypePassword SwitchCredentialType = "password"
	// SwitchCredentialTypePublicKey indicates SSH public key-based authentication
	SwitchCredentialTypePublicKey SwitchCredentialType = "publickey"
)

// SwitchCredentials defines the credentials used to access the switch.
type SwitchCredentials struct {
	// Type is the type of switch credentials.
	// This is currently limited to "password", but will be expanded to others.
	// +kubebuilder:validation:Enum=password;publickey
	// +kubebuilder:default=password
	Type SwitchCredentialType `json:"type"`

	// The name of the secret containing the switch credentials.
	// For password authentication, requires keys "username" and "password"
	// For SSH key authentication, requires keys "username" and "ssh-privatekey".
	// In both cases, an optional "enable-secret" key can be provided if needed
	// to enable privileged mode.
	// +kubebuilder:validation:MinLength=1
	SecretName string `json:"secretName"`
}

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
	// See https://github.com/openstack/networking-generic-switch/blob/master/setup.cfg
	// +kubebuilder:validation:Required
	DeviceType string `json:"deviceType"`

	// Credentials references the secret containing switch authentication credentials.
	// +kubebuilder:validation:Required
	Credentials SwitchCredentials `json:"credentials"`

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
	DisableCertificateVerification *bool `json:"disableCertificateVerification,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Namespaced,shortName=bms
// +kubebuilder:printcolumn:name="Driver",type="string",JSONPath=".spec.driver",description="Switch driver type"
// +kubebuilder:printcolumn:name="Device Type",type="string",JSONPath=".spec.deviceType",description="Switch device type"
// +kubebuilder:printcolumn:name="Address",type="string",JSONPath=".spec.address",description="Switch address"
// +kubebuilder:printcolumn:name="Credential Type",type="string",JSONPath=".spec.credentials.type",description="Credential Type"

// BareMetalSwitch represents a Top-of-Rack switch managed by Ironic Networking.
type BareMetalSwitch struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec BareMetalSwitchSpec `json:"spec,omitempty"`
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
