# Inspect Annotation

Inspection results are stored on the BareMetalHost as
`status.hardware` and in the owned **HardwareData** resource.
Inspection normally runs after registration, unless it is disabled.

## Disabling inspection

Prefer `spec.inspectionMode` on the BareMetalHost:

- `agent` — boot the IPA ramdisk and collect hardware details (default
  when unset)
- `fast` — out-of-band inspection via the BMC (for example Redfish),
  without booting a ramdisk
- `disabled` — skip inspection and move from registering to preparing

The `inspect.metal3.io: disabled` annotation is still honored for
backward compatibility. Do not set both the annotation and
`spec.inspectionMode` to conflicting values; the webhook rejects that.

When inspection is disabled, hardware details can be supplied with the
`inspect.metal3.io/hardwaredetails` annotation. That annotation is
consumed:

- at any time when inspection is disabled
- when there is no existing HardwareDetails data in the status

The `inspect.metal3.io/hardwaredetails` annotation is removed when
successfully processed, or when status is already set, generating an
event in each case.

The annotation value must match the hardware status field schema, or a
subset of that schema, for example:

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

## Requesting re-inspection

To refresh inventory after a hardware change, annotate an `available`
host (the older `ready` name is still accepted) with `inspect.metal3.io`
(any value other than `disabled`). The host moves to `inspecting` until
inspection completes. The operator then removes the annotation.

Re-inspection is not performed on provisioned hosts, because it would
reboot the machine and interrupt workloads.
