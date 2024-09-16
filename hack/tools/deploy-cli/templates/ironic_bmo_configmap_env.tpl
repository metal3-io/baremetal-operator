HTTP_PORT=6180
{{- if .DeployKeepAlived }}
PROVISIONING_IP=172.22.0.2
PROVISIONING_INTERFACE=ironicendpoint
{{- else }}
PROVISIONING_INTERFACE=eth2
IRONIC_INSPECTOR_VLAN_INTERFACES=all
{{ end }}
DHCP_RANGE=172.22.0.10,172.22.0.100
DEPLOY_KERNEL_URL=http://172.22.0.2:6180/images/ironic-python-agent.kernel
DEPLOY_RAMDISK_URL=http://172.22.0.2:6180/images/ironic-python-agent.initramfs
CACHEURL=http://172.22.0.1/images
IRONIC_KERNEL_PARAMS=console=ttyS0
USE_IRONIC_INSPECTOR=false
{{- if .DeployMariadb }}
IRONIC_USE_MARIADB=true
{{- else }}
IRONIC_USE_MARIADB=false
{{ end }}
{{- if eq .RestartContainerCertificateUpdated "true" }}
RESTART_CONTAINER_CERTIFICATE_UPDATED="true"
{{- else }}
RESTART_CONTAINER_CERTIFICATE_UPDATED="false"
{{ end }}
{{- if .DeployTLS }}
IRONIC_ENDPOINT=https://172.22.0.2:6385/v1/
{{- else }}
IRONIC_ENDPOINT=http://172.22.0.2:6385/v1/
{{ end }}
