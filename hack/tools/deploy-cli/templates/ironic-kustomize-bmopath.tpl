apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
resources:
- ../../../config/namespace
{{- if and .DeployBasicAuth .DeployTLS }}
- ../../overlays/basic-auth_tls/
{{- else if .DeployBasicAuth }}
- ../../base/
{{ end }}
namePrefix: baremetal-operator-
namespace: baremetal-operator-system
components:
{{- if and .DeployBasicAuth (not .DeployTLS) }}
- ../../components/basic-auth/
{{ end }}
{{- if .DeployKeepAlived }}
- ../../components/keepalived/
{{ end }}
{{- if .DeployMariadb }}
- ../../components/mariadb/
{{ end }}

{{- if .DeployBasicAuth }}
secretGenerator:
- files:
  - htpasswd=ironic-htpasswd
  name: ironic-htpasswd
  type: Opaque
{{ end }}
configMapGenerator:
- behavior: create
  envs:
  - ironic_bmo_configmap.env
  name: ironic-bmo-configmap
{{/* This configMap is solely so that we can replace the IP placeholders */}}
- name: ironic-host-ip
  literals:
  - IRONIC_HOST_IP={{ .IronicHostIP }}
  - MARIADB_HOST_IP={{ .MariaDBHostIP }}

replacements:
{{/* Replace *_HOST_IP in certificates with the *_HOST_IP from the configmap */}}
  - source:
      kind: ConfigMap
      name: ironic-host-ip
      fieldPath: .data.IRONIC_HOST_IP
    targets:
      - select:
          version: v1
          group: cert-manager.io
          kind: Certificate
          name: ironic-cert
        fieldPaths:
          - .spec.ipAddresses.0
      - select:
          version: v1
          group: cert-manager.io
          kind: Certificate
          name: ironic-cacert
        fieldPaths:
          - .spec.ipAddresses.0
{{- if .DeployMariadb }}
  - source:
      kind: ConfigMap
      name: ironic-host-ip
      fieldPath: .data.MARIADB_HOST_IP
    targets:
      - select:
          version: v1
          group: cert-manager.io
          kind: Certificate
          name: mariadb-cert
        fieldPaths:
          - .spec.ipAddresses.0
{{ end }}
