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

// ClockSpeed is a clock speed in MHz
// +kubebuilder:validation:Format=double
type ClockSpeed float64

// ClockSpeed multipliers.
const (
	MegaHertz ClockSpeed = 1.0
	GigaHertz            = 1000 * MegaHertz
)

// Capacity is a disk size in Bytes.
type Capacity int64

// Capacity multipliers.
const (
	Byte     Capacity = 1
	KibiByte          = Byte * 1024
	KiloByte          = Byte * 1000
	MebiByte          = KibiByte * 1024
	MegaByte          = KiloByte * 1000
	GibiByte          = MebiByte * 1024
	GigaByte          = MegaByte * 1000
	TebiByte          = GibiByte * 1024
	TeraByte          = GigaByte * 1000
)

// DiskType is a disk type, i.e. HDD, SSD, NVME.
type DiskType string

// DiskType constants.
const (
	HDD  DiskType = "HDD"
	SSD  DiskType = "SSD"
	NVME DiskType = "NVME"
)

// CPU describes one processor on the host.
type CPU struct {
	Arch           string     `json:"arch,omitempty"`
	Model          string     `json:"model,omitempty"`
	ClockMegahertz ClockSpeed `json:"clockMegahertz,omitempty"`
	Flags          []string   `json:"flags,omitempty"`
	Count          int        `json:"count,omitempty"`
}

// Storage describes one storage device (disk, SSD, etc.) on the host.
type Storage struct {
	// A Linux device name of the disk, e.g.
	// "/dev/disk/by-path/pci-0000:01:00.0-scsi-0:2:0:0". This will be a name
	// that is stable across reboots if one is available.
	Name string `json:"name,omitempty"`

	// A list of alternate Linux device names of the disk, e.g. "/dev/sda".
	// Note that this list is not exhaustive, and names may not be stable
	// across reboots.
	AlternateNames []string `json:"alternateNames,omitempty"`

	// Whether this disk represents rotational storage.
	// This field is not recommended for usage, please
	// prefer using 'Type' field instead, this field
	// will be deprecated eventually.
	Rotational bool `json:"rotational,omitempty"`

	// Device type, one of: HDD, SSD, NVME.
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Enum=HDD;SSD;NVME;
	Type DiskType `json:"type,omitempty"`

	// The size of the disk in Bytes
	SizeBytes Capacity `json:"sizeBytes,omitempty"`

	// The name of the vendor of the device
	Vendor string `json:"vendor,omitempty"`

	// Hardware model
	Model string `json:"model,omitempty"`

	// The serial number of the device
	SerialNumber string `json:"serialNumber,omitempty"`

	// The WWN of the device
	WWN string `json:"wwn,omitempty"`

	// The WWN Vendor extension of the device
	WWNVendorExtension string `json:"wwnVendorExtension,omitempty"`

	// The WWN with the extension
	WWNWithExtension string `json:"wwnWithExtension,omitempty"`

	// The SCSI location of the device
	HCTL string `json:"hctl,omitempty"`
}

// VLANID is a 12-bit 802.1Q VLAN identifier
// +kubebuilder:validation:Type=integer
// +kubebuilder:validation:Minimum=0
// +kubebuilder:validation:Maximum=4094
type VLANID int32

// VLAN represents the name and ID of a VLAN.
type VLAN struct {
	ID VLANID `json:"id,omitempty"`

	Name string `json:"name,omitempty"`
}

// LLDP represents Link Layer Discovery Protocol data for a network interface.
type LLDP struct {
	// The switch chassis ID from LLDP
	// +optional
	SwitchID string `json:"switchID,omitempty"`

	// The switch port ID from LLDP
	// +optional
	PortID string `json:"portID,omitempty"`

	// The switch system name from LLDP
	// +optional
	SwitchSystemName string `json:"switchSystemName,omitempty"`
}

// NIC describes one network interface on the host.
type NIC struct {
	// The name of the network interface, e.g. "en0"
	Name string `json:"name,omitempty"`

	// The vendor and product IDs of the NIC, e.g. "0x8086 0x1572"
	Model string `json:"model,omitempty"`

	// The device MAC address
	// +kubebuilder:validation:Pattern=`[0-9a-fA-F]{2}(:[0-9a-fA-F]{2}){5}`
	MAC string `json:"mac,omitempty"`

	// The IP address of the interface. This will be an IPv4 or IPv6 address
	// if one is present.  If both IPv4 and IPv6 addresses are present in a
	// dual-stack environment, two nics will be output, one with each IP.
	IP string `json:"ip,omitempty"`

	// The speed of the device in Gigabits per second
	SpeedGbps int `json:"speedGbps,omitempty"`

	// The VLANs available
	VLANs []VLAN `json:"vlans,omitempty"`

	// The untagged VLAN ID
	//nolint:tagliatelle
	VLANID VLANID `json:"vlanId,omitempty"`

	// Whether the NIC is PXE Bootable
	PXE bool `json:"pxe,omitempty"`

	// LLDP data for this interface
	// +optional
	LLDP *LLDP `json:"lldp,omitempty"`
}

// Firmware describes the firmware on the host.
type Firmware struct {
	// The BIOS for this firmware
	BIOS BIOS `json:"bios,omitempty"`
}

// BIOS describes the BIOS version on the host.
type BIOS struct {
	// The release/build date for this BIOS
	Date string `json:"date,omitempty"`

	// The vendor name for this BIOS
	Vendor string `json:"vendor,omitempty"`

	// The version of the BIOS
	Version string `json:"version,omitempty"`
}

// HardwareSystemVendor stores details about the whole hardware system.
type HardwareSystemVendor struct {
	Manufacturer string `json:"manufacturer,omitempty"`
	ProductName  string `json:"productName,omitempty"`
	SerialNumber string `json:"serialNumber,omitempty"`
}

// HardwareDetails collects all of the information about hardware
// discovered on the host.
type HardwareDetails struct {
	// System vendor information.
	SystemVendor HardwareSystemVendor `json:"systemVendor,omitempty"`
	// System firmware information.
	Firmware Firmware `json:"firmware,omitempty"`
	// The host's amount of memory in Mebibytes.
	RAMMebibytes int `json:"ramMebibytes,omitempty"`
	// List of network interfaces for the host.
	NIC []NIC `json:"nics,omitempty"`
	// List of storage (disk, SSD, etc.) available to the host.
	Storage []Storage `json:"storage,omitempty"`
	// Details of the CPU(s) in the system.
	CPU CPU `json:"cpu,omitempty"`
	// Name of the host at the inspection time.
	Hostname string `json:"hostname,omitempty"`
}

// HardwareDataSpec defines the desired state of HardwareData.
type HardwareDataSpec struct {
	// The hardware discovered on the host during its inspection.
	HardwareDetails *HardwareDetails `json:"hardware,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:path=hardwaredata,scope=Namespaced,shortName=hd
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp",description="Time duration since creation of HardwareData"

// HardwareData is the Schema for the hardwaredata API.
type HardwareData struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec HardwareDataSpec `json:"spec,omitempty"`
}

//+kubebuilder:object:root=true

// HardwareDataList contains a list of HardwareData.
type HardwareDataList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []HardwareData `json:"items"`
}

func init() {
	SchemeBuilder.Register(&HardwareData{}, &HardwareDataList{})
}
