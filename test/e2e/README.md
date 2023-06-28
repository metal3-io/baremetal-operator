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
Baremetal Operator. The script also creates a VM and configures VBMC to be used
as BareMetalHost and BMC respectively.

It is also possible to run the tests with the fixture provider instead of
Ironic, but please note that it is quite questionable to call this configuration
"end to end". This is how to run the e2e test suite with the fixture provider:

```bash
export E2E_CONF_FILE=${REPO_ROOT}/test/e2e/config/fixture.yaml
export USE_EXISTING_CLUSTER="false"
make test-e2e
```
