# Serial console access

Create a `HostSerialConsole` with the same name and namespace as a
`BareMetalHost` to request serial console access. Delete the request to close
access. There is no `enabled` field and consumers cannot supply backend
addresses or credentials in the request.

```yaml
apiVersion: metal3.io/v1alpha1
kind: HostSerialConsole
metadata:
  name: worker-0
  namespace: metal3
spec: {}
```

## Prerequisites

- Enable Ironic's `ipmitool-socat` console interface and install `socat`.
   Ensure console-capable nodes select this interface before provisioning.
   Avoid forcing it as a global default in mixed-hardware deployments:
   hardware types such as `manual-management` do not support it. BMO can select
   it for an available node, but does not put an active node into maintenance
   to change its interface.
- Configure Ironic's `[console] port_range` with free TCP listener ports.
   These are conductor ports, not the BMC's IPMI UDP port. BMO leaves terminal
   port allocation to Ironic, and updates routing when the allocation changes.
- Ensure the conductor's advertised console address and allocated ports are
   reachable from cluster workloads. Do not advertise loopback addresses.
- The host must be registered and available, provisioned, or externally
   provisioned. Other states report `Ready=False` until eligible.
- The operating system must send its serial console to the appropriate UART.

### Ironic configuration

Merge the following settings into the **Ironic conductor's** configuration,
not BMO's environment:

```ini
[DEFAULT]
enabled_console_interfaces = ipmitool-socat,no-console

[console]
port_range = 20000:20100
socat_address = 192.0.2.10
```

Replace `192.0.2.10` with an address actually assigned to the conductor and
reachable from cluster workloads. Allow the TCP range through the conductor's
firewall. It is unrelated to the BMC's UDP IPMI port, normally 623. With
ironic-image's environment-based configuration, the corresponding variables are
`OS_DEFAULT__ENABLED_CONSOLE_INTERFACES`, `OS_CONSOLE__PORT_RANGE` and
`OS_CONSOLE__SOCAT_ADDRESS`. Retain other enabled interfaces your deployment uses.

Do not force a global `default_console_interface`: some hardware types do not
support socat. For an existing available node, an administrator can select it
before provisioning with `baremetal node set NODE_ID --console-interface
ipmitool-socat`. The Ironic image must contain both `ipmitool` and `socat`.

### BMO networking and BMC certificates

Automatic Redfish discovery runs in **BMO**, not in the Ironic conductor. BMO
therefore needs HTTPS access to the BMC and trust for its certificates, in
addition to BMO's ordinary connection to Ironic. A reachable Ironic endpoint
does not prove that BMO can reach the BMC network. Configure routes, firewall
rules and `HTTP_PROXY`, `HTTPS_PROXY` and `NO_PROXY` as appropriate.

For a private BMC CA, create a CA Secret in the BMO deployment's namespace:

```sh
kubectl -n baremetal-operator-system create secret generic bmc-cacert \
  --from-file=bmc-ca.crt=/path/to/bmc-ca.pem
```

An upstream Kustomize component mounts that Secret read-only and adds it to
Go's trust directories without removing the standard system trust paths. For
example, create `config/overlays/console/kustomization.yaml`:

```yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
resources:
- ../../default
components:
- ../../components/bmc-ca
```

Apply it with `kubectl apply -k config/overlays/console`. The component sets
`SSL_CERT_DIR` to include `/opt/metal3/certs/bmc-ca`. In other deployment systems,
mount the CA and configure the same environment variable yourself. Restart BMO
after changing trusted CA files, because Go caches system roots. Ironic still
needs its own BMC trust configuration; this component configures BMO only.

If BMO cannot access the BMC network, supply `bmc.serialConsoleAddress` instead.
An explicit IPMI address skips BMO-side Redfish discovery; Ironic, not BMO,
establishes the IPMI connection. Do not disable certificate verification merely
to work around a missing CA mount.

### OpenShift deployments

BMO uses CBO's shared service account on OpenShift. Update CBO's RBAC for
`hostserialconsoles`, its status/finalizers, Services and EndpointSlices, and
register the new validating webhook through CBO. Installing only a new BMO
image and CRD is insufficient: forbidden informer watches prevent the shared
manager cache from syncing and can stop ordinary host registration as well.

Configure the conductor through CBO rather than editing its managed Deployment
by hand. A development image update does not automatically update the installed
release's RBAC manifests.

## Hardware connection settings

IPMI-managed hosts reuse the BMH's BMC address and credentials. Redfish discovery
supports standard `ComputerSystem.SerialConsole.IPMI` connection details. It
also supports an enabled, single-system Manager advertising IPMI in
`SerialConsole.ConnectTypesSupported`, with a linked
`ManagerNetworkProtocol.IPMI` containing `ProtocolEnabled` and `Port`.
An explicitly disabled ComputerSystem SOL service is not overridden by a
Manager advertisement.

The Redfish hostname is reused for IPMI. A shared Manager does not identify which
system its SOL selects, so BMO does not infer an endpoint from that standard
Manager shape. Supply an explicit address for shared controllers, Redfish
proxies, or IPMI endpoints on a different address. A Systems collection with
multiple members requires a ComputerSystem path in the BMC address.

Compatibility with SushyTools' `SerialConsole.Oem.SushyTools` extension is
retained separately; it is not assumed to be standard Redfish Manager metadata.

When discovery is unavailable or incorrect, set the connection on BMH, not on
the console request:

```yaml
spec:
  bmc:
    address: redfish://bmc.example/redfish/v1/Systems/1
    credentialsName: worker-0-bmc
    serialConsoleAddress: ipmi://192.0.2.20:623
```

The override is supported for non-IPMI management. The BMC Secret's `username`
and `password` are reused unless **both** `serialConsoleUsername` and
`serialConsolePassword` are supplied. IPMI-managed hosts must reuse management
credentials because Ironic uses the same connection settings for power control.

If a BMC requires an explicit IPMI cipher suite, append `?cipher_suite=3` (or
another supported nonzero suite ID) to its IPMI URL. For example, the development
VirtualBMC SOL implementation accepts suite 3 but rejects suite 17. Client
defaults vary, so select suite 3 explicitly when needed for that emulator.
This is an administrator-selected compatibility setting, not a tenant option;
leave it unset to retain the backend's default.

## Connecting

```sh
kubectl wait -n metal3 hostserialconsole/worker-0 \
  --for=condition=Ready --timeout=120s
kubectl get -n metal3 hostserialconsole worker-0 \
  -o jsonpath='{.status.consoleURL}{"\n"}'
```

Connect from a workload with cluster network access using a TCP client such as
`nc`. For example, using an existing pod named `console-client` with `nc`
installed in the same namespace:

```sh
service=$(kubectl get -n metal3 hsc worker-0 -o jsonpath='{.status.serviceName}')
kubectl exec -it -n metal3 console-client -- nc "${service}.metal3.svc" 2023
```

The URL points to a managed ClusterIP Service on TCP port 2023; an
EndpointSlice routes to Ironic's actual, potentially dynamic port. The Service
is selectorless: `kubectl port-forward service/...` does not support this kind
of Service. No LoadBalancer or NodePort is created automatically.

For dual-stack conductor DNS, BMO uses the Service family selected by the API
server. A single-family backend requires that family to be enabled for Services
in the cluster; the Service does not translate between IPv4 and IPv6.

Ironic's socat console supports only one connected TCP client at a time. BMO
does not repeatedly call Ironic's console-show API: that operation can deactivate
SOL and disconnect the current client. Backend node reads monitor enabled state,
conductor identity and port allocation without interrupting the session.

To disconnect cleanly from `ipmitool-socat`, press **Enter, then `~.`**. Closing
the TCP connection abruptly can leave the SOL payload occupied. If a later
connection reports that SOL is already active, first verify that no legitimate
client is using it, then delete and recreate the request to close that session.
Do not attach TCP readiness/liveness probes or a second reader to this Service:
they can consume the backend's only client slot.

```sh
kubectl delete -n metal3 hostserialconsole worker-0
```

Deletion withdraws the Service and EndpointSlice, then waits for Ironic to
confirm the console is disabled. Cleanup does not require the BMC Secret and
also succeeds if Ironic has already removed the node. Deprovisioning, detach,
and host deletion expire the request rather than reopening access for the next
workload. Once bound, a request records the BMH's deprovisioning generation;
it expires when that generation changes even if the controller missed the
intermediate deprovisioning state. Pausing the host pauses normal console
reconciliation while retaining the last observed URL. Readiness is `Unknown`
while paused or when the management API cannot confirm a previously available
console. Observation errors do not delete its routing; a confirmed missing or
disabled backend does. Explicit request deletion still performs cleanup.
Console errors do not block BMH power management or provisioning.

## Troubleshooting

Inspect `kubectl describe -n metal3 hsc worker-0` for readiness reasons:

- `ConsoleUnsupported`: the selected provisioner does not implement consoles.
- `HostNotReady`: the host is not registered or is in a transitional state.
- `ConsoleUnavailable`: setup failed; check Ironic's interface/port range, BMC
   connectivity, credentials and certificate trust.
- `ConsoleObservationFailed`: the last observed endpoint is retained, but its
   current backend state cannot be confirmed. Check the Ironic API or DNS.
- `Paused`: normal reconciliation is paused; deleting the request still closes
   access.

`Ready=True` means the backend listener and Kubernetes routing are configured,
not that IPMI authentication or a guest getty has been tested. A connection
printing `Unable to establish IPMI v2 / RMCP+ session` needs investigation of
the BMC address, credentials and cipher suite. `SOL Session operational` without
guest output can instead mean that the guest has no serial getty or is using a
different UART. Check the guest's kernel console arguments and
`serial-getty@ttyS0.service` where applicable. Ironic's IPA kernel parameters
configure the provisioning ramdisk, not the installed operating system.

## Request permissions

The editor and viewer ClusterRoles are supplied separately from the operator's
own permissions. Install them explicitly, then bind only within the namespace
containing the hosts:

```sh
kubectl apply -f config/base/rbac/hostserialconsole_editor_role.yaml
kubectl apply -f config/base/rbac/hostserialconsole_viewer_role.yaml
kubectl create rolebinding console-administrators -n metal3 \
  --clusterrole=hostserialconsole-editor-role --group=trusted-host-admins
```

The editor can create and delete requests, but cannot edit BMH connection
settings or request status. The viewer can inspect requests and endpoints.
Neither role authenticates TCP clients; network access must be controlled
separately.

## Security and limitations

Console request RBAC is separate from BMH write access. Restrict request creation
to trusted host administrators. This initial same-namespace API does not resolve
HostClaims or implement tenant authorization.

A Kubernetes Service is routing, **not authentication or network isolation**.
Use appropriate network/firewall policy to restrict client access, including
direct access to the conductor's listener. TCP console traffic is not encrypted.
Deleting routing alone cannot terminate an existing TCP connection; confirmed
backend disable is what closes the listener and session. Do not remove the
request's finalizer merely to bypass an unavailable backend without arranging
out-of-band cleanup.

Provisioners without serial-console support report `ConsoleUnsupported` and
do not attach a backend cleanup finalizer. Do not switch provisioners while
requests are active: a replacement backend may be unable to revoke an existing
console. Routing cleanup failures retain the finalizer, but do not prevent
the controller from attempting backend disable.

## Development verification

`make test` includes `make test-envtest`, which creates local API servers with
IPv4-only and IPv6-only Service ranges. It validates generated console CRDs,
Service defaults, EndpointSlice metadata and finalization without Ironic, VMs
or a running cluster. The target downloads and caches pinned test assets; set
`KUBEBUILDER_ASSETS` to an existing envtest binary directory to reuse it.
