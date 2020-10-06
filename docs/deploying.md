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
├── tls
│   ├── kustomization.yaml
│   └── tls_ca.yaml
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

## Deployment commands

There is a useful deployment script that configures and deploys BareMetal
Operator and Ironic. It requires some variables :

- IRONIC_HOST : domain name for Ironic and inspector
- IRONIC_HOST_IP : IP on which Ironic and inspector are listening

In addition you can configure the following variables. They are **optional**.
If you leave them unset, then passwords and certificates will be generated
for you.

- KUBECTL_ARGS : Additional arguments to kubectl apply
- IRONIC_USERNAME : username for ironic
- IRONIC_PASSWORD : password for ironic
- IRONIC_INSPECTOR_USERNAME : username for inspector
- IRONIC_INSPECTOR_PASSWORD : password for inspector
- IRONIC_CACERT_FILE : CA certificate path for ironic
- IRONIC_CAKEY_FILE : CA certificate key path, unneeded if ironic
  certificates exist
- IRONIC_CERT_FILE : Ironic certificate path
- IRONIC_KEY_FILE : Ironic certificate key path
- IRONIC_INSPECTOR_CERT_FILE : Inspector certificate path
- IRONIC_INSPECTOR_KEY_FILE : Inspector certificate key path
- IRONIC_INSPECTOR_CACERT_FILE : CA certificate path for inspector, defaults to
  IRONIC_CACERT_FILE
- IRONIC_INSPECTOR_CAKEY_FILE : CA certificate key path, unneeded if inspector
  certificates exist

Then run :

```sh
./tools/deploy.sh <deploy-BMO> <deploy-Ironic> <deploy-TLS> <deploy-Basic-Auth> <deploy-Keepalived>
```

- `deploy-BMO` : deploy BareMetal Operator : "true" or "false"
- `deploy-Ironic` : deploy Ironic : "true" or "false"
- `deploy-TLS` : deploy with TLS enabled : "true" or "false"
- `deploy-Basic-Auth` : deploy with Basic Auth enabled : "true" or "false"
- `deploy-Keepalived` : deploy with Keepalived for ironic : "true" or "false"

This will deploy BMO and / or Ironic with the proper configuration.

## Useful tips

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
