# Setup Development Environment

## With minikube

1. Install and launch minikube

   <https://kubernetes.io/docs/setup/minikube/>

1. Create a namespace to host the operator

    ```bash
    kubectl create namespace baremetal-operator-system
    ```

1. Install operator in the cluster

    ```bash
    eval $(go env)
    mkdir -p $GOPATH/src/github.com/metal3-io
    cd $GOPATH/src/github.com/metal3-io
    git clone https://github.com/metal3-io/baremetal-operator.git
    cd baremetal-operator
    kustomize build config/default | kubectl apply -f -
    ```

1. OR Launch the operator locally

    ```bash
    export OPERATOR_NAME=baremetal-operator
    export DEPLOY_KERNEL_URL=http://172.22.0.1/images/ironic-python-agent.kernel
    export DEPLOY_RAMDISK_URL=http://172.22.0.1/images/ironic-python-agent.initramfs
    export IRONIC_ENDPOINT=http://localhost:6385/v1/
    export IRONIC_INSPECTOR_ENDPOINT=http://localhost:5050/v1
    make run
    ```

1. Create the CR

    ```bash
    kubectl apply -f examples/example-host.yaml -n metal3
    ```

## Running without Ironic

In environments where Ironic is not available, and the only real need
is to be able to have some test data, use the test fixture provisioner
instead of the real Ironic provisioner by passing `-test-mode` to the
operator when launching it.

```bash
make run-test-mode
```

## Running a local instance of Ironic

There is a script available that will run a set of containers locally using
`podman` to stand up Ironic for development and testing.

See `tools/run_local_ironic.sh`.

Note that this script may need customizations to some of the `podman run`
commands, to include environment variables that configure the containers for
your environment. All ironic related environment variables are set by default
if they are not passed through the environment.

The following environment variables can be passed to configure the ironic:

- HTTP_PORT - port used by httpd server (default 6180)
- PROVISIONING_IP - provisioning interface IP address to use for ironic,
  dnsmasq(dhcpd) and httpd (default 172.22.0.1)
- CLUSTER_PROVISIONING_IP - cluster provisioning interface IP address (default 172.22.0.2)
- PROVISIONING_INTERFACE - interface to use for ironic, dnsmasq(dhcpd) and
  httpd (default ironicendpoint)
- CLUSTER_DHCP_RANGE - dhcp range to use for provisioning (default 172.22.0.10-172.22.0.100)
- DEPLOY_KERNEL_URL - the URL of the kernel to deploy ironic-python-agent
- DEPLOY_RAMDISK_URL - the URL of the ramdisk to deploy ironic-python-agent
- IRONIC_ENDPOINT - the endpoint of the ironic
- IRONIC_INSPECTOR_ENDPOINT - the endpoint of the ironic inspector
- CACHEURL - the URL of the cached images
- IRONIC_FAST_TRACK - whether to enable fast_track provisioning or not
  (default true)
- IRONIC_KERNEL_PARAMS - Kernel parameters to pass to IPA (default console=ttyS0)
- IRONIC_INSPECTOR_VLAN_INTERFACES - VLAN interfaces included in introspection,
       `all` - all VLANs on all interfaces, using LLDP information (default),
       interface - all VLANs on an interface, using LLDP information,
       interface.vlan - a particular VLAN interface, not using LLDP

In case you want to run the local ironic containers with TLS and basic
authentication enabled, you also need to export the following variables:

### TLS variables

- IRONIC_CACERT_FILE
- IRONIC_CERT_FILE
- IRONIC_KEY_FILE
- IRONIC_INSPECTOR_CACERT_FILE
- IRONIC_INSPECTOR_CERT_FILE
- IRONIC_INSPECTOR_KEY_FILE

### Basic authentication variables

- IRONIC_USERNAME
- IRONIC_PASSWORD
- IRONIC_INSPECTOR_USERNAME
- IRONIC_INSPECTOR_PASSWORD

The names of these variables are self explanatory. TLS variables expect the
path of the corresponding certificate/key file as their value. Basic
authentication variables expect the corresponding value as string.  Note that,
these variables **do not** have any default value. So if they are not set, the
ironic container will run with TLS and basic authentication disabled.

## Using Tilt for development

It is easy to use Tilt for BMO deployment. Once you have a local instance
of Ironic running, just run

```sh
  make tilt-up
```

and clean it with

```sh
  make kind-reset
```

It is also possible to develop Baremetal Operator using Tilt with CAPM3. Please
refer to
[the development setup guide of CAPM3](https://github.com/metal3-io/cluster-api-provider-metal3/blob/master/docs/dev-setup.md#tilt-for-dev-in-capm3)
and specially the [Baremetal Operator Integration](https://github.com/metal3-io/cluster-api-provider-metal3/blob/master/docs/dev-setup.md#including-baremetal-operator-and-ip-address-manager)

## Using libvirt VMs with Ironic

In order to use VMs as hosts, they need to be connected to
[vbmc](https://docs.openstack.org/project-deploy-guide/tripleo-docs/latest/environments/virtualbmc.html)
and the `bootMACAddress` field needs to be set to the MAC address of the
network interface that will PXE boot.

For example:

```yaml
apiVersion: metal3.io/v1alpha1
kind: BareMetalHost
metadata:
  name: worker-0
spec:
  online: true
  bmc:
    address: libvirt://192.168.122.1:6233/
    credentialsName: worker-0-bmc-secret
  bootMACAddress: 00:73:49:3a:76:8e
```

The `make-virt-host` utility can be used to generate a YAML file for
registering a host. It takes as input the name of the `virsh` domain
and produces as output the basic YAML to register that host properly,
with the boot MAC address and BMC address filled in.

```bash
$ go run cmd/make-virt-host/main.go openshift_worker_1
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
  bootMACAddress: 00:1a:74:74:e5:cf
```

The output can be passed directly to `kubectl apply` like this:

```bash
go run cmd/make-virt-host/main.go openshift_worker_1 | kubectl apply -f -
```

When the host is a *master*, include the `-consumer` and
`-consumer-namespace` options to associate the host with the existing
`Machine` object.

```bash
$ go run cmd/make-virt-host/main.go -consumer ostest-master-1 \
  -consumer-namespace openshift-machine-api  openshift_master_1
---
apiVersion: v1
kind: Secret
metadata:
  name: openshift-master-1-bmc-secret
type: Opaque
data:
  username: YWRtaW4=
  password: cGFzc3dvcmQ=

---
apiVersion: metal3.io/v1alpha1
kind: BareMetalHost
metadata:
  name: openshift-master-1
spec:
  online: true
  bmc:
    address: libvirt://192.168.122.1:6231/
    credentialsName: openshift-master-1-bmc-secret
  bootMACAddress: 00:c9:a0:f2:e0:59
  consumerRef:
    name: ostest-master-1
    namespace: openshift-machine-api
```

## Using Bare Metal Hosts

The `make-bm-worker` tool may be a more convenient way of creating
YAML definitions for workers than editing the files directly. If
deploying baremetal hosts you might want to consider setting
IRONIC_KERNEL_PARAMS="", when deploying ironic as the default directs
the console to ttyS0 and is lost in Bare Metal Hosts.

```bash
$ go run cmd/make-bm-worker/main.go -address 1.2.3.4 \
  -password password -user admin worker-99
---
apiVersion: v1
kind: Secret
metadata:
  name: worker-99-bmc-secret
type: Opaque
data:
  username: YWRtaW4=
  password: cGFzc3dvcmQ=

---
apiVersion: metal3.io/v1alpha1
kind: BareMetalHost
metadata:
  name: worker-99
spec:
  online: true
  bmc:
    address: 1.2.3.4
    credentialsName: worker-99-bmc-secret
    disableCertificateVerification: true
```
