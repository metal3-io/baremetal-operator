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

1. operator with ironic
2. operator without ironic
3. ironic without operator

A detailed overview of the configuration is presented in [Bare Metal Operator
and Ironic Configuration](deploying.md).
