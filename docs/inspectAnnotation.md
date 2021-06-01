# Inspect Annotation

Baremetalhost's(BMH) _Status_ sub-resource contains a _hardware_ key
which contains the result of introspection which is carried out during
BMH registration.

In some circumstances it may be desirable to disable this inspection process,
and provide data from external source. The _Inspect Annotation_ provides some
interfaces to enable this.

Note the `inspect.metal3.io/hardwaredetails` annotation is consumed:

* At any time when `inspect.metal3.io: disabled` is specified
* When there is no existing HardwareDetails data in the Status

The `inspect.metal3.io/hardwaredetails` annotation will be removed when
successfully processed or when the status is already set, generating an
event in each case.

The structure of the annotation's value should match the hardware status
field schema, or a subset of that schema, for example:

```yaml
inspect.metal3.io: disabled
inspect.metal3.io/hardwaredetails: '{"systemVendor":{"manufacturer":"QEMU",
"productName":"Standard PC (Q35 + ICH9, 2009)","serialNumber":""},
"firmware":{"bios":{"date":"","vendor":"","version":""}},"ramMebibytes":4096,
"nics":[{"name":"eth0","model":"0x1af4 0x0001","mac":"00:b7:8b:bb:3d:f6",
"ip":"172.22.0.64","speedGbps":0,"vlanId":0,"pxe":true}],
"storage":[{"name":"/dev/sda","rotational":true,"sizeBytes":53687091200,
"vendor":"QEMU","model":"QEMU HARDDISK","serialNumber":"drive-scsi0-0-0-0",
"hctl":"6:0:0:0"}],"cpu":{"arch":"x86_64",
"model":"Intel Xeon E3-12xx v2 (IvyBridge)","clockMegahertz":2494.224,
"flags":["foo"],"count":4},"hostname":"hwdAnnotation-0"}'
```

Apart from that, sometimes you might want to request re-inspection for an
already inspected host. This might be necessary when there was a hardware
change on the host and you want to ensure that BMH status contains the latest
inspection data about your host. To request a new inspection, simply annotating
the host with `inspect.metal3.io` is enough. Once inspection is requested, you should
see the BMH in `inspecting` state until inspection is completed and by the end of
inspection the `inspect.metal3.io` annotation will be removed by Baremetal Operator.

Note that, inspection can be requested only when BMH is in `Ready` state (i.e. before
it is provisioned). The reason for this limitation is because requesting an inspection
for provisioned BMH will result in rebooting the host, which will result in application
downtime running on that host.
