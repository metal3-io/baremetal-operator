# Running Baremetal-operator on kubernetes cluster

## Description

This document outlines the process of provisioning completely within
the cluster; starting with spinning the components
**Baremetal-operator and Ironic** over a
**On-Premises Kubernetes Cluster**, till the **provisioning of baremetal
machines** using only cluster resources.

## Prerequisites

Fully ready Kubernetes cluster (version >= 1.17)

## Spinning

* ### Clone the **Baremetal-operator**

```console
> git clone https://github.com/metal3-io/baremetal-operator.git
> cd baremetal-operator
```

* ### Start **Ironic**

We will be applying the ironic first as we need the cluster IP
of ironic pod to configure it in the config-maps of Baremetal-operator for
inter-pod communication.

```console
> kubectl apply -k ironic-deployement/default
> kubectl get pods -n default
```

* ### Update **config maps**

Take the Cluster-IP Address of ironic pod running in default namespace
and copy it in the config maps of Baremetal-operator by replace the default
provisioning-IP in *deploy/ironic_ci.env* as mentioned below.

```console
> vi deploy/ironic_ci.env
```

```properties
HTTP_PORT=6180
PROVISIONING_IP=<Ironic-ClusterIP>
PROVISIONING_CIDR=24
PROVISIONING_INTERFACE=ironicendpoint
DHCP_RANGE=<Cluster Dhcp range start>,<Cluster Dhcp range end>
DEPLOY_KERNEL_URL=http://<Ironic-ClusterIP>:6180/images/ironic-python-agent.kernel
DEPLOY_RAMDISK_URL=http://<Ironic-ClusterIP>:6180/images/ironic-python-agent.initramfs
IRONIC_ENDPOINT=http://<Ironic-ClusterIP>:6385/v1/
IRONIC_INSPECTOR_ENDPOINT=http://<Ironic-ClusterIP>:5050/v1/
CACHEURL=http://<Ironic-ClusterIP>/images
IRONIC_FAST_TRACK=false
```

Do the same in *default/ironic_bmo_configmap.env* and
one may also have to change *provisioning-interface*
(by default it is PROVISIONING_INTERFACE=eth2) as per the OS and hardware specs.

```console
> vi default/ironic_bmo_configmap.env
```

```properties
HTTP_PORT=6180
PROVISIONING_INTERFACE=eth2
DHCP_RANGE=<Cluster Dhcp range start>,<Cluster Dhcp range end>
DEPLOY_KERNEL_URL=http://<Ironic-ClusterIP>:6180/images/ironic-python-agent.kernel
DEPLOY_RAMDISK_URL=http://<Ironic-ClusterIP>:6180/images/ironic-python-agent.initramfs
IRONIC_ENDPOINT=http://<Ironic-ClusterIP>:6385/v1/
IRONIC_INSPECTOR_ENDPOINT=http://<Ironic-ClusterIP>:5050/v1/
CACHEURL=http://<Ironic-ClusterIP>/images
IRONIC_FAST_TRACK=false
```

* ### Start Baremetal-operator

The below command will apply all CRDs, namespace, RBAC
and Baremetal-Operator.


```console
> kubectl apply -k deploy/default
```

Now you should be able to see the pods belonging to baremetal-operator
and ironic running.

## Provisioning Baremetal

The below portion discusses about yaml configuration applied for provisioning
baremetal-machine.

* ### Operating System for provisioning

Download the desired OS (in this case CentOS 7 qcow) to provision the
baremetals.

```console
> wget https://cloud.centos.org/centos/7/images/CentOS-7-x86_64-GenericCloud-1905.qcow2
```

Using md5sum make a md5 checksum file ending the name with '.md5sum'.

```console
> md5sum CentOS-7-x86_64-GenericCloud-1905.qcow2 > \
CentOS-7-x86_64-GenericCloud-1905.qcow2.md5sum
```

Copy the OS image and md5sum to the ironic pod -> ironic container
running in cluster.

```console
> kubectl cp CentOS-7-x86_64-GenericCloud-1905.qcow2 \
pod/<ironic-pod>:/shared/html/images/CentOS-7-x86_64-GenericCloud-1905.qcow2 \
-c ironic
> kubectl cp CentOS-7-x86_64-GenericCloud-1905.qcow2.md5sum pod/<ironic-pod>: \
/shared/html/images/CentOS-7-x86_64-GenericCloud-1905.qcow2.md5sum \
-c ironic
```

 * ### Baremetal configuration

Turn on/enable the **IPMI** feature in the baremetal server.
Use base64 to encode username and password of baremetal.

```console
> echo -n 'admin' | base64
YWRtaW4=
> echo -n 'password' | base64
cGFzc3dvcmQ=
```

Paste the encoded credentials in the data.username and data.password
Use the below yaml to configure a **Baremetal-machine**

```yaml
# bmh-idrack.yaml
apiVersion: v1
kind: Secret
metadata:
  name: bmc-secret
  namespace: metal3
type: Opaque
data:
  username: YWRtaW4=
  password: cGFzc3dvcmQ=

---
apiVersion: metal3.io/v1alpha1
kind: BareMetalHost
metadata:
  name: bmh-idrac
  namespace: metal3
spec:
  online: true
  bmc:
    address: idrac://10.X.X.X
    credentialsName: bmc-secret
    disableCertificateVerification: true
  bootMACAddress:  1C:XX:XX:XX:XX
  consumerRef:
    kind: BareMetalMachine
    name: baremetal-machine-idrac
  image:
    checksum: http://<Ironic-Cluster-IP>:6180/images/CentOS-7-x86_64-GenericCloud.qcow2.md5sum
    url: http://<Ironic-Cluster-IP>:6180/images/CentOS-7-x86_64-GenericCloud.qcow2
```

```console
> kubectl apply -f bmc-idrack.yaml
> kubectl get bmh -n metal3
```

At this point, the ironic will perform certain operations and the
state of Baremetal host or **bmh** keeps changing as mentioned in the *provisioning*
section of **[api.md](api.md)**
