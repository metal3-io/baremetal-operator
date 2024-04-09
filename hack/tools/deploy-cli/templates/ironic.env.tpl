HTTP_PORT=6180
PROVISIONING_INTERFACE=eth2
DHCP_RANGE=172.22.0.10,172.22.0.100
DEPLOY_KERNEL_URL=http://172.22.0.2:6180/images/ironic-python-agent.kernel
DEPLOY_RAMDISK_URL=http://172.22.0.2:6180/images/ironic-python-agent.initramfs
CACHEURL=http://172.22.0.1/images
{{- if .DeployTLS }}
IRONIC_ENDPOINT=https://172.22.0.2:6385/v1/
{{- else }}
IRONIC_ENDPOINT=http://172.22.0.2:6385/v1/
{{ end }}
