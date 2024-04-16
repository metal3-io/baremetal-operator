# Bare Metal Operator

[![CLOMonitor](https://img.shields.io/endpoint?url=https://clomonitor.io/api/projects/cncf/metal3-io/badge)](https://clomonitor.io/projects/cncf/metal3-io)
[![OpenSSF Scorecard](https://api.securityscorecards.dev/projects/github.com/metal3-io/baremetal-operator/badge)](https://securityscorecards.dev/viewer/?uri=github.com/metal3-io/baremetal-operator)
[![Ubuntu E2E Integration 1.6 build status](https://jenkins.nordix.org/buildStatus/icon?job=metal3-periodic-ubuntu-e2e-integration-test-release-1-6&subject=Ubuntu%20e2e%20integration%201.6)](https://jenkins.nordix.org/view/Metal3%20Periodic/job/metal3-periodic-ubuntu-e2e-integration-test-release-1-6/)
[![CentOS E2E Integration 1.6 build status](https://jenkins.nordix.org/buildStatus/icon?job=metal3-periodic-centos-e2e-integration-test-release-1-6&subject=Centos%20e2e%20integration%201.6)](https://jenkins.nordix.org/view/Metal3%20Periodic/job/metal3-periodic-centos-e2e-integration-test-release-1-6/)

The Bare Metal Operator implements a Kubernetes API for managing bare metal
hosts. It maintains an inventory of available hosts as instances of the
`BareMetalHost` Custom Resource Definition. The Bare Metal Operator knows how
to:

* Inspect the host’s hardware details and report them on the corresponding
  `BareMetalHost`. This includes information about CPUs, RAM, disks, NICs, and
  more.
* Provision hosts with a desired image.
* Clean a host’s disk contents before or after provisioning.

More capabilities are being added regularly. See open issues and pull requests
for more information on work in progress.

For more information about Metal³, the Bare Metal Operator, and other related
components, see the [Metal³ docs](https://github.com/metal3-io/metal3-docs).

## Resources

* [API documentation](docs/api.md)
* [Setup Development Environment](docs/dev-setup.md)
* [Configuration](docs/configuration.md)
* [Testing](docs/testing.md)
* [Publishing Images](docs/publishing-images.md)

## Integration tests

Currently a PR in BMO is tested with CAPM3 integration tests. There are two
types of CAPM3 integration test, CAPM3 e2e integration tests and ansible
integration tests. You will see one of each kind as a required test on a BMO PR.
Eventually these tests will be replaced with BMO's own
[e2e tests](test/e2e/README.md). BMO e2e tests are currently under active
development. Here are the tests which run on different branches in BMO:

**Release-0.5 branch:**

* **/test metal3-periodic-centos-e2e-integration-test-release-1-6** runs CAPM3
  e2e integration tests with CAPM3 release-1.6 branch and BMO **release-0.5**
  branch on Centos
* **/test metal3-periodic-ubuntu-e2e-integration-test-release-1-6** runs
  e2e integration tests with CAPM3 release-1.6 branch and BMO **release-0.5**
  branch on Ubuntu

**Release-0.4 branch:**

* **/test metal3-periodic-centos-e2e-integration-test-release-1-5** runs CAPM3
  e2e integration tests with CAPM3 release-1.5 branch and BMO **release-0.4**
  branch on Centos
* **/test metal3-periodic-ubuntu-e2e-integration-test-release-1-5** runs
  e2e integration tests with CAPM3 release-1.5 branch and BMO **release-0.4**
  branch on Ubuntu

**Release-0.3 branch:**

* **/test metal3-periodic-centos-e2e-integration-test-release-1-4** runs CAPM3
  e2e integration tests with CAPM3 release-1.4 branch and BMO **release-0.3**
  branch on Centos
* **/test metal3-periodic-ubuntu-e2e-integration-test-release-1-4** runs
  e2e integration tests with CAPM3 release-1.4 branch and BMO **release-0.3**
  branch on Ubuntu

### Important Notes

Whenever there is a change in `config/` directory, please remember to run the
following command:

`make manifests`

This will render the `config/render/capm3.yaml`. Please do not change the
content of `config/render/capm3.yaml` manually.
