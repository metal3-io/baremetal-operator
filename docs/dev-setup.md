Setup Development Environment
=============================

## Install the operator-sdk

Follow the instructions in the Quick Start section of
https://github.com/operator-framework/operator-sdk to check out and
install the operator-sdk tools.

## With minishift

1. Install and launch minishift

   https://docs.okd.io/latest/minishift/getting-started/index.html

2. Ensure you're logged in to the correct context and login as a normal user called developer.

    ```
    oc config use-context minishift
    oc login
    Username: developer
    ```

3. Create a project to host the operator

    ```
    oc new-project bmo-project
    ```

4. Install operator-sdk

    ```
    go get github.com/metalkube/baremetal-operator
    cd ~/go/src/github.com/metalkube/baremetal-operator
    oc --as system:admin apply -f deploy/service_account.yaml
    oc --as system:admin apply -f deploy/role.yaml
    oc --as system:admin apply -f deploy/role_binding.yaml
    oc --as system:admin apply -f deploy/crds/metalkube_v1alpha1_baremetalhost_crd.yaml
    ```

5. Launch the operator locally

    ```
    export OPERATOR_NAME=baremetal-operator
    operator-sdk up local --namespace=bmo-project
    ```

6. Create the CR

    ```
    oc apply -f deploy/crds/metalkube_v1alpha1_baremetalhost_cr.yaml
    ```

## Running without Ironic

In environments where Ironic is not available, and the only real need
is to be able to have some test data, use the test fixture provisioner
instead of the real Ironic provisioner by passing `-test-mode` to the
operator when launching it.

```
operator-sdk up local --operator-flags "-test-mode"
```

## Using libvirt VMs with Ironic

In order to use VMs as hosts, they need to be connected to [vbmc](https://docs.openstack.org/tripleo-docs/latest/install/environments/virtualbmc.html) and
the `bootMACAddress` field needs to be set to the MAC address of the
network interface that will PXE boot.

For example:

```yaml
apiVersion: metalkube.org/v1alpha1
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

The `make-worker` utility can be used to generate a YAML file for
registering a host. It takes as input the name of the `virsh` domain
and produces as output the basic YAML to register that host properly,
with the boot MAC address and BMC address filled in.

```
$ go run cmd/make-worker/main.go openshift_worker_1
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
apiVersion: metalkube.org/v1alpha1
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
$ go run cmd/make-worker/main.go openshift_worker_1 | oc apply -f -
```
