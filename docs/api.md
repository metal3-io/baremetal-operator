# API and Resource Definitions

## BareMetalHost

**Metal³** introduces the concept of **BareMetalHost** resource, which
defines a physical host and its properties. See [BareMetalHost
CR](https://doc.crds.dev/github.com/metal3-io/baremetal-operator/metal3.io/BareMetalHost/v1alpha1)
or check the source code at `apis/metal3.io/v1alpha1/baremetalhost_types.go`
for a detailed API description. See the [user
guide](https://book.metal3.io/bmo/introduction) for information on how to
manage hosts.

## Triggering Provisioning

Several conditions must be met in order to initiate provisioning.

1. The host `spec.image.url` field must contain a URL for a valid
   image file that is visible from within the cluster and from the
   host receiving the image.
2. The host must have `online` set to `true` so that the operator will
   keep the host powered on.
3. The host must have all of the BMC details.

To initiate deprovisioning, clear the image URL from the host spec.

## Unmanaged Hosts

Hosts created without BMC details will be left in the `unmanaged`
state until the details are provided. Unmanaged hosts cannot be
provisioned and their power state is undefined.

## Pausing reconciliation

It is possible to pause the reconciliation of a BareMetalHost object by adding
an annotation `baremetalhost.metal3.io/paused`. **Metal³**  provider sets the
value of this annotation as `metal3.io/capm3` when the cluster to which the
**BareMetalHost** belongs, is paused and removes it when the cluster is
not paused. If you want to pause the reconciliation of **BareMetalHost** you can
put any value on this annotation **other than `metal3.io/capm3`**. Please make
sure that you remove the annotation  **only if the value of the annotation is
not `metal3.io/capm3`, but another value that you have provided**. Removing the
annotation will enable the reconciliation again.

## HostFirmwareSettings

A **HostFirmwareSettings** resource is used to manage BIOS settings for a host,
there is a one-to-one mapping with **BareMetalHosts**.  A
**HostFirmwareSettings** resource is created when BIOS settings are read from
Ironic as the host moves to the Ready state.  These settings are the complete
actual BIOS configuration names returned from the BMC, typically 100-200
settings per host, as compared to the three vendor-independent fields stored in
the **BareMetalHosts** `firmware` field.

See [HostFirmwareSettings
CR](https://doc.crds.dev/github.com/metal3-io/baremetal-operator/metal3.io/HostFirmwareSettings/v1alpha1)
or check the source code at `apis/metal3.io/v1alpha1/hostfirmwaresettings_types.go`
for a detailed API description. See the [firmware settings
guide](https://book.metal3.io/bmo/firmware_settings) for information on how to
change firmware settings.

## FirmwareSchema

A **FirmwareSchema** resource contains the limits each setting, specific to
each host.  This data comes directly from the BMC via Ironic. It can be used
to prevent misconfiguration of the **HostFirmwareSettings** *spec* field so
that invalid values are not sent to the host. The **FirmwareSchema** has a
unique identifier derived from its settings and limits. Multiple hosts may therefore
have the same **FirmwareSchema** identifier so its likely that more than one
**HostFirmwareSettings** reference the same **FirmwareSchema** when
hardware of the same vendor and model are used.

See [FirmwareSchema
CR](https://doc.crds.dev/github.com/metal3-io/baremetal-operator/metal3.io/FirmwareSchema/v1alpha1)
or check the source code at `apis/metal3.io/v1alpha1/firmwareschema_types.go`
for a detailed API description.

## HardwareData

A **HardwareData** resource contains hardware specifications data of a
specific host and it is tightly coupled to its owner resource
BareMetalHost. The data in the HardwareData comes from Ironic after a
successful inspection phase. As such, operator will create HardwareData
resource for a specific BareMetalHost during transitioning phase from
inspecting into available state of the BareMetalHost. HardwareData gets
deleted automatically by the operator whenever its BareMetalHost is
deleted. Deprovisioning of the BareMetalHost should not trigger the
deletion of HardwareData, but during next provisioning it can be
re-created (with the same name and namespace) with the latest inspection
data retrieved from Ironic. HardwareData holds the same name and
namespace as its corresponding BareMetalHost resource. Currently,
HardwareData doesn't have *Status* subresource but only the *Spec*.

See [HardwareData
CR](https://doc.crds.dev/github.com/metal3-io/baremetal-operator/metal3.io/HardwareData/v1alpha1)
or check the source code at `apis/metal3.io/v1alpha1/hardwaredata_types.go`
for a detailed API description.

## PreprovisioningImage

A **PreprovisioningImage** resource is automatically created by
baremetal-operator for each BareMetalHost to ensure creation of a
*preprovisioning image* for it. In this context, a preprovisioning image
is an ISO or initramfs file that contains the [Ironic
agent](https://docs.openstack.org/ironic-python-agent/). The relevant
parts of BareMetalHost are copied to the PreprovisioningImage *Spec*,
the resulting image is expected to appear in the *Status*.

The baremetal-operator project contains a simple controller for
PreprovisioningImages that uses images provided in the environment
variables `DEPLOY_ISO_URL` and `DEPLOY_RAMDISK_URL`. More sophisticated
controllers may be written downstream (for example, the OpenShift
[image-customization-controller](https://github.com/openshift/image-customization-controller)).

See [PreprovisioningImage
CR](https://doc.crds.dev/github.com/metal3-io/baremetal-operator/metal3.io/PreprovisioningImage/v1alpha1)
or check the source code at `apis/metal3.io/v1alpha1/preprovisioningimage_types.go`
for a detailed API description.
