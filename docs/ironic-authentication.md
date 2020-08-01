# Authenticating to Ironic

Because hosts under the control of Metal³ need to contact the Ironic and Ironic
Inspector APIs during inspection and provisioning, it is highly advisable to
require authentication on those APIs, since the provisioned hosts running user
workloads will remain connected to the provisioning network.

## Configuration

The `baremetal-operator` supports connecting to Ironic and Ironic Inspector
configured with the following `auth_strategy` modes:

* `noauth` (no authentication)
* `http_basic` (HTTP [Basic access authentication](https://en.wikipedia.org/wiki/Basic_access_authentication))

Note that Keystone authentication methods are not yet supported.

Authentication configuration is read from the filesystem, beginning at the root
directory specified in the environment variable `METAL3_AUTH_ROOT_DIR`. If this
variable is empty or not specified, the default is `/opt/metal3/auth`.

Within the root directory there are separate subdirectories, `ironic` for
Ironic client configuration, and `ironic-inspector` for Ironic Inspector client
configuration. (This allows the data to be populated from separate secrets when
deploying in Kubernetes.)

### `noauth`

This is the default, and will be chosen if the auth root directory does not
exist. In this mode, the baremetal-operator does not attempt to do any
authentication against the Ironic APIs.

### `http_basic`

This mode is configured by files in each authentication subdirectory named
`username` and `password`, and containing the Basic auth username and password,
respectively.
