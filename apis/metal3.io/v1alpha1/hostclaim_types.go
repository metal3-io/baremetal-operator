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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/selection"
)

const (
	// Finalizer for HostClaims that are handled by the hostclaim_controller.
	HostClaimFinalizer = "metal3.io/hostclaim"

	// DisassociatedLabel marks a HostClaim that lost its association with its BareMetalHost and is
	// being held for manual review before a new association is performed. Any value blocks reassociation.
	// Remove the label once the situation has been investigated to let reassociation proceed.
	DisassociatedLabel = "hostclaim.metal3.io/disassociated"

	// AssociateCondition documents the status of the association of HostClaim with a BareMetalHost.
	AssociatedCondition = "Associated"

	// BareMetalHostAssociatedReason is the reason used when the HostClaim is successfully associated with a BareMetalHost.
	BareMetalHostAssociatedReason = "BareMetalHostAssociated"
	// MissingBareMetalHostReason is used when the HostClaim was previously
	// associated with a BareMetalHost that has since disappeared or whose
	// ConsumerRef no longer points back to the hostclaim.
	MissingBareMetalHostReason = "MissingBareMetalHost"
	// NoBareMetalHostReason is a reason used when no BareMetalHost matching the constraints is found.
	NoBareMetalHostReason = "NoBareMetalHost"
	// BadBareMetalHostStatusReason is a reason used when the status of the associated BareMetalHost cannot be marshalled.
	BadBareMetalHostStatusReason = "BadBareMetalHostStatus"
	// HostClaimAnnotationNotSetReason is a reason used when the annotation on the hostclaim cannot be set.
	HostClaimAnnotationNotSetReason = "HostClaimAnnotationNotSet"
	// PauseAnnotationSetFailedReason is a reason used when propagating the pause annotation fails.
	PauseAnnotationSetFailedReason = "PauseAnnotationSetFailed"
	// PauseAnnotationRemoveFailedReason is a reason used when the removal of the pause annotation fails.
	PauseAnnotationRemoveFailedReason = "PauseAnnotationRemoveFailed"
	// HostPausedReason is a reason used when Host or Cluster is paused.
	HostPausedReason = "HostPaused"
	// HostClaimDeletingReason is used while the hostclaim is being deleted.
	HostClaimDeletingReason = "HostClaimDeleting"
	// HostClaimDeletionFailedReason is used when the deletion of hostclaim encountered an unexpected error.
	HostClaimDeletionFailedReason = "HostClaimDeletionFailed"
	// UnexpectedDisassociationReason is the AssociatedCondition reason set when a HostClaim's
	// BareMetalHost association was lost outside of normal deletion/retry flow (the BareMetalHost
	// disappeared, or its ConsumerRef no longer points back to this claim). The condition message
	// explains that manual review is required; see DisassociatedLabel.
	UnexpectedDisassociationReason = "UnexpectedDisassociation"

	// SynchronizedCondition documents the status of the transfer of information from the
	// HostClaim to the BareMetalHost.
	SynchronizedCondition = "Synchronized"

	// ConfigurationSyncedReason is the reason used when the synchronization of secrets is successful.
	ConfigurationSyncedReason = "ConfigurationSynced"
	// BadUserDataSecretReason is a reason used when the secret for user data cannot be synchronized.
	BadUserDataSecretReason = "UserDataSecretSyncFailure"
	// BadMetaDataSecretReason is a reason used when the secret for meta data cannot be synchronized.
	BadMetaDataSecretReason = "MetaDataSecretSyncFailure"
	// BadNetworkDataSecretReason is a reason used when the secret for meta data cannot be synchronized.
	BadNetworkDataSecretReason = "NetworkDataSecretSyncFailure"
	// BareMetalHostNotSynchronizedReason is the reason used when the synchronization of BareMetalHost state
	// is not successful.
	BareMetalHostNotSynchronizedReason = "BareMetalHostNotSynchronized"

	// HostReadyCondition is a mirror of the associated Host Ready condition.
	HostReadyCondition = "HostReady"
)

// HostClaimSpec defines the desired state of HostClaim.
type HostClaimSpec struct {
	// poweredOn indicates if the compute resource should be powered on. Changing this value will trigger
	// a change in power state of the targeted host.
	PoweredOn bool `json:"poweredOn"`

	// image holds the details of the image to be provisioned. Populating
	// the image will cause the target host to start provisioning.
	// +optional
	Image *Image `json:"image,omitempty"`

	// userData holds the reference to the Secret containing the user data
	// which is passed to the Config Drive and interpreted by the
	// first-boot software such as cloud-init. The format of user data is
	// specific to the first-boot software.
	// +optional
	UserData *corev1.SecretReference `json:"userData,omitempty"`

	// networkData holds the reference to the Secret containing network
	// configuration which is passed to the Config Drive and interpreted
	// by the first boot software such as cloud-init.
	// +optional
	NetworkData *corev1.SecretReference `json:"networkData,omitempty"`

	// metaData holds the reference to the Secret containing host metadata
	// (e.g. meta_data.json) which is passed to the Config Drive.
	// +optional
	MetaData *corev1.SecretReference `json:"metaData,omitempty"`

	// customDeploy defines a custom deploy procedure. This is an advanced feature that allows
	// using a custom deploy step provided by a site-specific deployment
	// ramdisk. Most users will want to use "image" instead. Setting this
	// field triggers provisioning.
	// +optional
	CustomDeploy *CustomDeploy `json:"customDeploy,omitempty"`

	// hostSelector specifies matching criteria for labels on BareMetalHosts.
	// This is used to limit the set of BareMetalHost objects considered for
	// claiming for a HostClaim.
	// +optional
	HostSelector HostSelector `json:"hostSelector,omitempty"`

	// consumerRef can be used to store information about something
	// that is using a host. When it is not empty, the host is
	// considered "in use". The common use case is a link to a Machine
	// resource when the host is used by Cluster API.
	// +optional
	ConsumerRef *corev1.ObjectReference `json:"consumerRef,omitempty"`

	// failureDomain is the failure domain unique identifier this HostClaim
	// should be attached to, as defined in Cluster API. It is implemented
	// when set as a preference for binding BareMetalHost having the label
	// infrastructure.cluster.x-k8s.io/failure-domain set to the value of
	// the field.
	// +optional
	FailureDomain string `json:"failureDomain,omitempty"`

	// disassociationPolicy controls how a HostClaim reacts to losing its association with
	// its BareMetalHost. When absent, or set to manual, it
	// sets hostclaim.metal3.io/disassociated and waits for manual review. If set to
	// auto-recover, the HostClaim automatically reassociates.
	// +optional
	// +kubebuilder:validation:Enum:=manual;auto-recover
	DisassociationPolicy DisassociationPolicy `json:"disassociationPolicy,omitempty"`
}

type DisassociationPolicy string

const (
	// DisassociationPolicyAutoRecover is used to opt a
	// HostClaim out of manual-review fencing and performs automatic reassociation.
	DisassociationPolicyAutoRecover DisassociationPolicy = "auto-recover"
	// DisassociationPolicyManual is the default disassociation policy.
	// The HostClaim will not reassociate until the label is removed.
	DisassociationPolicyManual DisassociationPolicy = "manual"
)

// HostSelector specifies matching criteria for labels on BareMetalHosts.
// This is used to limit the set of BareMetalHost objects considered for
// claiming for a Machine.
type HostSelector struct {
	// Key/value pairs of labels that must exist on a chosen BareMetalHost
	// +optional
	MatchLabels map[string]string `json:"matchLabels,omitempty"`

	// Label match expressions that must be true on a chosen BareMetalHost
	// +optional
	MatchExpressions []HostSelectorRequirement `json:"matchExpressions,omitempty"`

	// InNamespace specifies a single namespace where the BareMetalHost should
	// reside. If not specified, the selection will be done over all available
	// namespaces with a compliant policy.
	// +optional
	InNamespace string `json:"inNamespace,omitempty"`
}

type HostSelectorRequirement struct {
	Key      string             `json:"key"`
	Operator selection.Operator `json:"operator"`
	Values   []string           `json:"values"`
}

// HostClaimStatus defines the observed state of HostClaim.
type HostClaimStatus struct {
	// LastUpdated identifies when this status was last observed.
	// +optional
	LastUpdated *metav1.Time `json:"lastUpdated,omitempty"`

	// Conditions defines current service state of the HostClaim.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// BareMetalHost is a pointer to the name of the bound BareMetalHost
	// +optional
	BareMetalHost *ObjectReference `json:"bareMetalHost,omitempty"`
	// HardwareData is a pointer to the name of the bound HardwareData
	// structure.
	// +optional
	HardwareData *ObjectReference `json:"hardwareData,omitempty"`

	// The currently detected power state of the host. This field may get
	// briefly out of sync with the actual state of the hardware while
	// provisioning processes are running.
	PoweredOn bool `json:"poweredOn,omitempty"`
}

// GetConditions returns the set of conditions for this object.
func (h *HostClaim) GetConditions() []metav1.Condition {
	return h.Status.Conditions
}

// SetConditions sets conditions for an API object.
func (h *HostClaim) SetConditions(conditions []metav1.Condition) {
	h.Status.Conditions = conditions
}

type ObjectReference struct {
	// `namespace` is the namespace of the object bound
	Namespace string `json:"namespace"`
	// `name` is the name of the object bound
	Name string `json:"name"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// HostClaim is the Schema for the hostclaims API.
type HostClaim struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   HostClaimSpec   `json:"spec,omitempty"`
	Status HostClaimStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// HostClaimList contains a list of HostClaim.
type HostClaimList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []HostClaim `json:"items"`
}

func init() {
	SchemeBuilder.Register(&HostClaim{}, &HostClaimList{})
}
