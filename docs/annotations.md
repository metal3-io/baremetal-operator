# Status and Inspect Annotations

## Status Annotation
Baremetalhost's(BMH) _Status_ sub-resource contain a handful of critical data
regarding the BMH's state. If the _Status_ is not moved with the object when we
pivot the BMH from the management cluster to target cluster , the
Baremetal Operator(BMO) considers it as a new object and triggers introspection
for the BMH since its _Status_ is empty. _Status_ being empty means although the
 BMH had a _ready_ state in management cluster (for example), it would be again
 in _registering_ and _inspecting_ states in target cluster since we have an
 BMH with empty _Status_ in hand. This is the
main motivation to take the BMH _Status_ also with the object when we move the
object from management cluster to target cluster.

To solve this issue, BMO now puts the _Status_ of a BMH as an _Annotation_ at
the end of each reconciliation loop. The name of the annotation is,
`baremetalhost.metal3.io/status`. This field holds the entire _Status_
sub-resource of BMH. As such within this annotation you will find all the fields
that belong to a BMH _Status_ sub-resource.

The _Status Annotation_ ensures that the _Status_ of a BMH is always preserved
as an annotation. In addition to this, BMO is now enhanced to reconstruct a BMH
object _Status_ if it is empty and the _Status Annotation_ is present.
This makes sure that  when we pivot a BMH with  _Status Annotation_ ,
the BMO at the target cluster can reconstruct the BMH _Status_ and so the
reconciliation of BMH starts from the point it was before pivot. This
essentially ensures all the critical data residing in BMH _Status_ sub-resource
is retained and BMH does not suffer any accidental introspection.

Note that in the case where only the hardware field requires update, the
[inspect annotation](inspectAnnotation.md) may also be used.

Here is an example of a _Status annotation_:

```yaml

baremetalhost.metal3.io/status: '{"operationalStatus":"OK","lastUpdated":
"2020-05-13T15:03:45Z", "hardwareProfile":"unknown","hardware": {"systemVendor":
{"manufacturer":"QEMU","productName":"Standard  PC (Q35 + ICH9, 2009)",
"serialNumber":""},"firmware":{"bios":{"date":"","vendor":"", "version":""}},
"ramMebibytes":4096,"nics":[{"name":"eth0","model":"0x1af4
x0001","mac":"00:55:4a:4a:79:1c","ip":"172.22.0.83","speedGbps":0, "vlanId":0,
"pxe":true},{"name":"eth1","model":"0x1af4 0x0001","mac":"00:55:4a:4a:79:1e",
"ip":"192.168.111.20","speedGbps":0,"vlanId":0,"pxe":false}],"storage":[{ "name"
:"/dev/sda","rotational":true, "sizeBytes":53687091200,"vendor":"QEMU",
"model":"QEMUHARDDISK", "serialNumber":"drive-scsi0-0-0-0","hctl":"6:0:0:0"}],
"cpu":{"arch":"x86_64","model":"Intel Xeon E3-12xx v2 (IvyBridge)",
"clockMegahertz":2593.992,"flags":["aes","apic","arat", "avx","clflush","cmov",
"constant_tsc","cx16","cx8","de","eagerfpu","ept","erms","f16c","flexpriority",
"fpu","fsgsbase","fxsr","hypervisor" ,"lahf_lm","lm","mca","mce","mmx","msr",
"mtrr","nopl","nx","pae","pat", "pclmulqdq","pge","pni","popcnt","pse","pse36",
"rdrand","rdtscp", "rep_good","sep","smep","sse","sse2","sse4_1","sse4_2",
"ssse3","syscall","tpr_shadow","tsc","tsc_adjust","tsc_deadline_timer","vme",
"vmx","vnmi","vpid","x2apic","xsave","xsaveopt","xtopology"],"count":4}
,"hostname":"node-0"},"provisioning":{"state":"provisioned",
"ID":"73c01ec4-5438-4b50-a49d-4cc4633b2ccb",
"image":{"url":"http://172.22.0.1/images/bionic-server-cloudimg-amd64.img
","checksum":"http://172.22.0.1/images/bionic-server-cloudimg-amd64.img.md5sum
"}},"goodCredentials":{"credentials":{"name":"node-0-bmc-secret", "namespace":
  "metal3"},"credentialsVersion":"6139"},"triedCredentials":
{"credentials":{"name":"node-0-bmc-secret","namespace":"metal3"},
"credentialsVersion":"6139"},"errorMessage":"","poweredOn":true,
"operationHistory":{"register":{"start":"2020-05-13T14:57:10Z",
"end":"2020-05-13T14:57:11Z"},"inspect":{"start": "2020-05-13T14:46:13Z",
"end":"2020-05-13T14:48:23Z"},"provision":
{"start":"2020-05-13T14:57:12Z","end":"2020-05-13T15:03:45Z"},
deprovision":{"start":null,"end":null}}}'

```

## Inspect Annotation

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

