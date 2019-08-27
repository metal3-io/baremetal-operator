# API and Resource Definitions

## BareMetalHost

The BareMetalHost resource defines the properties of a physical host
necessary to manage and provision it.

### Spec Fields

*bmc* -- The connection information for the BMC controller on the host.

*bmc.address* -- The URL for communicating with the BMC controller. When
communicating over IPMI, this should be a URL of the form
`ipmi://<host>:<port>` (an unadorned `<host>:<port>` is also accepted).
Specifying the port is optional; the default port is 623. Dell iDRAC is also
supported, by using the scheme `idrac://` (or `idrac+http://` to disable TLS)
in place of `https://` in the iDRAC URL; only the hostname or IP address is
required - `idrac://host.example` is equivalent to
`idrac+https://host.example:443/wsman`. Fujitsu iRMC is also supported,
by using the scheme `irmc://<host>:<port>` with `<port>` is optional.
Redfish is also supported, by using the scheme `redfish://` (or
`redfish+http://` to disable TLS) in place of `https://` in the Redfish URL;
the hostname or IP address, and the path to the system ID are required -
for example `redfish://myhost.example/redfish/v1/Systems/MySystemExample`

*bmc.credentials* -- A reference to a Secret containing the connection
data, at least username and password, for the BMC.

*online* -- A boolean indicating whether the host should be powered on
(true) or off (false). Changing this value will trigger a change in
power state on the physical host.

*consumerRef* -- A reference to another object that is using the
host. For example, a Machine when the host is being used by the
machine-api.

*externallyProvisioned* -- A boolean indicating whether something else
is managing the image running on the host. When set, if no image is
provided, the host's power status and hardware inventory will be
monitored. If this flag is set, no provisioning or deprovisioning
operations are performed for the host.

*image.url* -- The URL of an image to deploy to the host.

*image.checksum* -- An md5 checksum or URL to a file with a checksum
for the image at *image.url*.

*userData* -- A reference to the Secret containing the user data to be
passed to the host before it boots.

*description* -- A human-provided string to help identify the host.

```
---
apiVersion: v1
kind: Secret
metadata:
  name: openshift-worker-1-bmc-secret
type: Opaque
data:
  username: YWRtaW4=
  password: cGFzc3dvcmQ=

---
apiVersion: metal3.io/v1alpha1
kind: BareMetalHost
metadata:
  name: openshift-worker-1
spec:
  online: true
  bmc:
    address: libvirt://192.168.122.1:6234/
    credentialsName: openshift-worker-1-bmc-secret
  bootMACAddress: 00:11:55:9e:1d:f7
  userData:
    namespace: openshift-machine-api
    name: worker-user-data
  image:
    url: "http://172.22.0.1/images/rhcos-ootpa-latest.qcow2"
    checksum: "http://172.22.0.1/images/rhcos-ootpa-latest.qcow2.md5sum"
```

### Status Fields

*consumerRef* -- The thing using the host. Usually a Machine, linking
the host to a Node.

*lastUpdated* -- The timestamp of the last time the status for the
host was updated.

*operationalStatus* -- The status of the server. Value is one of the
following:
  * *OK* -- The host is configured correctly and is manageable.
  * *discovered* -- The host is only partially configured, such as
  when the BMC address is known but the login credentials are not.
  * *error* -- There is an error with the configuration data for the
  host or there is a problem with the host itself. Refer to the
  *errorMessage* field in the status section for more details about
  the error condition.

*errorMessage* -- Details for any error.

*hardware* -- The details for hardware capabilities discovered on the
host. These are filled in by the provisioning agent when the host is
registered.

*hardware.nics* -- List of network interfaces for the host.

*hardware.nics.mac* -- The MAC address of the NIC.

*hardware.nics.ip* -- The IP address of the NIC, if one was assigned
when the discovery agent ran.

*hardware.storage* -- List of storage (disk, SSD, etc.) available to
the host.

*hardware.storage.size* -- Size in GB of the storage location.

*hardware.storage.info* -- Information string about the storage.

*hardware.cpus* -- List of CPUs in the system.

*hardware.cpus.type* -- The type of the CPU.

*hardware.cpus.speed* -- The speed in GHz of the CPU.

*hardwareProfile* -- The name of the hardware profile that matches the
hardware discovered on the host. Details about the hardware are saved
to the *hardware* section of the status. If the hardware does not
match a known profile, the value "unknown" is used.  In practice, so far
this affects which device the deployed OS image is written to.  The
following `hardwareProfile` settings and the corresponding root device
are listed here, and are definitely subject to change.  (Note that the
default is `unknown`.)

| **hardwareProfile** | **Root Device** |
|---------------------|-----------------|
| `unknown`           | /dev/sda        |
|  `libvirt`          |  /dev/vda       |
| `dell`              | HCTL: 0:0:0:0   |
| `dell-raid`         | HCTL: 0:2:0:0   |

*poweredOn* -- Boolean indicating whether the host is powered on. See
*online*.

*provisioning* -- Settings related to deploying an image to the host.

*provisioning.state* -- The current state of any ongoing provisioning
operation.  One of:
  * *<empty string>* -- No provisioning is happening.
  * *registration error* -- The details for the BMC for the host are
    incorrect or incomplete and the host could not be managed.
  * *registering* -- The details for the BMC for the host are being
    checked.
  * *match profile* -- The details for the hardware found on the host
    are being compared against known profiles.
  * *ready* -- The host is available to be consumed.
  * *validation error* -- The details for the BMC for the host are
    incorrect or incomplete and the host could not be managed.
  * *provisioning* -- An image is being written to the host.
  * *provisioning error* -- The image could not be written to the host.
  * *provisioned* -- An image has been written to the host completely.
  * *externally provisioned* -- The host has a consumer, but metal3
    did not write an image to the host.
  * *deprovisioning* -- The host storage is being wiped.
  * *inspecting* -- The hardware details for the host are being
    collected.
  * *power management error* -- The host could not be powered on or
    off, as requested through the *online* field setting.

```
apiVersion: metal3.io/v1alpha1
kind: BareMetalHost
metadata:
  creationTimestamp: 2019-02-08T20:10:32Z
  finalizers:
  - baremetalhost.metal3.io
  generation: 9
  name: example-baremetalhost
  namespace: bmo-project
  resourceVersion: "1750818"
  selfLink: /apis/metal3.io/v1alpha1/namespaces/bmo-project/baremetalhosts/example-baremetalhost
  uid: 96837048-2bdd-11e9-8df7-525400f68198
spec:
  bmc:
    credentials:
      name: bmc1-secret
    address: ipmi://192.168.122.1:6233
  online: true
status:
  errorMessage: ""
  hardware:
    cpus: null
    nics: null
    storage: null
  hardwareProfile: unknown
  image: ""
  lastUpdated: 2019-02-11T17:44:30Z
  operationalStatus: OK
  provisioningID: ""
```

## Triggering Provisioning

Several conditions must be met in order to initiate provisioning.

1. The host `spec.image.url` field must contain a URL for a valid
   image file that is visible from within the cluster and from the
   host receiving the image.
2. The host must have `online` set to `true` so that the operator will
   keep the host powered on.

To initiate deprovisioning, clear the image URL from the host spec.
