# Bare Metal Operator

[![CLOMonitor](https://img.shields.io/endpoint?url=https://clomonitor.io/api/projects/cncf/metal3-io/badge)](https://clomonitor.io/projects/cncf/metal3-io)
[![OpenSSF Scorecard](https://api.securityscorecards.dev/projects/github.com/metal3-io/baremetal-operator/badge)](https://securityscorecards.dev/viewer/?uri=github.com/metal3-io/baremetal-operator)
[![Ubuntu daily main build status](https://jenkins.nordix.org/buildStatus/icon?job=metal3_daily_main_integration_test_ubuntu&subject=Ubuntu%20daily%20main)](https://jenkins.nordix.org/view/Metal3/job/metal3_daily_main_integration_test_ubuntu/)
[![CentOS daily main build status](https://jenkins.nordix.org/buildStatus/icon?job=metal3_daily_main_integration_test_centos&subject=CentOS%20daily%20main)](https://jenkins.nordix.org/view/Metal3/job/metal3_daily_main_integration_test_centos/)
[![BMO e2e periodic main](https://jenkins.nordix.org/view/Metal3%20Periodic/job/metal3-bmo-e2e-test-periodic/badge/icon?subject=BMO%20e2e%20periodic%20main)](https://jenkins.nordix.org/view/Metal3%20Periodic/job/metal3-bmo-e2e-test-periodic/)

The Bare Metal Operator implements a Kubernetes API for managing bare metal
hosts. It maintains an inventory of available hosts as instances of the
`BareMetalHost` Custom Resource Definition. The Bare Metal Operator knows how
to:

- Inspect the host’s hardware details and report them on the corresponding
  `BareMetalHost`. This includes information about CPUs, RAM, disks, NICs, and
  more.
- Provision hosts with a desired image.
- Clean a host’s disk contents before or after provisioning.

More capabilities are being added regularly. See open issues and pull requests
for more information on work in progress.

For more information about Metal³, the Bare Metal Operator, and other related
components, see the [Metal³ docs](https://github.com/metal3-io/metal3-docs).

## Resources

- [API documentation](docs/api.md)
- [Setup Development Environment](docs/dev-setup.md)
- [Configuration](docs/configuration.md)
- [Testing](docs/testing.md)
- [Publishing Images](docs/publishing-images.md)

## Integration tests

Currently a PR in BMO is tested with CAPM3 integration tests. There are two
types of CAPM3 integration test, CAPM3 e2e integration tests and ansible
integration tests. You will see one of each kind as a required test on a BMO PR.
Eventually these tests will be replaced with BMO's own
[e2e tests](test/e2e/README.md). BMO e2e tests are currently under active
development. Here are the tests which run on different branches in BMO:

**E2E tests:**

- **/test metal3-bmo-e2e-test-pull** runs required BMO e2e tests on your PR.
  Works for main and release-0.5 branches.
- **/test metal3-bmo-e2e-test-optional-pull** runs optional BMO e2e tests on
  your PR. These are under active development currently.

For more details on the e2e tests, see [test/e2e/README.md](test/e2e/README.md).

**Main branch:**

- **/test-centos-e2e-integration-main** runs CAPM3 e2e integration tests with
  CAPM3 main branch and BMO **main** branch on Centos
- **/test-ubuntu-integration-main** runs ansible integration tests with CAPM3
  main branch and BMO **main** branch on Ubuntu

**Release-0.5 branch:**

- **/test-centos-e2e-integration-release-1-6** runs CAPM3 e2e integration tests
  with CAPM3 release-1.6 branch and BMO **release-0.5** branch on Centos
- **/test-ubuntu-integration-release-1-6** runs ansible integration tests with
  CAPM3 release-1.6 branch and BMO **release-0.5** branch on Ubuntu

**Release-0.4 branch:**

- **/test-centos-e2e-integration-release-1-5** runs CAPM3 e2e integration tests
  with CAPM3 release-1.5 branch and BMO **release-0.4** branch on Centos
- **/test-ubuntu-integration-release-1-5** runs ansible integration tests with
  CAPM3 release-1.5 branch and BMO **release-0.4** branch on Ubuntu

**Release-0.3 branch:**

- **/test-centos-e2e-integration-release-1-4** runs CAPM3 e2e integration tests
  with CAPM3 release-1.4 branch and BMO **release-0.3** branch on Centos
- **/test-ubuntu-integration-release-1-4** runs ansible integration tests with
  CAPM3 release-1.4 branch and BMO **release-0.3** branch on Ubuntu

### Important Notes

Whenever there is a change in `config/` directory, please remember to run the
following command:

`make manifests`

This will render the `config/render/capm3.yaml`. Please do not change the
content of `config/render/capm3.yaml` manually.
