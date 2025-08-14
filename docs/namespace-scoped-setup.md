# Namespace-Scoped Baremetal Operator (BMO) Deployment

This document explains how to configure the Baremetal Operator (BMO) to operate
within specific Kubernetes namespaces rather than across the entire cluster.
This is crucial for multi-tenant environments or when you need to limit the
operator's permissions and scope for security reasons.

## 1. Generating Namespace-Scoped Manifests

To deploy BMO with namespace-specific configurations, you need to generate
specialized Kubernetes manifests.

**How to generate:**

Run the following `make` command:

```bash
make manifests-namespaced
````

This command leverages the Kustomize overlay located at
`config/overlays/namespaced`. This overlay is designed to transform
cluster-scoped resources (like `ClusterRole` and `ClusterRoleBinding`) into
their namespace-scoped equivalents (`Role` and `RoleBinding`), ensuring BMO
operates only within designated namespaces.

## 2\. Configuring BMO to Watch Specific Namespaces

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

To configure the operator to automatically watch the same namespace it's
deployed in, modify the `config/base/manager.yaml` file. Add the following
environment variable configuration under spec.template.spec.containers.env`:

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

## 3\. Restricting RBAC Permissions (Roles and RoleBindings)

When BMO operates in a namespace-scoped mode, its Kubernetes permissions must
be updated from cluster-wide (`ClusterRole`, `ClusterRoleBinding`) to
namespace-specific (`Role`, `RoleBinding`).

**Steps:**

1.**Generate manifests:**

  ```bash
    make manifests
  ```

2.**Add manifests for each namespaces:**

  ```yaml
  apiVersion: v1
  kind: Namespace
  metadata:
    name: bar
  ---
  apiVersion: v1
  kind: Namespace
  metadata:
    name: foo
  ---
  apiVersion: v1
  kind: Namespace
  metadata:
    name: quo
  ---
  ```

2.**Change/Add Roles for each namespaces:**

  ```yaml
  ---
  apiVersion: rbac.authorization.k8s.io/v1
  kind: Role
  metadata:
    name: baremetal-operator-manager-role
    namespace: bar
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

4.**Change/Add RoleBindings for each namespaces:**

  ```yaml
  ---
  apiVersion: rbac.authorization.k8s.io/v1
  kind: RoleBinding
  metadata:
    name: baremetal-operator-manager-rolebinding
    namespace: bar
  roleRef:
    apiGroup: rbac.authorization.k8s.io
    kind: Role
    name: baremetal-operator-manager-role
  subjects:
  - kind: ServiceAccount
    name: baremetal-operator-controller-manager
    namespace: baremetal-operator-system
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
  ---
  apiVersion: rbac.authorization.k8s.io/v1
  kind: RoleBinding
  metadata:
    name: baremetal-operator-manager-rolebinding
    namespace: quo
  roleRef:
    apiGroup: rbac.authorization.k8s.io
    kind: Role
    name: baremetal-operator-manager-role
  ```

## 4\. Using the Kustomize Custom Plugin for Namespace-Scoped Mode

To simplify the generation of namespace-scoped manifests, including `Roles` and
`RoleBindings` for multiple namespaces, we provide a custom Kustomize plugin
called `namespaceAnnotator`.

**How it works:**

The `namespaceAnnotator` plugin automatically generates `Roles`,
`RoleBindings`, and patches namespace-specific manifests based on a list of
target namespaces you provide.

**Steps to use the plugin:**

1. **Define target namespaces:**
    You need to specify the list of namespaces in two files within the
    `config/overlays/namespaced` folder:

      * **`namespaced-manager-patch.yaml`**: This file configures the
      `WATCH_NAMESPACE` environment variable for the BMO deployment.

        ```yaml
        # config/overlays/namespaced/namespaced-manager-patch.yaml
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
                  value: foo,bar,quo # <--- Define your comma-separated
                                    #namespaces here
        ```

      * **`roles-ns-annotator.yaml`**: This file contains the Kustomize plugin
      configuration, where you list the namespaces for which `Roles` and
      `RoleBindings` should be generated.

        ```yaml
        # config/overlays/namespaced/roles-ns-annotator.yaml
        apiVersion: transformers.example.co/v1
        kind: ValueAnnotator
        metadata:
          annotations:
            config.kubernetes.io/function: |
              exec:
                # Path to your compiled plugin binary, relative to the kustomization.yaml
                path: ../../../hack/tools/kustomize-namespace-annotator/namespaceAnnotator
          name: notImportantHere
        values:
        - foo # <--- List your target namespaces here
        - bar
        - quo
        ```

2. **Generate manifests with the plugin:**

  After setting the namespaces in both files, run the following `make` command:

```bash
  make manifests-namespaced
```

  This will generate the complete set of `Roles`, `RoleBindings`, and namespace
  namespace-scoped manifests for the specified namespaces.

## 5\. Current Restrictions and Known Issues

* **Predefined Watch Namespaces:** Due to limitations in `controller-runtime`,
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
