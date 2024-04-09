apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
resources:
- https://github.com/metal3-io/baremetal-operator/config/namespace/
- https://github.com/metal3-io/baremetal-operator/config/base/
namespace: baremetal-operator-system

components:
{{- if .DeployBasicAuth }}
- https://github.com/metal3-io/baremetal-operator/config/components/basic-auth/
{{ end }}
{{- if .DeployTLS }}
- https://github.com/metal3-io/baremetal-operator/config/components/tls/
{{ end }}

{{- if .DeployBasicAuth }}
secretGenerator:
- name: ironic-credentials
  files:
  - username=ironic-username
  - password=ironic-password
  type: Opaque
{{ end }}
configMapGenerator:
- name: ironic
  behavior: create
  envs:
  - ironic.env

