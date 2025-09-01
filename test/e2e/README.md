# Baremetal Operator end to end tests

The work here is heavily inspired by the Cluster API e2e framework.

The idea is to have fairly flexible framework that allows running tests in
different contexts. For CI we mainly rely on libvirt, but the tests are meant to
be reusable so that they can run also on real hardware. This is accomplished by
tuning the tests based on a configuration file and flags. Examples can be seen
in `config`.

In the CI system we use a script ([ci-e2e.sh](../../hack/ci-e2e.sh)) to make
some preparations before running the test suite. The script creates VMs to
represent BareMetalHosts, and configures either VBMC or sushy-tools to be used
as BMC. Ironic runs in the "host network" of the kind cluster in the test. The
kind cluster is then configured to expose the relevant ports on the actual host
so that they can be reached from the BareMetalHost VMs.

Currently there are two sets of tests, which cannot be ran together in the same
cluster. One is the "optional" set, currently consists of only the
[upgrade tests](upgrade_test.go), and the "main" set, which are the ones
required to run in every BMO PR. One can switch between these sets by
manipulating the `GINKGO_FOCUS` and `GINKGO_SKIP` env vars. In the default
setting, the script sets `GINKGO_SKIP` to `upgrade`.

E.g. Here is how to run the E2E main tests:

```bash
./hack/ci-e2e.sh
```

And here is how to run the E2E optional tests:

```bash
export GINKGO_FOCUS="upgrade"
./hack/ci-e2e.sh
```

`GINKGO_FOCUS` can be set manually to run specific tests. The options for these
can be found as the first string value (formatting included) of the line with
`Describe` or `It`. These can also be combined with other proceeding sections to
match to even more specific test sections. The value `GINKGO_FOCUS` uses is a
regexp that should match the description of the spec but not match the regexp
specified in `GINKGO_SKIP`.

Example:

```go
var _ = Describe("basic", ...
  It("should control power cycle of BMH though annotations", ...
...
  )
)
```

Could be used with:

```bash
export GINKGO_FOCUS="basic should control power"
```

Additionally, if you wish to run multiple different tests, just maually
add another `--focus=` with string to the root Makefile's `test-e2e`
target.

Skipping tests works otherwise similiarly to adding focus, but in the Makefile
`GINKGO_SKIP` is split separated by space. Thus, you can either use
test-specific words with it or you can add another `--skip=` with a longer
string to the `test-e2e` target.

`BMC_PROTOCOL` can also be set manually. By default the [ci-e2e.sh](https://github.com/metal3-io/baremetal-operator/blob/main/hack/ci-e2e.sh)
script runs it as `redfish`, but it can also be set to `redfish-virtualmedia`,
`redfish`, or `ipmi`. Ipmi uses `vbmc` as the BMO e2e emulator, whereas the
others use `sushy-tools`.

After the tests are run, please ensure proper cleanup before running them again.
The due process for ensuring all is clean for the next run is (in the
root directory):

```bash
./hack/clean-e2e.sh
make clean
sudo rm -rf ./test/e2e/images
```

In addition, make sure related docker containers are removed as well.

It is also possible to run the tests with the fixture provider instead of
Ironic. Without any changes, the whole suite (including optional tests) will be
run. Please note, however, that it is quite questionable to call this
configuration "end to end".

Fixture provider is configured to be the default one. This is how to run the e2e
test suite with it:

```bash
make test-e2e
```

## BMCs config

In case you want to run the tests with your own hardware, the information
regarding BMCs should be stored in a yaml file, whose path is exported to
`E2E_BMCS_CONF_FILE` variable (please take a look at
[bmcs-redfish-virtualmedia.yaml](config/bmcs-redfish-virtualmedia.yaml)
to understand the file structure).

## Tests

Here is a list of the currently implemented tests. As mentioned above, we divide
them into two sets, one required and one optional. The required tests must pass
on every PR before merging. Optional tests may be triggered for extra
verification but are not expected to run on all PRs. All tests are checked with
both redfish and ipmi protocols in CI.

**Required tests:**

- Inspection: Check that a BareMetalHost is registered and inspected when
  created, and that it becomes available after the inspection.
- External inspection: Check that a BareMetalHost, with inspection disabled and
  hardware details added through an annotation, skips inspection, accepts the
  hardware details from the annotation and becomes available.
- Re-inspection: Check that an available BareMetalHost is re-inspected when the
  inspect annotation is added and that this updates the hardware details.
- Provisioning: Check that an available BareMetalHost can be provisioned, that
  it can be detatched, deleted and re-created, with the status annotation,
  without affecting the host. Finally checks that the BareMetalHost can be
  deprovisioned and becomes available again.
- Live-ISO: Check that an available BareMetalHost can be provisioned with a
  live-ISO image.
- Automated cleaning: Check that when automated cleaning is enabled the disks are
  cleaned after deprovision.

**Optional tests:**

- Bare Metal Operator upgrade: Check that an older version of the Bare Metal
  Operator works by creating a BareMetalHost with external inspection. Then
  upgrade the Bare Metal Operator and check the the BareMetalHost can be
  provisioned with the upgraded version.
