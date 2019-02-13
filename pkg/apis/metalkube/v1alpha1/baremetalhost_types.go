package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NOTE: json tags are required.  Any new fields you add must have
// json tags for the fields to be serialized.

const (
	// BareMetalHostFinalizer is the name of the finalizer added to
	// hosts to block delete operations until the physical host can be
	// deprovisioned.
	BareMetalHostFinalizer string = "baremetalhost.metalkube.org"

	// OperationalStatusLabel is the name of the label added to the
	// host with the operating status.
	OperationalStatusLabel string = "metalkube.org/operational-status"

	// OperationalStatusError is the status value for the
	// OperationalStatusLabel when the host has an error condition and
	// should not be used.
	OperationalStatusError string = "error"

	// OperationalStatusOnline is the status value for the
	// OperationalStatusLabel when the host is powered on and running.
	OperationalStatusOnline string = "online"

	// OperationalStatusOffline is the status value for the
	// OperationalStatusLabel when the host is powered off.
	OperationalStatusOffline string = "offline"

	// HardwareProfileLabel is the name of the label added to the host
	// with the discovered hardware profile.
	HardwareProfileLabel string = "metalkube.org/hardware-profile"
)

// BMCDetails contains the information necessary to communicate with
// the bare metal controller module on host.
type BMCDetails struct {
	IP string `json:"ip"`
	// The name of the secret containing the BMC credentials (requires
	// keys "username" and "password").
	Credentials corev1.SecretReference `json:"credentials"`
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

// CPU describes one processor on the host.
type CPU struct {
	Type  string
	Speed int // GHz
}

// Storage describes one storage device (disk, SSD, etc.) on the host.
type Storage struct {
	Size int    // GB
	Info string // model, etc.
}

// NIC describes one network interface on the host.
type NIC struct {
	MAC string
	IP  string
}

// HardwareDetails collects all of the information about hardware
// discovered on the host.
type HardwareDetails struct {
	NIC     []NIC     `json:"nics"`
	Storage []Storage `json:"storage"`
	CPUs    []CPU     `json:"cpus"`
}

// CredentialsStatus contains the reference and version of the last
// set of BMC credentials the controller was able to validate.
type CredentialsStatus struct {
	Reference *corev1.SecretReference `json:"credentials,omitempty"`
	Version   string                  `json:"credentialsVersion,omitempty"`
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

	// the last credentials we were able to validate as working
	GoodCredentials CredentialsStatus `json:"goodCredentials"`

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

// SetErrorMessage updates the ErrorMessage in the host Status struct
// when necessary and returns true when a change is made or false when
// no change is made.
func (host *BareMetalHost) SetErrorMessage(message string) bool {
	if host.Status.ErrorMessage != message {
		host.Status.ErrorMessage = message
		return true
	}
	return false
}

// SetLabel updates the given label when necessary and returns true
// when a change is made or false when no change is made.
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

// SetOperationalStatus updates the OperationalStatusLabel and returns
// true when a change is made or false when no change is made.
func (host *BareMetalHost) SetOperationalStatus(status string) bool {
	return host.SetLabel(OperationalStatusLabel, status)
}

// CredentialsNeedValidation compares the secret with the last one
// known to work and report if the new ones need to be checked.
func (host *BareMetalHost) CredentialsNeedValidation(currentSecret corev1.Secret) bool {
	currentRef := host.Status.GoodCredentials.Reference
	currentVersion := host.Status.GoodCredentials.Version
	newRef := host.Spec.BMC.Credentials

	switch {
	case currentRef == nil:
		return true
	case currentRef.Name != newRef.Name:
		return true
	case currentRef.Namespace != newRef.Namespace:
		return true
	case currentVersion != currentSecret.ObjectMeta.ResourceVersion:
		return true
	}
	return false
}

// UpdateGoodCredentials modifies the GoodCredentials portion of the
// Status struct to record the details of the secret containing
// credentials known to work.
func (host *BareMetalHost) UpdateGoodCredentials(currentSecret corev1.Secret) {
	host.Status.GoodCredentials.Version = currentSecret.ObjectMeta.ResourceVersion
	host.Status.GoodCredentials.Reference = &host.Spec.BMC.Credentials
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
