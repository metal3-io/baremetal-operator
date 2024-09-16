apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
resources:
- ../../namespace/
- ../../base/
namespace: baremetal-operator-system

components:
{{- if .DeployBasicAuth }}
- ../../components/basic-auth/
{{ end }}
{{- if .DeployTLS }}
- ../../components/tls/
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
