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
Baremetal Operator. The script creates a VM to represent a BareMetalHost,
and configures either VBMC or sushy-tools to be used as BMC.

Currently there are two sets of tests, which cannot be ran together in the same
cluster. One is the "optional" set, currently consists of only the [upgrade tests](upgrade_test.go),
and the "main" set, which are the ones required to run in every BMO PR. One can
switch between these sets by manipulating the `GINKGO_FOCUS` and `GINKGO_SKIP` env
vars. In the default setting, the script sets `GINKGO_SKIP` to `upgrade`.

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
Ironic. Without any changes, the whole suite (including optional tests) will be run.
Please note, however, that it is quite questionable to call this configuration
"end to end".

Fixture provider is configured to be the default one. This is how to run the e2e test suite with it:

```bash
make test-e2e
```
