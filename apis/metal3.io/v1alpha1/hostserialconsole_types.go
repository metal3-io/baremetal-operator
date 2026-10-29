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

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

const HostSerialConsoleFinalizer = "hostserialconsole.metal3.io"

// HostSerialConsoleSpec is intentionally empty. Creating the resource requests
// access; deleting it revokes access. Hardware configuration belongs on BMH.
type HostSerialConsoleSpec struct{}

// SerialConsoleEndpoint records the backend endpoint. It is a checkpoint to
// avoid querying Ironic's console API, which can interrupt an active session.
type SerialConsoleEndpoint struct {
	Host string `json:"host"`
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	Port int32 `json:"port"`
}

// HostSerialConsoleStatus describes the console request and its Service.
type HostSerialConsoleStatus struct {
	// HostDeprovisioningGeneration binds the request to the host's workload
	// lifecycle. A changed generation expires the request even if the
	// deprovisioning state itself was not observed. Nil means not yet bound.
	// +kubebuilder:validation:Minimum=0
	// +optional
	HostDeprovisioningGeneration *int64 `json:"hostDeprovisioningGeneration,omitempty"`
	// ProvisionerID is checkpointed before enabling to allow crash-safe cleanup.
	// +optional
	ProvisionerID string `json:"provisionerID,omitempty"`
	// ConsoleURL is the in-cluster TCP URL of the managed Service.
	// +optional
	ConsoleURL string `json:"consoleURL,omitempty"`
	// ServiceName is the managed Service in this resource's namespace.
	// +optional
	ServiceName string `json:"serviceName,omitempty"`
	// BackendID identifies the provisioner node, conductor and console allocation.
	// +optional
	BackendID string `json:"backendID,omitempty"`
	// BackendEndpoint is controller bookkeeping, not the client-facing endpoint.
	// +optional
	BackendEndpoint *SerialConsoleEndpoint `json:"backendEndpoint,omitempty"`
	// Conditions includes Ready, which indicates whether access is available.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:shortName=hsc
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="URL",type=string,JSONPath=`.status.consoleURL`

// HostSerialConsole requests a shared serial console for the BareMetalHost with
// the same name and namespace. At most one TCP client can use it at a time.
type HostSerialConsole struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              HostSerialConsoleSpec   `json:"spec,omitempty"`
	Status            HostSerialConsoleStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// HostSerialConsoleList contains a list of HostSerialConsole requests.
type HostSerialConsoleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []HostSerialConsole `json:"items"`
}

func init() {
	SchemeBuilder.Register(&HostSerialConsole{}, &HostSerialConsoleList{})
}
