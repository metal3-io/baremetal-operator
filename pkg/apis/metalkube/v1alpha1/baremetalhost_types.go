package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NOTE: json tags are required.  Any new fields you add must have
// json tags for the fields to be serialized.

// NOTE(dhellmann): Update docs/api.md when changing these data structure.

const (
	BareMetalHostFinalizer   string = "baremetalhost.metalkube.org"
	OperationalStatusLabel   string = "metalkube.org/operational-status"
	OperationalStatusError   string = "error"
	OperationalStatusOnline  string = "online"
	OperationalStatusOffline string = "offline"
	HardwareProfileLabel     string = "metalkube.org/hardware-profile"
)

type BMCDetails struct {
	IP string `json:"ip"`
	// The name of the secret containing the BMC credentials (requires
	// keys "username" and "password").
	Credentials *corev1.SecretReference `json:"credentials"`
}

// BareMetalHostSpec defines the desired state of BareMetalHost
type BareMetalHostSpec struct {
	// Important: Run "operator-sdk generate k8s" to regenerate code
	// after modifying this file

	// Taints is the full, authoritative list of taints to apply to
	// the corresponding Machine. This list will overwrite any
	// modifications made to the Machine on an ongoing basis.
	// +optional
	Taints []corev1.Taint `json:"taints,omitempty"`

	// How do we connect to the BMC?
	BMC BMCDetails `json:"bmc"`

	// Should the server be online?
	Online bool `json:"online"`
}

// FIXME(dhellmann): We probably want some other module to own these
// data structures.
type CPU struct {
	Type  string `json:"type"`
	Speed int    `json:"speed"` // GHz
}

type Storage struct {
	Size int    `json:"size"` // GB
	Info string `json:"info"` // model, etc.
}

type NIC struct {
	MAC string `json:"mac"`
	IP  string `json:"ip"`
}

type HardwareDetails struct {
	NIC     []NIC     `json:"nics"`
	Storage []Storage `json:"storage"`
	CPUs    []CPU     `json:"cpus"`
}

// BareMetalHostStatus defines the observed state of BareMetalHost
type BareMetalHostStatus struct {
	// Important: Run "operator-sdk generate k8s" to regenerate code
	// after modifying this file

	// MachineRef will point to the corresponding Machine if it exists.
	// +optional
	MachineRef *corev1.ObjectReference `json:"machineRef,omitempty"`

	// LastUpdated identifies when this status was last observed.
	// +optional
	LastUpdated *metav1.Time `json:"lastUpdated,omitempty"`

	HardwareDetails HardwareDetails `json:"hardware"`

	// UUID in ironic
	ProvisioningID string `json:"provisioningID"`

	// the last thing we deployed here
	Image string `json:"image"`

	ErrorMessage string `json:"errorMessage"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// BareMetalHost is the Schema for the baremetalhosts API
// +k8s:openapi-gen=true
type BareMetalHost struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BareMetalHostSpec   `json:"spec,omitempty"`
	Status BareMetalHostStatus `json:"status,omitempty"`
}

func (host *BareMetalHost) SetErrorMessage(message string) bool {
	if host.Status.ErrorMessage != message {
		host.Status.ErrorMessage = message
		return true
	}
	return false
}

func (host *BareMetalHost) SetLabel(name, value string) bool {
	if host.Labels == nil {
		host.Labels = make(map[string]string)
	}
	if host.Labels[name] != value {
		host.Labels[name] = value
		return true
	}
	return false
}

func (host *BareMetalHost) SetOperationalStatus(status string) bool {
	return host.SetLabel(OperationalStatusLabel, status)
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// BareMetalHostList contains a list of BareMetalHost
type BareMetalHostList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BareMetalHost `json:"items"`
}

func init() {
	SchemeBuilder.Register(&BareMetalHost{}, &BareMetalHostList{})
}
