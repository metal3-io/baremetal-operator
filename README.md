# Bare Metal Operator

The Bare Metal Operator implements a Kubernetes API for managing bare metal
hosts.  It maintains an inventory of available hosts as instances of the
`BareMetalHost` Custom Resource Definition.  The Bare Metal Operator knows how
to:

* Inspect the host’s hardware details and report them on the corresponding
  `BareMetalHost`.  This includes information about CPUs, RAM, disks, NICs, and
  more.
* Provision hosts with a desired image
* Clean a host’s disk contents before or after provisioning.

More capabilities are being added regularly.  See open issues and pull requests
for more information on work in progress.

For more information about Metal³, the Bare Metal Operator, and other related
components, see the [Metal³ docs](https://github.com/metal3-io/metal3-docs).

## Resources

* [API documentation](docs/api.md)
* [Setup Development Environment](docs/dev-setup.md)
* [Configuration](docs/configuration.md)
* [Testing](docs/testing.md)
* [Publishing Images](docs/publishing-images.md)
