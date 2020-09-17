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

### Deploying with TLS

For this, you need to first copy your Ironic CA Certificate or the ironic
certificate itself, then build the kustomization for baremetal operator:

```sh
   cp <path-to-certificate> $BMOPATH/deploy/tls/ca.crt
   ./tools/bin/kustomize build $BMOPATH/deploy/tls | kubectl apply -f -
```

Then you need to copy all your certificates and build the kustomization for
ironic. The Ironic CA certificate is not required

```sh
   cp <path-to-ca-certificate> $BMOPATH/ironic-deployment/tls/default/ironic-ca.crt
   cp <path-to-ca-certificate> $BMOPATH/ironic-deployment/tls/default/ironic-inspector-ca.crt

   cp <path-to-tls-certificate> $BMOPATH/ironic-deployment/tls/default/ironic.crt
   cp <path-to-tls-cert-key> $BMOPATH/ironic-deployment/tls/default/ironic.key

   cp <path-to-inspector-tls-certificate> \
   $BMOPATH/ironic-deployment/tls/default/ironic-inspector.crt
   cp <path-to-inspector-tls-cert-key> \
   $BMOPATH/ironic-deployment/tls/default/ironic-inspector.key

   ./tools/bin/kustomize build $BMOPATH/ironic-deployment/tls/default | \
   kubectl apply -f -
```

### Deploying with Keepalived and TLS

For this, you need to first copy your Ironic CA Certificate, then build the
kustomization for baremetal operator:

```sh
   cp <path-to-ca-certificate> $BMOPATH/deploy/tls/ca.crt
   ./tools/bin/kustomize build $BMOPATH/deploy/tls | kubectl apply -f -
```

Then you need to copy all your certificates and build the kustomization for
ironic:

```sh
   cp <path-to-ca-certificate> $BMOPATH/ironic-deployment/tls/keepalived/ironic-ca.crt
   cp <path-to-ca-certificate> $BMOPATH/ironic-deployment/tls/keepalived/ironic-inspector-ca.crt

   cp <path-to-tls-certificate> $BMOPATH/ironic-deployment/tls/keepalived/ironic.crt
   cp <path-to-tls-cert-key> $BMOPATH/ironic-deployment/tls/keepalived/ironic.key

   cp <path-to-inspector-tls-certificate> \
   $BMOPATH/ironic-deployment/tls/keepalived/ironic-inspector.crt
   cp <path-to-inspector-tls-cert-key> \
   $BMOPATH/ironic-deployment/tls/keepalived/ironic-inspector.key

   ./tools/bin/kustomize build $BMOPATH/ironic-deployment/tls/keepalived | \
   kubectl apply -f -
```

### Deploying with Basic Authentication

You can deploy each of the alternatives above with basic authentication
enabled. For this, you first need to follow the instructions of the above setup
other than running kustomize.

Then define your username and password

```sh
  export IRONIC_USERNAME="<username>"
  export IRONIC_PASSWORD="<password>"
  export IRONIC_INSPECTOR_USERNAME="<username>"
  export IRONIC_INSPECTOR_PASSWORD="<password>"
```

Then you can choose which scenario to deploy for BMO. It can be :

- `default` : No TLS
- `tls` : TLS setup

```sh
  BMO_SCENARIO="tls"
```

Then run the following to deploy BMO :

```sh
  echo "${IRONIC_USERNAME}" > "$BMOPATH/deploy/basic-auth/${BMO_SCENARIO}/ironic-username"
  echo "${IRONIC_PASSWORD}" > "$BMOPATH/deploy/basic-auth/${BMO_SCENARIO}/ironic-password"

  echo "${IRONIC_INSPECTOR_USERNAME}" > "$BMOPATH/deploy/basic-auth/${BMO_SCENARIO}/ironic-inspector-username"
  echo "${IRONIC_INSPECTOR_PASSWORD}" > "$BMOPATH/deploy/basic-auth/${BMO_SCENARIO}/ironic-inspector-password"

  ./tools/bin/kustomize build $BMOPATH/deploy/basic-auth/${BMO_SCENARIO}
```

Then you need to deploy Ironic and you can choose which scenario to deploy.
It can be :

- `default` : No TLS, no Keepalived
- `keepalived` : No TLS, Keepalived enabled
- `tls/default` : TLS setup, no Keepalived
- `tls/keepalived` : TLS setup, Keepalived enabled

```sh
  IRONIC_SCENARIO="tls/keepalived"
```

```sh
  export KSTM_PATH="${BMOPATH}/ironic-deployment/basic-auth/${IRONIC_SCENARIO}"

  cat $BMOPATH/ironic-deployment/basic-auth/ironic-auth-config-tpl | envsubst > \
  "${KSTM_PATH}/ironic-auth-config"

  cat $BMOPATH/ironic-deployment/basic-auth/ironic-inspector-auth-config-tpl | \
  envsubst > "${KSTM_PATH}/ironic-inspector-auth-config"

  cat $BMOPATH/ironic-deployment/basic-auth/ironic-rpc-auth-config-tpl | \
  envsubst > "${KSTM_PATH}/ironic-rpc-auth-config"

  htpasswd -c -b -B "${KSTM_PATH}/HTTP_BASIC_HTPASSWD" "${IRONIC_USERNAME}" \
  "${IRONIC_PASSWORD}"

  ./tools/bin/kustomize build "${KSTM_PATH}"
```

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
