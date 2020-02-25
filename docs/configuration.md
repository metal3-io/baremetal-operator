Configuration Settings
======================

The operator supports several configuration options for controlling
its interaction with Ironic.

`DEPLOY_RAMDISK_URL` -- The URL for the ramdisk of the image
containing the Ironic agent.

`DEPLOY_KERNEL_URL` -- The URL for the kernel to go with the deploy
ramdisk.

`IRONIC_ENDPOINT` -- The URL for the operator to use when talking to
Ironic.

`IRONIC_INSPECTOR_ENDPOINT` -- The URL for the operator to use when talking to
Ironic Inspector.

`BMO_CONCURRENCY` -- The number of concurrent reconciles performed by the
Operator. Default is 3.

Kustomization Configuration
---------------------------

It is possible to deploy ```baremetal-operator``` with three different operator
configurations, namely:

1. operator without ironic
2. operator with ironic
3. operator with ironic and keepalived

A detailed overview of the Kustomization configuration is presented in [Ironic
Keepalived Configuration](ironic-endpoint-keepalived-configuration.md)
