# Provisioner plugins

BMO loads its provisioner at runtime from a Go plugin `.so`. The manager binary
is decoupled from provisioner-specific dependencies (e.g. the Ironic client).

## Migrating from previous releases

This release replaces the compile-time provisioner selection with a runtime
plugin. If you are upgrading from an earlier BMO version, a few flags have
changed:

| Old                          | New                                          |
|------------------------------|----------------------------------------------|
| `-demo-mode`                 | `-provisioner=demo`                          |
| `-test-mode`                 | `-provisioner=fixture`                       |
| `-ironic-name=<name>` flag   | `IRONIC_NAME=<name>` env var                 |
| `-ironic-namespace=<ns>` flag| `IRONIC_NAMESPACE=<ns>` env var              |

## What ships in the BMO image

The container image bakes two plugins under `/plugins/`:

| Path                                 | Provisioner      |
|--------------------------------------|------------------|
| `/plugins/ironic-provisioner.so`     | ironic (default) |
| `/plugins/demo-provisioner.so`       | demo             |

The `fixture` provisioner is compiled into the manager and selected with
`-provisioner=fixture` for tests and offline runs. No plugin `.so` is loaded
in that mode.

## Selecting a plugin

Plugins are selected by name with the `-provisioner=<name>` flag. The manager
resolves the name to a `.so` under the plugin directory:

```text
-provisioner=<name>   # default: ironic
```

The flag value must match `^[a-z0-9][a-z0-9-]*$` (lowercase letters, digits,
and hyphens) so it can't escape the plugin directory through the file path.

Resolves to `${PROVISIONER_PLUGIN_DIR}/<name>-provisioner.so`. The
`PROVISIONER_PLUGIN_DIR` environment variable defaults to `/plugins` (matching
the image layout) and can be overridden to point at any directory holding
`.so` files that follow the `<name>-provisioner.so` naming convention. For
example, with `PROVISIONER_PLUGIN_DIR=/opt/bmo/plugins -provisioner=foobar`,
the manager loads `/opt/bmo/plugins/foobar-provisioner.so`.

The plugin must self-report a name (via its `PluginName()` symbol) that
matches the `<name>` portion of the file. A `foo-provisioner.so` whose
`PluginName()` returns `"bar"` is rejected at load time.

## Writing a custom plugin

A plugin is `package main` exporting these symbols:

```go
func PluginName() string
func NewProvisionerFactory(provisioner.PluginConfig) (provisioner.Factory, error)
// Optional, see the next section:
func HostConfigure(provisioner.HostConfigureInput) (provisioner.HostRequirements, error)
```

See [the demo plugin](../pkg/provisioner/demo/plugin/main.go) for a minimal
reference, and [the ironic plugin](../pkg/provisioner/ironic/plugin/main.go)
for one that uses `HostConfigure`.

Build (output filename must follow the `<name>-provisioner.so` convention so
the manager can resolve `-provisioner=<name>`):

```sh
CGO_ENABLED=1 go build -buildmode=plugin -trimpath -o foobar-provisioner.so ./path/to/plugin
# building against a BMO checkout instead of a release? see Source paths below
```

## Plugin configuration

The host passes initialization data through `provisioner.PluginConfig`:

- `Logger` — a `logr.Logger` named after the plugin.
- `Features` — a slice of host capabilities (e.g. `FeaturePreprovisioningImage`)
   the plugin can probe with `config.HasFeature(...)`.
- `K8sClient` and `APIReader` — the manager's controller-runtime client and
   uncached reader, populated only after the manager has been built. Don't
   call into them from `init()`.
- `ProvisionerNamespace` — host-resolved default namespace for the
   provisioner (`POD_NAMESPACE`, falling back to the watch namespace).

Plugins read their own external config (env vars, files, etc.) directly. The
host stays generic.

## Host-side requirements (optional `HostConfigure`)

If the plugin needs the host to register a CRD scheme or scope its cache to a
namespaced resource, it can export a third symbol that runs **before** the
manager is built:

```go
func HostConfigure(input provisioner.HostConfigureInput) (provisioner.HostRequirements, error)
```

`HostConfigureInput` carries `Logger`, `Features`, and `ProvisionerNamespace`,
like `PluginConfig`. `HostRequirements` carries:

- `AddToScheme func(*runtime.Scheme) error` — added to the host's scheme.
- `CacheByObject map[client.Object]cache.ByObject` — merged into the
   manager's `cache.Options`.

Plugins that don't need either omit the symbol; the host then proceeds with
zero requirements. The ironic plugin uses `HostConfigure` to register
`ironicv1alpha1` and (when `IRONIC_NAME` is set and a namespace resolves from
`IRONIC_NAMESPACE` or the host default) to namespace-scope the Ironic CR
informer.

## Process-wide globals and init() side effects

A Go plugin loads into the *same process* as the host, so the plugin and host
share every process-wide global: scheme registries, prometheus default
registerers, klog/logr global loggers, signal handlers, and so on. Two
consequences for plugin authors:

- Any `init()` function in a package the plugin imports runs at
  `plugin.Open` time, before the manager exists. Don't dial the Kubernetes
  API or otherwise touch the cluster from `init()`.
- If your plugin imports a controller-runtime package that registers types
  into a global `scheme.Scheme` other than the one the host wires up in its
  own `main.init()`, the host's manager won't see those types. Register
  schemes via the `PluginConfig` plumbing or expose your own helper that the
  host calls explicitly.

## Toolchain lock

Plugins must be built with the **same Go toolchain and same build flags** as
the BMO binary they load into, and every module shared with the host (directly
or transitively, including stdlib) must resolve to the **exact same version**.
Deps unique to the plugin can be added freely because they don't participate
in the version check. Mismatches produce:

```text
plugin was built with a different version of package <pkg>
```

For an in-tree plugin you get this for free, since the root `go.mod` is shared.

For an out-of-tree plugin maintained in its own repo:

- Pin BMO in the plugin's `go.mod` at the release you target. Go's module
  resolution will then pick versions of shared deps (`k8s.io/*`,
  `controller-runtime`, `go-logr`, etc.) compatible with BMO's closure:

  ```sh
  go get github.com/metal3-io/baremetal-operator@vX.Y.Z
  go mod tidy
  ```

- Build with the same Go toolchain (see `go-version` target in BMO's
  Makefile):

  ```sh
  CGO_ENABLED=1 go build -buildmode=plugin -trimpath -o foobar-provisioner.so ./plugin
  ```

- If `plugin.Open` reports a mismatch on a specific package, align it in
  your `go.mod` to the version the BMO release uses, then rebuild. Use
  `go get <pkg>@<ver>`, or `exclude` the higher one: under `-trimpath` the
  recorded path carries the module's selected version, so a
  `replace <pkg> vX => <pkg> vY` does not align it.

### Source paths

Recorded source paths are part of a package's fingerprint too. `-trimpath`
removes them, but it records a bare import path for the main module and
`module@version` for a dependency, and BMO is the main module when the released
binary is built while it is a dependency when your plugin is built. Released
images therefore record BMO's own packages in the dependency form, using the
release tag (see `hack/plugin-build-flags.sh`).

So build with `-trimpath` and pin BMO at the release you target. Let BMO's
`go.mod` decide the versions of `apis` and `pkg/hardwareutils` rather than
requiring them yourself, since the release recorded whatever it resolves.

```sh
go get github.com/metal3-io/baremetal-operator@vX.Y.Z
CGO_ENABLED=1 go build -buildmode=plugin -trimpath -o foobar-provisioner.so ./plugin
```

The directory you build in no longer matters, the version does. Only tagged
releases carry one, so an image built from a branch records no version and
takes plugins built the same way. A `replace` pointing at a local BMO checkout
also records something else, and needs the same rewrite applied to your own
build, which the next section spells out.

### When the release requires older submodules

The recipe above assumes the release requires `apis` and `pkg/hardwareutils` at
its own version, so resolving them hands you the source that release was built
from. Releases have shipped without that bump, so check what you resolved:

```sh
go list -m -f '{{.Version}}' github.com/metal3-io/baremetal-operator/apis
```

A version older than the release you pinned means the requirement was never
bumped, and resolving it gives submodule source the release does not contain.
Usually it will not compile against `pkg/provisioner` at all, and where it
does, the package contents still differ from the manager's, so `plugin.Open`
rejects the plugin.

Build against a checkout of the release instead. The replaces supply the
source the release actually contains, while the rewrite keeps recording the
versions its `go.mod` names, which is what the manager recorded:

```sh
git clone --depth 1 --branch vX.Y.Z \
    https://github.com/metal3-io/baremetal-operator /tmp/bmo
go mod edit \
    -replace github.com/metal3-io/baremetal-operator=/tmp/bmo \
    -replace github.com/metal3-io/baremetal-operator/apis=/tmp/bmo/apis \
    -replace github.com/metal3-io/baremetal-operator/pkg/hardwareutils=/tmp/bmo/pkg/hardwareutils
go mod tidy
CGO_ENABLED=1 go build -buildmode=plugin \
    $(/tmp/bmo/hack/plugin-build-flags.sh /tmp/bmo vX.Y.Z) \
    -o foobar-provisioner.so ./plugin
```

Leave the flags unquoted, they expand to two arguments. Pass the release tag
rather than a version of your own, since that is what the image recorded.
`Dockerfile.plugin-test` is a worked example, and CI builds it, so this path
stays exercised rather than becoming folklore.
