# Running Bare Metal Operator with or without Ironic

This document explains the deployment scenarios of deploying Bare Metal
Operator(BMO) with or without Ironic as well as deploying only Ironic scenario.

**These are the deployment use cases in this document:**

1. Deploying baremetal-operator with Ironic.

2. Deploying baremetal-operator without Ironic.

3. Deploying only Ironic.

## Current structure of baremetal-operator config directory

```diff
tree config/
config/
├── basic-auth
│   ├── default
│   │   ├── credentials_patch.yaml
│   │   └── kustomization.yaml
│   └── tls
│       ├── credentials_patch.yaml
│       └── kustomization.yaml
├── certmanager
│   ├── certificate.yaml
│   ├── kustomization.yaml
│   └── kustomizeconfig.yaml
├── crd
│   ├── bases
│   │   ├── metal3.io_baremetalhosts.yaml
│   │   ├── metal3.io_firmwareschemas.yaml
│   │   └── metal3.io_hostfirmwaresettings.yaml
│   ├── kustomization.yaml
│   ├── kustomizeconfig.yaml
│   └── patches
│       ├── cainjection_in_baremetalhosts.yaml
│       ├── cainjection_in_firmwareschemas.yaml
│       ├── cainjection_in_hostfirmwaresettings.yaml
│       ├── webhook_in_baremetalhosts.yaml
│       ├── webhook_in_firmwareschemas.yaml
│       └── webhook_in_hostfirmwaresettings.yaml
├── default
│   ├── ironic.env
│   ├── kustomization.yaml
│   ├── manager_auth_proxy_patch.yaml
│   ├── manager_webhook_patch.yaml
│   └── webhookcainjection_patch.yaml
├── kustomization.yaml
├── manager
│   ├── kustomization.yaml
│   └── manager.yaml
├── namespace
│   ├── kustomization.yaml
│   └── namespace.yaml
├── OWNERS
├── prometheus
│   ├── kustomization.yaml
│   └── monitor.yaml
├── rbac
│   ├── auth_proxy_client_clusterrole.yaml
│   ├── auth_proxy_role_binding.yaml
│   ├── auth_proxy_role.yaml
│   ├── auth_proxy_service.yaml
│   ├── baremetalhost_editor_role.yaml
│   ├── baremetalhost_viewer_role.yaml
│   ├── firmwareschema_editor_role.yaml
│   ├── firmwareschema_viewer_role.yaml
│   ├── hostfirmwaresettings_editor_role.yaml
│   ├── hostfirmwaresettings_viewer_role.yaml
│   ├── kustomization.yaml
│   ├── leader_election_role_binding.yaml
│   ├── leader_election_role.yaml
│   ├── role_binding.yaml
│   └── role.yaml
├── render
│   └── capm3.yaml
├── samples
│   ├── metal3.io_v1alpha1_baremetalhost.yaml
│   ├── metal3.io_v1alpha1_firmwareschema.yaml
│   └── metal3.io_v1alpha1_hostfirmwaresettings.yaml
├── tls
│   ├── kustomization.yaml
│   └── tls_ca_patch.yaml
└── webhook
    ├── kustomization.yaml
    ├── kustomizeconfig.yaml
    ├── manifests.yaml
    └── service_patch.yaml
```

The `config` directory has one top level folder for deployment, namely `default`
and it deploys only baremetal-operator through kustomization file calling
`manager` folder. In addition, `basic-auth`, `certmanager`, `crd`, `namespace`,
`prometheus`, `rbac`, `tls` and `webhook`folders have their own kustomization
and yaml files. `samples` folder includes yaml representation of sample CRDs.

## Current structure of ironic-deployment directory

```diff
tree ironic-deployment/
ironic-deployment/
├── base
│   ├── ironic.yaml
│   └── kustomization.yaml
├── components
│   ├── basic-auth
│   │   ├── auth.yaml
│   │   ├── ironic-htpasswd
│   │   └── kustomization.yaml
│   ├── keepalived
│   │   ├── ironic_bmo_configmap.env
│   │   ├── keepalived_patch.yaml
│   │   └── kustomization.yaml
│   └── tls
│       ├── certificate.yaml
│       ├── kustomization.yaml
│       ├── kustomizeconfig.yaml
│       └── tls.yaml
├── default
│   ├── ironic_bmo_configmap.env
│   └── kustomization.yaml
├── overlays
│   ├── basic-auth_tls
│   │   ├── basic-auth_tls.yaml
│   │   └── kustomization.yaml
│   └── basic-auth_tls_keepalived
│       └── kustomization.yaml
├── OWNERS
└── README.md
```

The `ironic-deployment` folder contains kustomizations for deploying
Ironic. It makes use of kustomize components for basic auth, TLS and
keepalived configurations. This makes it easy to combine the
configurations, for example basic auth + TLS. There are some ready made
overlays in the `overlays` folder that shows how this can be done. For
more information, check the readme in the `ironic-deployment` folder.

## Deployment commands

There is a useful deployment script that configures and deploys BareMetal
Operator and Ironic. It requires some variables :

- IRONIC_HOST : domain name for Ironic
- IRONIC_HOST_IP : IP on which Ironic is listening

In addition you can configure the following variables. They are **optional**.
If you leave them unset, then passwords and certificates will be generated
for you.

- KUBECTL_ARGS : Additional arguments to kubectl apply
- IRONIC_USERNAME : username for ironic
- IRONIC_PASSWORD : password for ironic
- IRONIC_CACERT_FILE : CA certificate path for ironic
- IRONIC_CAKEY_FILE : CA certificate key path, unneeded if ironic
  certificates exist
- IRONIC_CERT_FILE : Ironic certificate path
- IRONIC_KEY_FILE : Ironic certificate key path
  certificates exist
- MARIADB_KEY_FILE: Path to the key of MariaDB
- MARIADB_CERT_FILE:  Path to the cert of MariaDB
- MARIADB_CAKEY_FILE: Path to the CA key of MariaDB
- MARIADB_CACERT_FILE: Path to the CA certificate of MariaDB

Then run :

```sh
./tools/deploy.sh [-b -i -t -n -k]
```

- `-b`: deploy BMO
- `-i`: deploy Ironic
- `-t`: deploy with TLS enabled
- `-n`: deploy without authentication
- `-k`: deploy with keepalived

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
$PROVISIONING_INTERFACE

```

In case you are deploying baremetal-operator locally, make sure to populate and
export these environment variables before deploying.
