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
	"time"

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
	BareMetalHostFinalizer string = "baremetalhost.metal3.io"

	// PausedAnnotation is the annotation that pauses the reconciliation (triggers
	// an immediate requeue)
	PausedAnnotation = "baremetalhost.metal3.io/paused"

	// StatusAnnotation is the annotation that keeps a copy of the Status of BMH
	// This is particularly useful when we pivot BMH. If the status
	// annotation is present and status is empty, BMO will reconstruct BMH Status
	// from the status annotation.
	StatusAnnotation = "baremetalhost.metal3.io/status"
)

// RootDeviceHints holds the hints for specifying the storage location
// for the root filesystem for the image.
type RootDeviceHints struct {
	// A Linux device name like "/dev/vda". The hint must match the
	// actual value exactly.
	DeviceName string `json:"deviceName,omitempty"`

	// A SCSI bus address like 0:0:0:0. The hint must match the actual
	// value exactly.
	HCTL string `json:"hctl,omitempty"`

	// A vendor-specific device identifier. The hint can be a
	// substring of the actual value.
	Model string `json:"model,omitempty"`

	// The name of the vendor or manufacturer of the device. The hint
	// can be a substring of the actual value.
	Vendor string `json:"vendor,omitempty"`

	// Device serial number. The hint must match the actual value
	// exactly.
	SerialNumber string `json:"serialNumber,omitempty"`

	// The minimum size of the device in Gigabytes.
	// +kubebuilder:validation:Minimum=0
	MinSizeGigabytes int `json:"minSizeGigabytes,omitempty"`

	// Unique storage identifier. The hint must match the actual value
	// exactly.
	WWN string `json:"wwn,omitempty"`

	// Unique storage identifier with the vendor extension
	// appended. The hint must match the actual value exactly.
	WWNWithExtension string `json:"wwnWithExtension,omitempty"`

	// Unique vendor storage identifier. The hint must match the
	// actual value exactly.
	WWNVendorExtension string `json:"wwnVendorExtension,omitempty"`

	// True if the device should use spinning media, false otherwise.
	Rotational *bool `json:"rotational,omitempty"`
}

// BootMode is the boot mode of the system
// +kubebuilder:validation:Enum=UEFI;legacy
type BootMode string

// Allowed boot mode from metal3
const (
	UEFI            BootMode = "UEFI"
	Legacy          BootMode = "legacy"
	DefaultBootMode BootMode = UEFI
)

// OperationalStatus represents the state of the host
type OperationalStatus string

const (
	// OperationalStatusOK is the status value for when the host is
	// configured correctly and is manageable.
	OperationalStatusOK OperationalStatus = "OK"

	// OperationalStatusDiscovered is the status value for when the
	// host is only partially configured, such as when when the BMC
	// address is known but the login credentials are not.
	OperationalStatusDiscovered OperationalStatus = "discovered"

	// OperationalStatusError is the status value for when the host
	// has any sort of error.
	OperationalStatusError OperationalStatus = "error"
)

// ErrorType indicates the class of problem that has caused the Host resource
// to enter an error state.
type ErrorType string

const (
	// RegistrationError is an error condition occurring when the
	// controller is unable to connect to the Host's baseboard management
	// controller.
	RegistrationError ErrorType = "registration error"
	// InspectionError is an error condition occurring when an attempt to
	// obtain hardware details from the Host fails.
	InspectionError ErrorType = "inspection error"
	// ProvisioningError is an error condition occuring when the controller
	// fails to provision or deprovision the Host.
	ProvisioningError ErrorType = "provisioning error"
	// PowerManagementError is an error condition occurring when the
	// controller is unable to modify the power state of the Host.
	PowerManagementError ErrorType = "power management error"
)

// ProvisioningState defines the states the provisioner will report
// the host has having.
type ProvisioningState string

const (
	// StateNone means the state is unknown
	StateNone ProvisioningState = ""

	// StateUnmanaged means there is insufficient information available to
	// register the host
	StateUnmanaged ProvisioningState = "unmanaged"

	// StateRegistrationError means there was an error registering the
	// host with the backend
	StateRegistrationError ProvisioningState = "registration error"

	// StateRegistering means we are telling the backend about the host
	StateRegistering ProvisioningState = "registering"

	// StateMatchProfile means we are comparing the discovered details
	// against known hardware profiles
	StateMatchProfile ProvisioningState = "match profile"

	// StateReady means the host can be consumed
	StateReady ProvisioningState = "ready"

	// StateAvailable means the host can be consumed
	StateAvailable ProvisioningState = "available"

	// StateProvisioning means we are writing an image to the host's
	// disk(s)
	StateProvisioning ProvisioningState = "provisioning"

	// StateProvisioningError means we are writing an image to the
	// host's disk(s)
	StateProvisioningError ProvisioningState = "provisioning error"

	// StateProvisioned means we have written an image to the host's
	// disk(s)
	StateProvisioned ProvisioningState = "provisioned"

	// StateExternallyProvisioned means something else is managing the
	// image on the host
	StateExternallyProvisioned ProvisioningState = "externally provisioned"

	// StateDeprovisioning means we are removing an image from the
	// host's disk(s)
	StateDeprovisioning ProvisioningState = "deprovisioning"

	// StateInspecting means we are running the agent on the host to
	// learn about the hardware components available there
	StateInspecting ProvisioningState = "inspecting"

	// StatePowerManagementError means something went wrong trying to
	// power the server on or off.
	StatePowerManagementError ProvisioningState = "power management error"

	// StateDeleting means we are in the process of cleaning up the host
	// ready for deletion
	StateDeleting ProvisioningState = "deleting"
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

	// DisableCertificateVerification disables verification of server
	// certificates when using HTTPS to connect to the BMC. This is
	// required when the server certificate is self-signed, but is
	// insecure because it allows a man-in-the-middle to intercept the
	// connection.
	DisableCertificateVerification bool `json:"disableCertificateVerification,omitempty"`
}

// IDRACConfig the BIOS configuration in the iDRAC driver supports
type IDRACConfig struct {
	// Possible values: string value.
	// +kubebuilder:validation:MaxLength=64
	AssetTag string `json:"assetTag,omitempty"`

	// Possible values: Enabled, Disabled.
	// +kubebuilder:validation:Enum="Enabled";"Disabled"
	BootSeqRetry string `json:"bootSeqRetry,omitempty"`

	// Possible values: Enabled, Disabled, ControlledTurboLimitMinus1, ControlledTurboLimitMinus2, ControlledTurboLimitMinus3.
	// +kubebuilder:validation:Enum="Enabled";"Disabled";"ControlledTurboLimitMinus1";"ControlledTurboLimitMinus2";"ControlledTurboLimitMinus3"
	ControlledTurbo string `json:"controlledTurbo,omitempty"`

	// Possible values: Enabled, Disabled.
	// +kubebuilder:validation:Enum="Enabled";"Disabled"
	CorrEccSmi string `json:"corrEccSmi,omitempty"`

	// Possible values: Enabled, Disabled.
	// +kubebuilder:validation:Enum="Enabled";"Disabled"
	CPUInterconnectBusLinkPower string `json:"cpuInterconnectBusLinkPower,omitempty"`

	// Possible values: Enabled, Disabled.
	// +kubebuilder:validation:Enum="Enabled";"Disabled"
	DcuIPPrefetcher string `json:"dcuIpPrefetcher,omitempty"`

	// Possible values: Enabled, Disabled.
	// +kubebuilder:validation:Enum="Enabled";"Disabled"
	DcuStreamerPrefetcher string `json:"dcuStreamerPrefetcher,omitempty"`

	// Possible values: Enabled, Disabled.
	// +kubebuilder:validation:Enum="Enabled";"Disabled"
	DynamicCoreAllocation string `json:"dynamicCoreAllocation,omitempty"`

	// Possible values: AtaMode, AhciMode, RaidMode, Off.
	// +kubebuilder:validation:Enum="AtaMode";"AhciMode";"RaidMode";"Off"
	EmbSata string `json:"embSata,omitempty"`

	// Possible values: Enabled, Disabled.
	// +kubebuilder:validation:Enum="Enabled";"Disabled"
	HddFailover string `json:"hddFailover,omitempty"`

	// Possible values: OptimizerMode, SpareMode, MirrorMode, AdvEccMode, SpareWithAdvEccMode, FaultResilientMode, NUMAFaultResilientMode.
	// +kubebuilder:validation:Enum="OptimizerMode";"SpareMode";"MirrorMode";"AdvEccMode";"SpareWithAdvEccMode";"FaultResilientMode";"NUMAFaultResilientMode"
	MemOpMode string `json:"memOpMode,omitempty"`

	// Possible values: Enabled, Disabled.
	// +kubebuilder:validation:Enum="Enabled";"Disabled"
	MemTest string `json:"memTest,omitempty"`

	// Possible values: Enabled, Disabled.
	// +kubebuilder:validation:Enum="Enabled";"Disabled"
	NodeInterleave string `json:"nodeInterleave,omitempty"`

	// Possible values: Enabled, Disabled.
	// +kubebuilder:validation:Enum="Enabled";"Disabled"
	PowerSaver string `json:"powerSaver,omitempty"`

	// Possible values: Enabled, Disabled.
	// +kubebuilder:validation:Enum="Enabled";"Disabled"
	ProcAdjCacheLine string `json:"procAdjCacheLine,omitempty"`

	// Possible values: Enabled, Disabled.
	// +kubebuilder:validation:Enum="Enabled";"Disabled"
	ProcAts string `json:"procAts,omitempty"`

	// Possible values: Nominal, Level1.
	// +kubebuilder:validation:Enum="Nominal";"Level1"
	ProcConfigTdp string `json:"procConfigTdp,omitempty"`

	// Possible values: All, 1, 2, 4, 6, 8.
	// +kubebuilder:validation:Enum="All";"1";"2";"4";"6";"8"
	ProcCores string `json:"procCores,omitempty"`

	// Possible values: Enabled, Disabled.
	// +kubebuilder:validation:Enum="Enabled";"Disabled"
	ProcExecuteDisable string `json:"procExecuteDisable,omitempty"`

	// Possible values: Enabled, Disabled.
	// +kubebuilder:validation:Enum="Enabled";"Disabled"
	ProcX2Apic string `json:"procX2Apic,omitempty"`

	// Possible values: MaxDataRate, 8GTps, 6GTps.
	// +kubebuilder:validation:Enum="MaxDataRate";"8GTps";"6GTps"
	QpiSpeed string `json:"qpiSpeed,omitempty"`

	// Possible values: Enabled, Disabled.
	// +kubebuilder:validation:Enum="Enabled";"Disabled"
	ProcHwPrefetcher string `json:"procHwPrefetcher,omitempty"`

	// Possible values: Enabled, Disabled.
	// +kubebuilder:validation:Enum="Enabled";"Disabled"
	RtidSetting string `json:"rtidSetting,omitempty"`

	// Possible values: Enabled, Disabled.
	// +kubebuilder:validation:Enum="Enabled";"Disabled"
	SecurityFreezeLock string `json:"securityFreezeLock,omitempty"`

	// Possible values: EarlySnoop, HomeSnoop, ClusterOnDie, OpportunisticSnoopBroadcast.
	// +kubebuilder:validation:Enum="EarlySnoop";"HomeSnoop";"ClusterOnDie";"OpportunisticSnoopBroadcast"
	SnoopMode string `json:"snoopMode,omitempty"`

	// Possible values: Enabled, Disabled.
	// +kubebuilder:validation:Enum="Enabled";"Disabled"
	SriovGlobalEnable string `json:"enableSriovGlobal,omitempty"`

	// Possible values: PerfPerWattOptimizedDapc, PerfPerWattOptimizedOs, PerfOptimized, DenseCfgOptimized, Custom.
	// +kubebuilder:validation:Enum="PerfPerWattOptimizedDapc";"PerfPerWattOptimizedOs";"PerfOptimized";"DenseCfgOptimized";"Custom"
	SysProfile string `json:"sysProfile,omitempty"`

	// Possible values: NotAvailable, HpcProfile, LowLatencyOptimizedProfile.
	// +kubebuilder:validation:Enum="NotAvailable";"HpcProfile";"LowLatencyOptimizedProfile"
	WorkloadProfile string `json:"workloadProfile,omitempty"`

	// Possible values: Enabled, Disabled.
	// +kubebuilder:validation:Enum="Enabled";"Disabled"
	WriteCache string `json:"writeCache,omitempty"`
}

// ILOConfig the supported BIOS settings and the corresponding brief description for each of the settings.
type ILOConfig struct {
	// Configure additional memory protection with ECC (Error Checking and Correcting).
	// Allowed values are AdvancedEcc, OnlineSpareAdvancedEcc, MirroredAdvancedEcc.
	// +kubebuilder:validation:Enum="AdvancedEcc";"OnlineSpareAdvancedEcc";"MirroredAdvancedEcc"
	AdvancedMemProtection string `json:"advancedMemProtection,omitempty"`

	// Configure the server to automatically power on when AC power is applied to the system.
	// Allowed values are AlwaysPowerOn, AlwaysPowerOff, RestoreLastState.
	// +kubebuilder:validation:Enum="AlwaysPowerOn";"AlwaysPowerOff";"RestoreLastState"
	AutoPowerOn string `json:"autoPowerOn,omitempty"`

	// Configure how the system attempts to boot devices per the Boot Order when no bootable device is found.
	// Allowed values are RetryIndefinitely, AttemptOnce, ResetAfterFailed.
	// +kubebuilder:validation:Enum="RetryIndefinitely";"AttemptOnce";"ResetAfterFailed"
	BootOrderPolicy string `json:"bootOrderPolicy,omitempty"`

	// Enables the Operating System to request processor frequency changes
	// even if the Power Regulator option on the server configured for Dynamic Power Savings Mode.
	// Allowed values are Enabled, Disabled.
	// +kubebuilder:validation:Enum="Enabled";"Disabled"
	CollabPowerControl string `json:"collabPowerControl,omitempty"`

	// Configure when the System ROM executes power calibration during the boot process.
	// Allowed values are Enabled, Disabled, Auto.
	// +kubebuilder:validation:Enum="Enabled";"Disabled";"Auto"
	DynamicPowerCapping string `json:"dynamicPowerCapping,omitempty"`

	// Enable the System BIOS to control processor performance and power states depending on the processor workload.
	// Allowed values are Fast, Slow.
	// +kubebuilder:validation:Enum="Fast";"Slow"
	DynamicPowerResponse string `json:"dynamicPowerResponse,omitempty"`

	// Enable or disable the Intelligent Provisioning functionality.
	// Allowed values are Enabled, Disabled.
	// +kubebuilder:validation:Enum="Enabled";"Disabled"
	IntelligentProvisioning string `json:"intelligentProvisioning,omitempty"`

	// Exposes certain chipset devices that can be used with the Intel Performance Monitoring Toolkit.
	// Allowed values are Enabled, Disabled.
	// +kubebuilder:validation:Enum="Enabled";"Disabled"
	IntelPerfMonitoring string `json:"intelPerfMonitoring,omitempty"`

	// Hypervisor or operating system supporting this option can use hardware capabilities provided
	// by Intel’s Virtualization Technology for Directed I/O.
	// Allowed values are Enabled, Disabled.
	// +kubebuilder:validation:Enum="Enabled";"Disabled"
	IntelProcVtd string `json:"intelProcVtd,omitempty"`

	// Set the QPI Link frequency to a lower speed.
	// Allowed values are Auto, MinQpiSpeed.
	// +kubebuilder:validation:Enum="Auto";"MinQpiSpeed"
	IntelQpiFreq string `json:"intelQpiFreq,omitempty"`

	// Option to modify Intel TXT support.
	// Allowed values are Enabled, Disabled.
	// +kubebuilder:validation:Enum="Enabled";"Disabled"
	IntelTxt string `json:"intelTxt,omitempty"`

	// Set the power profile to be used.
	// Allowed values are BalancedPowerPerf, MinPower, MaxPerf, Custom.
	// +kubebuilder:validation:Enum="BalancedPowerPerf";"MinPower";"MaxPerf";"Custom"
	PowerProfile string `json:"powerProfile,omitempty"`

	// Determines how to regulate the power consumption.
	// Allowed values are DynamicPowerSavings, StaticLowPower, StaticHighPerf, OsControl.
	// +kubebuilder:validation:Enum="DynamicPowerSavings";"StaticLowPower";"StaticHighPerf";"OsControl"
	PowerRegulator string `json:"powerRegulator,omitempty"`

	// Enable or disable the Advanced Encryption Standard Instruction Set (AES-NI) in the processor.
	// Allowed values are Enabled, Disabled.
	// +kubebuilder:validation:Enum="Enabled";"Disabled"
	ProcAes string `json:"procAes,omitempty"`

	// Disable processor cores using Intel’s Core Multi-Processing (CMP) Technology.
	// Allowed values are Integers ranging from 0 to 24.
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=24
	ProcCoreDisable int64 `json:"procCoreDisable,omitempty"`

	// Protect your system against malicious code and viruses.
	// Allowed values are Enabled, Disabled.
	// +kubebuilder:validation:Enum="Enabled";"Disabled"
	ProcNoExecute string `json:"procNoExecute,omitempty"`

	// Enables the processor to transition to a higher frequency than the processor’s rated speed using Turbo Boost Technology
	// if the processor has available power and is within temperature specifications.
	// Allowed values are Enabled, Disabled.
	// +kubebuilder:validation:Enum="Enabled";"Disabled"
	ProcTurbo string `json:"procTurbo,omitempty"`

	// If enabled, SR-IOV support enables a hypervisor to create virtual instances of a PCI-express device, potentially increasing performance.
	// If enabled, the BIOS allocates additional resources to PCI-express devices.
	// Allowed values are Enabled, Disabled.
	// +kubebuilder:validation:Enum="Enabled";"Disabled"
	Sriov string `json:"sriov,omitempty"`

	// select the fan cooling solution for the system.
	// Allowed values are OptimalCooling, IncreasedCooling, MaxCooling
	// +kubebuilder:validation:Enum="OptimalCooling";"IncreasedCooling";"MaxCooling"
	ThermalConfig string `json:"thermalConfig,omitempty"`

	// Control the reaction of the system to caution level thermal events.
	// Allowed values are Enabled, Disabled.
	// +kubebuilder:validation:Enum="Enabled";"Disabled"
	ThermalShutdown string `json:"thermalShutdown,omitempty"`

	// Enables or Disables the System BIOS boot using native UEFI graphics drivers.
	// Allowed values are Enabled, Disabled.
	// +kubebuilder:validation:Enum="Enabled";"Disabled"
	UefiOptimizedBoot string `json:"uefiOptimizedBoot,omitempty"`

	// Change the Workload Profile to accomodate your desired workload.
	// This setting is only applicable to ProLiant Gen10 servers with iLO 5 management systems.
	// Allowed values are GeneralPowerEfficientCompute, GeneralPeakFrequencyCompute,
	// GeneralThroughputCompute, Virtualization-PowerEfficient, Virtualization-MaxPerformance,
	// LowLatency, MissionCritical, TransactionalApplicationProcessing, HighPerformanceCompute,
	// DecisionSupport, GraphicProcessing, I/OThroughput, Custom
	// +kubebuilder:validation:Enum="GeneralPowerEfficientCompute";"GeneralPeakFrequencyCompute";"GeneralThroughputCompute";"Virtualization-PowerEfficient";"Virtualization-MaxPerformance";"LowLatency";"MissionCritical";"TransactionalApplicationProcessing";"HighPerformanceCompute";"DecisionSupport";"GraphicProcessing";"OThroughput";"Custom"
	WorkloadProfile string `json:"workloadProfile,omitempty"`
}

// IRMCConfig the BIOS configuration in the iRMC driver supports
type IRMCConfig struct {
	// Specifies from which drives can be booted.
	// This supports following options: UefiAndLegacy, LegacyOnly, UefiOnly.
	// +kubebuilder:validation:Enum="UefiAndLegacy";"LegacyOnly";"UefiOnly"
	BootOptionFilter string `json:"bootOptionFilter,omitempty"`

	// The UEFI FW checks the controller health status.
	// This supports following options: true, false.
	// +kubebuilder:validation:Enum="true";"false"
	CheckControllersHealthStatusEnabled string `json:"checkControllersHealthStatusEnabled,omitempty"`

	// The processor loads the requested cache line and the adjacent cache line.
	// This supports following options: true, false.
	// +kubebuilder:validation:Enum="true";"false"
	CPUActiveProcessorCores string `json:"cpuActiveProcessorCores,omitempty"`

	// The number of active processor cores 1…n. Option 0 indicates that all available processor cores are active.
	// +kubebuilder:validation:Minimum=0
	CPUAdjacentCacheLinePrefetchEnabled int64 `json:"cpuAdjacentCacheLinePrefetchEnabled,omitempty"`

	// The system BIOS can be written. Flash BIOS update is possible.
	// This supports following options: true, false.
	// +kubebuilder:validation:Enum="true";"false"
	FlashWriteEnabled string `json:"flashWriteEnabled,omitempty"`

	// Boot Options will not be removed from “Boot Option Priority” list.
	// This supports following options: true, false.
	// +kubebuilder:validation:Enum="true";"false"
	KeepVoidBootOptionsEnabled string `json:"keepVoidBootOptionsEnabled,omitempty"`

	// Specifies whether the Compatibility Support Module (CSM) is executed.
	// This supports following options: true, false.
	// +kubebuilder:validation:Enum="true";"false"
	LaunchCsmEnabled string `json:"launchCsmEnabled,omitempty"`

	// Prevents the OS from overruling any energy efficiency policy setting of the setup.
	// This supports following options: true, false.
	// +kubebuilder:validation:Enum="true";"false"
	OsEnergyPerformanceOverrideEnabled string `json:"osEnergyPerformanceOverrideEnabled,omitempty"`

	// Active State Power Management (ASPM) is used to power-manage the PCI Express links, thus consuming less power.
	// This supports following options: Disabled, Auto, L0Limited, L1only, L0Force.
	// +kubebuilder:validation:Enum="Disabled";"Auto";"L0Limited";"L1only";"L0Force"
	PciAspmSupport string `json:"pciAspmSupport,omitempty"`

	// Specifies if memory resources above the 4GB address boundary can be assigned to PCI devices.
	// This supports following options: true, false.
	// +kubebuilder:validation:Enum="true";"false"
	PciAbove4gDecodingEnabled string `json:"pciAbove4gDecodingEnabled,omitempty"`

	// Specifies whether the switch on sources for the system are managed by the BIOS or the ACPI operating system.
	// This supports following options: BiosControlled, AcpiControlled.
	// +kubebuilder:validation:Enum="BiosControlled";"AcpiControlled"
	PowerOnSource string `json:"powerOnSource,omitempty"`

	// Single Root IO Virtualization Support is active.
	// This supports following options: true, false.
	// +kubebuilder:validation:Enum="true";"false"
	SingleRootIoVirtualizationSupportEnabled string `json:"singleRootIoVirtualizationSupportEnabled,omitempty"`
}

// FirmwareConfig contains the configuration that you want to configure BIOS settings in Bare metal server
type FirmwareConfig struct {
	// Supports the virtualization of platform hardware.
	// This supports following options: true, false.
	// +kubebuilder:validation:Enum="true";"false"
	VirtualizationEnabled string `json:"virtualizationEnabled,omitempty"`

	// Allows a single physical processor core to appear as several logical processors.
	// This supports following options: true, false.
	// +kubebuilder:validation:Enum="true";"false"
	SimultaneousMultithreadingEnabled string `json:"simultaneousMultithreadingEnabled,omitempty"`

	// The integrated Dell Remote Access Controller (iDRAC) is an out-of-band management platform on Dell EMC servers.
	IDRAC *IDRACConfig `json:"idrac,omitempty"`

	// iLO driver enables to take advantage of features of iLO management engine in HPE ProLiant servers.
	ILO *ILOConfig `json:"ilo,omitempty"`

	// The iRMC driver enables control FUJITSU PRIMERGY via ServerView Common Command Interface (SCCI).
	IRMC *IRMCConfig `json:"irmc,omitempty"`
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
	BMC BMCDetails `json:"bmc,omitempty"`

	// BIOS configuration for bare metal server
	Firmware *FirmwareConfig `json:"firmware,omitempty"`

	// What is the name of the hardware profile for this host? It
	// should only be necessary to set this when inspection cannot
	// automatically determine the profile.
	HardwareProfile string `json:"hardwareProfile,omitempty"`

	// Provide guidance about how to choose the device for the image
	// being provisioned.
	RootDeviceHints *RootDeviceHints `json:"rootDeviceHints,omitempty"`

	// Select the method of initializing the hardware during
	// boot. Defaults to UEFI.
	// +optional
	BootMode BootMode `json:"bootMode,omitempty"`

	// Which MAC address will PXE boot? This is optional for some
	// types, but required for libvirt VMs driven by vbmc.
	// +kubebuilder:validation:Pattern=`[0-9a-fA-F]{2}(:[0-9a-fA-F]{2}){5}`
	BootMACAddress string `json:"bootMACAddress,omitempty"`

	// Should the server be online?
	Online bool `json:"online"`

	// ConsumerRef can be used to store information about something
	// that is using a host. When it is not empty, the host is
	// considered "in use".
	ConsumerRef *corev1.ObjectReference `json:"consumerRef,omitempty"`

	// Image holds the details of the image to be provisioned.
	Image *Image `json:"image,omitempty"`

	// UserData holds the reference to the Secret containing the user
	// data to be passed to the host before it boots.
	UserData *corev1.SecretReference `json:"userData,omitempty"`

	// NetworkData holds the reference to the Secret containing network
	// configuration (e.g content of network_data.json which is passed
	// to Config Drive).
	NetworkData *corev1.SecretReference `json:"networkData,omitempty"`

	// MetaData holds the reference to the Secret containing host metadata
	// (e.g. meta_data.json which is passed to Config Drive).
	MetaData *corev1.SecretReference `json:"metaData,omitempty"`

	// Description is a human-entered text used to help identify the host
	Description string `json:"description,omitempty"`

	// ExternallyProvisioned means something else is managing the
	// image running on the host and the operator should only manage
	// the power status and hardware inventory inspection. If the
	// Image field is filled in, this field is ignored.
	ExternallyProvisioned bool `json:"externallyProvisioned,omitempty"`
}

// ChecksumType holds the algorithm name for the checksum
// +kubebuilder:validation:Enum=md5;sha256;sha512
type ChecksumType string

const (
	// MD5 checksum type
	MD5 ChecksumType = "md5"

	// SHA256 checksum type
	SHA256 ChecksumType = "sha256"

	// SHA512 checksum type
	SHA512 ChecksumType = "sha512"
)

// Image holds the details of an image either to provisioned or that
// has been provisioned.
type Image struct {
	// URL is a location of an image to deploy.
	URL string `json:"url"`

	// Checksum is the checksum for the image.
	Checksum string `json:"checksum"`

	// ChecksumType is the checksum algorithm for the image.
	// e.g md5, sha256, sha512
	ChecksumType ChecksumType `json:"checksumType,omitempty"`

	// DiskFormat contains the format of the image (raw, qcow2, ...)
	// Needs to be set to raw for raw images streaming
	// +kubebuilder:validation:Enum=raw;qcow2;vdi;vmdk
	DiskFormat *string `json:"format,omitempty"`
}

// FIXME(dhellmann): We probably want some other module to own these
// data structures.

// ClockSpeed is a clock speed in MHz
type ClockSpeed float64

// ClockSpeed multipliers
const (
	MegaHertz ClockSpeed = 1.0
	GigaHertz            = 1000 * MegaHertz
)

// Capacity is a disk size in Bytes
type Capacity int64

// Capacity multipliers
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

// CPU describes one processor on the host.
type CPU struct {
	Arch           string     `json:"arch"`
	Model          string     `json:"model"`
	ClockMegahertz ClockSpeed `json:"clockMegahertz"`
	Flags          []string   `json:"flags"`
	Count          int        `json:"count"`
}

// Storage describes one storage device (disk, SSD, etc.) on the host.
type Storage struct {
	// A name for the disk, e.g. "disk 1 (boot)"
	Name string `json:"name"`

	// Whether this disk represents rotational storage
	Rotational bool `json:"rotational"`

	// The size of the disk in Bytes
	SizeBytes Capacity `json:"sizeBytes"`

	// The name of the vendor of the device
	Vendor string `json:"vendor,omitempty"`

	// Hardware model
	Model string `json:"model,omitempty"`

	// The serial number of the device
	SerialNumber string `json:"serialNumber"`

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

// VLAN represents the name and ID of a VLAN
type VLAN struct {
	ID VLANID `json:"id"`

	Name string `json:"name,omitempty"`
}

// NIC describes one network interface on the host.
type NIC struct {
	// The name of the NIC, e.g. "nic-1"
	Name string `json:"name"`

	// The name of the model, e.g. "virt-io"
	Model string `json:"model"`

	// The device MAC addr
	// +kubebuilder:validation:Pattern=`[0-9a-fA-F]{2}(:[0-9a-fA-F]{2}){5}`
	MAC string `json:"mac"`

	// The IP address of the device
	IP string `json:"ip"`

	// The speed of the device
	SpeedGbps int `json:"speedGbps"`

	// The VLANs available
	VLANs []VLAN `json:"vlans,omitempty"`

	// The untagged VLAN ID
	VLANID VLANID `json:"vlanId"`

	// Whether the NIC is PXE Bootable
	PXE bool `json:"pxe"`
}

// Firmware describes the firmware on the host.
type Firmware struct {
	// The BIOS for this firmware
	BIOS BIOS `json:"bios"`
}

// BIOS describes the BIOS version on the host.
type BIOS struct {
	// The release/build date for this BIOS
	Date string `json:"date"`

	// The vendor name for this BIOS
	Vendor string `json:"vendor"`

	// The version of the BIOS
	Version string `json:"version"`
}

// HardwareDetails collects all of the information about hardware
// discovered on the host.
type HardwareDetails struct {
	SystemVendor HardwareSystemVendor `json:"systemVendor"`
	Firmware     Firmware             `json:"firmware"`
	RAMMebibytes int                  `json:"ramMebibytes"`
	NIC          []NIC                `json:"nics"`
	Storage      []Storage            `json:"storage"`
	CPU          CPU                  `json:"cpu"`
	Hostname     string               `json:"hostname"`
}

// HardwareSystemVendor stores details about the whole hardware system.
type HardwareSystemVendor struct {
	Manufacturer string `json:"manufacturer"`
	ProductName  string `json:"productName"`
	SerialNumber string `json:"serialNumber"`
}

// CredentialsStatus contains the reference and version of the last
// set of BMC credentials the controller was able to validate.
type CredentialsStatus struct {
	Reference *corev1.SecretReference `json:"credentials,omitempty"`
	Version   string                  `json:"credentialsVersion,omitempty"`
}

// Match compares the saved status information with the name and
// content of a secret object.
func (cs CredentialsStatus) Match(secret corev1.Secret) bool {
	switch {
	case cs.Reference == nil:
		return false
	case cs.Reference.Name != secret.ObjectMeta.Name:
		return false
	case cs.Reference.Namespace != secret.ObjectMeta.Namespace:
		return false
	case cs.Version != secret.ObjectMeta.ResourceVersion:
		return false
	}
	return true
}

// OperationMetric contains metadata about an operation (inspection,
// provisioning, etc.) used for tracking metrics.
type OperationMetric struct {
	// +nullable
	Start metav1.Time `json:"start,omitempty"`
	// +nullable
	End metav1.Time `json:"end,omitempty"`
}

// Duration returns the length of time that was spent on the
// operation. If the operation is not finished, it returns 0.
func (om OperationMetric) Duration() time.Duration {
	if om.Start.IsZero() {
		return 0
	}
	return om.End.Time.Sub(om.Start.Time)
}

// OperationHistory holds information about operations performed on a
// host.
type OperationHistory struct {
	Register    OperationMetric `json:"register,omitempty"`
	Inspect     OperationMetric `json:"inspect,omitempty"`
	Provision   OperationMetric `json:"provision,omitempty"`
	Deprovision OperationMetric `json:"deprovision,omitempty"`
}

// BareMetalHostStatus defines the observed state of BareMetalHost
type BareMetalHostStatus struct {
	// Important: Run "operator-sdk generate k8s" to regenerate code
	// after modifying this file

	// OperationalStatus holds the status of the host
	// +kubebuilder:validation:Enum="";OK;discovered;error
	OperationalStatus OperationalStatus `json:"operationalStatus"`

	// ErrorType indicates the type of failure encountered when the
	// OperationalStatus is OperationalStatusError
	// +kubebuilder:validation:Enum=registration error;inspection error;provisioning error;power management error
	ErrorType ErrorType `json:"errorType,omitempty"`

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
	GoodCredentials CredentialsStatus `json:"goodCredentials,omitempty"`

	// the last credentials we sent to the provisioning backend
	TriedCredentials CredentialsStatus `json:"triedCredentials,omitempty"`

	// the last error message reported by the provisioning subsystem
	ErrorMessage string `json:"errorMessage"`

	// indicator for whether or not the host is powered on
	PoweredOn bool `json:"poweredOn"`

	// OperationHistory holds information about operations performed
	// on this host.
	OperationHistory OperationHistory `json:"operationHistory"`

	// ErrorCount records how many times the host has encoutered an error since the last successful operation
	ErrorCount int `json:"errorCount"`
}

// ProvisionStatus holds the state information for a single target.
type ProvisionStatus struct {
	// An indiciator for what the provisioner is doing with the host.
	State ProvisioningState `json:"state"`

	// The machine's UUID from the underlying provisioning tool
	ID string `json:"ID"`

	// Image holds the details of the last image successfully
	// provisioned to the host.
	Image Image `json:"image,omitempty"`

	// The RootDevicehints set by the user
	RootDeviceHints *RootDeviceHints `json:"rootDeviceHints,omitempty"`

	// BootMode indicates the boot mode used to provision the node
	BootMode BootMode `json:"bootMode,omitempty"`

	// The Bios set by the user
	Firmware *FirmwareConfig `json:"firmware,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// BareMetalHost is the Schema for the baremetalhosts API
// +k8s:openapi-gen=true
// +kubebuilder:resource:shortName=bmh;bmhost
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.operationalStatus",description="Operational status"
// +kubebuilder:printcolumn:name="Provisioning Status",type="string",JSONPath=".status.provisioning.state",description="Provisioning status"
// +kubebuilder:printcolumn:name="Consumer",type="string",JSONPath=".spec.consumerRef.name",description="Consumer using this host"
// +kubebuilder:printcolumn:name="BMC",type="string",JSONPath=".spec.bmc.address",description="Address of management controller"
// +kubebuilder:printcolumn:name="Hardware Profile",type="string",JSONPath=".status.hardwareProfile",description="The type of hardware detected"
// +kubebuilder:printcolumn:name="Online",type="string",JSONPath=".spec.online",description="Whether the host is online or not"
// +kubebuilder:printcolumn:name="Error",type="string",JSONPath=".status.errorMessage",description="Most recent error"
// +kubebuilder:object:root=true
type BareMetalHost struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BareMetalHostSpec   `json:"spec,omitempty"`
	Status BareMetalHostStatus `json:"status,omitempty"`
}

// BootMode returns the boot method to use for the host.
func (host *BareMetalHost) BootMode() BootMode {
	mode := host.Spec.BootMode
	if mode == "" {
		return DefaultBootMode
	}
	return mode
}

// Available returns true if the host is available to be provisioned.
func (host *BareMetalHost) Available() bool {
	if host.Spec.ConsumerRef != nil {
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
// and increases the ErrorCount
func (host *BareMetalHost) SetErrorMessage(errType ErrorType, message string) {
	host.Status.OperationalStatus = OperationalStatusError
	host.Status.ErrorType = errType
	host.Status.ErrorMessage = message
	host.Status.ErrorCount++
}

// ClearError removes any existing error message.
func (host *BareMetalHost) ClearError() (dirty bool) {
	dirty = host.SetOperationalStatus(OperationalStatusOK)
	var emptyErrType ErrorType = ""
	if host.Status.ErrorType != emptyErrType {
		host.Status.ErrorType = emptyErrType
		dirty = true
	}
	if host.Status.ErrorMessage != "" {
		host.Status.ErrorMessage = ""
		dirty = true
	}
	if host.Status.ErrorCount != 0 {
		host.Status.ErrorCount = 0
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

// HasBMCDetails returns true if the BMC details are set
func (host *BareMetalHost) HasBMCDetails() bool {
	return host.Spec.BMC.Address != "" || host.Spec.BMC.CredentialsName != ""
}

// NeedsHardwareProfile returns true if the profile is not set
func (host *BareMetalHost) NeedsHardwareProfile() bool {
	return host.Status.HardwareProfile == ""
}

// HardwareProfile returns the hardware profile name for the host.
func (host *BareMetalHost) HardwareProfile() string {
	return host.Status.HardwareProfile
}

// SetHardwareProfile updates the hardware profile name and returns
// true when a change is made or false when no change is made.
func (host *BareMetalHost) SetHardwareProfile(name string) (dirty bool) {
	if host.Status.HardwareProfile != name {
		host.Status.HardwareProfile = name
		dirty = true
	}
	return dirty
}

// SetOperationalStatus updates the OperationalStatus field and returns
// true when a change is made or false when no change is made.
func (host *BareMetalHost) SetOperationalStatus(status OperationalStatus) bool {
	if host.Status.OperationalStatus != status {
		host.Status.OperationalStatus = status
		return true
	}
	return false
}

// OperationalStatus returns the contents of the OperationalStatus
// field.
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

// NeedsHardwareInspection looks at the state of the host to determine
// if hardware inspection should be run.
func (host *BareMetalHost) NeedsHardwareInspection() bool {
	if host.Spec.ExternallyProvisioned {
		// Never perform inspection if we already know something is
		// using the host and we didn't provision it.
		return false
	}
	if host.WasProvisioned() {
		// Never perform inspection if we have already provisioned
		// this host, because we don't want to reboot it.
		return false
	}
	return host.Status.HardwareDetails == nil
}

// NeedsProvisioning compares the settings with the provisioning
// status and returns true when more work is needed or false
// otherwise.
func (host *BareMetalHost) NeedsProvisioning() bool {
	if !host.Spec.Online {
		// The host is not supposed to be powered on.
		return false
	}
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
	return false
}

// WasProvisioned returns true when we think we have placed an image
// on the host.
func (host *BareMetalHost) WasProvisioned() bool {
	if host.Spec.ExternallyProvisioned {
		return false
	}
	if host.Status.Provisioning.Image.URL != "" {
		// We have an image provisioned.
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

// UpdateTriedCredentials modifies the TriedCredentials portion of the
// Status struct to record the details of the secret containing
// credentials known to work.
func (host *BareMetalHost) UpdateTriedCredentials(currentSecret corev1.Secret) {
	host.Status.TriedCredentials.Version = currentSecret.ObjectMeta.ResourceVersion
	host.Status.TriedCredentials.Reference = &corev1.SecretReference{
		Name:      currentSecret.ObjectMeta.Name,
		Namespace: currentSecret.ObjectMeta.Namespace,
	}
}

// NewEvent creates a new event associated with the object and ready
// to be published to the kubernetes API.
func (host *BareMetalHost) NewEvent(reason, message string) corev1.Event {
	t := metav1.Now()
	return corev1.Event{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: reason + "-",
			Namespace:    host.ObjectMeta.Namespace,
		},
		InvolvedObject: corev1.ObjectReference{
			Kind:       "BareMetalHost",
			Namespace:  host.Namespace,
			Name:       host.Name,
			UID:        host.UID,
			APIVersion: GroupVersion.String(),
		},
		Reason:  reason,
		Message: message,
		Source: corev1.EventSource{
			Component: "metal3-baremetal-controller",
		},
		FirstTimestamp:      t,
		LastTimestamp:       t,
		Count:               1,
		Type:                corev1.EventTypeNormal,
		ReportingController: "metal3.io/baremetal-controller",
		Related:             host.Spec.ConsumerRef,
	}
}

// OperationMetricForState returns a pointer to the metric for the given
// provisioning state.
func (host *BareMetalHost) OperationMetricForState(operation ProvisioningState) (metric *OperationMetric) {
	history := &host.Status.OperationHistory
	switch operation {
	case StateRegistering:
		metric = &history.Register
	case StateInspecting:
		metric = &history.Inspect
	case StateProvisioning:
		metric = &history.Provision
	case StateDeprovisioning:
		metric = &history.Deprovision
	}
	return
}

// GetImageChecksum returns the hash value and its algo.
func (host *BareMetalHost) GetImageChecksum() (string, string, bool) {
	if host.Spec.Image == nil {
		return "", "", false
	}

	checksum := host.Spec.Image.Checksum
	checksumType := host.Spec.Image.ChecksumType

	if checksum == "" {
		// Return empty if checksum is not provided
		return "", "", false
	}
	if checksumType == "" {
		// If only checksum is specified. Assume type is md5
		return checksum, string(MD5), true
	}
	switch checksumType {
	case MD5, SHA256, SHA512:
		return checksum, string(checksumType), true
	default:
		return "", "", false
	}
}

// +kubebuilder:object:root=true

// BareMetalHostList contains a list of BareMetalHost
type BareMetalHostList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BareMetalHost `json:"items"`
}

func init() {
	SchemeBuilder.Register(&BareMetalHost{}, &BareMetalHostList{})
}
