package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// NOTE: json tags are required.  Any new fields you add must have
// json tags for the fields to be serialized.

// NOTE(dhellmann): Update docs/api.md when changing these data structure.

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
	CredentialsName string `json:"credentialsName"`
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

	// MachineRef is a reference to the machine.openshift.io/Machine
	MachineRef *corev1.ObjectReference `json:"machineRef,omitempty"`
}

// FIXME(dhellmann): We probably want some other module to own these
// data structures.

// CPU describes one processor on the host.
type CPU struct {
	Type     string `json:"type"`
	SpeedGHz int    `json:"speedGHz"`
}

// Storage describes one storage device (disk, SSD, etc.) on the host.
type Storage struct {
	SizeGiB int    `json:"sizeGiB"`
	Info    string `json:"info"`
}

// NIC describes one network interface on the host.
type NIC struct {
	MAC       string `json:"mac"`
	IP        string `json:"ip"`
	SpeedGbps int    `json:"speedGbps"`
}

// HardwareDetails collects all of the information about hardware
// discovered on the host.
type HardwareDetails struct {
	RAMGiB  int       `json:"ramGiB"`
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

// setLabel updates the given label when necessary and returns true
// when a change is made or false when no change is made.
func (host *BareMetalHost) setLabel(name, value string) bool {
	if host.Labels == nil {
		host.Labels = make(map[string]string)
	}
	if host.Labels[name] != value {
		host.Labels[name] = value
		return true
	}
	return false
}

// getLabel returns the value associated with the given label. If
// there is no value, an empty string is returned.
func (host *BareMetalHost) getLabel(name string) string {
	if host.Labels == nil {
		return ""
	}
	return host.Labels[name]
}

// SetHardwareProfile updates the HardwareProfileLabel and returns
// true when a change is made or false when no change is made.
func (host *BareMetalHost) SetHardwareProfile(name string) bool {
	return host.setLabel(HardwareProfileLabel, name)
}

// SetOperationalStatus updates the OperationalStatusLabel and returns
// true when a change is made or false when no change is made.
func (host *BareMetalHost) SetOperationalStatus(status string) bool {
	return host.setLabel(OperationalStatusLabel, status)
}

// OperationalStatus returns the value associated with the
// OperationalStatusLabel
func (host *BareMetalHost) OperationalStatus() string {
	status := host.getLabel(OperationalStatusLabel)
	if status == "" {
		return "unknown"
	}
	return status
}

// CredentialsKey returns a NamespacedName suitable for loading the
// Secret containing the credentials associated with the host.
func (host *BareMetalHost) CredentialsKey() types.NamespacedName {
	return types.NamespacedName{
		Name:      host.Spec.BMC.CredentialsName,
		Namespace: host.ObjectMeta.Namespace,
	}
}

// CredentialsNeedValidation compares the secret with the last one
// known to work and report if the new ones need to be checked.
func (host *BareMetalHost) CredentialsNeedValidation(currentSecret corev1.Secret) bool {
	currentRef := host.Status.GoodCredentials.Reference
	currentVersion := host.Status.GoodCredentials.Version
	newName := host.Spec.BMC.CredentialsName

	switch {
	case currentRef == nil:
		return true
	case currentRef.Name != newName:
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
	host.Status.GoodCredentials.Reference = &corev1.SecretReference{
		Name:      currentSecret.ObjectMeta.Name,
		Namespace: currentSecret.ObjectMeta.Namespace,
	}
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
