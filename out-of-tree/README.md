# Starlark provisioner plugin (out-of-tree)

A standalone Go module that builds the Starlark provisioner as a BMO
`-buildmode=plugin` `.so`, loaded by the operator from `/plugins`. It pins an
upstream BMO release (**`v0.14.0-beta.0`**), exactly as a third-party plugin
author would.

The provisioner delegates every `pkg/provisioner.Provisioner` method to a
function in a user-supplied Starlark script. The reference script
[`scripts/efi-vmedia.star`](scripts/efi-vmedia.star) targets Ironic redfish
virtualmedia UEFI.

## Build

This module has its own `Makefile`:

```sh
make image          # operator image with the layered, load-tested plugin
make plugin         # build a loadable .so locally
make compile-check  # quick compile only
```

Go plugin loading requires the plugin and the operator to share the exact same
module graph and Go toolchain, so the build fetches the pinned BMO source and
compiles against it. `make compile-check` skips that and is not loadable. To
build against a different BMO ref, retarget with the script below first, then
`make image`. The image defaults to `--provisioner starlark`.

## Retarget to another BMO ref

[`hack/retarget-bmo.sh <ref>`](hack/retarget-bmo.sh) points the module at any BMO
tag, branch, or commit. It fetches that source and updates `go.mod`, the
`Dockerfile` toolchain images, and the `Makefile` version to match. Restore with
`git checkout go.mod go.sum Dockerfile Makefile`.

```sh
hack/retarget-bmo.sh v0.14.0-beta.0
hack/retarget-bmo.sh main https://github.com/you/baremetal-operator.git
```

## Run

The operator resolves `--provisioner starlark` to
`/plugins/starlark-provisioner.so`. The script path comes from
`STARLARK_PROVISIONER_SCRIPT` (defaulted in the image to the reference script):

```sh
baremetal-operator --provisioner starlark
```
