# Out-of-Band Inspection via Redfish

This document describes the Redfish resources the Starlark provisioner reads to
inspect a host without booting a ramdisk. It is the reference for operators and
BMC vendors who want to know which Redfish endpoints and properties must be
served for inspection to produce useful data.

The collection is performed by the `redfish_inventory` builtin, which a
provisioner script calls from `inspect_hardware`. It queries the BMC over
Redfish and returns a dict shaped like the `HardwareDetails` inspection object.

It is written to be implementable from end to end. Two audiences are served:

- Someone building or validating the **service side**: BMC firmware, a Redfish
  emulator, an aggregator, or a test double that this collector must be able to
  inspect. Everything the collector puts on the wire and everything it requires
  back is specified in [Transport contract](#transport-contract),
  [Request sequence](#request-sequence), and
  [Redfish endpoints and expected payloads](#redfish-endpoints-and-expected-payloads).
- Someone reimplementing the **collector side** in another provisioner or
  language. The exact selection, fallback, and degradation rules are in
  [Value rules](#value-rules), [Degradation and error semantics](#degradation-and-error-semantics),
  and [Appendix: collection algorithm](#appendix-collection-algorithm).

## Scope

Everything needed to serve or reimplement the inspection path is here: the JSON
shapes, the enumeration values, the request sequence, the conventions, and the
failure behavior. Building a service that this collector can inspect, or a mock
or emulator to test against, does not require reading the DMTF documents.

The DMTF specifications remain the reference for a general purpose Redfish
service, which this document deliberately does not describe:

- Everything outside the inspection path: the session and account services,
  events and subscriptions, tasks, actions such as `ComputerSystem.Reset`,
  virtual media, BIOS and firmware settings, and OEM extensions. Other Redfish
  builtins in this plugin do use some of those.
- Service-wide obligations a conformant service has regardless of this
  collector: the `$metadata` and `odata` service documents, ETag handling,
  `@odata.type` versioning rules, the `Status` and `Links` objects, and the
  message registries behind extended error payloads.
- The authoritative property list of any resource. Only the properties this
  collector consumes are described here, and a real BMC will and should serve
  many more.

Version floors and enumeration values below were transcribed from the DSP8010
2026.1 bundle, so they substitute for reading the schemas only for the
properties listed.

## Contents

- [Scope](#scope)
- [DMTF specifications](#dmtf-specifications)
- [How it works](#how-it-works)
- [Transport contract](#transport-contract)
- [Request sequence](#request-sequence)
- [Redfish resources consulted](#redfish-resources-consulted)
- [Redfish endpoints and expected payloads](#redfish-endpoints-and-expected-payloads)
- [Collection contract](#collection-contract)
- [JSON conventions](#json-conventions)
- [Enumerations](#enumerations)
- [Value rules](#value-rules)
- [Field mapping](#field-mapping)
- [Degradation and error semantics](#degradation-and-error-semantics)
- [Output contract](#output-contract)
- [Minimum required Redfish support](#minimum-required-redfish-support)
- [Selecting sections](#selecting-sections)
- [Implementing a backend](#implementing-a-backend)
- [Not collected](#not-collected)
- [Appendix: collection algorithm](#appendix-collection-algorithm)

## DMTF specifications

Property names, payload shapes, and version floors in this document come from
the published DMTF standards rather than from any single vendor
implementation.

| Document | Version | Link |
|---|---|---|
| DSP0266, Redfish Specification | 1.24.0 | [PDF](https://www.dmtf.org/sites/default/files/standards/documents/DSP0266_1.24.0.pdf) |
| DSP0268, Redfish Data Model Specification | 2026.1 | [PDF](https://www.dmtf.org/sites/default/files/standards/documents/DSP0268_2026.1.pdf) |
| DSP2046, Redfish Resource and Schema Guide | 2026.1 | [PDF](https://www.dmtf.org/sites/default/files/standards/documents/DSP2046_2026.1.pdf) |
| DSP8010, Redfish Schema Bundle | 2026.1 | [Schema index](https://redfish.dmtf.org/redfish/schema_index) |

Individual schemas are published as JSON Schema at
`https://redfish.dmtf.org/schemas/v1/{Schema}.json` and as CSDL at
`https://redfish.dmtf.org/schemas/v1/{Schema}_v1.xml`. Complete example
payloads for a whole service are in the
[DMTF mockups](https://redfish.dmtf.org/redfish/mockups/v1).

Redfish versions two things separately. The schema bundle has a release
number such as `2018.1`, and each resource inside it has its own version such
as `ComputerSystem 1.5.0`. Version floors below are given as the resource
version followed by the bundle release that first shipped it, for example
`ComputerSystem 1.5.0 (2017.3)`.

Every version in this document was checked against the DSP8010 2026.1 bundle,
where the newest versions of the resources used here are ServiceRoot 1.21.0,
ComputerSystem 1.28.0, Processor 1.23.0, Memory 1.24.0, EthernetInterface
1.12.5, Storage 1.22.0, Drive 1.22.0, SimpleStorage 1.3.2, UpdateService
1.17.1, and SoftwareInventory 1.14.0. No property the collector reads is
deprecated in those versions.

## How it works

`redfish_inventory` opens one HTTP client against the Redfish service, resolves
the first `ComputerSystem`, and reads hardware data from that system and a few
of its subordinate collections. Collection is best effort. Any property the BMC
does not serve is simply omitted, so a partial BMC yields a partial inventory
rather than a failure. The only hard failures are being unable to reach the
service root and being unable to resolve a system, both listed in
[Degradation and error semantics](#degradation-and-error-semantics).

```python
def inspect_hardware(host, data, restart_on_failure, refresh, force_reboot):
    creds = host["BMCCredentials"]
    details = redfish_inventory(
        endpoint = "https://bmc.example.com",
        username = creds["Username"],
        password = creds["Password"],
        insecure = False,
        fields = ["cpu", "ramMebibytes", "nics", "storage"],
    )
    return {"hardwareDetails": details}
```

### Builtin arguments

| Argument | Type | Required | Default | Meaning |
|---|---|:--:|---|---|
| `endpoint` | string | Yes | — | Redfish service base URL, scheme and authority only, for example `https://bmc.example.com` or `https://10.0.0.5:8443`. No trailing slash and no path component. |
| `username` | string | Yes | — | Redfish account name. An empty string sends every request anonymously. |
| `password` | string | Yes | — | Account password. An empty string with a non-empty username also sends every request anonymously, because both halves are required before the `Authorization` header is added. |
| `insecure` | bool | No | `False` | `True` skips BMC certificate verification. |
| `fields` | list of string | No | `None` | Sections to collect. `None` or an empty list collects everything. See [Selecting sections](#selecting-sections). Unknown names select nothing, and non-string entries are ignored. |

The return value is a dict, never `None`. A BMC that serves nothing usable
returns an empty dict. The script decides whether that is enough to report as
`hardwareDetails`.

## Transport contract

This section is the wire-level contract a service must satisfy. Everything here
is observable behavior of the collector, not a restatement of DSP0266.

### Endpoint URL

Every request URI is formed by string concatenation of `endpoint` and a path,
with no URL normalization:

```text
request URL = endpoint + path
```

Two consequences bind the service:

- `endpoint` must carry no trailing slash and no path. `https://bmc/` produces
  `https://bmc//redfish/v1/`, which most services reject.
- Every `@odata.id` the service returns must be an **origin-relative absolute
  path** beginning with `/`, on the same scheme, host, and port as `endpoint`.
  A fully qualified `@odata.id` such as
  `https://bmc.example.com/redfish/v1/Systems/1` becomes
  `https://bmc.example.comhttps://bmc.example.com/redfish/v1/Systems/1` and
  fails. This is the single most common interoperability defect.

### Methods

Inventory collection is read-only. Only `GET` is issued. No `PATCH`, `POST`,
`DELETE`, or `HEAD` request is made, so no ETag or `If-Match` handling is
involved, and no state on the BMC is modified. The other Redfish builtins in
this plugin (power, boot, virtual media) do write, but they are out of scope
here.

### Authentication

- The service root is fetched **before** credentials are configured, so
  `GET /redfish/v1/` must succeed **without** an `Authorization` header. This
  matches DSP0266, which requires the service root to be accessible without
  authentication. A service that answers `401` on the root cannot be inspected
  at all.
- Every subsequent request carries HTTP Basic authentication:
  `Authorization: Basic base64(username ":" password)`. The header is added on
  each request; no session is created.
- No Redfish session is established, so `/redfish/v1/SessionService` and
  `Links.Sessions` are never touched and no `X-Auth-Token` is used. Nothing is
  logged out at the end, and no `DELETE` reaches the service.
- The account needs read access to the resources in
  [Redfish resources consulted](#redfish-resources-consulted). A read-only role
  is sufficient.

### Request headers

Each request is sent with exactly these headers, plus whatever Go's HTTP client
adds (`Host`, `Accept-Encoding`):

| Header | Value | Notes |
|---|---|---|
| `User-Agent` | `gofish/1.0` | Fixed. |
| `Accept` | `application/json` | The service must be able to answer JSON. |
| `Content-Type` | `application/json` | Present even on bodyless `GET` requests. Services that reject a `Content-Type` on a `GET` will fail. |
| `Connection` | `close` | Set on every request. |
| `Authorization` | `Basic ...` | Absent on the service root request, present afterwards. |

No `OData-Version` header is sent, and none is required in the response. No
query parameters are appended: `$expand`, `$select`, `$filter`, `only`, and
`$skip`/`$top` are never used, so `ProtocolFeaturesSupported` is ignored.

### Responses

| Aspect | Requirement |
|---|---|
| Status codes | `200`, `201`, `202`, and `204` are accepted. Any other status is an error for that request, including `3xx` that the HTTP client did not already follow. |
| Redirects | Followed automatically, up to 10 hops. Keep them same-origin: Go strips the `Authorization` header on a cross-host redirect, which turns into a `401`. |
| Body | JSON object. The body must parse as JSON even for the resources whose properties are all optional. |
| Unknown properties | Ignored. A service may return the full resource; only the properties in this document are read. |
| Error bodies | Parsed as the DSP0266 extended error object, `{"error": {"code": ..., "message": ..., "@Message.ExtendedInfo": [...]}}`, and surfaced in the script-visible error text. A non-conforming body is surfaced verbatim. |
| Compression | Optional. `Accept-Encoding` is whatever Go's transport negotiates, normally gzip. |

### Connection behavior

| Aspect | Value |
|---|---|
| Concurrency | One request in flight at a time. Member fetches are issued from separate goroutines but are serialized by a semaphore of 1, so the service never sees overlapping requests from one inventory call. |
| Keep-alive | Disabled. `Connection: close` is sent, so expect roughly one TCP connection per request under HTTP/1.1. |
| HTTP version | HTTP/1.1, or HTTP/2 if the service negotiates it via ALPN. |
| Per-request timeout | 30 seconds, covering connect, TLS handshake, headers, and body. |
| Overall timeout | The reconcile context. Cancellation aborts the in-flight request and the whole builtin. |
| Proxy | `HTTP_PROXY`, `HTTPS_PROXY`, and `NO_PROXY` from the environment are honored. |
| Retries | None. A failed request is not retried inside one inventory call. |

Because requests are serialized with a 30 second ceiling each, request count is
the dominant cost. See [Request sequence](#request-sequence).

### TLS

- Minimum negotiated version is TLS 1.2.
- With `insecure = False` the BMC certificate is validated against the system
  trust store of the plugin container. To trust a private BMC CA, add the CA to
  that trust store, either by baking it into the image or by pointing
  `SSL_CERT_FILE` or `SSL_CERT_DIR` at a bundle that includes it. The Ironic CA
  bundle at `IRONIC_CACERT_FILE` is used by the generic `http_request` builtin
  and is **not** consulted for Redfish connections.
- The certificate must be valid for the host in `endpoint`, including a
  matching SAN. IP-address endpoints need an IP SAN.
- With `insecure = True` verification is skipped entirely, which is the usual
  setting for self-signed BMC certificates.
- Plain `http://` endpoints work but are unauthenticated in transport terms and
  send Basic credentials in the clear.

## Request sequence

Requests are issued in this order, all serialized. `N` is the number of members
in a collection.

| Step | Request | Count | Condition |
|---|---|---|---|
| 1 | `GET /redfish/v1/` | 1 | Always. Unauthenticated. |
| 2 | `GET {ServiceRoot.Systems}` | 1 | Always. |
| 3 | `GET` each member of the Systems collection | N systems | Always. Every member is fetched even though only the first is used. |
| 4 | `GET {ServiceRoot.UpdateService}` | 1 | `firmware` selected and the link is present. |
| 5 | `GET {UpdateService.FirmwareInventory}` | 1 | Step 4 succeeded and the link is present. |
| 6 | `GET` each firmware inventory member | N entries | Step 5 succeeded. All entries are fetched before the BIOS entry is picked. |
| 7 | `GET {ComputerSystem.Memory}` and each member | 1 + N DIMMs | `ramMebibytes` selected **and** `MemorySummary.TotalSystemMemoryGiB` is absent or not positive. |
| 8 | `GET {ComputerSystem.Processors}` and each member | 1 + N processors | `cpu` selected and the link is present. |
| 9 | `GET {ComputerSystem.EthernetInterfaces}` and each member | 1 + N interfaces | `nics` selected and the link is present. |
| 10 | `GET {ComputerSystem.Storage}` and each member | 1 + N controllers | `storage` selected and the link is present. |
| 11 | `GET` each `Drive` link of each storage controller | N drives | Step 10 returned controllers. |
| 12 | `GET {ComputerSystem.SimpleStorage}` and each member | 1 + N controllers | `storage` selected and step 10/11 produced no drives. |

Section order within one call is `systemVendor`, `firmware`, `ramMebibytes`,
`cpu`, `nics`, `storage`. `systemVendor` and `firmware.bios.version` add no
requests, since they come from the already-fetched `ComputerSystem`.

Worked example for a two-socket server with one system, two processors, four
NICs, one storage controller with eight drives, `MemorySummary` present, and a
firmware inventory of 40 entries:

```text
1                      service root
1 + 1                  systems collection + 1 system
1 + 1 + 40             update service + firmware inventory + 40 entries
1 + 2                  processors collection + 2 processors
1 + 4                  ethernet collection + 4 interfaces
1 + 1 + 8              storage collection + 1 controller + 8 drives
= 63 serialized GETs
```

Dropping `firmware` from `fields` removes 42 of them. On BMCs with large
firmware inventories this is the single most effective tuning knob.

A service that returns fully expanded collection members collapses the per
member fetches; see [Collection contract](#collection-contract).

## Redfish resources consulted

Collection URIs are discovered from the service root and resource links, so the
paths below are canonical examples rather than hardcoded strings. Nothing in the
collector assumes a particular URI layout; only `/redfish/v1/` is fixed.

1. Service root, `/redfish/v1/`
1. Systems collection, `/redfish/v1/Systems`, first member is used
1. The system resource and its subordinate collections
   - `/redfish/v1/Systems/{id}/Processors`
   - `/redfish/v1/Systems/{id}/Memory` (fallback for total memory)
   - `/redfish/v1/Systems/{id}/EthernetInterfaces`
   - `/redfish/v1/Systems/{id}/Storage` and each drive
   - `/redfish/v1/Systems/{id}/SimpleStorage` (fallback for storage)
1. Firmware inventory, `/redfish/v1/UpdateService/FirmwareInventory` (only for
   the BIOS vendor and date)

Each resource maps to one DMTF schema. The version column is the oldest
resource version the collector can work with, naming the property that sets
the floor where one property is responsible for it.

| Schema | Version floor | Definition |
|---|---|---|
| ServiceRoot | 1.1.0 (2016.2), 1.0.0 without `UpdateService` | [JSON](https://redfish.dmtf.org/schemas/v1/ServiceRoot.json), [CSDL](https://redfish.dmtf.org/schemas/v1/ServiceRoot_v1.xml) |
| ComputerSystem | 1.1.0 (2016.1) for the `Storage` and `Memory` links, 1.5.0 (2017.3) for `ProcessorSummary.LogicalProcessorCount` | [JSON](https://redfish.dmtf.org/schemas/v1/ComputerSystem.json), [CSDL](https://redfish.dmtf.org/schemas/v1/ComputerSystem_v1.xml) |
| Processor | 1.0.0 (Redfish 1.0) | [JSON](https://redfish.dmtf.org/schemas/v1/Processor.json), [CSDL](https://redfish.dmtf.org/schemas/v1/Processor_v1.xml) |
| Memory | 1.0.0 (2016.1) | [JSON](https://redfish.dmtf.org/schemas/v1/Memory.json), [CSDL](https://redfish.dmtf.org/schemas/v1/Memory_v1.xml) |
| EthernetInterface | 1.0.0 (Redfish 1.0) | [JSON](https://redfish.dmtf.org/schemas/v1/EthernetInterface.json), [CSDL](https://redfish.dmtf.org/schemas/v1/EthernetInterface_v1.xml) |
| Storage | 1.0.0 (2016.1) | [JSON](https://redfish.dmtf.org/schemas/v1/Storage.json), [CSDL](https://redfish.dmtf.org/schemas/v1/Storage_v1.xml) |
| Drive | 1.0.0 (2016.1) | [JSON](https://redfish.dmtf.org/schemas/v1/Drive.json), [CSDL](https://redfish.dmtf.org/schemas/v1/Drive_v1.xml) |
| SimpleStorage | 1.1.0 (2016.1) for `Devices.CapacityBytes` | [JSON](https://redfish.dmtf.org/schemas/v1/SimpleStorage.json), [CSDL](https://redfish.dmtf.org/schemas/v1/SimpleStorage_v1.xml) |
| UpdateService | 1.0.0 (2016.2) | [JSON](https://redfish.dmtf.org/schemas/v1/UpdateService.json), [CSDL](https://redfish.dmtf.org/schemas/v1/UpdateService_v1.xml) |
| SoftwareInventory | 1.2.0 (2018.1) for `Manufacturer` and `ReleaseDate` | [JSON](https://redfish.dmtf.org/schemas/v1/SoftwareInventory.json), [CSDL](https://redfish.dmtf.org/schemas/v1/SoftwareInventory_v1.xml) |
| Resource | 1.1.0 (2016.1) for the `Identifier` durable names | [JSON](https://redfish.dmtf.org/schemas/v1/Resource.json), [CSDL](https://redfish.dmtf.org/schemas/v1/Resource_v1.xml) |

## Redfish endpoints and expected payloads

The examples below are trimmed to the properties the collector reads. Real
responses carry many more fields, which are ignored. Each block is followed by a
property table giving the JSON type, whether the collector needs it, and what it
feeds. "Required" here means required for that one output field, not required by
Redfish.

Link shapes, `null` handling, and value types are specified once in
[JSON conventions](#json-conventions); enumeration values are listed in
[Enumerations](#enumerations).

### Service root

`GET /redfish/v1/`, schema
[ServiceRoot](https://redfish.dmtf.org/schemas/v1/ServiceRoot.json) 1.0.0
(Redfish 1.0). The `UpdateService` link arrived in 1.1.0 (2016.2).

```json
{
  "@odata.id": "/redfish/v1/",
  "Systems": { "@odata.id": "/redfish/v1/Systems" },
  "UpdateService": { "@odata.id": "/redfish/v1/UpdateService" }
}
```

| Property | JSON type | Required | Feeds |
|---|---|:--:|---|
| `Systems.@odata.id` | string (path) | Yes | Everything. Absent means no system can be found and the call fails. |
| `UpdateService.@odata.id` | string (path) | No | `firmware.bios.vendor` and `firmware.bios.date`. |

`RedfishVersion` is read by the client for informational purposes only; no
behavior branches on it.

### Systems collection

`GET /redfish/v1/Systems`, schema
[ComputerSystemCollection](https://redfish.dmtf.org/schemas/v1/ComputerSystemCollection.json).
Collections follow the resource collection pattern in DSP0266, so members are
links rather than expanded resources and each one costs a separate GET.

```json
{
  "Members@odata.count": 1,
  "Members": [ { "@odata.id": "/redfish/v1/Systems/System.1" } ]
}
```

| Property | JSON type | Required | Feeds |
|---|---|:--:|---|
| `Members` | array | Yes | The first element is the inspected system. An empty array fails the call. |
| `Members[].@odata.id` | string (path) | Yes | The system URI, unless the member is expanded inline. |
| `Members@odata.count` | number | No | Ignored. The array length is authoritative. |
| `Members@odata.nextLink` | string (path) | No | Followed if present. |

Only the first member is inspected, but **every** member is fetched first, and a
failure on any of them fails the whole call. A multi-node chassis that exposes
four systems therefore pays four GETs and must serve all four.

### Computer system

`GET /redfish/v1/Systems/System.1`, schema
[ComputerSystem](https://redfish.dmtf.org/schemas/v1/ComputerSystem.json)
1.0.0 (Redfish 1.0). The `Storage` and `Memory` links arrived in 1.1.0
(2016.1) and `ProcessorSummary.LogicalProcessorCount` in 1.5.0 (2017.3).

```json
{
  "Id": "System.1",
  "Manufacturer": "Contoso",
  "Model": "PowerServe R720",
  "SerialNumber": "SN0123456789",
  "BiosVersion": "2.14.1",
  "MemorySummary": { "TotalSystemMemoryGiB": 128 },
  "ProcessorSummary": { "Count": 2, "LogicalProcessorCount": 64 },
  "Processors": { "@odata.id": "/redfish/v1/Systems/System.1/Processors" },
  "Memory": { "@odata.id": "/redfish/v1/Systems/System.1/Memory" },
  "EthernetInterfaces": { "@odata.id": "/redfish/v1/Systems/System.1/EthernetInterfaces" },
  "Storage": { "@odata.id": "/redfish/v1/Systems/System.1/Storage" },
  "SimpleStorage": { "@odata.id": "/redfish/v1/Systems/System.1/SimpleStorage" }
}
```

| Property | JSON type | Required | Feeds |
|---|---|:--:|---|
| `Manufacturer` | string | No | `systemVendor.manufacturer` |
| `Model` | string | No | `systemVendor.productName` |
| `SerialNumber` | string | No | `systemVendor.serialNumber` |
| `BiosVersion` | string | No | `firmware.bios.version` |
| `MemorySummary.TotalSystemMemoryGiB` | number, may be fractional | No | `ramMebibytes`, preferred over the `Memory` collection |
| `ProcessorSummary.LogicalProcessorCount` | integer | No | `cpu.count`, preferred |
| `ProcessorSummary.Count` | integer | No | `cpu.count`, fallback |
| `Processors.@odata.id` | string (path) | No | `cpu.arch`, `cpu.model` |
| `Memory.@odata.id` | string (path) | No | `ramMebibytes` fallback |
| `EthernetInterfaces.@odata.id` | string (path) | No | `nics` |
| `Storage.@odata.id` | string (path) | No | `storage` |
| `SimpleStorage.@odata.id` | string (path) | No | `storage` fallback |

`HostName` is **not** read. See [Not collected](#not-collected).

### Processors

`GET /redfish/v1/Systems/System.1/Processors` then every member, schema
[Processor](https://redfish.dmtf.org/schemas/v1/Processor.json) 1.0.0
(Redfish 1.0). Only the first member contributes data, but all are fetched.

```json
{
  "Id": "CPU.1",
  "InstructionSet": "x86-64",
  "Model": "Contoso Xeon 6338 32C"
}
```

| Property | JSON type | Required | Feeds |
|---|---|:--:|---|
| `InstructionSet` | string enum | No | `cpu.arch` |
| `Model` | string | No | `cpu.model` |

The full `InstructionSet` enumeration is `x86`, `x86-64`, `IA-64`, `ARM-A32`,
`ARM-A64`, `MIPS32`, `MIPS64`, `PowerISA`, `RV32`, `RV64`, and `OEM`. `x86-64`
maps to `x86_64` and `ARM-A64` maps to `aarch64`; anything else is passed
through unchanged. Values are matched exactly, so `X86-64` or `x86_64` in the
payload will not be mapped.

`TotalCores` and `TotalThreads` are not read: the CPU count always comes from
`ProcessorSummary`.

### Memory (fallback for total RAM)

`GET /redfish/v1/Systems/System.1/Memory` then each member, schema
[Memory](https://redfish.dmtf.org/schemas/v1/Memory.json) 1.0.0 (2016.1).
Queried only when `MemorySummary.TotalSystemMemoryGiB` is absent or not
positive.

```json
{
  "Id": "DIMM.1",
  "CapacityMiB": 16384
}
```

| Property | JSON type | Required | Feeds |
|---|---|:--:|---|
| `CapacityMiB` | integer | Yes for this fallback | `ramMebibytes`, summed over all members |

Empty DIMM slots should be served either as members with no `CapacityMiB` or
with `0`; both are skipped by the sum. `Status.State` is not consulted, so a
service must not report absent modules with a nonzero capacity.

### Ethernet interfaces

`GET /redfish/v1/Systems/System.1/EthernetInterfaces` then each member, schema
[EthernetInterface](https://redfish.dmtf.org/schemas/v1/EthernetInterface.json)
1.0.0 (Redfish 1.0).

```json
{
  "Id": "NIC.1",
  "Name": "Integrated NIC 1 Port 1",
  "MACAddress": "aa:bb:cc:dd:ee:01",
  "PermanentMACAddress": "aa:bb:cc:dd:ee:01"
}
```

| Property | JSON type | Required | Feeds |
|---|---|:--:|---|
| `MACAddress` | string | Yes, or `PermanentMACAddress` | `nics[].mac` |
| `PermanentMACAddress` | string | No | `nics[].mac` fallback |
| `Name` | string | No | `nics[].name` |
| `Id` | string | No | `nics[].name` fallback |

Interfaces with neither MAC are skipped. Formatting matters downstream: emit
colon-separated MACs such as `aa:bb:cc:dd:ee:01`. Redfish also permits hyphen
separators, but the `BareMetalHost` CRD validates `nics[].mac` against
`[0-9a-fA-F]{2}(:[0-9a-fA-F]{2}){5}`, so a hyphen-separated address is rejected
by the API server when the status is written.

This collection is the system-scoped one. Manager (BMC) interfaces under
`/redfish/v1/Managers/{id}/EthernetInterfaces` are not read, so the BMC's own
NIC does not appear in the inventory.

### Storage and drives

`GET /redfish/v1/Systems/System.1/Storage` then each member, then each linked
drive. Schemas are
[Storage](https://redfish.dmtf.org/schemas/v1/Storage.json) 1.0.0 (2016.1) and
[Drive](https://redfish.dmtf.org/schemas/v1/Drive.json) 1.0.0 (2016.1).

```json
{
  "Id": "Controller.1",
  "Drives": [ { "@odata.id": ".../Storage/Controller.1/Drives/Disk.0" } ]
}
```

| Property | JSON type | Required | Feeds |
|---|---|:--:|---|
| `Drives` | array of link objects | Yes | One `storage[]` entry per drive |
| `Drives[].@odata.id` | string (path) | Yes | The drive URI |

`GET .../Storage/Controller.1/Drives/Disk.0`

```json
{
  "Id": "Disk.0",
  "Name": "Disk 0",
  "CapacityBytes": 960197124096,
  "MediaType": "SSD",
  "Protocol": "NVMe",
  "Model": "Contoso NVMe 960G",
  "Manufacturer": "Contoso",
  "SerialNumber": "S3W1NA0M700001",
  "Identifiers": [
    { "DurableName": "naa.5000c500a1b2c3d4", "DurableNameFormat": "NAA" }
  ]
}
```

| Property | JSON type | Required | Feeds |
|---|---|:--:|---|
| `CapacityBytes` | integer | Yes | `storage[].sizeBytes` |
| `MediaType` | string enum | No | `storage[].type`, `storage[].rotational` |
| `Protocol` | string enum | No | `storage[].type` when `NVMe` |
| `Model` | string | No | `storage[].model` |
| `Manufacturer` | string | No | `storage[].vendor` |
| `SerialNumber` | string | No | `storage[].serialNumber` |
| `Identifiers[]` | array of `{DurableName, DurableNameFormat}` | No | `storage[].wwn` from the first `NAA` entry |
| `Name` | string | No | `storage[].name` |
| `Id` | string | No | `storage[].name` fallback |

The `MediaType` enumeration is `HDD`, `SSD`, and `SMR`, all present since
Drive 1.0.0. `SMR` is shingled magnetic recording, so it reports type `HDD`
and rotational true. `Protocol` values come from the shared
[Protocol](https://redfish.dmtf.org/schemas/v1/Protocol.json) enumeration; the
literal `NVMe` (that exact casing) is the only value with special meaning.
`Identifiers` is the `Identifier` complex type from
[Resource](https://redfish.dmtf.org/schemas/v1/Resource.json) 1.1.0 (2016.1),
whose `DurableNameFormat` carries the `NAA` value used for the WWN.

Drives reached only through `Chassis` links, or listed under
`Storage.Volumes` rather than `Storage.Drives`, are not seen. A service that
wants its disks inspected must link them from `Storage.Drives`.

### Simple storage (fallback for storage)

`GET /redfish/v1/Systems/System.1/SimpleStorage` then each member, schema
[SimpleStorage](https://redfish.dmtf.org/schemas/v1/SimpleStorage.json) 1.0.0
(Redfish 1.0), with `Devices.CapacityBytes` added in 1.1.0 (2016.1). Queried
only when no drives were found under `Storage`.

```json
{
  "Id": "Controller.1",
  "Devices": [
    {
      "Name": "SATA Bay 1",
      "CapacityBytes": 1000204886016,
      "Model": "Contoso HDD 1T",
      "Manufacturer": "Contoso"
    }
  ]
}
```

| Property | JSON type | Required | Feeds |
|---|---|:--:|---|
| `Devices[].CapacityBytes` | integer | Yes | `storage[].sizeBytes` |
| `Devices[].Name` | string | No | `storage[].name` (no `Id` fallback here) |
| `Devices[].Model` | string | No | `storage[].model` |
| `Devices[].Manufacturer` | string | No | `storage[].vendor` |

`SimpleStorage` entries never carry `type`, `rotational`, `serialNumber`, or
`wwn`, because the schema has no equivalent properties. A device whose four
readable properties are all absent or zero contributes no entry.

### Update service and firmware inventory

`GET /redfish/v1/UpdateService`, schema
[UpdateService](https://redfish.dmtf.org/schemas/v1/UpdateService.json) 1.0.0
(2016.2).

```json
{
  "FirmwareInventory": { "@odata.id": "/redfish/v1/UpdateService/FirmwareInventory" }
}
```

`GET /redfish/v1/UpdateService/FirmwareInventory` then each member, schema
[SoftwareInventory](https://redfish.dmtf.org/schemas/v1/SoftwareInventory.json)
1.0.0 (2016.2).

```json
{
  "Id": "BIOS",
  "Name": "System BIOS",
  "Version": "2.14.1",
  "Manufacturer": "Contoso",
  "ReleaseDate": "2026-03-01T00:00:00Z"
}
```

| Property | JSON type | Required | Feeds |
|---|---|:--:|---|
| `Id` | string | No | BIOS entry matching |
| `Name` | string | No | BIOS entry matching |
| `SoftwareId` | string | No | BIOS entry matching |
| `Manufacturer` | string | No | `firmware.bios.vendor` |
| `ReleaseDate` | string | No | `firmware.bios.date`, copied verbatim |

Entry selection: `Name`, `Id`, and `SoftwareId` are concatenated, lowercased,
and split on space, `.`, `_`, and `-`; the entry matches if any resulting token
is exactly `bios`. The first matching member in collection order wins. So
`System BIOS`, `BIOS`, and `bios-x64` match, while `BIOSConnect` does not.
`Version` from this resource is not used; `firmware.bios.version` comes from
`ComputerSystem.BiosVersion`.

`Manufacturer` and `ReleaseDate` were added in SoftwareInventory 1.2.0
(2018.1), and `SoftwareId` in 1.1.0 (2016.3), so an older service exposes the
entry without them. This lookup is skipped when the service root has no
`UpdateService`. `ReleaseDate` is passed through as a string with no parsing or
reformatting, so whatever the BMC returns lands in the `BareMetalHost` status.

## Collection contract

Every collection above is handled the same way.

- Members come from the `Members` array. `Members@odata.count` is ignored.
- If a member object contains an `Id` property, it is treated as an **inline
  expanded resource** and used as-is, with no follow-up GET. A service that
  returns expanded members reduces the request count of a full inventory to one
  GET per collection. This is a valid and much faster way to serve the
  collector, and it does not require advertising `$expand` support, since the
  collector never asks for expansion.
- Otherwise the member's `@odata.id` is fetched.
- If a member object contains `@Message.ExtendedInfo`, it is treated as an
  error for that member rather than as data.
- `Members@odata.nextLink` is followed recursively until absent, and pages are
  concatenated in order. Paging is therefore supported but adds a serialized
  round trip per page.
- Member order is preserved in the **result**. "First system" and "first
  processor" mean the first element of `Members`, across pages, after removing
  members that failed to fetch.
- Member **request** order is not guaranteed. One goroutine per member is
  started and the semaphore lets them through one at a time, so the service may
  well be asked for member 2 before member 1. Never make a response depend on
  the order in which members were requested.
- A member that fails to fetch does not stop the other members from being
  fetched, but it does mark the whole collection call as failed. The
  consequences differ per collection and are listed in
  [Degradation and error semantics](#degradation-and-error-semantics).

## JSON conventions

These apply to every payload in this document.

### Links

A link is an object carrying a single `@odata.id` string:

```json
{
  "Storage": { "@odata.id": "/redfish/v1/Systems/1/Storage" }
}
```

- `href` is accepted as an alias when `@odata.id` is absent or empty, so
  `{"href": "/redfish/v1/Systems/1/Storage"}` resolves too. Prefer `@odata.id`;
  `href` is a client-side tolerance, not what DSP0266 specifies here.
- A link property set to `null`, or to an object carrying neither key, is
  treated exactly like an absent link. It does not fail the decode, so
  `"Memory": null` is a safe way to say "not implemented".
- **Never emit an empty `@odata.id`.** An empty link is not skipped: it
  resolves to the service root. A `Drives` array containing `{"@odata.id": ""}`
  therefore yields a phantom storage entry built from the service root
  document, typically `{"name": "Root Service", "rotational": false}`.
- `Storage.Drives` is an array of link objects. `Members` on every collection is
  an array of link objects, or of expanded resources as described in
  [Collection contract](#collection-contract).

### Resource identity

| Property | Role for the collector |
|---|---|
| `@odata.id` | Required on link objects and collection members. Not read from the fetched resource itself. |
| `Id` | Marks a collection member as inline expanded, and is the `name` fallback for NICs and drives. |
| `Name` | Optional, preferred over `Id` for NIC and drive names. |
| `@odata.type`, `@odata.context`, `@odata.etag` | Never read. Required of a conformant service by DSP0266, ignored here. |

Payloads are decoded structurally, by property name, so a resource whose
`@odata.type` names a different schema is not rejected. That is why an empty
link silently produces a drive-shaped service root rather than an error.

### Value types

| Redfish type | JSON | Handling |
|---|---|---|
| String | string | `null` decodes to an empty string, which is dropped from the output. |
| Integer | number without a fraction | Absent, `null`, `0`, and negative are all equivalent to "not reported". |
| Number | number | `MemorySummary.TotalSystemMemoryGiB` is the only fractional value read. |
| Date-time | string | `SoftwareInventory.ReleaseDate` should be an RFC 3339 timestamp but is never parsed; the string is stored as-is. |
| Enumeration | string | Matched exactly and case-sensitively. See [Enumerations](#enumerations). |
| Boolean | `true` / `false` | No boolean property is read. |

Property **names** are matched case-insensitively, so `macaddress` still decodes
into `MACAddress`. Enumeration **values** are not: `x86_64` in an
`InstructionSet` is passed through unmapped.

## Enumerations

Only four enumerations affect the output. Values not listed as mapped are
passed through or ignored, never rejected.

`Processor.InstructionSet`, complete enumeration since Processor 1.0.0:

| Value | Emitted as `cpu.arch` |
|---|---|
| `x86-64` | `x86_64` |
| `ARM-A64` | `aarch64` |
| `x86`, `IA-64`, `ARM-A32`, `MIPS32`, `MIPS64`, `PowerISA`, `RV32`, `RV64`, `OEM` | verbatim |

`Drive.MediaType`, complete enumeration since Drive 1.0.0:

| Value | `storage[].type` | `storage[].rotational` |
|---|---|---|
| `HDD` | `HDD` | `true` |
| `SMR` | `HDD` | `true` |
| `SSD` | `SSD` | `false` |
| absent or unknown | key omitted | `false` |

`Drive.Protocol`, from the shared `Protocol` enumeration. Only `NVMe` changes
the output, forcing `storage[].type` to `NVME` whatever the media type says.
The other values are accepted and ignored: `PCIe`, `AHCI`, `UHCI`, `SAS`,
`SATA`, `USB`, `FC`, `iSCSI`, `FCoE`, `FCP`, `FICON`, `NVMeOverFabrics`, `SMB`,
`NFSv3`, `NFSv4`, `HTTP`, `HTTPS`, `FTP`, `SFTP`, `iWARP`, `RoCE`, `RoCEv2`,
`I2C`, `TCP`, `UDP`, `TFTP`, `GenZ`, `MultiProtocol`, `InfiniBand`, `Ethernet`,
`NVLink`, `OEM`, `DisplayPort`, `HDMI`, `VGA`, `DVI`, `CXL`, `UPI`, `QPI`,
`eMMC`, `UET`, `UALink`.

`Identifier.DurableNameFormat`, from the shared `Resource` schema. Only `NAA`
is read, and it supplies `storage[].wwn`. The rest are ignored: `iQN`,
`FC_WWN`, `UUID`, `EUI`, `NQN`, `NSID`, `NGUID`, `MACAddress`, `GCXLID`.

## Value rules

These rules decide what ends up in the output. A reimplementation must follow
them to produce identical results.

| Rule | Detail |
|---|---|
| Absent equals zero | Numeric properties are only used when present **and** greater than zero. `CapacityBytes: 0`, `CapacityMiB: 0`, `LogicalProcessorCount: 0`, and a missing property are indistinguishable in the output. |
| Empty strings are dropped | A string property that is empty produces no output key at all, rather than an empty value. |
| GiB to MiB | `ramMebibytes = int(TotalSystemMemoryGiB * 1024)`, truncated toward zero. `127.5` GiB becomes `130560` MiB. Services that report fractional GiB lose sub-MiB precision, which is harmless; services that round to whole GiB lose up to 1 GiB. |
| Memory fallback | The `Memory` collection is only queried when the summary is absent or not positive. The sum sees every member that reports a positive `CapacityMiB`. |
| CPU count preference | `LogicalProcessorCount` first, `Count` second, otherwise no `count` key. |
| CPU arch and model | Taken from the **first** processor member only. A heterogeneous system reports the first processor's values. |
| Arch mapping | `x86-64` to `x86_64`, `ARM-A64` to `aarch64`, everything else verbatim, including `OEM` and vendor strings. |
| NIC MAC preference | `MACAddress` first, `PermanentMACAddress` second. No MAC means the interface is dropped. |
| Name fallback | `Name` if non-empty, otherwise `Id`, for NICs and drives. `SimpleStorage` devices have `Name` only. |
| Drive type | `Protocol == "NVMe"` gives `NVME` regardless of media type. Otherwise `HDD` for `HDD` or `SMR`, `SSD` for `SSD`, and no `type` key for anything else. |
| Rotational | `true` only for `MediaType` of `HDD` or `SMR`. The key is always present on `Storage` drives, including as `false`; it is never present on `SimpleStorage` entries. |
| WWN | The first `Identifiers` entry with `DurableNameFormat == "NAA"` and a non-empty `DurableName`. Other formats (`EUI`, `UUID`, `FC_WWN`, `iQN`) are ignored. The value is copied verbatim, including any `naa.` prefix the BMC uses. |
| Storage fallback | `SimpleStorage` is consulted only when the `Storage` walk produced zero drive entries, whether because of an error, a missing link, or genuinely empty collections. |
| Empty sections | A section that produces an empty map or list is omitted from the result entirely. |

## Field mapping

Each row lists a `HardwareDetails` field, the Redfish source it is read from,
the schema version that first defined that source, and whether it is required
for a useful inspection. Required means the field an Ironic style inspection
treats as mandatory (CPU, memory, a root disk, and a NIC MAC). Everything else
is optional enrichment.

| Inspection field | Redfish source | Since | Required |
|---|---|---|:--:|
| `cpu.count` | `ComputerSystem.ProcessorSummary.LogicalProcessorCount` then `Count` | ComputerSystem 1.5.0 (2017.3), `Count` since 1.0.0 | Yes |
| `cpu.arch` | `Processor.InstructionSet` (mapped to `x86_64` / `aarch64`) | 1.0.0 (Redfish 1.0) | Yes |
| `cpu.model` | `Processor.Model` | 1.0.0 (Redfish 1.0) | No |
| `ramMebibytes` | `ComputerSystem.MemorySummary.TotalSystemMemoryGiB`, else sum of `Memory.CapacityMiB` | ComputerSystem 1.0.0 (Redfish 1.0), Memory 1.0.0 reached through the `Memory` link in ComputerSystem 1.1.0 (2016.1) | Yes |
| `storage[].sizeBytes` | `Drive.CapacityBytes`, else `SimpleStorage.Devices.CapacityBytes` | Drive 1.0.0 (2016.1), SimpleStorage 1.1.0 (2016.1) | Yes |
| `storage[].type` | `Drive.MediaType` and `Drive.Protocol` (HDD / SSD / NVME) | 1.0.0 (2016.1) | No |
| `storage[].rotational` | `Drive.MediaType` of `HDD` or `SMR` | 1.0.0 (2016.1) | No |
| `storage[].model` | `Drive.Model`, else `SimpleStorage.Devices.Model` | Drive 1.0.0 (2016.1), SimpleStorage 1.0.0 (Redfish 1.0) | No |
| `storage[].vendor` | `Drive.Manufacturer`, else `SimpleStorage.Devices.Manufacturer` | Drive 1.0.0 (2016.1), SimpleStorage 1.0.0 (Redfish 1.0) | No |
| `storage[].serialNumber` | `Drive.SerialNumber` | 1.0.0 (2016.1) | No |
| `storage[].wwn` | `Drive.Identifiers` with `DurableNameFormat` of `NAA` | Drive 1.0.0 (2016.1), Resource 1.1.0 (2016.1) | No |
| `storage[].name` | `Drive.Name` then `Drive.Id`, else `SimpleStorage.Devices.Name` | 1.0.0 (2016.1) | No |
| `nics[].mac` | `EthernetInterface.MACAddress` then `PermanentMACAddress` | 1.0.0 (Redfish 1.0) | Yes |
| `nics[].name` | `EthernetInterface.Name` then `EthernetInterface.Id` | 1.0.0 (Redfish 1.0) | No |
| `systemVendor.manufacturer` | `ComputerSystem.Manufacturer` | 1.0.0 (Redfish 1.0) | No |
| `systemVendor.productName` | `ComputerSystem.Model` | 1.0.0 (Redfish 1.0) | No |
| `systemVendor.serialNumber` | `ComputerSystem.SerialNumber` | 1.0.0 (Redfish 1.0) | No |
| `firmware.bios.version` | `ComputerSystem.BiosVersion` | 1.0.0 (Redfish 1.0) | No |
| `firmware.bios.vendor` | `SoftwareInventory.Manufacturer` of the BIOS firmware entry | 1.2.0 (2018.1) | No |
| `firmware.bios.date` | `SoftwareInventory.ReleaseDate` of the BIOS firmware entry | 1.2.0 (2018.1) | No |

## Degradation and error semantics

Only failures reaching the service root or resolving a system fail the builtin.
Everything else degrades to a missing field. A failed builtin raises a Starlark
error prefixed `redfish_inventory:`, which aborts `inspect_hardware`. Starlark
has no exception handling, so a script cannot recover from it; a script that
must tolerate an unreachable BMC has to avoid calling the builtin, for example
by checking reachability first.

| Condition | Effect |
|---|---|
| Cannot reach the service root: DNS, connect, TLS, timeout, non-2xx on `GET /redfish/v1/`, or unparseable body | **Fatal.** No inventory. |
| Service root has no `Systems` link, or the collection has no members | **Fatal**, reported as `no computer system found`. |
| **Any** member of the Systems collection fails to fetch | **Fatal**, even when the first system itself was fetched successfully. |
| `Processors` link absent, collection fetch fails, or any processor member fails | `cpu.arch` and `cpu.model` omitted. `cpu.count` still comes from `ProcessorSummary`. |
| No `ProcessorSummary` counts | `cpu.count` omitted. If arch and model are also missing, the whole `cpu` section is omitted. |
| `Memory` link absent, collection fetch fails, or any DIMM member fails | `ramMebibytes` omitted. Only reachable when `MemorySummary` is absent. |
| `EthernetInterfaces` link absent, collection fetch fails, or **any** interface member fails | `nics` omitted **entirely**, including the interfaces that were fetched successfully. |
| An individual interface has no MAC | That interface is skipped; the rest are kept. |
| `Storage` link absent, collection fetch fails, or **any** storage member fails | The whole `Storage` walk is abandoned, then `SimpleStorage` is tried. |
| One controller's `Drives` fetch fails, including a single failing drive | That controller contributes no drives. Other controllers still contribute. |
| `SimpleStorage` absent or failing, with no `Storage` drives | `storage` omitted. |
| `UpdateService` link absent, or the resource or firmware inventory fails to fetch, or no entry matches `bios` | `firmware.bios.vendor` and `firmware.bios.date` omitted. `firmware.bios.version` still comes from `ComputerSystem`. |

The practical rule for a service implementation: **never advertise a link you
cannot serve, and never list a member you cannot return**. A collection that
404s on one of its own members is worse than a collection that is absent,
because it discards the siblings that did work.

## Output contract

The builtin returns a Starlark dict whose keys mirror the JSON tags of
`metal3api.HardwareDetails`. The script normally returns it under
`hardwareDetails`, and the provisioner decodes it with a JSON round trip into
`HardwareDetails`:

- Keys that do not exist on `HardwareDetails` are silently dropped.
- A value whose type does not match the Go field fails `inspect_hardware` with
  `parse hardwareDetails: ...`.
- Absent keys become Go zero values, which are omitted from the
  `BareMetalHost` status because every field is `omitempty`.

Shape and types, using the emitted key names:

```text
systemVendor.manufacturer     string
systemVendor.productName      string
systemVendor.serialNumber     string
firmware.bios.version         string
firmware.bios.vendor          string
firmware.bios.date            string
ramMebibytes                  int
cpu.count                     int
cpu.arch                      string
cpu.model                     string
nics[].mac                    string, colon-separated
nics[].name                   string
storage[].name                string
storage[].rotational          bool, always present for Storage drives
storage[].type                string, one of HDD, SSD, NVME
storage[].sizeBytes           int64, bytes
storage[].model               string
storage[].vendor              string
storage[].serialNumber        string
storage[].wwn                 string
```

Constraints enforced later by the `BareMetalHost` CRD, so a backend must respect
them or the status write is rejected:

| Field | Constraint |
|---|---|
| `nics[].mac` | Must contain `[0-9a-fA-F]{2}(:[0-9a-fA-F]{2}){5}`. |
| `storage[].type` | Must be exactly `HDD`, `SSD`, `NVME`, or absent. |
| `storage[].sizeBytes` | Signed 64-bit; bytes, not blocks. |

`storage[].name` here is a Redfish drive name such as `Disk.0` or `Disk 0`, not
a Linux device path. Anything downstream that expects `/dev/...`, such as a root
device hint matched by name, will not match an out-of-band inventory.

Complete example output for the payloads shown above, with all sections
selected:

```json
{
  "systemVendor": {
    "manufacturer": "Contoso",
    "productName": "PowerServe R720",
    "serialNumber": "SN0123456789"
  },
  "firmware": {
    "bios": {
      "version": "2.14.1",
      "vendor": "Contoso",
      "date": "2026-03-01T00:00:00Z"
    }
  },
  "ramMebibytes": 131072,
  "cpu": {
    "count": 64,
    "arch": "x86_64",
    "model": "Contoso Xeon 6338 32C"
  },
  "nics": [
    { "mac": "aa:bb:cc:dd:ee:01", "name": "Integrated NIC 1 Port 1" }
  ],
  "storage": [
    {
      "name": "Disk 0",
      "rotational": false,
      "type": "NVME",
      "sizeBytes": 960197124096,
      "model": "Contoso NVMe 960G",
      "vendor": "Contoso",
      "serialNumber": "S3W1NA0M700001",
      "wwn": "naa.5000c500a1b2c3d4"
    }
  ]
}
```

## Minimum required Redfish support

For an inspection that downstream provisioning can use, the BMC must serve:

- `ComputerSystem` with `ProcessorSummary` (a CPU count) and either
  `MemorySummary.TotalSystemMemoryGiB` or a populated `Memory` collection.
- `Processors` with at least `InstructionSet` on the first processor, so the
  CPU architecture can be reported.
- `Storage` with `Drives` that expose `CapacityBytes` (or a `SimpleStorage`
  collection with device capacity), so a root disk is discoverable.
- `EthernetInterfaces` with a `MACAddress` (or `PermanentMACAddress`) on at
  least one interface, so a port exists for provisioning.

A BMC that serves only some of these still returns a partial inventory. The
provisioner script decides whether that is enough.

### Minimum schema versions

The required set is satisfied by a service built on the 2016.1 schema bundle,
because that release introduced the `Storage` and `Memory` links on
`ComputerSystem` along with the `Storage`, `Drive`, and `Memory` resources
themselves. Everything else the collector reads is either older or optional.

| Goal | Floor |
|---|---|
| Required fields only | ComputerSystem 1.1.0, Storage 1.0.0, Drive 1.0.0, Memory 1.0.0, Processor 1.0.0, EthernetInterface 1.0.0 (2016.1) |
| Required fields via `SimpleStorage` and `MemorySummary` | ComputerSystem 1.0.0, SimpleStorage 1.1.0 (2016.1) |
| Logical processor count instead of socket count | ComputerSystem 1.5.0 (2017.3) |
| Every field the collector can report | SoftwareInventory 1.2.0 (2018.1) |

Redfish schemas are backward compatible within a major version, so a service
newer than these floors serves the same properties under the same names.

## Selecting sections

The optional `fields` argument limits collection to the named sections, which
also skips the Redfish calls for the sections you leave out. Valid section
names are the top level inspection keys:

| Section | Extra requests | Notes |
|---|---|---|
| `systemVendor` | none | Read from the already-fetched `ComputerSystem`. |
| `firmware` | 2 plus one per firmware entry | Usually the most expensive section. `bios.version` alone needs no extra request, but vendor and date do. |
| `ramMebibytes` | none, or 1 plus one per DIMM | Free when `MemorySummary` is served. |
| `cpu` | 1 plus one per processor | |
| `nics` | 1 plus one per interface | |
| `storage` | 1 plus one per controller plus one per drive, and the `SimpleStorage` walk on fallback | |

For example `fields = ["cpu", "ramMebibytes", "nics", "storage"]` collects the
required data and skips vendor and firmware lookups.

## Implementing a backend

### Minimum viable service

Ten documents are enough for a complete required-field inventory. Paths are
arbitrary as long as they match the `@odata.id` values, and `UpdateService` is
omitted here so no firmware lookup happens.

```text
GET /redfish/v1/
{
  "@odata.id": "/redfish/v1/",
  "@odata.type": "#ServiceRoot.v1_5_0.ServiceRoot",
  "Id": "RootService",
  "Name": "Root Service",
  "RedfishVersion": "1.6.0",
  "Systems": { "@odata.id": "/redfish/v1/Systems" }
}

GET /redfish/v1/Systems
{
  "@odata.id": "/redfish/v1/Systems",
  "@odata.type": "#ComputerSystemCollection.ComputerSystemCollection",
  "Name": "Computer System Collection",
  "Members@odata.count": 1,
  "Members": [ { "@odata.id": "/redfish/v1/Systems/1" } ]
}

GET /redfish/v1/Systems/1
{
  "@odata.id": "/redfish/v1/Systems/1",
  "@odata.type": "#ComputerSystem.v1_5_0.ComputerSystem",
  "Id": "1",
  "Name": "System",
  "Manufacturer": "Contoso",
  "Model": "PowerServe R720",
  "SerialNumber": "SN0123456789",
  "BiosVersion": "2.14.1",
  "MemorySummary": { "TotalSystemMemoryGiB": 128 },
  "ProcessorSummary": { "Count": 2, "LogicalProcessorCount": 64 },
  "Processors": { "@odata.id": "/redfish/v1/Systems/1/Processors" },
  "EthernetInterfaces": { "@odata.id": "/redfish/v1/Systems/1/EthernetInterfaces" },
  "Storage": { "@odata.id": "/redfish/v1/Systems/1/Storage" }
}

GET /redfish/v1/Systems/1/Processors
{
  "@odata.id": "/redfish/v1/Systems/1/Processors",
  "@odata.type": "#ProcessorCollection.ProcessorCollection",
  "Name": "Processor Collection",
  "Members": [ { "@odata.id": "/redfish/v1/Systems/1/Processors/CPU.1" } ]
}

GET /redfish/v1/Systems/1/Processors/CPU.1
{
  "@odata.id": "/redfish/v1/Systems/1/Processors/CPU.1",
  "@odata.type": "#Processor.v1_3_0.Processor",
  "Id": "CPU.1",
  "Name": "Processor 1",
  "InstructionSet": "x86-64",
  "Model": "Contoso Xeon 6338 32C"
}

GET /redfish/v1/Systems/1/EthernetInterfaces
{
  "@odata.id": "/redfish/v1/Systems/1/EthernetInterfaces",
  "@odata.type": "#EthernetInterfaceCollection.EthernetInterfaceCollection",
  "Name": "Ethernet Interface Collection",
  "Members": [ { "@odata.id": "/redfish/v1/Systems/1/EthernetInterfaces/NIC.1" } ]
}

GET /redfish/v1/Systems/1/EthernetInterfaces/NIC.1
{
  "@odata.id": "/redfish/v1/Systems/1/EthernetInterfaces/NIC.1",
  "@odata.type": "#EthernetInterface.v1_4_0.EthernetInterface",
  "Id": "NIC.1",
  "Name": "Integrated NIC 1 Port 1",
  "MACAddress": "aa:bb:cc:dd:ee:01",
  "PermanentMACAddress": "aa:bb:cc:dd:ee:01"
}

GET /redfish/v1/Systems/1/Storage
{
  "@odata.id": "/redfish/v1/Systems/1/Storage",
  "@odata.type": "#StorageCollection.StorageCollection",
  "Name": "Storage Collection",
  "Members": [ { "@odata.id": "/redfish/v1/Systems/1/Storage/RAID.1" } ]
}

GET /redfish/v1/Systems/1/Storage/RAID.1
{
  "@odata.id": "/redfish/v1/Systems/1/Storage/RAID.1",
  "@odata.type": "#Storage.v1_5_0.Storage",
  "Id": "RAID.1",
  "Name": "Storage Controller",
  "Drives": [
    { "@odata.id": "/redfish/v1/Systems/1/Storage/RAID.1/Drives/Disk.0" }
  ]
}

GET /redfish/v1/Systems/1/Storage/RAID.1/Drives/Disk.0
{
  "@odata.id": "/redfish/v1/Systems/1/Storage/RAID.1/Drives/Disk.0",
  "@odata.type": "#Drive.v1_5_0.Drive",
  "Id": "Disk.0",
  "Name": "Disk 0",
  "CapacityBytes": 960197124096,
  "MediaType": "SSD",
  "Protocol": "NVMe",
  "Model": "Contoso NVMe 960G",
  "Manufacturer": "Contoso",
  "SerialNumber": "S3W1NA0M700001",
  "Identifiers": [
    { "DurableName": "naa.5000c500a1b2c3d4", "DurableNameFormat": "NAA" }
  ]
}
```

That service yields the example output in
[Output contract](#output-contract) minus `firmware.bios.vendor` and
`firmware.bios.date`.

### Serving it from static files

Because collection is read-only, a static file tree behind any web server is a
complete backend for testing. Name each file after its `@odata.id` and let the
server map directory URIs to the JSON body:

```text
redfish/v1/index.json
redfish/v1/Systems/index.json
redfish/v1/Systems/1/index.json
redfish/v1/Systems/1/Processors/index.json
redfish/v1/Systems/1/Processors/CPU.1/index.json
...
```

```nginx
server {
    listen 8443 ssl;
    ssl_certificate     /etc/nginx/redfish.crt;
    ssl_certificate_key /etc/nginx/redfish.key;
    root /srv/redfish-mock;

    location / {
        default_type application/json;
        try_files $uri $uri/index.json =404;
    }

    auth_basic           "redfish";
    auth_basic_user_file /etc/nginx/redfish.htpasswd;

    # The service root must answer without credentials.
    location = /redfish/v1/ {
        auth_basic off;
        default_type application/json;
        try_files /redfish/v1/index.json =404;
    }
}
```

The [DMTF mockups](https://redfish.dmtf.org/redfish/mockups/v1) can be dropped
into such a tree directly. For a dynamic emulator that also implements power and
virtual media, use
[sushy-tools](https://opendev.org/openstack/sushy-tools), the emulator the
Metal3 E2E suite already uses.

### Verification

Reproduce exactly what the collector does, including the `endpoint + @odata.id`
concatenation that catches the most common defect:

```bash
set -o errexit -o nounset -o pipefail

BMC="https://bmc.example.com"      # no trailing slash
CRED="reader:secret"

# 1. Service root must answer with no credentials.
curl -fsSk "$BMC/redfish/v1/" | jq '{Systems, UpdateService}'

# 2. Systems collection and the first member.
SYS=$(curl -fsSku "$CRED" "$BMC/redfish/v1/Systems" \
      | jq -r '.Members[0]["@odata.id"]')
case "$SYS" in /*) ;; *) echo "@odata.id must be an absolute path"; exit 1 ;; esac

curl -fsSku "$CRED" "$BMC$SYS" > /tmp/system.json
jq '{Manufacturer, Model, SerialNumber, BiosVersion, MemorySummary,
     ProcessorSummary, Processors, Memory, EthernetInterfaces, Storage,
     SimpleStorage}' /tmp/system.json

# 3. Every advertised collection must resolve, and so must every member.
jq -r '[.Processors, .Memory, .EthernetInterfaces, .Storage, .SimpleStorage]
       | map(select(. != null) | .["@odata.id"]) | .[]' /tmp/system.json |
while read -r COLL; do
  curl -fsSku "$CRED" "$BMC$COLL" | jq -r '.Members[] | .["@odata.id"]' |
  while read -r M; do curl -fsSku "$CRED" "$BMC$M" > /dev/null; done
done

# 4. Drives hang off each storage controller and must resolve too.
STOR=$(jq -r '.Storage["@odata.id"] // empty' /tmp/system.json)
if [ -n "$STOR" ]; then
  curl -fsSku "$CRED" "$BMC$STOR" | jq -r '.Members[] | .["@odata.id"]' |
  while read -r C; do
    curl -fsSku "$CRED" "$BMC$C" | jq -r '.Drives[]? | .["@odata.id"]' |
    while read -r D; do
      curl -fsSku "$CRED" "$BMC$D" | jq '{CapacityBytes, MediaType, Protocol}'
    done
  done
fi
```

Any `curl` failure in step 3 or 4 is a member the collector would also fail on,
which costs the whole section.

For general schema conformance beyond this collector, run the
[Redfish Service Validator](https://github.com/DMTF/Redfish-Service-Validator).

### Conformance checklist

| Check | Symptom when wrong |
|---|---|
| `GET /redfish/v1/` succeeds with no `Authorization` header | Every inspection fails at connect. |
| All `@odata.id` values are absolute paths starting with `/`, same origin | Every request after the root 404s or fails to parse. |
| No `@odata.id` is ever empty, especially in `Drives` | An empty link resolves to the service root and becomes a phantom storage entry. |
| Nothing depends on the order in which collection members are requested | Members may be requested out of order, one at a time. |
| `GET` requests are accepted with a `Content-Type: application/json` header | Requests rejected with `400`. |
| Responses use `200`/`204` and never `3xx` to another host | Cross-host redirects lose the `Authorization` header and become `401`. |
| Every member listed in a `Members` array is fetchable | The whole section is discarded, not just that member. |
| Systems collection members are all fetchable | The entire inventory fails. |
| Sequential requests are accepted without rate limiting | `429` is treated as a hard error for that request; no retry happens. |
| One TCP connection per request is tolerated | Connection-limited BMCs stall past the 30 second timeout. |
| Every request answers within 30 seconds | Section lost, or whole call lost if it is the root or systems collection. |
| Enumerations use exact DSP8010 casing (`x86-64`, `NVMe`, `HDD`, `NAA`) | Arch passed through unmapped, drive type or WWN missing. |
| MACs use colon separators | Status write rejected by the API server. |
| `CapacityBytes` is in bytes and positive | Disk missing from the inventory. |
| `TotalSystemMemoryGiB` is in GiB | Memory off by a factor of 1024 or 1000. |
| Absent data is omitted, not reported as `0` or `""` | Indistinguishable from absent, which is fine, but never report a fabricated value. |

## Not collected

These `HardwareDetails` fields have no equivalent anywhere in the current
schema bundle, because they describe operating system state or Linux naming
rather than hardware the BMC models:

- `cpu.flags`. The nearest property is
  `Processor.ProcessorId.IdentificationRegisters`, which is raw CPUID content
  and not a flag list.
- `nics[].pxe`. The nearest property is `BootMode` in
  [NetworkDeviceFunction](https://redfish.dmtf.org/schemas/v1/NetworkDeviceFunction.json)
  1.0.0, which is the configured boot mode of a device function rather than a
  report of which port booted.
- `storage[].hctl`, `alternateNames`, `wwnWithExtension`, and
  `wwnVendorExtension`.

The rest of the gaps are a collector choice rather than a Redfish limit. The
schemas define these, and each one costs extra requests into a resource tree
`redfish_inventory` does not walk today:

| Inspection field | Redfish source | Since |
|---|---|---|
| `hostname` | `ComputerSystem.HostName`, which is usually empty until the OS sets it | 1.0.0 (Redfish 1.0) |
| `cpu.clockMegahertz` | `Processor.MaxSpeedMHz`, which is the rated maximum rather than the current speed | 1.0.0 (Redfish 1.0) |
| `nics[].ip` | `EthernetInterface.IPv4Addresses` and `IPv6Addresses` | 1.0.0 (Redfish 1.0) |
| `nics[].speedGbps` | `EthernetInterface.SpeedMbps` | 1.0.0 (Redfish 1.0) |
| `nics[].vlans` | `EthernetInterface.VLAN`, the `VLANs` collection being deprecated since 1.7.0 | 1.0.0 (Redfish 1.0) |
| `nics[].lldp` | `Ethernet.LLDPReceive` in [Port](https://redfish.dmtf.org/schemas/v1/Port.json) 1.4.0, through `EthernetInterface.Links.Ports` | EthernetInterface 1.9.0 |
| `nics[].model`, `nics[].pciAddress` | `VendorId` and `DeviceId` in [PCIeFunction](https://redfish.dmtf.org/schemas/v1/PCIeFunction.json), through the `PCIeDevices` collection | PCIeFunction 1.0.0 |

Values in that table describe link and address state, so they are only
meaningful once the host has been powered on and configured.

`firmware.bios.vendor` and `firmware.bios.date` are served only when the BMC
exposes a BIOS entry in its firmware inventory built on SoftwareInventory
1.2.0 (2018.1) or newer, so they may be absent even though inspection
succeeds.

## Appendix: collection algorithm

Reference for a reimplementation. `get(uri)` performs one authenticated GET and
raises on a non-2xx status. `members(uri)` implements
[Collection contract](#collection-contract): it returns an empty list for an
absent link and raises if any member failed to fetch, even when others
succeeded. `positive(x)` is true only for a present value greater than zero.
`drop_empty(m)` removes keys whose value is an empty string, so no empty keys
are ever emitted.

```python
def redfish_inventory(endpoint, username, password, insecure, fields):
    want = (lambda k: True) if not fields else (lambda k: k in fields)
    out = {}

    root = get("/redfish/v1/")                      # unauthenticated
    systems = members(root.get("Systems"))          # raises if a member failed
    if not systems:
        raise Error("no computer system found")     # also when the link is absent
    sys = systems[0]

    if want("systemVendor"):
        vendor = drop_empty({
            "manufacturer": sys.get("Manufacturer"),
            "productName":  sys.get("Model"),
            "serialNumber": sys.get("SerialNumber"),
        })
        if vendor:
            out["systemVendor"] = vendor

    if want("firmware"):
        bios = drop_empty({"version": sys.get("BiosVersion")})
        try:
            us = get(root["UpdateService"])
            for entry in members(us["FirmwareInventory"]):
                tokens = split(lower(entry.Name + " " + entry.Id + " " +
                                     entry.SoftwareId), " ._-")
                if "bios" in tokens:
                    bios |= drop_empty({"vendor": entry.get("Manufacturer"),
                                        "date":   entry.get("ReleaseDate")})
                    break
        except Error:
            pass                                    # version-only firmware
        if bios:
            out["firmware"] = {"bios": bios}

    if want("ramMebibytes"):
        gib = sys.get("MemorySummary", {}).get("TotalSystemMemoryGiB")
        if positive(gib):
            mib = int(gib * 1024)                   # truncating
        else:
            try:
                mib = sum(m["CapacityMiB"] for m in members(sys["Memory"])
                          if positive(m.get("CapacityMiB")))
            except Error:
                mib = 0
        if mib > 0:
            out["ramMebibytes"] = mib

    if want("cpu"):
        cpu = {}
        summary = sys.get("ProcessorSummary", {})
        for key in ("LogicalProcessorCount", "Count"):
            if positive(summary.get(key)):
                cpu["count"] = int(summary[key])
                break
        try:
            procs = members(sys["Processors"])
            if procs:                               # first member only
                cpu |= drop_empty({"arch":  map_arch(procs[0].get("InstructionSet")),
                                   "model": procs[0].get("Model")})
        except Error:
            pass
        if cpu:
            out["cpu"] = cpu

    if want("nics"):
        try:
            nics = []
            for ni in members(sys["EthernetInterfaces"]):
                mac = ni.get("MACAddress") or ni.get("PermanentMACAddress")
                if not mac:
                    continue
                nics.append(drop_empty({"mac": mac,
                                        "name": ni.get("Name") or ni.get("Id")}))
            if nics:
                out["nics"] = nics
        except Error:
            pass                                    # whole section dropped

    if want("storage"):
        disks = []
        try:
            for ctrl in members(sys["Storage"]):
                try:
                    for d in [get(link) for link in ctrl.get("Drives", [])]:
                        disks.append(drive_entry(d))
                except Error:
                    continue                        # this controller only
        except Error:
            disks = []
        if not disks:
            try:
                for ctrl in members(sys["SimpleStorage"]):
                    for dev in ctrl.get("Devices", []):
                        entry = drop_empty({"name":   dev.get("Name"),
                                            "model":  dev.get("Model"),
                                            "vendor": dev.get("Manufacturer")})
                        if positive(dev.get("CapacityBytes")):
                            entry["sizeBytes"] = int(dev["CapacityBytes"])
                        if entry:
                            disks.append(entry)
            except Error:
                disks = []
        if disks:
            out["storage"] = disks

    return out

def map_arch(instruction_set):
    return {"x86-64": "x86_64", "ARM-A64": "aarch64"}.get(
        instruction_set, instruction_set or "")

def drive_entry(d):
    media = d.get("MediaType")
    if d.get("Protocol") == "NVMe":
        disk_type = "NVME"
    else:
        disk_type = {"HDD": "HDD", "SMR": "HDD", "SSD": "SSD"}.get(media, "")

    entry = {"rotational": media in ("HDD", "SMR")}   # always present
    entry |= drop_empty({
        "name":         d.get("Name") or d.get("Id"),
        "type":         disk_type,
        "model":        d.get("Model"),
        "vendor":       d.get("Manufacturer"),
        "serialNumber": d.get("SerialNumber"),
        "wwn":          first_naa(d.get("Identifiers", [])),
    })
    if positive(d.get("CapacityBytes")):
        entry["sizeBytes"] = int(d["CapacityBytes"])
    return entry

def first_naa(identifiers):
    for i in identifiers:
        if i.get("DurableNameFormat") == "NAA" and i.get("DurableName"):
            return i["DurableName"]
    return ""
```
