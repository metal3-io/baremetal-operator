# Maintain Ironic Endpoint with Keepalived

The motivation behind maintaining Ironic Endpoint with Keepalived is to ensure
that the Ironic Endpoint IP is also passed onto the target cluster control
plane. This also guarantees that once pivoting is done and the management
cluster is taken down, target cluster controlplane can re-claim the ironic
endpoint IP through keepalived. The end goal is to make ironic endpoint
reachable in the target cluster.

## Command to deploy Ironic with Keepalived container

```bash

kustomize build $BMOPATH/ironic-deployment/keepalived | kubectl apply -f -

```

where $BMOPATH points to the baremetal-operator path.

## Ironic Keepalived Container

Ironic Endpoint IP is maintained with Keepalived which now runs in a separate
docker container in bmo deployment. The container is named
`ironic-endpoint-keepalived`. The container files reside in
`resources/keepalived-docker` path.

```bash

tree resources/keepalived-docker/

├── Dockerfile
├── manage-keepalived.sh
├── OWNERS
└── sample.keepalived.conf

```

It is assumed that the docker image is uploaded in some registry and the image
URL is used with `ironic-deployment/keepalived/keepalived_patch.yaml` to replace
the default image URL with the correct URL through kustomization.

**Important Note**
When the baremetal-operator is deployed through metal3-dev-env, this container
inherits the following environment variables through configmap:

```bash

$PROVISIONING_IP
$PROVISIONING_INTERFACE

```

In case you are deploying baremetal-operator locally, make sure to populate and
export these environment variables before deploying.
