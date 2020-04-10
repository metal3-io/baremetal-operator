# Maintain Ironic Endpoint with Keepalived

The motivation behind maintaining Ironic Endpoint with Keepalived is to ensure
that the Ironic Endpoint IP is also passed onto the target cluster control
plane. This also guarantees that once pivoting is done and the management
cluster is taken down, target cluster controlplane can re-claim the ironic
endpoint IP through keepalived. The end goal is to make ironic endpoint
reachable in the target cluster.

To this end, we have restructred the `baremetal-operator/deploy` folder
structure to better reflect the kustomization workflow as we now have
different deployment scenarios i.e. operator-with-ironic or operator-with-
ironic-and-keepalived. The following directory tree visualizes the new structure
in more detail.

## Kustomization Structure

```diff
    tree deploy/

    deploy/
    ├── crds
    │   ├── kustomization.yaml
    │   └── metal3.io_baremetalhosts_crd.yaml
    ├── default
    │   ├── ironic_bmo_configmap.env
    │   └── kustomization.yaml
    ├── ironic-keepalived-config
    │   ├── image_patch.yaml
    │   ├── ironic_bmo_configmap.env
    │   └── kustomization.yaml
    ├── ironic_ci.env
    ├── namespace
    │   ├── kustomization.yaml
    │   └── namespace.yaml
    ├── operator
    │   ├── ironic
    │   │   ├── kustomization.yaml
    │   │   └── operator_ironic.yaml
    │   ├── ironic_keepalived
    │   │   ├── kustomization.yaml
    │   │   └── operator_ironic_keepalived.yaml
    │   └── no_ironic
    │       ├── kustomization.yaml
    │       └── operator.yaml
    ├── rbac

```

As mentioned before, since we have different deployment scenarios for baremetal
operator, we have re-organized the deploy directory structure and used
hierarchical kustomization. We have added a third operator deployment scenario
which is ironic with keepalived. The three operator deployment scenarios are :-

1. operator without ironic
2. operator with ironic
3. operator with ironic and keepalived

Since, the current baremetal-operator deploys only operator with ironic through
kustomization, we exclude operator without ironic from the deployment. As you
can see, the deploy directory has two top level folders for deployment now,
namely `default` and `ironic-keepalived-config`. `default` deploys
the current baremetal operator workflow which is operator with ironic. Whereas,
`ironic-keepalived-config` deploys the operator with ironic and keepalived.
In addition, `crds`, `namespace` and `rbac` directories have their own
kuztomization and yaml files. As the name implies,
`ironic-keepalived-config/image_patch.yaml` patches the default image URL
through kustomization.

### Command to deploy baremetal operator

```diff

kustomize build $BMOPATH/deploy/ironic-keepalived-config | kubectl apply -f-

```

where $BMOPATH points to the baremetal-operator path.

## Ironic Keepalived Container

Ironic Endpoint IP is maintained with Keepalived which now runs in a separate
docker container in bmo deployment. The container is named
`ironic-endpoint-keepalived`. The container files reside in
`resources/keepalived-docker` path.

```diff

tree resources/keepalived-docker/

├── Dockerfile
├── manage-keepalived.sh
└── sample.keepalived.conf

```

It is assumed that the docker image is uploaded in some registry and the image
URL is used with `ironic-keepalived-config/image_patch.yaml` to replace the
default image URL with the correct URL through kustomization.

**Important Note**
When the baremetal-operator is deployed through metal3-dev-env, this container
inherits the following environment variables through configmap:

```ini

$PROVISIONING_IP
$PROVISIONING_CIDR
$PROVISIONING_INTERFACE

```

In case you are deploying baremetak-operator locally, make sure to populate and
export these environment variables before deploying.
