Configuration Settings
======================

The operator supports several configuration options for controlling
its interaction with Ironic.

The provisioner itself is loaded at runtime from a plugin; see
[Provisioner plugins](plugin-provisioners.md) for selecting and
authoring plugins.

`DEPLOY_RAMDISK_URL` -- The URL for the ramdisk of the image
containing the Ironic agent.

`DEPLOY_KERNEL_URL` -- The URL for the kernel to go with the deploy
ramdisk.

`DEPLOY_ISO_URL` -- The URL for the ISO containing the Ironic agent for
drivers that support ISO boot. Optional if kernel/ramdisk are set.

Architecture-specific variants are also accepted, in this order:
`DEPLOY_KERNEL_URL_AARCH64` (or `_X86_64`), then
`DEPLOY_KERNEL_BY_ARCH=aarch64:http://...,x86_64:http://...`, then the
base `DEPLOY_KERNEL_URL`. The same pattern applies to
`DEPLOY_RAMDISK_URL` and `DEPLOY_ISO_URL`.

`IRONIC_NAME` -- The name of the [Ironic resource][IronicCR] to take
configuration from.

`IRONIC_NAMESPACE` -- The namespace of the Ironic resource. Only used when
`IRONIC_NAME` is set. Defaults to `WATCH_NAMESPACE`.

`IRONIC_ENDPOINT` -- The URL for the operator to use when talking to
Ironic. Not used when `IRONIC_NAME` is set.

`IRONIC_CACERT_FILE` -- The path of the CA certificate file of Ironic, if needed

`IRONIC_INSECURE` -- ("True", "False") Whether to skip the ironic certificate
validation. It is highly recommend to not set it to True.

`IRONIC_CLIENT_CERT_FILE` -- The path of the Client certificate file of Ironic,
if needed. Both Client certificate and Client private key must be defined for
client certificate authentication (mTLS) to be enabled.

`IRONIC_CLIENT_PRIVATE_KEY_FILE` -- The path of the Client private key file of Ironic,
if needed. Both Client certificate and Client private key must be defined for
client certificate authentication (mTLS) to be enabled.

`IRONIC_SKIP_CLIENT_SAN_VERIFY` -- ("True", "False") Whether to skip the ironic
client certificate SAN validation.

`IRONIC_CLIENT_CACHE_TTL` -- Duration during which an Ironic client and its
parameters are cached. Defaults to "5s". Set to "0" to disable caching.

`IRONIC_CLIENT_TIMEOUT` -- Timeout for HTTP requests to the Ironic API.
Must be positive. Defaults to "30s".

`BMO_CONCURRENCY` -- The number of concurrent reconciles performed by the
Operator. Default is the number of CPUs, but no less than 2 and no more than 8.

`PROVISIONING_LIMIT` -- The desired maximum number of hosts that could be (de)provisioned
simultaneously by the Operator. The limit does not apply to hosts that use
virtual media for provisioning. The Operator will try to enforce this limit,
but overflows could happen in case of slow provisioners and / or higher number of
concurrent reconciles. For such reasons, it is highly recommended to keep
BMO_CONCURRENCY value lower than the requested PROVISIONING_LIMIT. Default is 20.

`IRONIC_EXTERNAL_URL_V6` -- This is the URL where Ironic will find the
image for nodes that use IPv6. In dual stack environments, this can be
used to tell Ironic which IP version it should set on the BMC.

`PROVISIONING_NETWORK_DISABLED` -- Set to `true` if your deployment does not
feature a provisioning network. This option disables drivers that require a
provisioning network (such as IPMI).

`WATCH_NAMESPACE` -- Namespace(s) for the operator to watch. Empty
watches all namespaces. A comma-separated list watches several
namespaces.

`PROVISIONER_PLUGIN_DIR` -- Directory of provisioner `.so` plugins.
Defaults to `/plugins`. See [Provisioner plugins](plugin-provisioners.md).

`LIVE_ISO_FORCE_PERSISTENT_BOOT_DEVICE` -- (`Default`, `Always`,
`Never`) Controls persistent boot device for live-ISO deployments.

`DIRECT_DEPLOY_FORCE_PERSISTENT_BOOT_DEVICE` -- (`Default`, `Always`,
`Never`) Controls persistent boot device for direct-deploy images.

`IRONIC_NETWORKING_ENABLED` -- Set to `true` to enable the
BareMetalSwitch controller. When enabled, the following must also be
set:

- `IRONIC_SWITCH_CONFIGS_SECRET` -- Secret name written with generated
  switch configuration
- `IRONIC_SWITCH_CREDENTIALS_SECRET` -- Secret name that receives
  switch credentials for Ironic
- `IRONIC_SWITCH_CREDENTIALS_PATH` -- Filesystem path where switch
  credentials are materialized

The `-hostclaims` manager flag enables the HostClaim controller.
That feature is under development and is not ready for use.
PreprovisioningImage integration is enabled with
`-build-preprov-image`.

[IronicCR]: https://github.com/metal3-io/ironic-standalone-operator/blob/main/docs/api.md#ironic

Kustomization Configuration
---------------------------

It is possible to deploy ```baremetal-operator``` with three different operator
configurations, namely:

1. operator with ironic
1. operator without ironic
1. ironic without operator

A detailed overview of the configuration is presented in [Bare Metal Operator
and Ironic Configuration](https://book.metal3.io/bmo/install_baremetal_operator)

Notes on external Ironic
------------------------

When an external Ironic is used, the following requirements must be met:

- Either HTTP basic or no-auth authentication must be used (Keystone is not
  supported).
- API version 1.89 or newer must be available. That is the minimum
  BMO will accept (virtual media attach/detach). When Ironic supports
  a newer microversion, BMO negotiates up to 1.110 (deployment abort),
  also using 1.93 (virtual media GET), 1.95 (disable power off), and
  1.109 (health API) as they become available.
