# How to restrict BMO scope to a single namespace!

The guide is based on the instructions in this documentation
https://sdk.operatorframework.io/docs/building-operators/golang/operator-scope/

## To generate manifests for namespace scoped BMO

To generate namespace-scoped manifests, run the `manifests-kustomize-namespaced`
make target. This will create manifests configured for the namespace
`single-ns-bmh`. For further details on the steps involved, or if you
prefer to manually achieve similar results, please refer to the continuation
of the documentation.

## Watching resources in specific Namespaces

When setting up the manager, you can use the environment variable
`WATCH_NAMESPACE` to restrict the operator to a specific namespace. If
`WATCH_NAMESPACE` is unset or set to an empty string, the operator will
monitor all namespaces. To limit it to a specific namespace, set
`WATCH_NAMESPACE` to that namespace.

For example, to configure the operator to watch the same namespace where
it is deployed, update the `config/base/manager.yaml` file. Add the
following configuration under `spec.template.spec.containers.env`:

```yaml
- name: WATCH_NAMESPACE
  valueFrom:
    fieldRef:
      fieldPath: metadata.namespace
```

## Restricting Roles and permissions

When BMO is restricted to a single namespace, the RBAC permissions need
to be updated accordingly. Instead of using `ClusterRole`, you will use
`Role`.

The `Role` is defined in the file `config/base/rbac/role_ns.yaml`. This
file is auto-generated based on Kubebuilder RBAC markers, specifically those
in `<some>_controller.go`. The default namespace marking is set to `""`,
which results in a `ClusterRole`. To restrict it to a specific namespace,
update this value accordingly.

You can automatically update the Kubebuilder RBAC markers by running:

```bash
python update_kubebuilder_rbac.py controllers/metal3.io/ your-namespace
```

The first argument specifies the directory to search, and the second is
the new namespace. To revert the change, simply set the namespace to `""`.

After updating the markers, generate the new manifests by running:

```bash
    make manifests
```

Ensure that `config/base/rbac/role_ns.yaml` has been updated to a `Role`.

Due to limitations in Kubebuilder generation, the `RoleBinding` will not
be updated automatically. However, a Kustomization overlay is provided to
replace `ClusterRoleBinding` with `RoleBinding`. This overlay can be found
in `config/overlays/namespaced`.

Alternatively, you can manually update `config/base/rbac/role_binding.yaml`
to achieve the desired outcome. Below is an example of how to modify the
`role_binding.yaml` file:

```yaml:
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: manager-rolebinding
  namespace: your-namespace
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: manager-role
  namespace: your-namespace
subjects:
- kind: ServiceAccount
  name: controller-manager
  namespace: system
```

Replace `your-namespace` and other fields as necessary to match your
specific configuration.

After this you can run `make manifests-kustomize` to get correct RoleBinding generated

