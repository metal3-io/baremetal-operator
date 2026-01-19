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

// HostDeployPolicySpec defines the desired state of HostDeployPolicy.
type HostDeployPolicySpec struct {
	// HostClaimNamespaces constrains the namespaces of the HostClaims allowed
	// to bind the BareMetalHosts in the same namespace as the HostDeployPolicy
	HostClaimNamespaces *HostClaimNamespaces `json:"hostClaimNamespaces,omitempty"`
}

type HostClaimNamespaces struct {
	// Namespaces is a list of namespace names where the hostClaim is authorized to reside in.
	Names []string `json:"names,omitempty"`
	// NameMatches is a string interpreted as a regular expression that must be matched by the
	// namespace of the HostClaim.
	NameMatches string `json:"nameMatches,omitempty"`
	// HasLabels is a list of label names and their associated value.
	// The namespace should have all of those labels. If the value
	// is specified, it must also match.
	HasLabels []NameValuePair `json:"hasLabels,omitempty"`
}

type NameValuePair struct {
	// Name of the expected label.
	Name string `json:"name"`
	// If specified, expected value of the label.
	Value string `json:"value,omitempty"`
}

// HostDeployPolicyStatus defines the observed state of HostDeployPolicy.
type HostDeployPolicyStatus struct {
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// HostDeployPolicy is the Schema for the hostdeploypolicies API.
type HostDeployPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   HostDeployPolicySpec   `json:"spec,omitempty"`
	Status HostDeployPolicyStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// HostDeployPolicyList contains a list of HostDeployPolicy.
type HostDeployPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []HostDeployPolicy `json:"items"`
}

func init() {
	SchemeBuilder.Register(&HostDeployPolicy{}, &HostDeployPolicyList{})
}
