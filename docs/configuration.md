Configuration Settings
======================

The operator supports several configuration options for controlling
its interaction with Ironic.

`DEPLOY_RAMDISK_URL` -- The URL for the ramdisk of the image
containing the Ironic agent.

`DEPLOY_KERNEL_URL` -- The URL for the kernel to go with the deploy
ramdisk.

`DEPLOY_ISO_URL` -- The URL for the ISO containing the Ironic agent for
drivers that support ISO boot. Optional if kernel/ramdisk are set.

`IRONIC_ENDPOINT` -- The URL for the operator to use when talking to
Ironic.

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

Kustomization Configuration
---------------------------

It is possible to deploy ```baremetal-operator``` with three different operator
configurations, namely:

1. operator with ironic
2. operator without ironic
3. ironic without operator

A detailed overview of the configuration is presented in [Bare Metal Operator
and Ironic Configuration](deploying.md).

Notes on external Ironic
------------------------

When an external Ironic is used, the following requirements must be met:

* Either HTTP basic or no-auth authentication must be used (Keystone is not
  supported).

* API version 1.81 (2023.1 "Antelope" release cycle) or newer must be available.
