# API Reference

Packages:

- [metal3.io/v1alpha1](#metal3iov1alpha1)

# metal3.io/v1alpha1

Resource Types:

- [BareMetalHost](#baremetalhost)

- [BMCEventSubscription](#bmceventsubscription)

- [DataImage](#dataimage)

- [FirmwareSchema](#firmwareschema)

- [HardwareData](#hardwaredata)

- [HostFirmwareComponents](#hostfirmwarecomponents)

- [HostFirmwareSettings](#hostfirmwaresettings)

- [HostUpdatePolicy](#hostupdatepolicy)

- [PreprovisioningImage](#preprovisioningimage)




## BareMetalHost
<sup><sup>[↩ Parent](#metal3iov1alpha1 )</sup></sup>






BareMetalHost is the Schema for the baremetalhosts API

<table>
    <thead>
        <tr>
            <th>Name</th>
            <th>Type</th>
            <th>Description</th>
            <th>Required</th>
        </tr>
    </thead>
    <tbody><tr>
      <td><b>apiVersion</b></td>
      <td>string</td>
      <td>metal3.io/v1alpha1</td>
      <td>true</td>
      </tr>
      <tr>
      <td><b>kind</b></td>
      <td>string</td>
      <td>BareMetalHost</td>
      <td>true</td>
      </tr>
      <tr>
      <td><b><a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.20/#objectmeta-v1-meta">metadata</a></b></td>
      <td>object</td>
      <td>Refer to the Kubernetes API documentation for the fields of the `metadata` field.</td>
      <td>true</td>
      </tr><tr>
        <td><b><a href="#baremetalhostspec">spec</a></b></td>
        <td>object</td>
        <td>
          BareMetalHostSpec defines the desired state of BareMetalHost.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b><a href="#baremetalhoststatus">status</a></b></td>
        <td>object</td>
        <td>
          BareMetalHostStatus defines the observed state of BareMetalHost.<br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>


### BareMetalHost.spec
<sup><sup>[↩ Parent](#baremetalhost)</sup></sup>



BareMetalHostSpec defines the desired state of BareMetalHost.

<table>
    <thead>
        <tr>
            <th>Name</th>
            <th>Type</th>
            <th>Description</th>
            <th>Required</th>
        </tr>
    </thead>
    <tbody><tr>
        <td><b>online</b></td>
        <td>boolean</td>
        <td>
          Should the host be powered on? If the host is currently in a stable
state (e.g. provisioned), its power state will be forced to match
this value.<br/>
        </td>
        <td>true</td>
      </tr><tr>
        <td><b>architecture</b></td>
        <td>string</td>
        <td>
          CPU architecture of the host, e.g. "x86_64" or "aarch64". If unset,
eventually populated by inspection.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>automatedCleaningMode</b></td>
        <td>enum</td>
        <td>
          When set to disabled, automated cleaning will be skipped
during provisioning and deprovisioning.<br/>
          <br/>
            <i>Enum</i>: metadata, disabled<br/>
            <i>Default</i>: metadata<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b><a href="#baremetalhostspecbmc">bmc</a></b></td>
        <td>object</td>
        <td>
          How do we connect to the BMC (Baseboard Management Controller) on
the host?<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>bootMACAddress</b></td>
        <td>string</td>
        <td>
          The MAC address of the NIC used for provisioning the host. In case
of network boot, this is the MAC address of the PXE booting
interface. The MAC address of the BMC must never be used here!<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>bootMode</b></td>
        <td>enum</td>
        <td>
          Select the method of initializing the hardware during boot.
Defaults to UEFI. Legacy boot should only be used for hardware that
does not support UEFI correctly. Set to UEFISecureBoot to turn
secure boot on automatically after provisioning.<br/>
          <br/>
            <i>Enum</i>: UEFI, UEFISecureBoot, legacy<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b><a href="#baremetalhostspecconsumerref">consumerRef</a></b></td>
        <td>object</td>
        <td>
          ConsumerRef can be used to store information about something
that is using a host. When it is not empty, the host is
considered "in use". The common use case is a link to a Machine
resource when the host is used by Cluster API.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b><a href="#baremetalhostspeccustomdeploy">customDeploy</a></b></td>
        <td>object</td>
        <td>
          A custom deploy procedure. This is an advanced feature that allows
using a custom deploy step provided by a site-specific deployment
ramdisk. Most users will want to use "image" instead. Setting this
field triggers provisioning.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>description</b></td>
        <td>string</td>
        <td>
          Description is a human-entered text used to help identify the host.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>disablePowerOff</b></td>
        <td>boolean</td>
        <td>
          When set to true, power off of the node will be disabled,
instead, a reboot will be used in place of power on/off<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>externallyProvisioned</b></td>
        <td>boolean</td>
        <td>
          ExternallyProvisioned means something else has provisioned the
image running on the host, and the operator should only manage
the power status. This field is used for integration with already
provisioned hosts and when pivoting hosts between clusters. If
unsure, leave this field as false.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b><a href="#baremetalhostspecfirmware">firmware</a></b></td>
        <td>object</td>
        <td>
          Firmware (BIOS) configuration for bare metal server. If set, the
requested settings will be applied before the host is provisioned.
Only some vendor drivers support this field. An alternative is to
use HostFirmwareSettings resources that allow changing arbitrary
values and support the generic Redfish-based drivers.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>hardwareProfile</b></td>
        <td>string</td>
        <td>
          What is the name of the hardware profile for this host?
Hardware profiles are deprecated and should not be used.
Use the separate fields Architecture and RootDeviceHints instead.
Set to "empty" to prepare for the future version of the API
without hardware profiles.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b><a href="#baremetalhostspecimage">image</a></b></td>
        <td>object</td>
        <td>
          Image holds the details of the image to be provisioned. Populating
the image will cause the host to start provisioning.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>inspectionMode</b></td>
        <td>enum</td>
        <td>
          Specifies the mode for host inspection.
"disabled" - no inspection will be performed
"agent" - normal agent-based inspection will run<br/>
          <br/>
            <i>Enum</i>: disabled, agent<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b><a href="#baremetalhostspecmetadata">metaData</a></b></td>
        <td>object</td>
        <td>
          MetaData holds the reference to the Secret containing host metadata
which is passed to the Config Drive. By default, metadata will be
generated for the host, so most users do not need to set this field.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b><a href="#baremetalhostspecnetworkdata">networkData</a></b></td>
        <td>object</td>
        <td>
          NetworkData holds the reference to the Secret containing network
configuration which is passed to the Config Drive and interpreted
by the first boot software such as cloud-init.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>preprovisioningNetworkDataName</b></td>
        <td>string</td>
        <td>
          PreprovisioningNetworkDataName is the name of the Secret in the
local namespace containing network configuration which is passed to
the preprovisioning image, and to the Config Drive if not overridden
by specifying NetworkData.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b><a href="#baremetalhostspecraid">raid</a></b></td>
        <td>object</td>
        <td>
          RAID configuration for bare metal server. If set, the RAID settings
will be applied before the host is provisioned. If not, the current
settings will not be modified. Only one of the sub-fields
hardwareRAIDVolumes and softwareRAIDVolumes can be set at the same
time.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b><a href="#baremetalhostspecrootdevicehints">rootDeviceHints</a></b></td>
        <td>object</td>
        <td>
          Provide guidance about how to choose the device for the image
being provisioned. The default is currently to use /dev/sda as
the root device.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b><a href="#baremetalhostspectaintsindex">taints</a></b></td>
        <td>[]object</td>
        <td>
          Taints is the full, authoritative list of taints to apply to
the corresponding Machine. This list will overwrite any
modifications made to the Machine on an ongoing basis.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b><a href="#baremetalhostspecuserdata">userData</a></b></td>
        <td>object</td>
        <td>
          UserData holds the reference to the Secret containing the user data
which is passed to the Config Drive and interpreted by the
first-boot software such as cloud-init. The format of user data is
specific to the first-boot software.<br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>


### BareMetalHost.spec.bmc
<sup><sup>[↩ Parent](#baremetalhostspec)</sup></sup>



How do we connect to the BMC (Baseboard Management Controller) on
the host?

<table>
    <thead>
        <tr>
            <th>Name</th>
            <th>Type</th>
            <th>Description</th>
            <th>Required</th>
        </tr>
    </thead>
    <tbody><tr>
        <td><b>address</b></td>
        <td>string</td>
        <td>
          Address holds the URL for accessing the controller on the network.
The scheme part designates the driver to use with the host.<br/>
        </td>
        <td>true</td>
      </tr><tr>
        <td><b>credentialsName</b></td>
        <td>string</td>
        <td>
          The name of the secret containing the BMC credentials (requires
keys "username" and "password").<br/>
        </td>
        <td>true</td>
      </tr><tr>
        <td><b>disableCertificateVerification</b></td>
        <td>boolean</td>
        <td>
          DisableCertificateVerification disables verification of server
certificates when using HTTPS to connect to the BMC. This is
required when the server certificate is self-signed, but is
insecure because it allows a man-in-the-middle to intercept the
connection.<br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>


### BareMetalHost.spec.consumerRef
<sup><sup>[↩ Parent](#baremetalhostspec)</sup></sup>



ConsumerRef can be used to store information about something
that is using a host. When it is not empty, the host is
considered "in use". The common use case is a link to a Machine
resource when the host is used by Cluster API.

<table>
    <thead>
        <tr>
            <th>Name</th>
            <th>Type</th>
            <th>Description</th>
            <th>Required</th>
        </tr>
    </thead>
    <tbody><tr>
        <td><b>apiVersion</b></td>
        <td>string</td>
        <td>
          API version of the referent.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>fieldPath</b></td>
        <td>string</td>
        <td>
          If referring to a piece of an object instead of an entire object, this string
should contain a valid JSON/Go field access statement, such as desiredState.manifest.containers[2].
For example, if the object reference is to a container within a pod, this would take on a value like:
"spec.containers{name}" (where "name" refers to the name of the container that triggered
the event) or if no container name is specified "spec.containers[2]" (container with
index 2 in this pod). This syntax is chosen only to have some well-defined way of
referencing a part of an object.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>kind</b></td>
        <td>string</td>
        <td>
          Kind of the referent.
More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>name</b></td>
        <td>string</td>
        <td>
          Name of the referent.
More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#names<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>namespace</b></td>
        <td>string</td>
        <td>
          Namespace of the referent.
More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/namespaces/<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>resourceVersion</b></td>
        <td>string</td>
        <td>
          Specific resourceVersion to which this reference is made, if any.
More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#concurrency-control-and-consistency<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>uid</b></td>
        <td>string</td>
        <td>
          UID of the referent.
More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#uids<br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>


### BareMetalHost.spec.customDeploy
<sup><sup>[↩ Parent](#baremetalhostspec)</sup></sup>



A custom deploy procedure. This is an advanced feature that allows
using a custom deploy step provided by a site-specific deployment
ramdisk. Most users will want to use "image" instead. Setting this
field triggers provisioning.

<table>
    <thead>
        <tr>
            <th>Name</th>
            <th>Type</th>
            <th>Description</th>
            <th>Required</th>
        </tr>
    </thead>
    <tbody><tr>
        <td><b>method</b></td>
        <td>string</td>
        <td>
          Custom deploy method name.
This name is specific to the deploy ramdisk used. If you don't have
a custom deploy ramdisk, you shouldn't use CustomDeploy.<br/>
        </td>
        <td>true</td>
      </tr></tbody>
</table>


### BareMetalHost.spec.firmware
<sup><sup>[↩ Parent](#baremetalhostspec)</sup></sup>



Firmware (BIOS) configuration for bare metal server. If set, the
requested settings will be applied before the host is provisioned.
Only some vendor drivers support this field. An alternative is to
use HostFirmwareSettings resources that allow changing arbitrary
values and support the generic Redfish-based drivers.

<table>
    <thead>
        <tr>
            <th>Name</th>
            <th>Type</th>
            <th>Description</th>
            <th>Required</th>
        </tr>
    </thead>
    <tbody><tr>
        <td><b>simultaneousMultithreadingEnabled</b></td>
        <td>boolean</td>
        <td>
          Allows a single physical processor core to appear as several logical processors.<br/>
          <br/>
            <i>Enum</i>: true, false<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>sriovEnabled</b></td>
        <td>boolean</td>
        <td>
          SR-IOV support enables a hypervisor to create virtual instances of a PCI-express device, potentially increasing performance.<br/>
          <br/>
            <i>Enum</i>: true, false<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>virtualizationEnabled</b></td>
        <td>boolean</td>
        <td>
          Supports the virtualization of platform hardware.<br/>
          <br/>
            <i>Enum</i>: true, false<br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>


### BareMetalHost.spec.image
<sup><sup>[↩ Parent](#baremetalhostspec)</sup></sup>



Image holds the details of the image to be provisioned. Populating
the image will cause the host to start provisioning.

<table>
    <thead>
        <tr>
            <th>Name</th>
            <th>Type</th>
            <th>Description</th>
            <th>Required</th>
        </tr>
    </thead>
    <tbody><tr>
        <td><b>url</b></td>
        <td>string</td>
        <td>
          URL is a location of an image to deploy.<br/>
        </td>
        <td>true</td>
      </tr><tr>
        <td><b>checksum</b></td>
        <td>string</td>
        <td>
          Checksum is the checksum for the image. Required for all formats
except for "live-iso".<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>checksumType</b></td>
        <td>enum</td>
        <td>
          ChecksumType is the checksum algorithm for the image, e.g md5, sha256 or sha512.
The special value "auto" can be used to detect the algorithm from the checksum.
If missing, MD5 is used. If in doubt, use "auto".<br/>
          <br/>
            <i>Enum</i>: md5, sha256, sha512, auto<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>format</b></td>
        <td>enum</td>
        <td>
          Format contains the format of the image (raw, qcow2, ...).
When set to "live-iso", an ISO 9660 image referenced by the url will
be live-booted and not deployed to disk.<br/>
          <br/>
            <i>Enum</i>: raw, qcow2, vdi, vmdk, live-iso<br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>


### BareMetalHost.spec.metaData
<sup><sup>[↩ Parent](#baremetalhostspec)</sup></sup>



MetaData holds the reference to the Secret containing host metadata
which is passed to the Config Drive. By default, metadata will be
generated for the host, so most users do not need to set this field.

<table>
    <thead>
        <tr>
            <th>Name</th>
            <th>Type</th>
            <th>Description</th>
            <th>Required</th>
        </tr>
    </thead>
    <tbody><tr>
        <td><b>name</b></td>
        <td>string</td>
        <td>
          name is unique within a namespace to reference a secret resource.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>namespace</b></td>
        <td>string</td>
        <td>
          namespace defines the space within which the secret name must be unique.<br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>


### BareMetalHost.spec.networkData
<sup><sup>[↩ Parent](#baremetalhostspec)</sup></sup>



NetworkData holds the reference to the Secret containing network
configuration which is passed to the Config Drive and interpreted
by the first boot software such as cloud-init.

<table>
    <thead>
        <tr>
            <th>Name</th>
            <th>Type</th>
            <th>Description</th>
            <th>Required</th>
        </tr>
    </thead>
    <tbody><tr>
        <td><b>name</b></td>
        <td>string</td>
        <td>
          name is unique within a namespace to reference a secret resource.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>namespace</b></td>
        <td>string</td>
        <td>
          namespace defines the space within which the secret name must be unique.<br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>


### BareMetalHost.spec.raid
<sup><sup>[↩ Parent](#baremetalhostspec)</sup></sup>



RAID configuration for bare metal server. If set, the RAID settings
will be applied before the host is provisioned. If not, the current
settings will not be modified. Only one of the sub-fields
hardwareRAIDVolumes and softwareRAIDVolumes can be set at the same
time.

<table>
    <thead>
        <tr>
            <th>Name</th>
            <th>Type</th>
            <th>Description</th>
            <th>Required</th>
        </tr>
    </thead>
    <tbody><tr>
        <td><b><a href="#baremetalhostspecraidhardwareraidvolumesindex">hardwareRAIDVolumes</a></b></td>
        <td>[]object</td>
        <td>
          The list of logical disks for hardware RAID, if rootDeviceHints isn't used, first volume is root volume.
You can set the value of this field to `[]` to clear all the hardware RAID configurations.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b><a href="#baremetalhostspecraidsoftwareraidvolumesindex">softwareRAIDVolumes</a></b></td>
        <td>[]object</td>
        <td>
          The list of logical disks for software RAID, if rootDeviceHints isn't used, first volume is root volume.
If HardwareRAIDVolumes is set this item will be invalid.
The number of created Software RAID devices must be 1 or 2.
If there is only one Software RAID device, it has to be a RAID-1.
If there are two, the first one has to be a RAID-1, while the RAID level for the second one can be 0, 1, or 1+0.
As the first RAID device will be the deployment device,
enforcing a RAID-1 reduces the risk of ending up with a non-booting host in case of a disk failure.
Software RAID will always be deleted.<br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>


### BareMetalHost.spec.raid.hardwareRAIDVolumes[index]
<sup><sup>[↩ Parent](#baremetalhostspecraid)</sup></sup>



HardwareRAIDVolume defines the desired configuration of volume in hardware RAID.

<table>
    <thead>
        <tr>
            <th>Name</th>
            <th>Type</th>
            <th>Description</th>
            <th>Required</th>
        </tr>
    </thead>
    <tbody><tr>
        <td><b>level</b></td>
        <td>enum</td>
        <td>
          RAID level for the logical disk. The following levels are supported:
0, 1, 2, 5, 6, 1+0, 5+0, 6+0 (drivers may support only some of them).<br/>
          <br/>
            <i>Enum</i>: 0, 1, 2, 5, 6, 1+0, 5+0, 6+0<br/>
        </td>
        <td>true</td>
      </tr><tr>
        <td><b>controller</b></td>
        <td>string</td>
        <td>
          The name of the RAID controller to use.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>name</b></td>
        <td>string</td>
        <td>
          Name of the volume. Should be unique within the Node. If not
specified, the name will be auto-generated.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>numberOfPhysicalDisks</b></td>
        <td>integer</td>
        <td>
          Integer, number of physical disks to use for the logical disk.
Defaults to minimum number of disks required for the particular RAID
level.<br/>
          <br/>
            <i>Minimum</i>: 1<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>physicalDisks</b></td>
        <td>[]string</td>
        <td>
          Optional list of physical disk names to be used for the hardware RAID volumes. The disk names are interpreted
by the hardware RAID controller, and the format is hardware specific.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>rotational</b></td>
        <td>boolean</td>
        <td>
          Select disks with only rotational (if set to true) or solid-state
(if set to false) storage. By default, any disks can be picked.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>sizeGibibytes</b></td>
        <td>integer</td>
        <td>
          Size of the logical disk to be created in GiB. If unspecified or
set be 0, the maximum capacity of disk will be used for logical
disk.<br/>
          <br/>
            <i>Minimum</i>: 0<br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>


### BareMetalHost.spec.raid.softwareRAIDVolumes[index]
<sup><sup>[↩ Parent](#baremetalhostspecraid)</sup></sup>



SoftwareRAIDVolume defines the desired configuration of volume in software RAID.

<table>
    <thead>
        <tr>
            <th>Name</th>
            <th>Type</th>
            <th>Description</th>
            <th>Required</th>
        </tr>
    </thead>
    <tbody><tr>
        <td><b>level</b></td>
        <td>enum</td>
        <td>
          RAID level for the logical disk. The following levels are supported:
0, 1 and 1+0.<br/>
          <br/>
            <i>Enum</i>: 0, 1, 1+0<br/>
        </td>
        <td>true</td>
      </tr><tr>
        <td><b><a href="#baremetalhostspecraidsoftwareraidvolumesindexphysicaldisksindex">physicalDisks</a></b></td>
        <td>[]object</td>
        <td>
          A list of device hints, the number of items should be greater than or equal to 2.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>sizeGibibytes</b></td>
        <td>integer</td>
        <td>
          Size of the logical disk to be created in GiB.
If unspecified or set be 0, the maximum capacity of disk will be used for logical disk.<br/>
          <br/>
            <i>Minimum</i>: 0<br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>


### BareMetalHost.spec.raid.softwareRAIDVolumes[index].physicalDisks[index]
<sup><sup>[↩ Parent](#baremetalhostspecraidsoftwareraidvolumesindex)</sup></sup>



RootDeviceHints holds the hints for specifying the storage location
for the root filesystem for the image.

<table>
    <thead>
        <tr>
            <th>Name</th>
            <th>Type</th>
            <th>Description</th>
            <th>Required</th>
        </tr>
    </thead>
    <tbody><tr>
        <td><b>deviceName</b></td>
        <td>string</td>
        <td>
          A Linux device name like "/dev/vda", or a by-path link to it like
"/dev/disk/by-path/pci-0000:01:00.0-scsi-0:2:0:0". The hint must match
the actual value exactly.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>hctl</b></td>
        <td>string</td>
        <td>
          A SCSI bus address like 0:0:0:0. The hint must match the actual
value exactly.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>minSizeGigabytes</b></td>
        <td>integer</td>
        <td>
          The minimum size of the device in Gigabytes.<br/>
          <br/>
            <i>Minimum</i>: 0<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>model</b></td>
        <td>string</td>
        <td>
          A vendor-specific device identifier. The hint can be a
substring of the actual value.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>rotational</b></td>
        <td>boolean</td>
        <td>
          True if the device should use spinning media, false otherwise.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>serialNumber</b></td>
        <td>string</td>
        <td>
          Device serial number. The hint must match the actual value
exactly.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>vendor</b></td>
        <td>string</td>
        <td>
          The name of the vendor or manufacturer of the device. The hint
can be a substring of the actual value.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>wwn</b></td>
        <td>string</td>
        <td>
          Unique storage identifier. The hint must match the actual value
exactly.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>wwnVendorExtension</b></td>
        <td>string</td>
        <td>
          Unique vendor storage identifier. The hint must match the
actual value exactly.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>wwnWithExtension</b></td>
        <td>string</td>
        <td>
          Unique storage identifier with the vendor extension
appended. The hint must match the actual value exactly.<br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>


### BareMetalHost.spec.rootDeviceHints
<sup><sup>[↩ Parent](#baremetalhostspec)</sup></sup>



Provide guidance about how to choose the device for the image
being provisioned. The default is currently to use /dev/sda as
the root device.

<table>
    <thead>
        <tr>
            <th>Name</th>
            <th>Type</th>
            <th>Description</th>
            <th>Required</th>
        </tr>
    </thead>
    <tbody><tr>
        <td><b>deviceName</b></td>
        <td>string</td>
        <td>
          A Linux device name like "/dev/vda", or a by-path link to it like
"/dev/disk/by-path/pci-0000:01:00.0-scsi-0:2:0:0". The hint must match
the actual value exactly.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>hctl</b></td>
        <td>string</td>
        <td>
          A SCSI bus address like 0:0:0:0. The hint must match the actual
value exactly.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>minSizeGigabytes</b></td>
        <td>integer</td>
        <td>
          The minimum size of the device in Gigabytes.<br/>
          <br/>
            <i>Minimum</i>: 0<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>model</b></td>
        <td>string</td>
        <td>
          A vendor-specific device identifier. The hint can be a
substring of the actual value.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>rotational</b></td>
        <td>boolean</td>
        <td>
          True if the device should use spinning media, false otherwise.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>serialNumber</b></td>
        <td>string</td>
        <td>
          Device serial number. The hint must match the actual value
exactly.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>vendor</b></td>
        <td>string</td>
        <td>
          The name of the vendor or manufacturer of the device. The hint
can be a substring of the actual value.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>wwn</b></td>
        <td>string</td>
        <td>
          Unique storage identifier. The hint must match the actual value
exactly.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>wwnVendorExtension</b></td>
        <td>string</td>
        <td>
          Unique vendor storage identifier. The hint must match the
actual value exactly.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>wwnWithExtension</b></td>
        <td>string</td>
        <td>
          Unique storage identifier with the vendor extension
appended. The hint must match the actual value exactly.<br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>


### BareMetalHost.spec.taints[index]
<sup><sup>[↩ Parent](#baremetalhostspec)</sup></sup>



The node this Taint is attached to has the "effect" on
any pod that does not tolerate the Taint.

<table>
    <thead>
        <tr>
            <th>Name</th>
            <th>Type</th>
            <th>Description</th>
            <th>Required</th>
        </tr>
    </thead>
    <tbody><tr>
        <td><b>effect</b></td>
        <td>string</td>
        <td>
          Required. The effect of the taint on pods
that do not tolerate the taint.
Valid effects are NoSchedule, PreferNoSchedule and NoExecute.<br/>
        </td>
        <td>true</td>
      </tr><tr>
        <td><b>key</b></td>
        <td>string</td>
        <td>
          Required. The taint key to be applied to a node.<br/>
        </td>
        <td>true</td>
      </tr><tr>
        <td><b>timeAdded</b></td>
        <td>string</td>
        <td>
          TimeAdded represents the time at which the taint was added.
It is only written for NoExecute taints.<br/>
          <br/>
            <i>Format</i>: date-time<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>value</b></td>
        <td>string</td>
        <td>
          The taint value corresponding to the taint key.<br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>


### BareMetalHost.spec.userData
<sup><sup>[↩ Parent](#baremetalhostspec)</sup></sup>



UserData holds the reference to the Secret containing the user data
which is passed to the Config Drive and interpreted by the
first-boot software such as cloud-init. The format of user data is
specific to the first-boot software.

<table>
    <thead>
        <tr>
            <th>Name</th>
            <th>Type</th>
            <th>Description</th>
            <th>Required</th>
        </tr>
    </thead>
    <tbody><tr>
        <td><b>name</b></td>
        <td>string</td>
        <td>
          name is unique within a namespace to reference a secret resource.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>namespace</b></td>
        <td>string</td>
        <td>
          namespace defines the space within which the secret name must be unique.<br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>


### BareMetalHost.status
<sup><sup>[↩ Parent](#baremetalhost)</sup></sup>



BareMetalHostStatus defines the observed state of BareMetalHost.

<table>
    <thead>
        <tr>
            <th>Name</th>
            <th>Type</th>
            <th>Description</th>
            <th>Required</th>
        </tr>
    </thead>
    <tbody><tr>
        <td><b>errorCount</b></td>
        <td>integer</td>
        <td>
          ErrorCount records how many times the host has encoutered an error since the last successful operation<br/>
          <br/>
            <i>Default</i>: 0<br/>
        </td>
        <td>true</td>
      </tr><tr>
        <td><b>errorMessage</b></td>
        <td>string</td>
        <td>
          The last error message reported by the provisioning subsystem.<br/>
        </td>
        <td>true</td>
      </tr><tr>
        <td><b>operationalStatus</b></td>
        <td>enum</td>
        <td>
          OperationalStatus holds the status of the host<br/>
          <br/>
            <i>Enum</i>: , OK, discovered, error, delayed, detached, servicing<br/>
        </td>
        <td>true</td>
      </tr><tr>
        <td><b>poweredOn</b></td>
        <td>boolean</td>
        <td>
          The currently detected power state of the host. This field may get
briefly out of sync with the actual state of the hardware while
provisioning processes are running.<br/>
        </td>
        <td>true</td>
      </tr><tr>
        <td><b><a href="#baremetalhoststatusprovisioning">provisioning</a></b></td>
        <td>object</td>
        <td>
          Information tracked by the provisioner.<br/>
        </td>
        <td>true</td>
      </tr><tr>
        <td><b>errorType</b></td>
        <td>enum</td>
        <td>
          ErrorType indicates the type of failure encountered when the
OperationalStatus is OperationalStatusError<br/>
          <br/>
            <i>Enum</i>: provisioned registration error, registration error, inspection error, preparation error, provisioning error, power management error, servicing error<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b><a href="#baremetalhoststatusgoodcredentials">goodCredentials</a></b></td>
        <td>object</td>
        <td>
          The last credentials we were able to validate as working.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b><a href="#baremetalhoststatushardware">hardware</a></b></td>
        <td>object</td>
        <td>
          The hardware discovered to exist on the host.
This field will be removed in the next API version in favour of the
separate HardwareData resource.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>hardwareProfile</b></td>
        <td>string</td>
        <td>
          The name of the profile matching the hardware details.
Hardware profiles are deprecated and should not be relied on.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>lastUpdated</b></td>
        <td>string</td>
        <td>
          LastUpdated identifies when this status was last observed.<br/>
          <br/>
            <i>Format</i>: date-time<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b><a href="#baremetalhoststatusoperationhistory">operationHistory</a></b></td>
        <td>object</td>
        <td>
          OperationHistory holds information about operations performed
on this host.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b><a href="#baremetalhoststatustriedcredentials">triedCredentials</a></b></td>
        <td>object</td>
        <td>
          The last credentials we sent to the provisioning backend.<br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>


### BareMetalHost.status.provisioning
<sup><sup>[↩ Parent](#baremetalhoststatus)</sup></sup>



Information tracked by the provisioner.

<table>
    <thead>
        <tr>
            <th>Name</th>
            <th>Type</th>
            <th>Description</th>
            <th>Required</th>
        </tr>
    </thead>
    <tbody><tr>
        <td><b>ID</b></td>
        <td>string</td>
        <td>
          The hosts's ID from the underlying provisioning tool (e.g. the
Ironic node UUID).<br/>
        </td>
        <td>true</td>
      </tr><tr>
        <td><b>state</b></td>
        <td>string</td>
        <td>
          An indicator for what the provisioner is doing with the host.<br/>
        </td>
        <td>true</td>
      </tr><tr>
        <td><b>bootMode</b></td>
        <td>enum</td>
        <td>
          BootMode indicates the boot mode used to provision the host.<br/>
          <br/>
            <i>Enum</i>: UEFI, UEFISecureBoot, legacy<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b><a href="#baremetalhoststatusprovisioningcustomdeploy">customDeploy</a></b></td>
        <td>object</td>
        <td>
          Custom deploy procedure applied to the host.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b><a href="#baremetalhoststatusprovisioningfirmware">firmware</a></b></td>
        <td>object</td>
        <td>
          The firmware settings that have been applied.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b><a href="#baremetalhoststatusprovisioningimage">image</a></b></td>
        <td>object</td>
        <td>
          Image holds the details of the last image successfully
provisioned to the host.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b><a href="#baremetalhoststatusprovisioningraid">raid</a></b></td>
        <td>object</td>
        <td>
          The RAID configuration that has been applied.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b><a href="#baremetalhoststatusprovisioningrootdevicehints">rootDeviceHints</a></b></td>
        <td>object</td>
        <td>
          The root device hints used to provision the host.<br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>


### BareMetalHost.status.provisioning.customDeploy
<sup><sup>[↩ Parent](#baremetalhoststatusprovisioning)</sup></sup>



Custom deploy procedure applied to the host.

<table>
    <thead>
        <tr>
            <th>Name</th>
            <th>Type</th>
            <th>Description</th>
            <th>Required</th>
        </tr>
    </thead>
    <tbody><tr>
        <td><b>method</b></td>
        <td>string</td>
        <td>
          Custom deploy method name.
This name is specific to the deploy ramdisk used. If you don't have
a custom deploy ramdisk, you shouldn't use CustomDeploy.<br/>
        </td>
        <td>true</td>
      </tr></tbody>
</table>


### BareMetalHost.status.provisioning.firmware
<sup><sup>[↩ Parent](#baremetalhoststatusprovisioning)</sup></sup>



The firmware settings that have been applied.

<table>
    <thead>
        <tr>
            <th>Name</th>
            <th>Type</th>
            <th>Description</th>
            <th>Required</th>
        </tr>
    </thead>
    <tbody><tr>
        <td><b>simultaneousMultithreadingEnabled</b></td>
        <td>boolean</td>
        <td>
          Allows a single physical processor core to appear as several logical processors.<br/>
          <br/>
            <i>Enum</i>: true, false<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>sriovEnabled</b></td>
        <td>boolean</td>
        <td>
          SR-IOV support enables a hypervisor to create virtual instances of a PCI-express device, potentially increasing performance.<br/>
          <br/>
            <i>Enum</i>: true, false<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>virtualizationEnabled</b></td>
        <td>boolean</td>
        <td>
          Supports the virtualization of platform hardware.<br/>
          <br/>
            <i>Enum</i>: true, false<br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>


### BareMetalHost.status.provisioning.image
<sup><sup>[↩ Parent](#baremetalhoststatusprovisioning)</sup></sup>



Image holds the details of the last image successfully
provisioned to the host.

<table>
    <thead>
        <tr>
            <th>Name</th>
            <th>Type</th>
            <th>Description</th>
            <th>Required</th>
        </tr>
    </thead>
    <tbody><tr>
        <td><b>url</b></td>
        <td>string</td>
        <td>
          URL is a location of an image to deploy.<br/>
        </td>
        <td>true</td>
      </tr><tr>
        <td><b>checksum</b></td>
        <td>string</td>
        <td>
          Checksum is the checksum for the image. Required for all formats
except for "live-iso".<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>checksumType</b></td>
        <td>enum</td>
        <td>
          ChecksumType is the checksum algorithm for the image, e.g md5, sha256 or sha512.
The special value "auto" can be used to detect the algorithm from the checksum.
If missing, MD5 is used. If in doubt, use "auto".<br/>
          <br/>
            <i>Enum</i>: md5, sha256, sha512, auto<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>format</b></td>
        <td>enum</td>
        <td>
          Format contains the format of the image (raw, qcow2, ...).
When set to "live-iso", an ISO 9660 image referenced by the url will
be live-booted and not deployed to disk.<br/>
          <br/>
            <i>Enum</i>: raw, qcow2, vdi, vmdk, live-iso<br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>


### BareMetalHost.status.provisioning.raid
<sup><sup>[↩ Parent](#baremetalhoststatusprovisioning)</sup></sup>



The RAID configuration that has been applied.

<table>
    <thead>
        <tr>
            <th>Name</th>
            <th>Type</th>
            <th>Description</th>
            <th>Required</th>
        </tr>
    </thead>
    <tbody><tr>
        <td><b><a href="#baremetalhoststatusprovisioningraidhardwareraidvolumesindex">hardwareRAIDVolumes</a></b></td>
        <td>[]object</td>
        <td>
          The list of logical disks for hardware RAID, if rootDeviceHints isn't used, first volume is root volume.
You can set the value of this field to `[]` to clear all the hardware RAID configurations.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b><a href="#baremetalhoststatusprovisioningraidsoftwareraidvolumesindex">softwareRAIDVolumes</a></b></td>
        <td>[]object</td>
        <td>
          The list of logical disks for software RAID, if rootDeviceHints isn't used, first volume is root volume.
If HardwareRAIDVolumes is set this item will be invalid.
The number of created Software RAID devices must be 1 or 2.
If there is only one Software RAID device, it has to be a RAID-1.
If there are two, the first one has to be a RAID-1, while the RAID level for the second one can be 0, 1, or 1+0.
As the first RAID device will be the deployment device,
enforcing a RAID-1 reduces the risk of ending up with a non-booting host in case of a disk failure.
Software RAID will always be deleted.<br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>


### BareMetalHost.status.provisioning.raid.hardwareRAIDVolumes[index]
<sup><sup>[↩ Parent](#baremetalhoststatusprovisioningraid)</sup></sup>



HardwareRAIDVolume defines the desired configuration of volume in hardware RAID.

<table>
    <thead>
        <tr>
            <th>Name</th>
            <th>Type</th>
            <th>Description</th>
            <th>Required</th>
        </tr>
    </thead>
    <tbody><tr>
        <td><b>level</b></td>
        <td>enum</td>
        <td>
          RAID level for the logical disk. The following levels are supported:
0, 1, 2, 5, 6, 1+0, 5+0, 6+0 (drivers may support only some of them).<br/>
          <br/>
            <i>Enum</i>: 0, 1, 2, 5, 6, 1+0, 5+0, 6+0<br/>
        </td>
        <td>true</td>
      </tr><tr>
        <td><b>controller</b></td>
        <td>string</td>
        <td>
          The name of the RAID controller to use.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>name</b></td>
        <td>string</td>
        <td>
          Name of the volume. Should be unique within the Node. If not
specified, the name will be auto-generated.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>numberOfPhysicalDisks</b></td>
        <td>integer</td>
        <td>
          Integer, number of physical disks to use for the logical disk.
Defaults to minimum number of disks required for the particular RAID
level.<br/>
          <br/>
            <i>Minimum</i>: 1<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>physicalDisks</b></td>
        <td>[]string</td>
        <td>
          Optional list of physical disk names to be used for the hardware RAID volumes. The disk names are interpreted
by the hardware RAID controller, and the format is hardware specific.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>rotational</b></td>
        <td>boolean</td>
        <td>
          Select disks with only rotational (if set to true) or solid-state
(if set to false) storage. By default, any disks can be picked.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>sizeGibibytes</b></td>
        <td>integer</td>
        <td>
          Size of the logical disk to be created in GiB. If unspecified or
set be 0, the maximum capacity of disk will be used for logical
disk.<br/>
          <br/>
            <i>Minimum</i>: 0<br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>


### BareMetalHost.status.provisioning.raid.softwareRAIDVolumes[index]
<sup><sup>[↩ Parent](#baremetalhoststatusprovisioningraid)</sup></sup>



SoftwareRAIDVolume defines the desired configuration of volume in software RAID.

<table>
    <thead>
        <tr>
            <th>Name</th>
            <th>Type</th>
            <th>Description</th>
            <th>Required</th>
        </tr>
    </thead>
    <tbody><tr>
        <td><b>level</b></td>
        <td>enum</td>
        <td>
          RAID level for the logical disk. The following levels are supported:
0, 1 and 1+0.<br/>
          <br/>
            <i>Enum</i>: 0, 1, 1+0<br/>
        </td>
        <td>true</td>
      </tr><tr>
        <td><b><a href="#baremetalhoststatusprovisioningraidsoftwareraidvolumesindexphysicaldisksindex">physicalDisks</a></b></td>
        <td>[]object</td>
        <td>
          A list of device hints, the number of items should be greater than or equal to 2.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>sizeGibibytes</b></td>
        <td>integer</td>
        <td>
          Size of the logical disk to be created in GiB.
If unspecified or set be 0, the maximum capacity of disk will be used for logical disk.<br/>
          <br/>
            <i>Minimum</i>: 0<br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>


### BareMetalHost.status.provisioning.raid.softwareRAIDVolumes[index].physicalDisks[index]
<sup><sup>[↩ Parent](#baremetalhoststatusprovisioningraidsoftwareraidvolumesindex)</sup></sup>



RootDeviceHints holds the hints for specifying the storage location
for the root filesystem for the image.

<table>
    <thead>
        <tr>
            <th>Name</th>
            <th>Type</th>
            <th>Description</th>
            <th>Required</th>
        </tr>
    </thead>
    <tbody><tr>
        <td><b>deviceName</b></td>
        <td>string</td>
        <td>
          A Linux device name like "/dev/vda", or a by-path link to it like
"/dev/disk/by-path/pci-0000:01:00.0-scsi-0:2:0:0". The hint must match
the actual value exactly.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>hctl</b></td>
        <td>string</td>
        <td>
          A SCSI bus address like 0:0:0:0. The hint must match the actual
value exactly.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>minSizeGigabytes</b></td>
        <td>integer</td>
        <td>
          The minimum size of the device in Gigabytes.<br/>
          <br/>
            <i>Minimum</i>: 0<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>model</b></td>
        <td>string</td>
        <td>
          A vendor-specific device identifier. The hint can be a
substring of the actual value.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>rotational</b></td>
        <td>boolean</td>
        <td>
          True if the device should use spinning media, false otherwise.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>serialNumber</b></td>
        <td>string</td>
        <td>
          Device serial number. The hint must match the actual value
exactly.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>vendor</b></td>
        <td>string</td>
        <td>
          The name of the vendor or manufacturer of the device. The hint
can be a substring of the actual value.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>wwn</b></td>
        <td>string</td>
        <td>
          Unique storage identifier. The hint must match the actual value
exactly.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>wwnVendorExtension</b></td>
        <td>string</td>
        <td>
          Unique vendor storage identifier. The hint must match the
actual value exactly.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>wwnWithExtension</b></td>
        <td>string</td>
        <td>
          Unique storage identifier with the vendor extension
appended. The hint must match the actual value exactly.<br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>


### BareMetalHost.status.provisioning.rootDeviceHints
<sup><sup>[↩ Parent](#baremetalhoststatusprovisioning)</sup></sup>



The root device hints used to provision the host.

<table>
    <thead>
        <tr>
            <th>Name</th>
            <th>Type</th>
            <th>Description</th>
            <th>Required</th>
        </tr>
    </thead>
    <tbody><tr>
        <td><b>deviceName</b></td>
        <td>string</td>
        <td>
          A Linux device name like "/dev/vda", or a by-path link to it like
"/dev/disk/by-path/pci-0000:01:00.0-scsi-0:2:0:0". The hint must match
the actual value exactly.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>hctl</b></td>
        <td>string</td>
        <td>
          A SCSI bus address like 0:0:0:0. The hint must match the actual
value exactly.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>minSizeGigabytes</b></td>
        <td>integer</td>
        <td>
          The minimum size of the device in Gigabytes.<br/>
          <br/>
            <i>Minimum</i>: 0<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>model</b></td>
        <td>string</td>
        <td>
          A vendor-specific device identifier. The hint can be a
substring of the actual value.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>rotational</b></td>
        <td>boolean</td>
        <td>
          True if the device should use spinning media, false otherwise.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>serialNumber</b></td>
        <td>string</td>
        <td>
          Device serial number. The hint must match the actual value
exactly.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>vendor</b></td>
        <td>string</td>
        <td>
          The name of the vendor or manufacturer of the device. The hint
can be a substring of the actual value.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>wwn</b></td>
        <td>string</td>
        <td>
          Unique storage identifier. The hint must match the actual value
exactly.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>wwnVendorExtension</b></td>
        <td>string</td>
        <td>
          Unique vendor storage identifier. The hint must match the
actual value exactly.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>wwnWithExtension</b></td>
        <td>string</td>
        <td>
          Unique storage identifier with the vendor extension
appended. The hint must match the actual value exactly.<br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>


### BareMetalHost.status.goodCredentials
<sup><sup>[↩ Parent](#baremetalhoststatus)</sup></sup>



The last credentials we were able to validate as working.

<table>
    <thead>
        <tr>
            <th>Name</th>
            <th>Type</th>
            <th>Description</th>
            <th>Required</th>
        </tr>
    </thead>
    <tbody><tr>
        <td><b><a href="#baremetalhoststatusgoodcredentialscredentials">credentials</a></b></td>
        <td>object</td>
        <td>
          SecretReference represents a Secret Reference. It has enough information to retrieve secret
in any namespace<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>credentialsVersion</b></td>
        <td>string</td>
        <td>
          <br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>


### BareMetalHost.status.goodCredentials.credentials
<sup><sup>[↩ Parent](#baremetalhoststatusgoodcredentials)</sup></sup>



SecretReference represents a Secret Reference. It has enough information to retrieve secret
in any namespace

<table>
    <thead>
        <tr>
            <th>Name</th>
            <th>Type</th>
            <th>Description</th>
            <th>Required</th>
        </tr>
    </thead>
    <tbody><tr>
        <td><b>name</b></td>
        <td>string</td>
        <td>
          name is unique within a namespace to reference a secret resource.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>namespace</b></td>
        <td>string</td>
        <td>
          namespace defines the space within which the secret name must be unique.<br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>


### BareMetalHost.status.hardware
<sup><sup>[↩ Parent](#baremetalhoststatus)</sup></sup>



The hardware discovered to exist on the host.
This field will be removed in the next API version in favour of the
separate HardwareData resource.

<table>
    <thead>
        <tr>
            <th>Name</th>
            <th>Type</th>
            <th>Description</th>
            <th>Required</th>
        </tr>
    </thead>
    <tbody><tr>
        <td><b><a href="#baremetalhoststatushardwarecpu">cpu</a></b></td>
        <td>object</td>
        <td>
          Details of the CPU(s) in the system.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b><a href="#baremetalhoststatushardwarefirmware">firmware</a></b></td>
        <td>object</td>
        <td>
          System firmware information.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>hostname</b></td>
        <td>string</td>
        <td>
          <br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b><a href="#baremetalhoststatushardwarenicsindex">nics</a></b></td>
        <td>[]object</td>
        <td>
          List of network interfaces for the host.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>ramMebibytes</b></td>
        <td>integer</td>
        <td>
          The host's amount of memory in Mebibytes.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b><a href="#baremetalhoststatushardwarestorageindex">storage</a></b></td>
        <td>[]object</td>
        <td>
          List of storage (disk, SSD, etc.) available to the host.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b><a href="#baremetalhoststatushardwaresystemvendor">systemVendor</a></b></td>
        <td>object</td>
        <td>
          System vendor information.<br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>


### BareMetalHost.status.hardware.cpu
<sup><sup>[↩ Parent](#baremetalhoststatushardware)</sup></sup>



Details of the CPU(s) in the system.

<table>
    <thead>
        <tr>
            <th>Name</th>
            <th>Type</th>
            <th>Description</th>
            <th>Required</th>
        </tr>
    </thead>
    <tbody><tr>
        <td><b>arch</b></td>
        <td>string</td>
        <td>
          <br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>clockMegahertz</b></td>
        <td>number</td>
        <td>
          ClockSpeed is a clock speed in MHz<br/>
          <br/>
            <i>Format</i>: double<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>count</b></td>
        <td>integer</td>
        <td>
          <br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>flags</b></td>
        <td>[]string</td>
        <td>
          <br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>model</b></td>
        <td>string</td>
        <td>
          <br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>


### BareMetalHost.status.hardware.firmware
<sup><sup>[↩ Parent](#baremetalhoststatushardware)</sup></sup>



System firmware information.

<table>
    <thead>
        <tr>
            <th>Name</th>
            <th>Type</th>
            <th>Description</th>
            <th>Required</th>
        </tr>
    </thead>
    <tbody><tr>
        <td><b><a href="#baremetalhoststatushardwarefirmwarebios">bios</a></b></td>
        <td>object</td>
        <td>
          The BIOS for this firmware<br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>


### BareMetalHost.status.hardware.firmware.bios
<sup><sup>[↩ Parent](#baremetalhoststatushardwarefirmware)</sup></sup>



The BIOS for this firmware

<table>
    <thead>
        <tr>
            <th>Name</th>
            <th>Type</th>
            <th>Description</th>
            <th>Required</th>
        </tr>
    </thead>
    <tbody><tr>
        <td><b>date</b></td>
        <td>string</td>
        <td>
          The release/build date for this BIOS<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>vendor</b></td>
        <td>string</td>
        <td>
          The vendor name for this BIOS<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>version</b></td>
        <td>string</td>
        <td>
          The version of the BIOS<br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>


### BareMetalHost.status.hardware.nics[index]
<sup><sup>[↩ Parent](#baremetalhoststatushardware)</sup></sup>



NIC describes one network interface on the host.

<table>
    <thead>
        <tr>
            <th>Name</th>
            <th>Type</th>
            <th>Description</th>
            <th>Required</th>
        </tr>
    </thead>
    <tbody><tr>
        <td><b>ip</b></td>
        <td>string</td>
        <td>
          The IP address of the interface. This will be an IPv4 or IPv6 address
if one is present.  If both IPv4 and IPv6 addresses are present in a
dual-stack environment, two nics will be output, one with each IP.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>mac</b></td>
        <td>string</td>
        <td>
          The device MAC address<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>model</b></td>
        <td>string</td>
        <td>
          The vendor and product IDs of the NIC, e.g. "0x8086 0x1572"<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>name</b></td>
        <td>string</td>
        <td>
          The name of the network interface, e.g. "en0"<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>pxe</b></td>
        <td>boolean</td>
        <td>
          Whether the NIC is PXE Bootable<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>speedGbps</b></td>
        <td>integer</td>
        <td>
          The speed of the device in Gigabits per second<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>vlanId</b></td>
        <td>integer</td>
        <td>
          The untagged VLAN ID<br/>
          <br/>
            <i>Format</i>: int32<br/>
            <i>Minimum</i>: 0<br/>
            <i>Maximum</i>: 4094<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b><a href="#baremetalhoststatushardwarenicsindexvlansindex">vlans</a></b></td>
        <td>[]object</td>
        <td>
          The VLANs available<br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>


### BareMetalHost.status.hardware.nics[index].vlans[index]
<sup><sup>[↩ Parent](#baremetalhoststatushardwarenicsindex)</sup></sup>



VLAN represents the name and ID of a VLAN.

<table>
    <thead>
        <tr>
            <th>Name</th>
            <th>Type</th>
            <th>Description</th>
            <th>Required</th>
        </tr>
    </thead>
    <tbody><tr>
        <td><b>id</b></td>
        <td>integer</td>
        <td>
          VLANID is a 12-bit 802.1Q VLAN identifier<br/>
          <br/>
            <i>Format</i>: int32<br/>
            <i>Minimum</i>: 0<br/>
            <i>Maximum</i>: 4094<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>name</b></td>
        <td>string</td>
        <td>
          <br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>


### BareMetalHost.status.hardware.storage[index]
<sup><sup>[↩ Parent](#baremetalhoststatushardware)</sup></sup>



Storage describes one storage device (disk, SSD, etc.) on the host.

<table>
    <thead>
        <tr>
            <th>Name</th>
            <th>Type</th>
            <th>Description</th>
            <th>Required</th>
        </tr>
    </thead>
    <tbody><tr>
        <td><b>alternateNames</b></td>
        <td>[]string</td>
        <td>
          A list of alternate Linux device names of the disk, e.g. "/dev/sda".
Note that this list is not exhaustive, and names may not be stable
across reboots.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>hctl</b></td>
        <td>string</td>
        <td>
          The SCSI location of the device<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>model</b></td>
        <td>string</td>
        <td>
          Hardware model<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>name</b></td>
        <td>string</td>
        <td>
          A Linux device name of the disk, e.g.
"/dev/disk/by-path/pci-0000:01:00.0-scsi-0:2:0:0". This will be a name
that is stable across reboots if one is available.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>rotational</b></td>
        <td>boolean</td>
        <td>
          Whether this disk represents rotational storage.
This field is not recommended for usage, please
prefer using 'Type' field instead, this field
will be deprecated eventually.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>serialNumber</b></td>
        <td>string</td>
        <td>
          The serial number of the device<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>sizeBytes</b></td>
        <td>integer</td>
        <td>
          The size of the disk in Bytes<br/>
          <br/>
            <i>Format</i>: int64<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>type</b></td>
        <td>enum</td>
        <td>
          Device type, one of: HDD, SSD, NVME.<br/>
          <br/>
            <i>Enum</i>: HDD, SSD, NVME<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>vendor</b></td>
        <td>string</td>
        <td>
          The name of the vendor of the device<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>wwn</b></td>
        <td>string</td>
        <td>
          The WWN of the device<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>wwnVendorExtension</b></td>
        <td>string</td>
        <td>
          The WWN Vendor extension of the device<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>wwnWithExtension</b></td>
        <td>string</td>
        <td>
          The WWN with the extension<br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>


### BareMetalHost.status.hardware.systemVendor
<sup><sup>[↩ Parent](#baremetalhoststatushardware)</sup></sup>



System vendor information.

<table>
    <thead>
        <tr>
            <th>Name</th>
            <th>Type</th>
            <th>Description</th>
            <th>Required</th>
        </tr>
    </thead>
    <tbody><tr>
        <td><b>manufacturer</b></td>
        <td>string</td>
        <td>
          <br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>productName</b></td>
        <td>string</td>
        <td>
          <br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>serialNumber</b></td>
        <td>string</td>
        <td>
          <br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>


### BareMetalHost.status.operationHistory
<sup><sup>[↩ Parent](#baremetalhoststatus)</sup></sup>



OperationHistory holds information about operations performed
on this host.

<table>
    <thead>
        <tr>
            <th>Name</th>
            <th>Type</th>
            <th>Description</th>
            <th>Required</th>
        </tr>
    </thead>
    <tbody><tr>
        <td><b><a href="#baremetalhoststatusoperationhistorydeprovision">deprovision</a></b></td>
        <td>object</td>
        <td>
          OperationMetric contains metadata about an operation (inspection,
provisioning, etc.) used for tracking metrics.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b><a href="#baremetalhoststatusoperationhistoryinspect">inspect</a></b></td>
        <td>object</td>
        <td>
          OperationMetric contains metadata about an operation (inspection,
provisioning, etc.) used for tracking metrics.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b><a href="#baremetalhoststatusoperationhistoryprovision">provision</a></b></td>
        <td>object</td>
        <td>
          OperationMetric contains metadata about an operation (inspection,
provisioning, etc.) used for tracking metrics.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b><a href="#baremetalhoststatusoperationhistoryregister">register</a></b></td>
        <td>object</td>
        <td>
          OperationMetric contains metadata about an operation (inspection,
provisioning, etc.) used for tracking metrics.<br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>


### BareMetalHost.status.operationHistory.deprovision
<sup><sup>[↩ Parent](#baremetalhoststatusoperationhistory)</sup></sup>



OperationMetric contains metadata about an operation (inspection,
provisioning, etc.) used for tracking metrics.

<table>
    <thead>
        <tr>
            <th>Name</th>
            <th>Type</th>
            <th>Description</th>
            <th>Required</th>
        </tr>
    </thead>
    <tbody><tr>
        <td><b>end</b></td>
        <td>string</td>
        <td>
          <br/>
          <br/>
            <i>Format</i>: date-time<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>start</b></td>
        <td>string</td>
        <td>
          <br/>
          <br/>
            <i>Format</i>: date-time<br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>


### BareMetalHost.status.operationHistory.inspect
<sup><sup>[↩ Parent](#baremetalhoststatusoperationhistory)</sup></sup>



OperationMetric contains metadata about an operation (inspection,
provisioning, etc.) used for tracking metrics.

<table>
    <thead>
        <tr>
            <th>Name</th>
            <th>Type</th>
            <th>Description</th>
            <th>Required</th>
        </tr>
    </thead>
    <tbody><tr>
        <td><b>end</b></td>
        <td>string</td>
        <td>
          <br/>
          <br/>
            <i>Format</i>: date-time<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>start</b></td>
        <td>string</td>
        <td>
          <br/>
          <br/>
            <i>Format</i>: date-time<br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>


### BareMetalHost.status.operationHistory.provision
<sup><sup>[↩ Parent](#baremetalhoststatusoperationhistory)</sup></sup>



OperationMetric contains metadata about an operation (inspection,
provisioning, etc.) used for tracking metrics.

<table>
    <thead>
        <tr>
            <th>Name</th>
            <th>Type</th>
            <th>Description</th>
            <th>Required</th>
        </tr>
    </thead>
    <tbody><tr>
        <td><b>end</b></td>
        <td>string</td>
        <td>
          <br/>
          <br/>
            <i>Format</i>: date-time<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>start</b></td>
        <td>string</td>
        <td>
          <br/>
          <br/>
            <i>Format</i>: date-time<br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>


### BareMetalHost.status.operationHistory.register
<sup><sup>[↩ Parent](#baremetalhoststatusoperationhistory)</sup></sup>



OperationMetric contains metadata about an operation (inspection,
provisioning, etc.) used for tracking metrics.

<table>
    <thead>
        <tr>
            <th>Name</th>
            <th>Type</th>
            <th>Description</th>
            <th>Required</th>
        </tr>
    </thead>
    <tbody><tr>
        <td><b>end</b></td>
        <td>string</td>
        <td>
          <br/>
          <br/>
            <i>Format</i>: date-time<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>start</b></td>
        <td>string</td>
        <td>
          <br/>
          <br/>
            <i>Format</i>: date-time<br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>


### BareMetalHost.status.triedCredentials
<sup><sup>[↩ Parent](#baremetalhoststatus)</sup></sup>



The last credentials we sent to the provisioning backend.

<table>
    <thead>
        <tr>
            <th>Name</th>
            <th>Type</th>
            <th>Description</th>
            <th>Required</th>
        </tr>
    </thead>
    <tbody><tr>
        <td><b><a href="#baremetalhoststatustriedcredentialscredentials">credentials</a></b></td>
        <td>object</td>
        <td>
          SecretReference represents a Secret Reference. It has enough information to retrieve secret
in any namespace<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>credentialsVersion</b></td>
        <td>string</td>
        <td>
          <br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>


### BareMetalHost.status.triedCredentials.credentials
<sup><sup>[↩ Parent](#baremetalhoststatustriedcredentials)</sup></sup>



SecretReference represents a Secret Reference. It has enough information to retrieve secret
in any namespace

<table>
    <thead>
        <tr>
            <th>Name</th>
            <th>Type</th>
            <th>Description</th>
            <th>Required</th>
        </tr>
    </thead>
    <tbody><tr>
        <td><b>name</b></td>
        <td>string</td>
        <td>
          name is unique within a namespace to reference a secret resource.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>namespace</b></td>
        <td>string</td>
        <td>
          namespace defines the space within which the secret name must be unique.<br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>

## BMCEventSubscription
<sup><sup>[↩ Parent](#metal3iov1alpha1 )</sup></sup>






BMCEventSubscription is the Schema for the fast eventing API

<table>
    <thead>
        <tr>
            <th>Name</th>
            <th>Type</th>
            <th>Description</th>
            <th>Required</th>
        </tr>
    </thead>
    <tbody><tr>
      <td><b>apiVersion</b></td>
      <td>string</td>
      <td>metal3.io/v1alpha1</td>
      <td>true</td>
      </tr>
      <tr>
      <td><b>kind</b></td>
      <td>string</td>
      <td>BMCEventSubscription</td>
      <td>true</td>
      </tr>
      <tr>
      <td><b><a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.20/#objectmeta-v1-meta">metadata</a></b></td>
      <td>object</td>
      <td>Refer to the Kubernetes API documentation for the fields of the `metadata` field.</td>
      <td>true</td>
      </tr><tr>
        <td><b><a href="#bmceventsubscriptionspec">spec</a></b></td>
        <td>object</td>
        <td>
          <br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b><a href="#bmceventsubscriptionstatus">status</a></b></td>
        <td>object</td>
        <td>
          <br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>


### BMCEventSubscription.spec
<sup><sup>[↩ Parent](#bmceventsubscription)</sup></sup>





<table>
    <thead>
        <tr>
            <th>Name</th>
            <th>Type</th>
            <th>Description</th>
            <th>Required</th>
        </tr>
    </thead>
    <tbody><tr>
        <td><b>context</b></td>
        <td>string</td>
        <td>
          Arbitrary user-provided context for the event<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>destination</b></td>
        <td>string</td>
        <td>
          A webhook URL to send events to<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>hostName</b></td>
        <td>string</td>
        <td>
          A reference to a BareMetalHost<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b><a href="#bmceventsubscriptionspechttpheadersref">httpHeadersRef</a></b></td>
        <td>object</td>
        <td>
          A secret containing HTTP headers which should be passed along to the Destination
when making a request<br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>


### BMCEventSubscription.spec.httpHeadersRef
<sup><sup>[↩ Parent](#bmceventsubscriptionspec)</sup></sup>



A secret containing HTTP headers which should be passed along to the Destination
when making a request

<table>
    <thead>
        <tr>
            <th>Name</th>
            <th>Type</th>
            <th>Description</th>
            <th>Required</th>
        </tr>
    </thead>
    <tbody><tr>
        <td><b>name</b></td>
        <td>string</td>
        <td>
          name is unique within a namespace to reference a secret resource.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>namespace</b></td>
        <td>string</td>
        <td>
          namespace defines the space within which the secret name must be unique.<br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>


### BMCEventSubscription.status
<sup><sup>[↩ Parent](#bmceventsubscription)</sup></sup>





<table>
    <thead>
        <tr>
            <th>Name</th>
            <th>Type</th>
            <th>Description</th>
            <th>Required</th>
        </tr>
    </thead>
    <tbody><tr>
        <td><b>error</b></td>
        <td>string</td>
        <td>
          <br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>subscriptionID</b></td>
        <td>string</td>
        <td>
          <br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>

## DataImage
<sup><sup>[↩ Parent](#metal3iov1alpha1 )</sup></sup>






DataImage is the Schema for the dataimages API.

<table>
    <thead>
        <tr>
            <th>Name</th>
            <th>Type</th>
            <th>Description</th>
            <th>Required</th>
        </tr>
    </thead>
    <tbody><tr>
      <td><b>apiVersion</b></td>
      <td>string</td>
      <td>metal3.io/v1alpha1</td>
      <td>true</td>
      </tr>
      <tr>
      <td><b>kind</b></td>
      <td>string</td>
      <td>DataImage</td>
      <td>true</td>
      </tr>
      <tr>
      <td><b><a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.20/#objectmeta-v1-meta">metadata</a></b></td>
      <td>object</td>
      <td>Refer to the Kubernetes API documentation for the fields of the `metadata` field.</td>
      <td>true</td>
      </tr><tr>
        <td><b><a href="#dataimagespec">spec</a></b></td>
        <td>object</td>
        <td>
          DataImageSpec defines the desired state of DataImage.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b><a href="#dataimagestatus">status</a></b></td>
        <td>object</td>
        <td>
          DataImageStatus defines the observed state of DataImage.<br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>


### DataImage.spec
<sup><sup>[↩ Parent](#dataimage)</sup></sup>



DataImageSpec defines the desired state of DataImage.

<table>
    <thead>
        <tr>
            <th>Name</th>
            <th>Type</th>
            <th>Description</th>
            <th>Required</th>
        </tr>
    </thead>
    <tbody><tr>
        <td><b>url</b></td>
        <td>string</td>
        <td>
          Url is the address of the dataImage that we want to attach
to a BareMetalHost<br/>
        </td>
        <td>true</td>
      </tr></tbody>
</table>


### DataImage.status
<sup><sup>[↩ Parent](#dataimage)</sup></sup>



DataImageStatus defines the observed state of DataImage.

<table>
    <thead>
        <tr>
            <th>Name</th>
            <th>Type</th>
            <th>Description</th>
            <th>Required</th>
        </tr>
    </thead>
    <tbody><tr>
        <td><b><a href="#dataimagestatusattachedimage">attachedImage</a></b></td>
        <td>object</td>
        <td>
          Currently attached DataImage<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b><a href="#dataimagestatuserror">error</a></b></td>
        <td>object</td>
        <td>
          Error count and message when attaching/detaching<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>lastReconciled</b></td>
        <td>string</td>
        <td>
          Time of last reconciliation<br/>
          <br/>
            <i>Format</i>: date-time<br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>


### DataImage.status.attachedImage
<sup><sup>[↩ Parent](#dataimagestatus)</sup></sup>



Currently attached DataImage

<table>
    <thead>
        <tr>
            <th>Name</th>
            <th>Type</th>
            <th>Description</th>
            <th>Required</th>
        </tr>
    </thead>
    <tbody><tr>
        <td><b>url</b></td>
        <td>string</td>
        <td>
          <br/>
        </td>
        <td>true</td>
      </tr></tbody>
</table>


### DataImage.status.error
<sup><sup>[↩ Parent](#dataimagestatus)</sup></sup>



Error count and message when attaching/detaching

<table>
    <thead>
        <tr>
            <th>Name</th>
            <th>Type</th>
            <th>Description</th>
            <th>Required</th>
        </tr>
    </thead>
    <tbody><tr>
        <td><b>count</b></td>
        <td>integer</td>
        <td>
          <br/>
        </td>
        <td>true</td>
      </tr><tr>
        <td><b>message</b></td>
        <td>string</td>
        <td>
          <br/>
        </td>
        <td>true</td>
      </tr></tbody>
</table>

## FirmwareSchema
<sup><sup>[↩ Parent](#metal3iov1alpha1 )</sup></sup>






FirmwareSchema is the Schema for the firmwareschemas API.

<table>
    <thead>
        <tr>
            <th>Name</th>
            <th>Type</th>
            <th>Description</th>
            <th>Required</th>
        </tr>
    </thead>
    <tbody><tr>
      <td><b>apiVersion</b></td>
      <td>string</td>
      <td>metal3.io/v1alpha1</td>
      <td>true</td>
      </tr>
      <tr>
      <td><b>kind</b></td>
      <td>string</td>
      <td>FirmwareSchema</td>
      <td>true</td>
      </tr>
      <tr>
      <td><b><a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.20/#objectmeta-v1-meta">metadata</a></b></td>
      <td>object</td>
      <td>Refer to the Kubernetes API documentation for the fields of the `metadata` field.</td>
      <td>true</td>
      </tr><tr>
        <td><b><a href="#firmwareschemaspec">spec</a></b></td>
        <td>object</td>
        <td>
          FirmwareSchemaSpec defines the desired state of FirmwareSchema.<br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>


### FirmwareSchema.spec
<sup><sup>[↩ Parent](#firmwareschema)</sup></sup>



FirmwareSchemaSpec defines the desired state of FirmwareSchema.

<table>
    <thead>
        <tr>
            <th>Name</th>
            <th>Type</th>
            <th>Description</th>
            <th>Required</th>
        </tr>
    </thead>
    <tbody><tr>
        <td><b><a href="#firmwareschemaspecschemakey">schema</a></b></td>
        <td>map[string]object</td>
        <td>
          Map of firmware name to schema<br/>
        </td>
        <td>true</td>
      </tr><tr>
        <td><b>hardwareModel</b></td>
        <td>string</td>
        <td>
          The hardware model associated with this schema<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>hardwareVendor</b></td>
        <td>string</td>
        <td>
          The hardware vendor associated with this schema<br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>


### FirmwareSchema.spec.schema[key]
<sup><sup>[↩ Parent](#firmwareschemaspec)</sup></sup>



Additional data describing the firmware setting.

<table>
    <thead>
        <tr>
            <th>Name</th>
            <th>Type</th>
            <th>Description</th>
            <th>Required</th>
        </tr>
    </thead>
    <tbody><tr>
        <td><b>allowable_values</b></td>
        <td>[]string</td>
        <td>
          The allowable value for an Enumeration type setting.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>attribute_type</b></td>
        <td>enum</td>
        <td>
          The type of setting.<br/>
          <br/>
            <i>Enum</i>: Enumeration, String, Integer, Boolean, Password<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>lower_bound</b></td>
        <td>integer</td>
        <td>
          The lowest value for an Integer type setting.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>max_length</b></td>
        <td>integer</td>
        <td>
          Maximum length for a String type setting.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>min_length</b></td>
        <td>integer</td>
        <td>
          Minimum length for a String type setting.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>read_only</b></td>
        <td>boolean</td>
        <td>
          Whether or not this setting is read only.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>unique</b></td>
        <td>boolean</td>
        <td>
          Whether or not this setting's value is unique to this node, e.g.
a serial number.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>upper_bound</b></td>
        <td>integer</td>
        <td>
          The highest value for an Integer type setting.<br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>

## HardwareData
<sup><sup>[↩ Parent](#metal3iov1alpha1 )</sup></sup>






HardwareData is the Schema for the hardwaredata API.

<table>
    <thead>
        <tr>
            <th>Name</th>
            <th>Type</th>
            <th>Description</th>
            <th>Required</th>
        </tr>
    </thead>
    <tbody><tr>
      <td><b>apiVersion</b></td>
      <td>string</td>
      <td>metal3.io/v1alpha1</td>
      <td>true</td>
      </tr>
      <tr>
      <td><b>kind</b></td>
      <td>string</td>
      <td>HardwareData</td>
      <td>true</td>
      </tr>
      <tr>
      <td><b><a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.20/#objectmeta-v1-meta">metadata</a></b></td>
      <td>object</td>
      <td>Refer to the Kubernetes API documentation for the fields of the `metadata` field.</td>
      <td>true</td>
      </tr><tr>
        <td><b><a href="#hardwaredataspec">spec</a></b></td>
        <td>object</td>
        <td>
          HardwareDataSpec defines the desired state of HardwareData.<br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>


### HardwareData.spec
<sup><sup>[↩ Parent](#hardwaredata)</sup></sup>



HardwareDataSpec defines the desired state of HardwareData.

<table>
    <thead>
        <tr>
            <th>Name</th>
            <th>Type</th>
            <th>Description</th>
            <th>Required</th>
        </tr>
    </thead>
    <tbody><tr>
        <td><b><a href="#hardwaredataspechardware">hardware</a></b></td>
        <td>object</td>
        <td>
          The hardware discovered on the host during its inspection.<br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>


### HardwareData.spec.hardware
<sup><sup>[↩ Parent](#hardwaredataspec)</sup></sup>



The hardware discovered on the host during its inspection.

<table>
    <thead>
        <tr>
            <th>Name</th>
            <th>Type</th>
            <th>Description</th>
            <th>Required</th>
        </tr>
    </thead>
    <tbody><tr>
        <td><b><a href="#hardwaredataspechardwarecpu">cpu</a></b></td>
        <td>object</td>
        <td>
          Details of the CPU(s) in the system.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b><a href="#hardwaredataspechardwarefirmware">firmware</a></b></td>
        <td>object</td>
        <td>
          System firmware information.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>hostname</b></td>
        <td>string</td>
        <td>
          <br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b><a href="#hardwaredataspechardwarenicsindex">nics</a></b></td>
        <td>[]object</td>
        <td>
          List of network interfaces for the host.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>ramMebibytes</b></td>
        <td>integer</td>
        <td>
          The host's amount of memory in Mebibytes.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b><a href="#hardwaredataspechardwarestorageindex">storage</a></b></td>
        <td>[]object</td>
        <td>
          List of storage (disk, SSD, etc.) available to the host.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b><a href="#hardwaredataspechardwaresystemvendor">systemVendor</a></b></td>
        <td>object</td>
        <td>
          System vendor information.<br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>


### HardwareData.spec.hardware.cpu
<sup><sup>[↩ Parent](#hardwaredataspechardware)</sup></sup>



Details of the CPU(s) in the system.

<table>
    <thead>
        <tr>
            <th>Name</th>
            <th>Type</th>
            <th>Description</th>
            <th>Required</th>
        </tr>
    </thead>
    <tbody><tr>
        <td><b>arch</b></td>
        <td>string</td>
        <td>
          <br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>clockMegahertz</b></td>
        <td>number</td>
        <td>
          ClockSpeed is a clock speed in MHz<br/>
          <br/>
            <i>Format</i>: double<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>count</b></td>
        <td>integer</td>
        <td>
          <br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>flags</b></td>
        <td>[]string</td>
        <td>
          <br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>model</b></td>
        <td>string</td>
        <td>
          <br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>


### HardwareData.spec.hardware.firmware
<sup><sup>[↩ Parent](#hardwaredataspechardware)</sup></sup>



System firmware information.

<table>
    <thead>
        <tr>
            <th>Name</th>
            <th>Type</th>
            <th>Description</th>
            <th>Required</th>
        </tr>
    </thead>
    <tbody><tr>
        <td><b><a href="#hardwaredataspechardwarefirmwarebios">bios</a></b></td>
        <td>object</td>
        <td>
          The BIOS for this firmware<br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>


### HardwareData.spec.hardware.firmware.bios
<sup><sup>[↩ Parent](#hardwaredataspechardwarefirmware)</sup></sup>



The BIOS for this firmware

<table>
    <thead>
        <tr>
            <th>Name</th>
            <th>Type</th>
            <th>Description</th>
            <th>Required</th>
        </tr>
    </thead>
    <tbody><tr>
        <td><b>date</b></td>
        <td>string</td>
        <td>
          The release/build date for this BIOS<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>vendor</b></td>
        <td>string</td>
        <td>
          The vendor name for this BIOS<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>version</b></td>
        <td>string</td>
        <td>
          The version of the BIOS<br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>


### HardwareData.spec.hardware.nics[index]
<sup><sup>[↩ Parent](#hardwaredataspechardware)</sup></sup>



NIC describes one network interface on the host.

<table>
    <thead>
        <tr>
            <th>Name</th>
            <th>Type</th>
            <th>Description</th>
            <th>Required</th>
        </tr>
    </thead>
    <tbody><tr>
        <td><b>ip</b></td>
        <td>string</td>
        <td>
          The IP address of the interface. This will be an IPv4 or IPv6 address
if one is present.  If both IPv4 and IPv6 addresses are present in a
dual-stack environment, two nics will be output, one with each IP.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>mac</b></td>
        <td>string</td>
        <td>
          The device MAC address<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>model</b></td>
        <td>string</td>
        <td>
          The vendor and product IDs of the NIC, e.g. "0x8086 0x1572"<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>name</b></td>
        <td>string</td>
        <td>
          The name of the network interface, e.g. "en0"<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>pxe</b></td>
        <td>boolean</td>
        <td>
          Whether the NIC is PXE Bootable<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>speedGbps</b></td>
        <td>integer</td>
        <td>
          The speed of the device in Gigabits per second<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>vlanId</b></td>
        <td>integer</td>
        <td>
          The untagged VLAN ID<br/>
          <br/>
            <i>Format</i>: int32<br/>
            <i>Minimum</i>: 0<br/>
            <i>Maximum</i>: 4094<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b><a href="#hardwaredataspechardwarenicsindexvlansindex">vlans</a></b></td>
        <td>[]object</td>
        <td>
          The VLANs available<br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>


### HardwareData.spec.hardware.nics[index].vlans[index]
<sup><sup>[↩ Parent](#hardwaredataspechardwarenicsindex)</sup></sup>



VLAN represents the name and ID of a VLAN.

<table>
    <thead>
        <tr>
            <th>Name</th>
            <th>Type</th>
            <th>Description</th>
            <th>Required</th>
        </tr>
    </thead>
    <tbody><tr>
        <td><b>id</b></td>
        <td>integer</td>
        <td>
          VLANID is a 12-bit 802.1Q VLAN identifier<br/>
          <br/>
            <i>Format</i>: int32<br/>
            <i>Minimum</i>: 0<br/>
            <i>Maximum</i>: 4094<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>name</b></td>
        <td>string</td>
        <td>
          <br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>


### HardwareData.spec.hardware.storage[index]
<sup><sup>[↩ Parent](#hardwaredataspechardware)</sup></sup>



Storage describes one storage device (disk, SSD, etc.) on the host.

<table>
    <thead>
        <tr>
            <th>Name</th>
            <th>Type</th>
            <th>Description</th>
            <th>Required</th>
        </tr>
    </thead>
    <tbody><tr>
        <td><b>alternateNames</b></td>
        <td>[]string</td>
        <td>
          A list of alternate Linux device names of the disk, e.g. "/dev/sda".
Note that this list is not exhaustive, and names may not be stable
across reboots.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>hctl</b></td>
        <td>string</td>
        <td>
          The SCSI location of the device<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>model</b></td>
        <td>string</td>
        <td>
          Hardware model<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>name</b></td>
        <td>string</td>
        <td>
          A Linux device name of the disk, e.g.
"/dev/disk/by-path/pci-0000:01:00.0-scsi-0:2:0:0". This will be a name
that is stable across reboots if one is available.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>rotational</b></td>
        <td>boolean</td>
        <td>
          Whether this disk represents rotational storage.
This field is not recommended for usage, please
prefer using 'Type' field instead, this field
will be deprecated eventually.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>serialNumber</b></td>
        <td>string</td>
        <td>
          The serial number of the device<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>sizeBytes</b></td>
        <td>integer</td>
        <td>
          The size of the disk in Bytes<br/>
          <br/>
            <i>Format</i>: int64<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>type</b></td>
        <td>enum</td>
        <td>
          Device type, one of: HDD, SSD, NVME.<br/>
          <br/>
            <i>Enum</i>: HDD, SSD, NVME<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>vendor</b></td>
        <td>string</td>
        <td>
          The name of the vendor of the device<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>wwn</b></td>
        <td>string</td>
        <td>
          The WWN of the device<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>wwnVendorExtension</b></td>
        <td>string</td>
        <td>
          The WWN Vendor extension of the device<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>wwnWithExtension</b></td>
        <td>string</td>
        <td>
          The WWN with the extension<br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>


### HardwareData.spec.hardware.systemVendor
<sup><sup>[↩ Parent](#hardwaredataspechardware)</sup></sup>



System vendor information.

<table>
    <thead>
        <tr>
            <th>Name</th>
            <th>Type</th>
            <th>Description</th>
            <th>Required</th>
        </tr>
    </thead>
    <tbody><tr>
        <td><b>manufacturer</b></td>
        <td>string</td>
        <td>
          <br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>productName</b></td>
        <td>string</td>
        <td>
          <br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>serialNumber</b></td>
        <td>string</td>
        <td>
          <br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>

## HostFirmwareComponents
<sup><sup>[↩ Parent](#metal3iov1alpha1 )</sup></sup>






HostFirmwareComponents is the Schema for the hostfirmwarecomponents API.

<table>
    <thead>
        <tr>
            <th>Name</th>
            <th>Type</th>
            <th>Description</th>
            <th>Required</th>
        </tr>
    </thead>
    <tbody><tr>
      <td><b>apiVersion</b></td>
      <td>string</td>
      <td>metal3.io/v1alpha1</td>
      <td>true</td>
      </tr>
      <tr>
      <td><b>kind</b></td>
      <td>string</td>
      <td>HostFirmwareComponents</td>
      <td>true</td>
      </tr>
      <tr>
      <td><b><a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.20/#objectmeta-v1-meta">metadata</a></b></td>
      <td>object</td>
      <td>Refer to the Kubernetes API documentation for the fields of the `metadata` field.</td>
      <td>true</td>
      </tr><tr>
        <td><b><a href="#hostfirmwarecomponentsspec">spec</a></b></td>
        <td>object</td>
        <td>
          HostFirmwareComponentsSpec defines the desired state of HostFirmwareComponents.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b><a href="#hostfirmwarecomponentsstatus">status</a></b></td>
        <td>object</td>
        <td>
          HostFirmwareComponentsStatus defines the observed state of HostFirmwareComponents.<br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>


### HostFirmwareComponents.spec
<sup><sup>[↩ Parent](#hostfirmwarecomponents)</sup></sup>



HostFirmwareComponentsSpec defines the desired state of HostFirmwareComponents.

<table>
    <thead>
        <tr>
            <th>Name</th>
            <th>Type</th>
            <th>Description</th>
            <th>Required</th>
        </tr>
    </thead>
    <tbody><tr>
        <td><b><a href="#hostfirmwarecomponentsspecupdatesindex">updates</a></b></td>
        <td>[]object</td>
        <td>
          <br/>
        </td>
        <td>true</td>
      </tr></tbody>
</table>


### HostFirmwareComponents.spec.updates[index]
<sup><sup>[↩ Parent](#hostfirmwarecomponentsspec)</sup></sup>



FirmwareUpdate defines a firmware update specification.

<table>
    <thead>
        <tr>
            <th>Name</th>
            <th>Type</th>
            <th>Description</th>
            <th>Required</th>
        </tr>
    </thead>
    <tbody><tr>
        <td><b>component</b></td>
        <td>string</td>
        <td>
          <br/>
        </td>
        <td>true</td>
      </tr><tr>
        <td><b>url</b></td>
        <td>string</td>
        <td>
          <br/>
        </td>
        <td>true</td>
      </tr></tbody>
</table>


### HostFirmwareComponents.status
<sup><sup>[↩ Parent](#hostfirmwarecomponents)</sup></sup>



HostFirmwareComponentsStatus defines the observed state of HostFirmwareComponents.

<table>
    <thead>
        <tr>
            <th>Name</th>
            <th>Type</th>
            <th>Description</th>
            <th>Required</th>
        </tr>
    </thead>
    <tbody><tr>
        <td><b><a href="#hostfirmwarecomponentsstatuscomponentsindex">components</a></b></td>
        <td>[]object</td>
        <td>
          Components is the list of all available firmware components and their information.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b><a href="#hostfirmwarecomponentsstatusconditionsindex">conditions</a></b></td>
        <td>[]object</td>
        <td>
          Track whether updates stored in the spec are valid based on the schema<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>lastUpdated</b></td>
        <td>string</td>
        <td>
          Time that the status was last updated<br/>
          <br/>
            <i>Format</i>: date-time<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b><a href="#hostfirmwarecomponentsstatusupdatesindex">updates</a></b></td>
        <td>[]object</td>
        <td>
          Updates is the list of all firmware components that should be updated
they are specified via name and url fields.<br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>


### HostFirmwareComponents.status.components[index]
<sup><sup>[↩ Parent](#hostfirmwarecomponentsstatus)</sup></sup>



FirmwareComponentStatus defines the status of a firmware component.

<table>
    <thead>
        <tr>
            <th>Name</th>
            <th>Type</th>
            <th>Description</th>
            <th>Required</th>
        </tr>
    </thead>
    <tbody><tr>
        <td><b>component</b></td>
        <td>string</td>
        <td>
          <br/>
        </td>
        <td>true</td>
      </tr><tr>
        <td><b>initialVersion</b></td>
        <td>string</td>
        <td>
          <br/>
        </td>
        <td>true</td>
      </tr><tr>
        <td><b>currentVersion</b></td>
        <td>string</td>
        <td>
          <br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>lastVersionFlashed</b></td>
        <td>string</td>
        <td>
          <br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>updatedAt</b></td>
        <td>string</td>
        <td>
          <br/>
          <br/>
            <i>Format</i>: date-time<br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>


### HostFirmwareComponents.status.conditions[index]
<sup><sup>[↩ Parent](#hostfirmwarecomponentsstatus)</sup></sup>



Condition contains details for one aspect of the current state of this API Resource.

<table>
    <thead>
        <tr>
            <th>Name</th>
            <th>Type</th>
            <th>Description</th>
            <th>Required</th>
        </tr>
    </thead>
    <tbody><tr>
        <td><b>lastTransitionTime</b></td>
        <td>string</td>
        <td>
          lastTransitionTime is the last time the condition transitioned from one status to another.
This should be when the underlying condition changed.  If that is not known, then using the time when the API field changed is acceptable.<br/>
          <br/>
            <i>Format</i>: date-time<br/>
        </td>
        <td>true</td>
      </tr><tr>
        <td><b>message</b></td>
        <td>string</td>
        <td>
          message is a human readable message indicating details about the transition.
This may be an empty string.<br/>
        </td>
        <td>true</td>
      </tr><tr>
        <td><b>reason</b></td>
        <td>string</td>
        <td>
          reason contains a programmatic identifier indicating the reason for the condition's last transition.
Producers of specific condition types may define expected values and meanings for this field,
and whether the values are considered a guaranteed API.
The value should be a CamelCase string.
This field may not be empty.<br/>
        </td>
        <td>true</td>
      </tr><tr>
        <td><b>status</b></td>
        <td>enum</td>
        <td>
          status of the condition, one of True, False, Unknown.<br/>
          <br/>
            <i>Enum</i>: True, False, Unknown<br/>
        </td>
        <td>true</td>
      </tr><tr>
        <td><b>type</b></td>
        <td>string</td>
        <td>
          type of condition in CamelCase or in foo.example.com/CamelCase.<br/>
        </td>
        <td>true</td>
      </tr><tr>
        <td><b>observedGeneration</b></td>
        <td>integer</td>
        <td>
          observedGeneration represents the .metadata.generation that the condition was set based upon.
For instance, if .metadata.generation is currently 12, but the .status.conditions[x].observedGeneration is 9, the condition is out of date
with respect to the current state of the instance.<br/>
          <br/>
            <i>Format</i>: int64<br/>
            <i>Minimum</i>: 0<br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>


### HostFirmwareComponents.status.updates[index]
<sup><sup>[↩ Parent](#hostfirmwarecomponentsstatus)</sup></sup>



FirmwareUpdate defines a firmware update specification.

<table>
    <thead>
        <tr>
            <th>Name</th>
            <th>Type</th>
            <th>Description</th>
            <th>Required</th>
        </tr>
    </thead>
    <tbody><tr>
        <td><b>component</b></td>
        <td>string</td>
        <td>
          <br/>
        </td>
        <td>true</td>
      </tr><tr>
        <td><b>url</b></td>
        <td>string</td>
        <td>
          <br/>
        </td>
        <td>true</td>
      </tr></tbody>
</table>

## HostFirmwareSettings
<sup><sup>[↩ Parent](#metal3iov1alpha1 )</sup></sup>






HostFirmwareSettings is the Schema for the hostfirmwaresettings API.

<table>
    <thead>
        <tr>
            <th>Name</th>
            <th>Type</th>
            <th>Description</th>
            <th>Required</th>
        </tr>
    </thead>
    <tbody><tr>
      <td><b>apiVersion</b></td>
      <td>string</td>
      <td>metal3.io/v1alpha1</td>
      <td>true</td>
      </tr>
      <tr>
      <td><b>kind</b></td>
      <td>string</td>
      <td>HostFirmwareSettings</td>
      <td>true</td>
      </tr>
      <tr>
      <td><b><a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.20/#objectmeta-v1-meta">metadata</a></b></td>
      <td>object</td>
      <td>Refer to the Kubernetes API documentation for the fields of the `metadata` field.</td>
      <td>true</td>
      </tr><tr>
        <td><b><a href="#hostfirmwaresettingsspec">spec</a></b></td>
        <td>object</td>
        <td>
          HostFirmwareSettingsSpec defines the desired state of HostFirmwareSettings.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b><a href="#hostfirmwaresettingsstatus">status</a></b></td>
        <td>object</td>
        <td>
          HostFirmwareSettingsStatus defines the observed state of HostFirmwareSettings.<br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>


### HostFirmwareSettings.spec
<sup><sup>[↩ Parent](#hostfirmwaresettings)</sup></sup>



HostFirmwareSettingsSpec defines the desired state of HostFirmwareSettings.

<table>
    <thead>
        <tr>
            <th>Name</th>
            <th>Type</th>
            <th>Description</th>
            <th>Required</th>
        </tr>
    </thead>
    <tbody><tr>
        <td><b>settings</b></td>
        <td>map[string]int or string</td>
        <td>
          Settings are the desired firmware settings stored as name/value pairs.<br/>
        </td>
        <td>true</td>
      </tr></tbody>
</table>


### HostFirmwareSettings.status
<sup><sup>[↩ Parent](#hostfirmwaresettings)</sup></sup>



HostFirmwareSettingsStatus defines the observed state of HostFirmwareSettings.

<table>
    <thead>
        <tr>
            <th>Name</th>
            <th>Type</th>
            <th>Description</th>
            <th>Required</th>
        </tr>
    </thead>
    <tbody><tr>
        <td><b>settings</b></td>
        <td>map[string]string</td>
        <td>
          Settings are the firmware settings stored as name/value pairs<br/>
        </td>
        <td>true</td>
      </tr><tr>
        <td><b><a href="#hostfirmwaresettingsstatusconditionsindex">conditions</a></b></td>
        <td>[]object</td>
        <td>
          Track whether settings stored in the spec are valid based on the schema<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>lastUpdated</b></td>
        <td>string</td>
        <td>
          Time that the status was last updated<br/>
          <br/>
            <i>Format</i>: date-time<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b><a href="#hostfirmwaresettingsstatusschema">schema</a></b></td>
        <td>object</td>
        <td>
          FirmwareSchema is a reference to the Schema used to describe each
FirmwareSetting. By default, this will be a Schema in the same
Namespace as the settings but it can be overwritten in the Spec<br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>


### HostFirmwareSettings.status.conditions[index]
<sup><sup>[↩ Parent](#hostfirmwaresettingsstatus)</sup></sup>



Condition contains details for one aspect of the current state of this API Resource.

<table>
    <thead>
        <tr>
            <th>Name</th>
            <th>Type</th>
            <th>Description</th>
            <th>Required</th>
        </tr>
    </thead>
    <tbody><tr>
        <td><b>lastTransitionTime</b></td>
        <td>string</td>
        <td>
          lastTransitionTime is the last time the condition transitioned from one status to another.
This should be when the underlying condition changed.  If that is not known, then using the time when the API field changed is acceptable.<br/>
          <br/>
            <i>Format</i>: date-time<br/>
        </td>
        <td>true</td>
      </tr><tr>
        <td><b>message</b></td>
        <td>string</td>
        <td>
          message is a human readable message indicating details about the transition.
This may be an empty string.<br/>
        </td>
        <td>true</td>
      </tr><tr>
        <td><b>reason</b></td>
        <td>string</td>
        <td>
          reason contains a programmatic identifier indicating the reason for the condition's last transition.
Producers of specific condition types may define expected values and meanings for this field,
and whether the values are considered a guaranteed API.
The value should be a CamelCase string.
This field may not be empty.<br/>
        </td>
        <td>true</td>
      </tr><tr>
        <td><b>status</b></td>
        <td>enum</td>
        <td>
          status of the condition, one of True, False, Unknown.<br/>
          <br/>
            <i>Enum</i>: True, False, Unknown<br/>
        </td>
        <td>true</td>
      </tr><tr>
        <td><b>type</b></td>
        <td>string</td>
        <td>
          type of condition in CamelCase or in foo.example.com/CamelCase.<br/>
        </td>
        <td>true</td>
      </tr><tr>
        <td><b>observedGeneration</b></td>
        <td>integer</td>
        <td>
          observedGeneration represents the .metadata.generation that the condition was set based upon.
For instance, if .metadata.generation is currently 12, but the .status.conditions[x].observedGeneration is 9, the condition is out of date
with respect to the current state of the instance.<br/>
          <br/>
            <i>Format</i>: int64<br/>
            <i>Minimum</i>: 0<br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>


### HostFirmwareSettings.status.schema
<sup><sup>[↩ Parent](#hostfirmwaresettingsstatus)</sup></sup>



FirmwareSchema is a reference to the Schema used to describe each
FirmwareSetting. By default, this will be a Schema in the same
Namespace as the settings but it can be overwritten in the Spec

<table>
    <thead>
        <tr>
            <th>Name</th>
            <th>Type</th>
            <th>Description</th>
            <th>Required</th>
        </tr>
    </thead>
    <tbody><tr>
        <td><b>name</b></td>
        <td>string</td>
        <td>
          `name` is the reference to the schema.<br/>
        </td>
        <td>true</td>
      </tr><tr>
        <td><b>namespace</b></td>
        <td>string</td>
        <td>
          `namespace` is the namespace of the where the schema is stored.<br/>
        </td>
        <td>true</td>
      </tr></tbody>
</table>

## HostUpdatePolicy
<sup><sup>[↩ Parent](#metal3iov1alpha1 )</sup></sup>






HostUpdatePolicy is the Schema for the hostupdatepolicy API.

<table>
    <thead>
        <tr>
            <th>Name</th>
            <th>Type</th>
            <th>Description</th>
            <th>Required</th>
        </tr>
    </thead>
    <tbody><tr>
      <td><b>apiVersion</b></td>
      <td>string</td>
      <td>metal3.io/v1alpha1</td>
      <td>true</td>
      </tr>
      <tr>
      <td><b>kind</b></td>
      <td>string</td>
      <td>HostUpdatePolicy</td>
      <td>true</td>
      </tr>
      <tr>
      <td><b><a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.20/#objectmeta-v1-meta">metadata</a></b></td>
      <td>object</td>
      <td>Refer to the Kubernetes API documentation for the fields of the `metadata` field.</td>
      <td>true</td>
      </tr><tr>
        <td><b><a href="#hostupdatepolicyspec">spec</a></b></td>
        <td>object</td>
        <td>
          HostUpdatePolicySpec defines the desired state of HostUpdatePolicy.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>status</b></td>
        <td>object</td>
        <td>
          HostUpdatePolicyStatus defines the observed state of HostUpdatePolicy.<br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>


### HostUpdatePolicy.spec
<sup><sup>[↩ Parent](#hostupdatepolicy)</sup></sup>



HostUpdatePolicySpec defines the desired state of HostUpdatePolicy.

<table>
    <thead>
        <tr>
            <th>Name</th>
            <th>Type</th>
            <th>Description</th>
            <th>Required</th>
        </tr>
    </thead>
    <tbody><tr>
        <td><b>firmwareSettings</b></td>
        <td>enum</td>
        <td>
          Defines policy for changing firmware settings<br/>
          <br/>
            <i>Enum</i>: onPreparing, onReboot<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>firmwareUpdates</b></td>
        <td>enum</td>
        <td>
          Defines policy for updating firmware<br/>
          <br/>
            <i>Enum</i>: onPreparing, onReboot<br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>

## PreprovisioningImage
<sup><sup>[↩ Parent](#metal3iov1alpha1 )</sup></sup>






PreprovisioningImage is the Schema for the preprovisioningimages API.

<table>
    <thead>
        <tr>
            <th>Name</th>
            <th>Type</th>
            <th>Description</th>
            <th>Required</th>
        </tr>
    </thead>
    <tbody><tr>
      <td><b>apiVersion</b></td>
      <td>string</td>
      <td>metal3.io/v1alpha1</td>
      <td>true</td>
      </tr>
      <tr>
      <td><b>kind</b></td>
      <td>string</td>
      <td>PreprovisioningImage</td>
      <td>true</td>
      </tr>
      <tr>
      <td><b><a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.20/#objectmeta-v1-meta">metadata</a></b></td>
      <td>object</td>
      <td>Refer to the Kubernetes API documentation for the fields of the `metadata` field.</td>
      <td>true</td>
      </tr><tr>
        <td><b><a href="#preprovisioningimagespec">spec</a></b></td>
        <td>object</td>
        <td>
          PreprovisioningImageSpec defines the desired state of PreprovisioningImage.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b><a href="#preprovisioningimagestatus">status</a></b></td>
        <td>object</td>
        <td>
          PreprovisioningImageStatus defines the observed state of PreprovisioningImage.<br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>


### PreprovisioningImage.spec
<sup><sup>[↩ Parent](#preprovisioningimage)</sup></sup>



PreprovisioningImageSpec defines the desired state of PreprovisioningImage.

<table>
    <thead>
        <tr>
            <th>Name</th>
            <th>Type</th>
            <th>Description</th>
            <th>Required</th>
        </tr>
    </thead>
    <tbody><tr>
        <td><b>acceptFormats</b></td>
        <td>[]enum</td>
        <td>
          acceptFormats is a list of acceptable image formats.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>architecture</b></td>
        <td>string</td>
        <td>
          architecture is the processor architecture for which to build the image.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>networkDataName</b></td>
        <td>string</td>
        <td>
          networkDataName is the name of a Secret in the local namespace that
contains network data to build in to the image.<br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>


### PreprovisioningImage.status
<sup><sup>[↩ Parent](#preprovisioningimage)</sup></sup>



PreprovisioningImageStatus defines the observed state of PreprovisioningImage.

<table>
    <thead>
        <tr>
            <th>Name</th>
            <th>Type</th>
            <th>Description</th>
            <th>Required</th>
        </tr>
    </thead>
    <tbody><tr>
        <td><b>architecture</b></td>
        <td>string</td>
        <td>
          architecture is the processor architecture for which the image is built<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b><a href="#preprovisioningimagestatusconditionsindex">conditions</a></b></td>
        <td>[]object</td>
        <td>
          conditions describe the state of the built image<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>extraKernelParams</b></td>
        <td>string</td>
        <td>
          extraKernelParams is a string with extra parameters to pass to the
kernel when booting the image over network. Only makes sense for initrd images.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>format</b></td>
        <td>enum</td>
        <td>
          format is the type of image that is available at the download url:
either iso or initrd.<br/>
          <br/>
            <i>Enum</i>: iso, initrd<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>imageUrl</b></td>
        <td>string</td>
        <td>
          imageUrl is the URL from which the built image can be downloaded.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>kernelUrl</b></td>
        <td>string</td>
        <td>
          kernelUrl is the URL from which the kernel of the image can be downloaded.
Only makes sense for initrd images.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b><a href="#preprovisioningimagestatusnetworkdata">networkData</a></b></td>
        <td>object</td>
        <td>
          networkData is a reference to the version of the Secret containing the
network data used to build the image.<br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>


### PreprovisioningImage.status.conditions[index]
<sup><sup>[↩ Parent](#preprovisioningimagestatus)</sup></sup>



Condition contains details for one aspect of the current state of this API Resource.

<table>
    <thead>
        <tr>
            <th>Name</th>
            <th>Type</th>
            <th>Description</th>
            <th>Required</th>
        </tr>
    </thead>
    <tbody><tr>
        <td><b>lastTransitionTime</b></td>
        <td>string</td>
        <td>
          lastTransitionTime is the last time the condition transitioned from one status to another.
This should be when the underlying condition changed.  If that is not known, then using the time when the API field changed is acceptable.<br/>
          <br/>
            <i>Format</i>: date-time<br/>
        </td>
        <td>true</td>
      </tr><tr>
        <td><b>message</b></td>
        <td>string</td>
        <td>
          message is a human readable message indicating details about the transition.
This may be an empty string.<br/>
        </td>
        <td>true</td>
      </tr><tr>
        <td><b>reason</b></td>
        <td>string</td>
        <td>
          reason contains a programmatic identifier indicating the reason for the condition's last transition.
Producers of specific condition types may define expected values and meanings for this field,
and whether the values are considered a guaranteed API.
The value should be a CamelCase string.
This field may not be empty.<br/>
        </td>
        <td>true</td>
      </tr><tr>
        <td><b>status</b></td>
        <td>enum</td>
        <td>
          status of the condition, one of True, False, Unknown.<br/>
          <br/>
            <i>Enum</i>: True, False, Unknown<br/>
        </td>
        <td>true</td>
      </tr><tr>
        <td><b>type</b></td>
        <td>string</td>
        <td>
          type of condition in CamelCase or in foo.example.com/CamelCase.<br/>
        </td>
        <td>true</td>
      </tr><tr>
        <td><b>observedGeneration</b></td>
        <td>integer</td>
        <td>
          observedGeneration represents the .metadata.generation that the condition was set based upon.
For instance, if .metadata.generation is currently 12, but the .status.conditions[x].observedGeneration is 9, the condition is out of date
with respect to the current state of the instance.<br/>
          <br/>
            <i>Format</i>: int64<br/>
            <i>Minimum</i>: 0<br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>


### PreprovisioningImage.status.networkData
<sup><sup>[↩ Parent](#preprovisioningimagestatus)</sup></sup>



networkData is a reference to the version of the Secret containing the
network data used to build the image.

<table>
    <thead>
        <tr>
            <th>Name</th>
            <th>Type</th>
            <th>Description</th>
            <th>Required</th>
        </tr>
    </thead>
    <tbody><tr>
        <td><b>name</b></td>
        <td>string</td>
        <td>
          <br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>version</b></td>
        <td>string</td>
        <td>
          <br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>