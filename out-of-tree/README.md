# Starlark provisioner plugin (out-of-tree)

A standalone Go module that builds the Starlark provisioner as a BMO
`-buildmode=plugin` `.so`, loaded by the operator from `/plugins`. It pins an
upstream BMO ref, like any third-party plugin. As committed that ref is
**`main`**, recorded in `Dockerfile` and `Makefile` as `BMO_VERSION` and in
`go.mod` as a pseudo-version. Point it at a release with
[`hack/retarget-bmo.sh`](#retarget-to-another-bmo-ref).

Every `pkg/provisioner.Provisioner` method is delegated to a function in a
user-supplied Starlark script. Two reference scripts ship in the image, both
under `/scripts`.

| Script | Backend | Provisions | Notes |
|---|---|---|---|
| [`scripts/efi-vmedia.star`](scripts/efi-vmedia.star) | Ironic | Yes | Redfish virtualmedia UEFI, the image default |
| [`scripts/redfish-inspect.star`](scripts/redfish-inspect.star) | Redfish only | No | Out-of-band inspection with no Ironic and no ramdisk |

## Build

```sh
make image          # operator image with the plugin layered in
make plugin         # build a loadable .so locally
make compile-check  # quick compile only, not loadable
```

Go plugins must be built against the same module graph and toolchain as the
operator, so the build fetches the pinned BMO source and compiles against it.
The image defaults to `--provisioner starlark`.

## Retarget to another BMO ref

[`hack/retarget-bmo.sh <ref>`](hack/retarget-bmo.sh) points the module at any
BMO tag, branch, or commit, updating `go.mod`, the `Dockerfile` toolchain
images, and the `Makefile` version. Undo with
`git checkout go.mod go.sum Dockerfile Makefile`.

```sh
hack/retarget-bmo.sh v0.14.0-beta.0
hack/retarget-bmo.sh main https://github.com/you/baremetal-operator.git
```

## Run

The operator resolves `--provisioner starlark` to
`/plugins/starlark-provisioner.so`, with the script path from
`STARLARK_PROVISIONER_SCRIPT` (defaulted in the image to the reference script):

```sh
baremetal-operator --provisioner starlark
```

## Out-of-band inspection over Redfish

[`scripts/redfish-inspect.star`](scripts/redfish-inspect.star) inspects a host
through the BMC in seconds without booting a ramdisk, and also serves power,
health and virtual media. It needs no Ironic, so a host reaches `Available` but
never `Provisioned`. The Redfish resources it reads, and how it degrades on a
partial BMC, are specified in
[`docs/redfish-inspection.md`](docs/redfish-inspection.md).

```sh
STARLARK_PROVISIONER_SCRIPT=/scripts/redfish-inspect.star \
  baremetal-operator --provisioner starlark
```

| Variable | Default | Meaning |
|---|---|---|
| `REDFISH_INSPECT_FIELDS` | every section | Comma separated sections to collect, for example `cpu,ramMebibytes,nics,storage`. Dropping `firmware` removes the most requests. |
| `REDFISH_INSECURE` | `false` | `true` skips BMC certificate verification for every host, on top of the per host `disableCertificateVerification`. |

The BMC address must use a Redfish scheme (`redfish`, `redfish-virtualmedia`,
`idrac-redfish`, `ilo5-redfish`, and the other Redfish backed schemes), with an
optional `+http` or `+https` transport suffix. Anything else is rejected.
