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

package v1beta1

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
	// arch is the CPU architecture, e.g. "x86_64".
	Arch string `json:"arch,omitempty"`
	// model is the CPU model name.
	Model string `json:"model,omitempty"`
	// clockMegahertz is the CPU clock speed in megahertz.
	ClockMegahertz ClockSpeed `json:"clockMegahertz,omitempty"`
	// flags is the list of CPU flags.
	Flags []string `json:"flags,omitempty"`
	// threadCount is the number of CPU threads.
	ThreadCount int `json:"threadCount,omitempty"`
}

// Storage describes one storage device (disk, SSD, etc.) on the host.
type Storage struct {
	// name is a Linux device name of the disk, e.g.
	// "/dev/disk/by-path/pci-0000:01:00.0-scsi-0:2:0:0". This will be a name
	// that is stable across reboots if one is available.
	Name string `json:"name,omitempty"`

	// alternateNames is a list of alternate Linux device names of the disk, e.g. "/dev/sda".
	// Note that this list is not exhaustive, and names may not be stable
	// across reboots.
	AlternateNames []string `json:"alternateNames,omitempty"`

	// rotational indicates whether this disk represents rotational storage.
	// This field is not recommended for usage, please
	// prefer using 'Type' field instead, this field
	// will be deprecated eventually.
	Rotational bool `json:"rotational,omitempty"`

	// type is the device type, one of: HDD, SSD, NVME.
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Enum=HDD;SSD;NVME;
	Type DiskType `json:"type,omitempty"`

	// sizeBytes is the size of the disk in Bytes
	SizeBytes Capacity `json:"sizeBytes,omitempty"`

	// vendor is the name of the vendor of the device
	Vendor string `json:"vendor,omitempty"`

	// model is the hardware model of the device.
	Model string `json:"model,omitempty"`

	// serialNumber is the serial number of the device
	SerialNumber string `json:"serialNumber,omitempty"`

	// wwn is the WWN of the device
	WWN string `json:"wwn,omitempty"`

	// wwnVendorExtension is the WWN Vendor extension of the device
	WWNVendorExtension string `json:"wwnVendorExtension,omitempty"`

	// wwnWithExtension is the WWN with the extension
	WWNWithExtension string `json:"wwnWithExtension,omitempty"`

	// hctl is the SCSI location of the device
	HCTL string `json:"hctl,omitempty"`
}

// VLANID is a 12-bit 802.1Q VLAN identifier
// +kubebuilder:validation:Type=integer
// +kubebuilder:validation:Minimum=0
// +kubebuilder:validation:Maximum=4094
type VLANID int32

// VLAN represents the name and ID of a VLAN.
type VLAN struct {
	// id is the VLAN ID.
	ID VLANID `json:"id,omitempty"`

	// name is the VLAN name.
	Name string `json:"name,omitempty"`
}

// LLDP represents Link Layer Discovery Protocol data for a network interface.
type LLDP struct {
	// switchID is the switch chassis ID from LLDP
	// +optional
	SwitchID string `json:"switchID,omitempty"`

	// portID is the switch port ID from LLDP
	// +optional
	PortID string `json:"portID,omitempty"`

	// switchSystemName is the switch system name from LLDP
	// +optional
	SwitchSystemName string `json:"switchSystemName,omitempty"`
}

// NIC describes one network interface on the host.
type NIC struct {
	// name is the name of the network interface, e.g. "en0"
	Name string `json:"name,omitempty"`

	// model is the vendor and product IDs of the NIC, e.g. "0x8086 0x1572"
	Model string `json:"model,omitempty"`

	// mac is the device MAC address.
	// +kubebuilder:validation:Pattern=`[0-9a-fA-F]{2}(:[0-9a-fA-F]{2}){5}`
	MAC string `json:"mac,omitempty"`

	// ip is the IP address of the interface. This will be an IPv4 or IPv6 address
	// if one is present.  If both IPv4 and IPv6 addresses are present in a
	// dual-stack environment, two nics will be output, one with each IP.
	IP string `json:"ip,omitempty"`

	// speedGbps is the speed of the device in Gigabits per second
	SpeedGbps int `json:"speedGbps,omitempty"`

	// vlans is the list of available VLANs.
	VLANs []VLAN `json:"vlans,omitempty"`

	// vlanId is the untagged VLAN ID
	//nolint:tagliatelle
	VLANID VLANID `json:"vlanId,omitempty"`

	// pxe indicates whether the NIC is PXE bootable.
	PXE bool `json:"pxe,omitempty"`

	// lldp data for this interface
	// +optional
	LLDP *LLDP `json:"lldp,omitempty"`

	// pciAddress is the NIC PCI address
	// +optional
	PCIAddress string `json:"pciAddress,omitempty"`

	// infinibandClientID is the InfiniBand client ID of the interface, as discovered by Ironic.
	// Only set for InfiniBand interfaces.
	// +optional
	InfinibandClientID string `json:"infinibandClientID,omitempty"`
}

// Firmware describes the firmware on the host.
type Firmware struct {
	// bios is the BIOS for this firmware
	BIOS BIOS `json:"bios,omitempty"`
}

// BIOS describes the BIOS version on the host.
type BIOS struct {
	// date is the release/build date for this BIOS
	Date string `json:"date,omitempty"`

	// vendor is the vendor name for this BIOS
	Vendor string `json:"vendor,omitempty"`

	// version is the version of the BIOS
	Version string `json:"version,omitempty"`
}

// HardwareSystemVendor stores details about the whole hardware system.
type HardwareSystemVendor struct {
	// manufacturer is the system manufacturer.
	Manufacturer string `json:"manufacturer,omitempty"`
	// productName is the system product name.
	ProductName string `json:"productName,omitempty"`
	// serialNumber is the system serial number.
	SerialNumber string `json:"serialNumber,omitempty"`
}

// Accelerator describes an accelerator device (e.g. a GPU) discovered on
// the host via PCI device inspection.
type Accelerator struct {
	// vendorID is the PCI vendor ID of the device, e.g. "abcd".
	VendorID string `json:"vendorID,omitempty"`

	// deviceID is the PCI device ID of the device, e.g. "1234".
	DeviceID string `json:"deviceID,omitempty"`

	// type is the type of the accelerator device, e.g. "GPU".
	Type string `json:"type,omitempty"`

	// deviceInfo is a human readable description of the device.
	DeviceInfo string `json:"deviceInfo,omitempty"`

	// pciAddress is the PCI address of the device, e.g. "0000:3b:00.0".
	PCIAddress string `json:"pciAddress,omitempty"`
}

// HardwareDetails collects all of the information about hardware
// discovered on the host.
type HardwareDetails struct {
	// accelerators is the list of accelerators discovered on the host.
	Accelerators []Accelerator `json:"accelerators,omitempty"`
	// systemVendor holds system vendor information.
	SystemVendor HardwareSystemVendor `json:"systemVendor,omitempty"`
	// firmware holds system firmware information.
	Firmware Firmware `json:"firmware,omitempty"`
	// ramMebibytes is the host's amount of memory in Mebibytes.
	RAMMebibytes int `json:"ramMebibytes,omitempty"`
	// nics is the list of network interfaces for the host.
	NIC []NIC `json:"nics,omitempty"`
	// storage is the list of storage (disk, SSD, etc.) available to the host.
	Storage []Storage `json:"storage,omitempty"`
	// cpu holds details of the CPU(s) in the system.
	CPU CPU `json:"cpu,omitempty"`
	// hostname is the name of the host at the inspection time.
	Hostname string `json:"hostname,omitempty"`
}

// HardwareDataSpec defines the desired state of HardwareData.
type HardwareDataSpec struct {
	// hardware is the hardware discovered on the host during its inspection.
	HardwareDetails *HardwareDetails `json:"hardware,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:path=hardwaredata,scope=Namespaced,shortName=hd
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp",description="Time duration since creation of HardwareData"

// HardwareData is the Schema for the hardwaredata API.
type HardwareData struct {
	metav1.TypeMeta `json:",inline"`
	// metadata is the standard object metadata.
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// spec holds the hardware data discovered on the host.
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
