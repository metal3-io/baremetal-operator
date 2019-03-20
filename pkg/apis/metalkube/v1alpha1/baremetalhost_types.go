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
)

// OperationalStatus represents the state of the host
type OperationalStatus string

const (
	// OperationalStatusOK is the status value for when the host is
	// configured correctly and not actively being managed.
	OperationalStatusOK OperationalStatus = "OK"

	// OperationalStatusInspecting is the status value for when the
	// host is powered on and running the discovery image to inspect
	// the hardware resources on the host.
	OperationalStatusInspecting OperationalStatus = "inspecting"

	// OperationalStatusError is the status value for when the host
	// has any sort of error.
	OperationalStatusError OperationalStatus = "error"
)

// BMCDetails contains the information necessary to communicate with
// the bare metal controller module on host.
type BMCDetails struct {

	// Address holds the URL for accessing the controller on the
	// network.
	Address string `json:"address"`

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

	// Which MAC address will PXE boot? This is optional for some
	// types, but required for libvirt VMs driven by vbmc.
	BootMACAddress string `json:"bootMACAddress"`

	// Should the server be online?
	Online bool `json:"online"`

	// MachineRef is a reference to the machine.openshift.io/Machine
	MachineRef *corev1.ObjectReference `json:"machineRef,omitempty"`

	// Image holds the details of the image to be provisioned.
	Image *Image `json:"image,omitempty"`

	// UserData holds the reference to the Secret containing the user
	// data to be passed to the host before it boots.
	UserData *corev1.SecretReference `json:"userData,omitempty"`
}

// Image holds the details of an image either to provisioned or that
// has been provisioned.
type Image struct {
	// URL is a location of an image to deploy.
	URL string `json:"url"`

	// Checksum is the checksum for the image.
	Checksum string `json:"checksum"`
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
	// A name for the disk, e.g. "disk 1 (boot)"
	Name string `json:"name"`

	// Type, e.g. SSD
	Type string `json:"type"`

	// The size of the disk in gigabyte
	SizeGiB int `json:"sizeGiB"`

	// Hardware model
	Model string `json:"model"`
}

// NIC describes one network interface on the host.
type NIC struct {
	// The name of the NIC, e.g. "nic-1"
	Name string `json:"name"`

	// The name of the model, e.g. "virt-io"
	Model string `json:"model"`

	// The name of the network, e.g. "Pod Networking"
	Network string `json:"network"`

	// The device MAC addr
	MAC string `json:"mac"`

	// The IP address of the device
	IP string `json:"ip"`

	// The speed of the device
	SpeedGbps int `json:"speedGbps"`
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

	// OperationalStatus holds the status of the host
	OperationalStatus OperationalStatus `json:"operationalStatus"`

	// MachineRef will point to the corresponding Machine if it exists.
	// +optional
	MachineRef *corev1.ObjectReference `json:"machineRef,omitempty"`

	// LastUpdated identifies when this status was last observed.
	// +optional
	LastUpdated *metav1.Time `json:"lastUpdated,omitempty"`

	// The name of the profile matching the hardware details.
	HardwareProfile string `json:"hardwareProfile"`

	// The hardware discovered to exist on the host.
	HardwareDetails *HardwareDetails `json:"hardware,omitempty"`

	// Information tracked by the provisioner.
	Provisioning ProvisionStatus `json:"provisioning"`

	// the last credentials we were able to validate as working
	GoodCredentials CredentialsStatus `json:"goodCredentials"`

	// the last error message reported by the provisioning subsystem
	ErrorMessage string `json:"errorMessage"`

	// indicator for whether or not the host is powered on
	PoweredOn bool `json:"poweredOn"`
}

// ProvisionStatus holds the state information for a single target.
type ProvisionStatus struct {
	// FIXME(dhellmann): This should be an enum of some sort.
	State string `json:"state"`
	// The machine's UUID from ironic
	ID string `json:"ID"`
	// Image holds the details of the last image successfully
	// provisioned to the host.
	Image Image `json:"image,omitempty"`
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

// Available returns true if the host is available to be provisioned.
func (host *BareMetalHost) Available() bool {
	if host.Spec.MachineRef != nil {
		return false
	}
	if host.GetDeletionTimestamp() != nil {
		return false
	}
	if host.HasError() {
		return false
	}
	return true
}

// SetErrorMessage updates the ErrorMessage in the host Status struct
// when necessary and returns true when a change is made or false when
// no change is made.
func (host *BareMetalHost) SetErrorMessage(message string) (dirty bool) {
	if host.Status.OperationalStatus != OperationalStatusError {
		host.Status.OperationalStatus = OperationalStatusError
		dirty = true
	}
	if host.Status.ErrorMessage != message {
		host.Status.ErrorMessage = message
		dirty = true
	}
	return dirty
}

// ClearError removes any existing error message.
func (host *BareMetalHost) ClearError() (dirty bool) {
	if host.Status.OperationalStatus != OperationalStatusOK {
		host.Status.OperationalStatus = OperationalStatusOK
		dirty = true
	}
	if host.Status.ErrorMessage != "" {
		host.Status.ErrorMessage = ""
		dirty = true
	}
	return dirty
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
func (host *BareMetalHost) SetHardwareProfile(name string) (dirty bool) {
	if host.Status.HardwareProfile != name {
		host.Status.HardwareProfile = name
		dirty = true
	}
	return dirty
}

// SetOperationalStatus updates the OperationalStatusLabel and returns
// true when a change is made or false when no change is made.
func (host *BareMetalHost) SetOperationalStatus(status OperationalStatus) bool {
	if host.Status.OperationalStatus != status {
		host.Status.OperationalStatus = status
		return true
	}
	return false
}

// OperationalStatus returns the value associated with the
// OperationalStatusLabel
func (host *BareMetalHost) OperationalStatus() OperationalStatus {
	return host.Status.OperationalStatus
}

// HasError returns a boolean indicating whether there is an error
// set for the host.
func (host *BareMetalHost) HasError() bool {
	return host.Status.ErrorMessage != ""
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

// NeedsProvisioning compares the settings with the provisioning
// status and returns true when more work is needed or false
// otherwise.
func (host *BareMetalHost) NeedsProvisioning() bool {
	if host.Spec.Image == nil {
		// Without an image, there is nothing to provision.
		return false
	}
	if host.Spec.Image.URL == "" {
		// We have an Image struct but it is empty
		return false
	}
	if host.Status.Provisioning.Image.URL == "" {
		// We have an image set, but not provisioned.
		return true
	}
	// FIXME(dhellmann): Compare the provisioned image against the one
	// we are supposed to have to make sure they match.
	return false
}

// NeedsDeprovisioning compares the settings with the provisioning
// status and returns true when the host should be deprovisioned.
func (host *BareMetalHost) NeedsDeprovisioning() bool {
	if host.Status.Provisioning.Image.URL == "" {
		return false
	}
	if host.Spec.Image == nil {
		return true
	}
	if host.Spec.Image.URL == "" {
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
