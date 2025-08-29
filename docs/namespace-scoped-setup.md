# Namespace-Scoped Baremetal Operator (BMO) Deployment

This document explains how to configure the Baremetal Operator (BMO) to operate
within specific Kubernetes namespaces rather than across the entire cluster.
This is crucial for multi-tenant environments or when you need to limit the
operator's permissions and scope for security reasons.

To deploy BMO with namespace-specific configurations, you need to create
Roles and Rolebinding for each Kubernetes namespace and configure BMO
controller to watch required namespaces.
Below, you will find detailed instructions on how to configure BMO to watch
specific namespaces.

## Configuring BMO controller to watch specific Namespaces

The BMO controller itself needs to be instructed which namespaces it should
monitor for resources.

**Using the `WATCH_NAMESPACE` environment variable:**

You can control the operator's watch scope by setting the `WATCH_NAMESPACE`
environment variable in its deployment.

* **Watch all namespaces (default):** If `WATCH_NAMESPACE` is unset or empty,
  BMO will monitor resources across all namespaces in the cluster.
* **Watch a single namespace:** Set `WATCH_NAMESPACE` to the desired
  namespace name (e.g., `WATCH_NAMESPACE=my-namespace`).
* **Watch multiple namespaces:** Provide a comma-separated list of namespaces
  (e.g., `WATCH_NAMESPACE=foo,bar,quo`).

**Example: Configuring BMO to watch its own deployed namespace:**

To configure the operator to automatically watch the same namespaces foo,
bar and quo, modify the Deployment manifest. Add the following environment
variable configuration under `spec.template.spec.containers.env`:

```yaml
# config/base/manager.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: controller-manager
  namespace: system
spec:
  template:
    spec:
      containers:
      - name: manager
        env:
        - name: WATCH_NAMESPACE
          value: foo,bar,quo
```

## Create Roles and RoleBindings for each namespace

When BMO operates in a namespace-scoped mode, you need to create `Roles` and
`Rolebindings` for each namespace that you added in `WATCH_NAMESPACE` in env
variable controller-manager Deployment. !!!IMPORTANT: Remove cluster wide
`ClusterRole` and `ClusterRoleBinding` if they are already exist in your
manifests.

**Steps:**

1.**Add manifests for each namespaces:**

  ```yaml
  apiVersion: v1
  kind: Namespace
  metadata:
    name: foo
  ```

2.**Add Roles for each namespaces:**

  ```yaml
  ---
  apiVersion: rbac.authorization.k8s.io/v1
  kind: Role
  metadata:
    name: baremetal-operator-manager-role
    namespace: foo
  rules:
  - apiGroups:
    - ""
    resources:
    - events
    verbs:
    - create
    - get
    - list
    - patch
    - update
    - watch
  - apiGroups:
    - ""
    resources:
    - secrets
    verbs:
    - delete
    - get
    - list
    - update
    - watch
  - apiGroups:
    - metal3.io
    resources:
    - baremetalhosts
    - bmceventsubscriptions
    - dataimages
    - firmwareschemas
    - hardwaredata
    - hostfirmwarecomponents
    - hostfirmwaresettings
    - preprovisioningimages
    verbs:
    - create
    - delete
    - get
    - list
    - patch
    - update
    - watch
  - apiGroups:
    - metal3.io
    resources:
    - baremetalhosts/finalizers
    - dataimages/finalizers
    - hardware/finalizers
    - hostfirmwarecomponents/finalizers
    verbs:
    - update
  - apiGroups:
    - metal3.io
    resources:
    - baremetalhosts/status
    - bmceventsubscriptions/status
    - dataimages/status
    - firmwareschemas/status
    - hostfirmwarecomponents/status
    - hostfirmwaresettings/status
    - preprovisioningimages/status
    verbs:
    - get
    - patch
    - update
  - apiGroups:
    - metal3.io
    resources:
    - hostupdatepolicies
    verbs:
    - get
    - list
    - update
    - watch
  ```

3.**Add RoleBindings for each namespaces:**

  ```yaml
  ---
  apiVersion: rbac.authorization.k8s.io/v1
  kind: RoleBinding
  metadata:
    name: baremetal-operator-manager-rolebinding
    namespace: foo
  roleRef:
    apiGroup: rbac.authorization.k8s.io
    kind: Role
    name: baremetal-operator-manager-role
  subjects:
  - kind: ServiceAccount
    name: baremetal-operator-controller-manager
    namespace: baremetal-operator-system
  ```

## Current Restrictions and Known Issues

* **Predefined Watched Namespaces:** Due to limitations in `controller-runtime`,
  the namespaces that BMO watches must be predefined **before** starting the
  controller. Dynamic addition or removal of watched namespaces after startup
  is not currently supported.

* **Deleting Watched Namespaces:** If a namespace that BMO is actively
  watching is deleted, the operator will continuously log errors. This is
  because the controller's cache will try to list resources in a non-existent
  namespace, leading to permission denied errors from Kubernetes.

  **Example Error Log:**

    ```yaml
  "level": "error",
  "ts": 1754219075.4855175,
  "logger": "controller-runtime.cache.UnhandledError",
  "msg": "Failed to watch",
  "reflector": "pkg/mod/k8s.io/client-go@v0.33.3/tools/cache/reflector.go:285",
  "type": "*v1alpha1.HostFirmwareComponents",
  "error": "failed to list *v1alpha1.HostFirmwareComponents:
    hostfirmwarecomponents.metal3.io is forbidden: User
    "system:serviceaccount:baremetal-operator-system:"
    "baremetal-operator-controller-manager" cannot list resource
    "hostfirmwarecomponents" in API group "metal3.io" in the namespace "inspection"",
    ```

Deleting watched namespaces can be recovered if you reapply `namespace`,
`Roles` and `Rolebindings` and BMO stops throwing above error.
