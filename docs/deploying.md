# Running Bare Metal Operator with or without Ironic

This document explains the deployment scenarios of deploying Bare Metal
Operator(BMO) with or without Ironic as well as deploying only Ironic scenario.

**These are the deployment use cases in this document:**

1. Deploying baremetal-operator with Ironic.

2. Deploying baremetal-operator without Ironic.

3. Deploying only Ironic.

## Current structure of baremetal-operator deployment directory

```diff
tree deploy/
deploy/
├── crds
│   ├── kustomization.yaml
│   └── metal3.io_baremetalhosts_crd.yaml
├── default
│   ├── ironic_bmo_configmap.env
│   ├── kustomization.yaml
│   └── kustomizeconfig.yaml
├── ironic_ci.env
├── namespace
│   ├── kustomization.yaml
│   └── namespace.yaml
├── operator
│   ├── bmo.yaml
│   └── kustomization.yaml
├── rbac
│   ├── kustomization.yaml
│   ├── role_binding.yaml
│   ├── role.yaml
│   └── service_account.yaml
└── role.yaml -> rbac/role.yaml
```

The `deploy` directory has one top level folder for deployment, namely `default`
and it deploys only baremetal-operator through kustomization file calling
`operator` folder, and also uses kustomization config file for teaching
kustomize where to look at when substituting variables. In addition, `crds`,
`namespace` and `rbac` folders have their own kustomization and yaml files.

## Current structure of ironic-deployment directory

```diff
tree ironic-deployment/
ironic-deployment/
├── default
│   ├── ironic_bmo_configmap.env
│   └── kustomization.yaml
├── ironic
│   ├── ironic.yaml
│   └── kustomization.yaml
└── keepalived
    ├── ironic_bmo_configmap.env
    ├── keepalived_patch.yaml
    └── kustomization.yaml
```

ironic-deployment folder has three top level folder for deployments,
namely `default`,  `ironic` and `keepalived`. `default` and `ironic` deploy
only ironic, `keepalived` deploys the ironic with keepalived. As the name
implies, `keepalived/keepalived_patch.yaml` patches the default image URL
through kustomization.
The user should run following commands to be able to meet requirements of each
use case as provided below:

### Commands to deploy baremetal-operator with Ironic

```diff
kustomize build $BMOPATH/deploy/default | kubectl apply -f-
kustomize build $BMOPATH/ironic-deployment/default | kubectl apply -f-
```

### Command to deploy baremetal-operator without Ironic

```diff
kustomize build $BMOPATH/deploy/default | kubectl apply -f-
```

### Command to deploy only Ironic

```diff
kustomize build $BMOPATH/ironic-deployment/default | kubectl apply -f-
```

where $BMOPATH points to the baremetal-operator path.

#### Useful tips

It is worth mentioning some tips for when the different configurations are
useful as well. For example:

1. Only BMO is deployed, in  a case when Ironic is already running, e.g. as part
   of Cluster API Provider Metal3
   [(CAPM3)](https://github.com/metal3-io/cluster-api-provider-metal3) when
   a successful pivoting state was met and ironic being deployed.

2. BMO and Ironic are deployed together, in a case when CAPM3 is not used and
   baremetal-operator and ironic containers to be deployed together.

3. Only Ironic is deployed, in a case when BMO is deployed as part of CAPM3 and
   only Ironic setup is sufficient, e.g.
   [clusterctl](https://cluster-api.sigs.k8s.io/clusterctl/commands/move.html)
   provided by Cluster API(CAPI) deploys BMO, so that it can take care of moving
   the BaremetalHost during the pivoting.

**Important Note**
When the baremetal-operator is deployed through metal3-dev-env, baremetal-operator
container inherits the following environment variables through configmap:

```ini

$PROVISIONING_IP
$PROVISIONING_CIDR
$PROVISIONING_INTERFACE

```

In case you are deploying baremetal-operator locally, make sure to populate and
export these environment variables before deploying.
