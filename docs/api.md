# API and Resource Definitions

## BareMetalHost

The BareMetalHost resource defines the properties of a physical host
necessary to manage and provision it.

### Spec Fields

*bmc* -- The connection information for the BMC controller on the host.

*bmc.address* -- The URL for communicating with the BMC controller.

*bmc.credentials* -- A reference to a Secret containing the connection
data, at least username and password, for the BMC.

*online* -- A boolean indicating whether the host should be powered on
(true) or off (false). Changing this value will trigger a change in
power state on the physical host.

```
---
apiVersion: v1
kind: Secret
metadata:
  name: bmc1-secret
type: Opaque
data:
  username: YWRtaW4=
  password: MWYyZDFlMmU2N2Rm
---
apiVersion: metalkube.org/v1alpha1
kind: BareMetalHost
metadata:
  name: example-baremetalhost
spec:
  online: true
  bmc:
    address: ipmi://192.168.122.1:6233
    credentials:
      name: bmc1-secret
```

### Status Fields

*machineRef* -- The Machine tying this host to a Node.

*lastUpdated* -- The timestamp of the last time the status for the
host was updated.

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

```
apiVersion: metalkube.org/v1alpha1
kind: BareMetalHost
metadata:
  creationTimestamp: 2019-02-08T20:10:32Z
  finalizers:
  - baremetalhost.metalkube.org
  generation: 9
  labels:
    metalkube.org/hardware-profile: unknown
    metalkube.org/operational-status: online
  name: example-baremetalhost
  namespace: bmo-project
  resourceVersion: "1750818"
  selfLink: /apis/metalkube.org/v1alpha1/namespaces/bmo-project/baremetalhosts/example-baremetalhost
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
  image: ""
  lastUpdated: 2019-02-11T17:44:30Z
  provisioningID: ""
```

## Labels

The BareMetalHost operator manages several labels with host status and
settings to make it easier to find specific hosts.

*metalkube.org/hardware-profile* -- The name of the hardware profile
that matches the hardware discovered on the host. Details about the
hardware are saved to the *hardware* section of the status. If the
hardware does not match a known profile, the value "unknown" is used.

*metalkube.org/operational-status* -- The status of the server.

  *online* -- The server is powered on and running.

  *offline* -- The server is powered off.

  *error* -- There is an error with the configuration data for the
  host or there is a problem with the host itself. Refer to the
  *errorMessage* field in the status section for more details about
  the error condition.
