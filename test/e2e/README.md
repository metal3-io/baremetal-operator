# Baremetal Operator end to end tests

The work here is heavily inspired by the Cluster API e2e framework.

The idea is to have fairly flexible framework that allows running tests in
different contexts. For CI we mainly rely on libvirt, but the tests are meant to
be reusable so that they can run also on real hardware. This is accomplished by
tuning the tests based on a configuration file and flags. Examples can be seen
in `config`.

In the CI system we set up a pre-configured minikube cluster that is then used
for the tests, as seen in [ci-e2e.sh](../../hack/ci-e2e.sh). This allows us to
have control over the network and have a static configuration for Ironic and the
Baremetal Operator. The script creates a VM to represent a BareMetalHost, and
configures either VBMC or sushy-tools to be used as BMC.

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

**Optional tests:**

- Bare Metal Operator upgrade: Check that an older version of the Bare Metal
  Operator works by creating a BareMetalHost with external inspection. Then
  upgrade the Bare Metal Operator and check the the BareMetalHost can be
  provisioned with the upgraded version.
