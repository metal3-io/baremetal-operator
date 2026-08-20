# Maintain Ironic Endpoint with Keepalived

Keepalived holds the Ironic API endpoint on a virtual IP so that the
address can move with the Ironic pod. After cluster pivoting, the
target cluster can reclaim that IP and keep Ironic reachable.

## Deploying Ironic with Keepalived

Keepalived is a kustomize component, not a standalone overlay root.
Use an overlay that includes it, for example:

```bash
kustomize build ironic-deployment/overlays/basic-auth_tls_keepalived | kubectl apply -f -
```

The component itself lives in
`ironic-deployment/components/keepalived`.
See [ironic-deployment/README.md](../ironic-deployment/README.md) for
the full kustomize layout and the `deploy.sh` / `deploy-cli` helpers.

## Ironic Keepalived container

The deployment adds a sidecar named `ironic-endpoint-keepalived`.
The image and container spec are in
`ironic-deployment/components/keepalived/keepalived_patch.yaml`
(default image `quay.io/metal3-io/keepalived`).
Override that image in your overlay if you build a custom image.

When Metal3 is deployed through metal3-dev-env, the container inherits
`PROVISIONING_IP` and `PROVISIONING_INTERFACE` from the Ironic
configmap. For a local deploy, set those values before applying the
manifests.
