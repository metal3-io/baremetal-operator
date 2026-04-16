# Provisioner plugins

BMO loads its provisioner at runtime from a Go plugin `.so`. The manager binary
is decoupled from provisioner-specific dependencies (e.g. the Ironic client).

## What ships in the BMO image

The container image bakes two plugins under `/plugins/`:

| Path                                 | Provisioner      |
|--------------------------------------|------------------|
| `/plugins/ironic-provisioner.so`     | ironic (default) |
| `/plugins/demo-provisioner.so`       | demo             |

The `fixture` provisioner is compiled into the manager and selected with
`--test-mode` for tests and offline runs.

## Selecting a plugin

```text
--provisioner-plugin=/path/to/custom.so
```

Also reads `$PROVISIONER_PLUGIN`. Defaults to the ironic path when unset.

## Writing a custom plugin

A plugin is `package main` exporting two symbols: `PluginName() string` and
`NewProvisionerFactory(provisioner.PluginConfig) (provisioner.Factory, error)`.
See [`pkg/provisioner/demo/plugin/main.go`](../pkg/provisioner/demo/plugin/main.go)
for a minimal reference.

Build:

```sh
CGO_ENABLED=1 go build -buildmode=plugin -o myprov.so ./path/to/plugin
```

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
  CGO_ENABLED=1 go build -buildmode=plugin -o myprov.so ./plugin
  ```

- If `plugin.Open` reports a mismatch on a specific package, align it in
  your `go.mod` (`replace` or `go get <pkg>@<ver>`) to the version used by
  the BMO release, then rebuild.
