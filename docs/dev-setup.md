Setup Development Environment
=============================

## Install the operator-sdk

Follow the instructions in the Quick Start section of
https://github.com/operator-framework/operator-sdk to check out and
install the operator-sdk tools.

## With minkube

1. Install and launch minikube

   https://kubernetes.io/docs/setup/minikube/

3. Create a namespace to host the operator

    ```
    kubectl create namespace metal3
    ```

4. Install operator-sdk

    ```
    eval $(go env)
    mkdir -p $GOPATH/src/github.com/metal3-io
    cd $GOPATH/src/github.com/metal3-io
    git clone https://github.com/metal3-io/baremetal-operator.git
    cd baremetal-operator
    kubectl apply -f deploy/service_account.yaml -n metal3
    kubectl apply -f deploy/role.yaml -n metal3
    kubectl apply -f deploy/role_binding.yaml
    kubectl apply -f deploy/crds/metal3_v1alpha1_baremetalhost_crd.yaml
    ```

5. Launch the operator locally

    ```
    export OPERATOR_NAME=baremetal-operator
    export DEPLOY_KERNEL_URL=http://172.22.0.1/images/ironic-python-agent.kernel
    export DEPLOY_RAMDISK_URL=http://172.22.0.1/images/ironic-python-agent.initramfs
    export IRONIC_ENDPOINT=http://localhost:6385/v1/
    export IRONIC_INSPECTOR_ENDPOINT=http://localhost:5050/v1
    operator-sdk up local --namespace=metal3
    ```

6. Create the CR

    ```
    kubectl apply -f deploy/crds/example-host.yaml
    ```

## Running without Ironic

In environments where Ironic is not available, and the only real need
is to be able to have some test data, use the test fixture provisioner
instead of the real Ironic provisioner by passing `-test-mode` to the
operator when launching it.

```
operator-sdk up local --operator-flags "-test-mode"
```

## Running a local instance of Ironic

There is a script available that will run a set of containers locally using
`podman` to stand up Ironic for development and testing.

See `tools/run_local_ironic.sh`.

Note that this script may need customizations to some of the `podman run`
commands, to include environment variables that configure the containers for
your environment.

## Using libvirt VMs with Ironic

In order to use VMs as hosts, they need to be connected to [vbmc](https://docs.openstack.org/tripleo-docs/latest/install/environments/virtualbmc.html) and
the `bootMACAddress` field needs to be set to the MAC address of the
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

```
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

The output can be passed directly to `oc apply` like this:

```
$ go run cmd/make-virt-host/main.go openshift_worker_1 | oc apply -f -
```

When the host is a *master*, include the `-machine` and
`-machine-namespace` options to associate the host with the existing
`Machine` object.

```
$ go run cmd/make-virt-host/main.go -machine ostest-master-1 -machine-namespace openshift-machine-api  openshift_master_1
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
  machineRef:
    name: ostest-master-1
    namespace: openshift-machine-api
```

## Using Bare Metal Hosts

The `make-bm-worker` tool may be a more convenient way of creating
YAML definitions for workers than editing the files directly.

```
$ go run cmd/make-bm-worker/main.go -address 1.2.3.4 -password password -user admin worker-99
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
```
