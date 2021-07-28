#!/bin/bash

set -ex

SCRIPTDIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )/.." && pwd )"

IRONIC_IMAGE=${IRONIC_IMAGE:-"quay.io/metal3-io/ironic:master"}
IRONIC_INSPECTOR_IMAGE=${IRONIC_INSPECTOR_IMAGE:-"quay.io/metal3-io/ironic"}
IRONIC_KEEPALIVED_IMAGE=${IRONIC_KEEPALIVED_IMAGE:-"quay.io/metal3-io/keepalived"}
IPA_DOWNLOADER_IMAGE=${IPA_DOWNLOADER_IMAGE:-"quay.io/metal3-io/ironic-ipa-downloader:master"}
IPA_BASEURI=${IPA_BASEURI:-}
IRONIC_DATA_DIR=${IRONIC_DATA_DIR:-"/opt/metal3-dev-env/ironic"}
CONTAINER_RUNTIME="${CONTAINER_RUNTIME:-podman}"
HTTP_PORT="6180"
PROVISIONING_IP="${PROVISIONING_IP:-"172.22.0.1"}"
CLUSTER_PROVISIONING_IP="${CLUSTER_PROVISIONING_IP:-"172.22.0.2"}"
PROVISIONING_INTERFACE="${PROVISIONING_INTERFACE:-"ironicendpoint"}"
CLUSTER_DHCP_RANGE="${CLUSTER_DHCP_RANGE:-"172.22.0.10,172.22.0.100"}"
IRONIC_KERNEL_PARAMS="${IRONIC_KERNEL_PARAMS:-"console=ttyS0"}"
IRONIC_BOOT_ISO_SOURCE="${IRONIC_BOOT_ISO_SOURCE:-"local"}"
export NAMEPREFIX=${NAMEPREFIX:-"capm3"}

IRONIC_CACERT_FILE="${IRONIC_CACERT_FILE:-}"
IRONIC_CERT_FILE="${IRONIC_CERT_FILE:-}"
IRONIC_KEY_FILE="${IRONIC_KEY_FILE:-}"
IRONIC_TLS_SETUP=${IRONIC_TLS_SETUP:-"true"}

IRONIC_INSPECTOR_CACERT_FILE="${IRONIC_INSPECTOR_CACERT_FILE:-}"
IRONIC_INSPECTOR_CERT_FILE="${IRONIC_INSPECTOR_CERT_FILE:-}"
IRONIC_INSPECTOR_KEY_FILE="${IRONIC_INSPECTOR_KEY_FILE:-}"

MARIADB_CACERT_FILE="${MARIADB_CACERT_FILE:-}"
MARIADB_CERT_FILE="${MARIADB_CERT_FILE:-}"
MARIADB_KEY_FILE="${MARIADB_KEY_FILE:-}"

# Ensure that the MariaDB key file allow a non-owned user to read.
if [ -n "${MARIADB_KEY_FILE}" ]
then
  chmod 604 "${MARIADB_KEY_FILE}"
fi

if [ -n "$IRONIC_CERT_FILE" ]; then
    export IRONIC_BASE_URL="https://${CLUSTER_PROVISIONING_IP}"
    if [ -z "$IRONIC_CACERT_FILE" ]; then
        export IRONIC_CACERT_FILE=$IRONIC_CERT_FILE
    fi
else
    export IRONIC_BASE_URL="http://${CLUSTER_PROVISIONING_IP}"
fi

DEPLOY_KERNEL_URL="${DEPLOY_KERNEL_URL:-"http://${CLUSTER_PROVISIONING_IP}:${HTTP_PORT}/images/ironic-python-agent.kernel"}"
DEPLOY_RAMDISK_URL="${DEPLOY_RAMDISK_URL:-"http://${CLUSTER_PROVISIONING_IP}:${HTTP_PORT}/images/ironic-python-agent.initramfs"}"
DEPLOY_ISO_URL=${DEPLOY_ISO_URL:-}
IRONIC_ENDPOINT="${IRONIC_ENDPOINT:-"${IRONIC_BASE_URL}:6385/v1/"}"
IRONIC_INSPECTOR_ENDPOINT="${IRONIC_INSPECTOR_ENDPOINT:-"${IRONIC_BASE_URL}:5050/v1/"}"
CACHEURL="${CACHEURL:-"http://${PROVISIONING_IP}/images"}"
IRONIC_FAST_TRACK="${IRONIC_FAST_TRACK:-"true"}"
INSPECTOR_REVERSE_PROXY_SETUP=${INSPECTOR_REVERSE_PROXY_SETUP:-"true"}
if [[ $IRONIC_TLS_SETUP == *false* ]]
then
  INSPECTOR_REVERSE_PROXY_SETUP="false" # No Revese proxy for Ironic inspector if TLS is not used
fi
IRONIC_INSPECTOR_VLAN_INTERFACES=${IRONIC_INSPECTOR_VLAN_INTERFACES:-"all"}

sudo mkdir -p "${IRONIC_DATA_DIR}"
sudo mkdir -p "${IRONIC_DATA_DIR}/auth"

cat << EOF | sudo tee "${IRONIC_DATA_DIR}/ironic-vars.env"
HTTP_PORT=${HTTP_PORT}
PROVISIONING_IP=${CLUSTER_PROVISIONING_IP}
PROVISIONING_INTERFACE=${PROVISIONING_INTERFACE}
DHCP_RANGE=${CLUSTER_DHCP_RANGE}
DEPLOY_KERNEL_URL=${DEPLOY_KERNEL_URL}
DEPLOY_RAMDISK_URL=${DEPLOY_RAMDISK_URL}
DEPLOY_ISO_URL=${DEPLOY_ISO_URL}
IRONIC_ENDPOINT=${IRONIC_ENDPOINT}
IRONIC_INSPECTOR_ENDPOINT=${IRONIC_INSPECTOR_ENDPOINT}
CACHEURL=${CACHEURL}
IRONIC_FAST_TRACK=${IRONIC_FAST_TRACK}
IRONIC_KERNEL_PARAMS=${IRONIC_KERNEL_PARAMS}
IRONIC_BOOT_ISO_SOURCE=${IRONIC_BOOT_ISO_SOURCE}
INSPECTOR_REVERSE_PROXY_SETUP=${INSPECTOR_REVERSE_PROXY_SETUP}
IRONIC_INSPECTOR_VLAN_INTERFACES=${IRONIC_INSPECTOR_VLAN_INTERFACES}
IPA_BASEURI=${IPA_BASEURI}
EOF

if [ "$IRONIC_TLS_SETUP" == "true" ]; then
# shellcheck disable=SC2086
cat << EOF | kubectl apply -f -
apiVersion: v1
data:
  tls.crt: ${IRONIC_CA_CERT_B64}
kind: Secret
metadata:
   name: ironic-cacert
   namespace: ${NAMEPREFIX}-system
type: Opaque
EOF
fi

sudo "${CONTAINER_RUNTIME}" pull "$IRONIC_IMAGE"
sudo "${CONTAINER_RUNTIME}" pull "$IRONIC_INSPECTOR_IMAGE"
sudo "${CONTAINER_RUNTIME}" pull "$IRONIC_KEEPALIVED_IMAGE"

CERTS_MOUNTS=""

if [ -n "$IRONIC_CACERT_FILE" ]; then
     CERTS_MOUNTS="-v ${IRONIC_CACERT_FILE}:/certs/ca/ironic/tls.crt "
fi

if [ -n "$IRONIC_CERT_FILE" ]; then
     CERTS_MOUNTS="${CERTS_MOUNTS} -v ${IRONIC_CERT_FILE}:/certs/ironic/tls.crt "
fi
if [ -n "$IRONIC_KEY_FILE" ]; then
     CERTS_MOUNTS="${CERTS_MOUNTS} -v ${IRONIC_KEY_FILE}:/certs/ironic/tls.key "
fi
if [ -n "$IRONIC_INSPECTOR_CACERT_FILE" ]; then
     CERTS_MOUNTS="${CERTS_MOUNTS} -v ${IRONIC_INSPECTOR_CACERT_FILE}:/certs/ca/ironic-inspector/tls.crt "
fi
if [ -n "$IRONIC_INSPECTOR_CERT_FILE" ]; then
     CERTS_MOUNTS="${CERTS_MOUNTS} -v ${IRONIC_INSPECTOR_CERT_FILE}:/certs/ironic-inspector/tls.crt "
fi
if [ -n "$IRONIC_INSPECTOR_KEY_FILE" ]; then
     CERTS_MOUNTS="${CERTS_MOUNTS} -v ${IRONIC_INSPECTOR_KEY_FILE}:/certs/ironic-inspector/tls.key "
fi

if [ -n "$MARIADB_CACERT_FILE" ]; then
     CERTS_MOUNTS="${CERTS_MOUNTS} -v ${MARIADB_CACERT_FILE}:/certs/ca/mariadb/tls.crt "
fi
if [ -n "$MARIADB_CERT_FILE" ]; then
     CERTS_MOUNTS="${CERTS_MOUNTS} -v ${MARIADB_CERT_FILE}:/certs/mariadb/tls.crt "
fi
if [ -n "$MARIADB_KEY_FILE" ]; then
     CERTS_MOUNTS="${CERTS_MOUNTS} -v ${MARIADB_KEY_FILE}:/certs/mariadb/tls.key "
fi

BASIC_AUTH_MOUNTS=""
IRONIC_HTPASSWD=""
if [ -n "$IRONIC_USERNAME" ]; then
     envsubst < "${SCRIPTDIR}/ironic-deployment/basic-auth/ironic-auth-config-tpl" > \
        "${IRONIC_DATA_DIR}/auth/ironic-auth-config"
     envsubst < "${SCRIPTDIR}/ironic-deployment/basic-auth/ironic-rpc-auth-config-tpl" > \
        "${IRONIC_DATA_DIR}/auth/ironic-rpc-auth-config"
     BASIC_AUTH_MOUNTS="-v ${IRONIC_DATA_DIR}/auth/ironic-auth-config:/auth/ironic/auth-config"
     BASIC_AUTH_MOUNTS="${BASIC_AUTH_MOUNTS} -v ${IRONIC_DATA_DIR}/auth/ironic-rpc-auth-config:/auth/ironic-rpc/auth-config"
     IRONIC_HTPASSWD="--env HTTP_BASIC_HTPASSWD=$(htpasswd -n -b -B "${IRONIC_USERNAME}" "${IRONIC_PASSWORD}")"
fi
IRONIC_INSPECTOR_HTPASSWD=""
if [ -n "$IRONIC_INSPECTOR_USERNAME" ]; then
     envsubst < "${SCRIPTDIR}/ironic-deployment/basic-auth/ironic-inspector-auth-config-tpl" > \
        "${IRONIC_DATA_DIR}/auth/ironic-inspector-auth-config"
     BASIC_AUTH_MOUNTS="${BASIC_AUTH_MOUNTS} -v ${IRONIC_DATA_DIR}/auth/ironic-inspector-auth-config:/auth/ironic-inspector/auth-config"
     IRONIC_INSPECTOR_HTPASSWD="--env HTTP_BASIC_HTPASSWD=$(htpasswd -n -b -B "${IRONIC_INSPECTOR_USERNAME}" "${IRONIC_INSPECTOR_PASSWORD}")"
fi


sudo mkdir -p "$IRONIC_DATA_DIR/html/images"

# The images directory should contain images and an associated md5sum.
#   - image.qcow2
#   - image.qcow2.md5sum
# By default, image directory points to dir having needed images when metal3-dev-env environment in use.
# In other cases user has to store images beforehand.

"$SCRIPTDIR/tools/remove_local_ironic.sh"

# set password for mariadb
mariadb_password=$(echo "$(date;hostname)"|sha256sum |cut -c-20)

POD=""

if [[ "${CONTAINER_RUNTIME}" == "podman" ]]; then
  # Remove existing pod
  if  sudo "${CONTAINER_RUNTIME}" pod exists ironic-pod ; then
      sudo "${CONTAINER_RUNTIME}" pod rm ironic-pod -f
  fi
  # Create pod
  sudo "${CONTAINER_RUNTIME}" pod create -n ironic-pod
  POD="--pod ironic-pod "
fi

# Start image downloader container
# shellcheck disable=SC2086
sudo "${CONTAINER_RUNTIME}" run -d --net host --privileged --name ipa-downloader \
     ${POD} --env-file "${IRONIC_DATA_DIR}/ironic-vars.env" \
     -v "$IRONIC_DATA_DIR:/shared" "${IPA_DOWNLOADER_IMAGE}" /usr/local/bin/get-resource.sh

sudo "${CONTAINER_RUNTIME}" wait ipa-downloader

# Start dnsmasq, http, mariadb, and ironic containers using same image

# See this file for env vars you can set, like IP, DHCP_RANGE, INTERFACE
# https://github.com/metal3-io/ironic/blob/master/rundnsmasq.sh
# shellcheck disable=SC2086
sudo "${CONTAINER_RUNTIME}" run -d --net host --privileged --name dnsmasq \
     ${POD} --env-file "${IRONIC_DATA_DIR}/ironic-vars.env" \
     -v "$IRONIC_DATA_DIR:/shared" --entrypoint /bin/rundnsmasq "${IRONIC_IMAGE}"

# https://github.com/metal3-io/ironic/blob/master/runmariadb.sh
# shellcheck disable=SC2086
sudo "${CONTAINER_RUNTIME}" run -d --net host --privileged --name mariadb \
     ${POD} ${CERTS_MOUNTS} --env-file "${IRONIC_DATA_DIR}/ironic-vars.env" \
     -v "$IRONIC_DATA_DIR:/shared" --entrypoint /bin/runmariadb \
     --env "MARIADB_PASSWORD=$mariadb_password" "${IRONIC_IMAGE}"

# See this file for additional env vars you may want to pass, like IP and INTERFACE
# https://github.com/metal3-io/ironic/blob/master/runironic-api.sh
# shellcheck disable=SC2086
sudo "${CONTAINER_RUNTIME}" run -d --net host --privileged --name ironic-api \
     ${POD} ${CERTS_MOUNTS} ${BASIC_AUTH_MOUNTS} ${IRONIC_HTPASSWD} \
     --env-file "${IRONIC_DATA_DIR}/ironic-vars.env" \
     --env "MARIADB_PASSWORD=$mariadb_password" \
     --entrypoint /bin/runironic-api \
     -v "$IRONIC_DATA_DIR:/shared" "${IRONIC_IMAGE}"

# See this file for additional env vars you may want to pass, like IP and INTERFACE
# https://github.com/metal3-io/ironic/blob/master/runironic-conductor.sh
# shellcheck disable=SC2086
sudo "${CONTAINER_RUNTIME}" run -d --net host --privileged --name ironic-conductor \
     ${POD} ${CERTS_MOUNTS} ${BASIC_AUTH_MOUNTS} ${IRONIC_HTPASSWD} \
     --env-file "${IRONIC_DATA_DIR}/ironic-vars.env" \
     --env "MARIADB_PASSWORD=$mariadb_password" \
     --entrypoint /bin/runironic-conductor \
     -v "$IRONIC_DATA_DIR:/shared" "${IRONIC_IMAGE}"

# Start ironic-endpoint-keepalived
# shellcheck disable=SC2086
sudo "${CONTAINER_RUNTIME}" run -d --net host --privileged --name ironic-endpoint-keepalived \
    ${POD} --env-file "${IRONIC_DATA_DIR}/ironic-vars.env" \
    -v "$IRONIC_DATA_DIR:/shared" "${IRONIC_KEEPALIVED_IMAGE}"

# Start ironic-log-watch
# shellcheck disable=SC2086
sudo "${CONTAINER_RUNTIME}" run -d --net host --privileged --name ironic-log-watch \
    ${POD} --entrypoint /bin/runlogwatch.sh \
     -v "$IRONIC_DATA_DIR:/shared" "${IRONIC_IMAGE}"

# Start Ironic Inspector
# shellcheck disable=SC2086
sudo "${CONTAINER_RUNTIME}" run -d --net host --privileged --name ironic-inspector \
     ${POD} ${CERTS_MOUNTS} ${BASIC_AUTH_MOUNTS} ${IRONIC_INSPECTOR_HTPASSWD} \
     --env-file "${IRONIC_DATA_DIR}/ironic-vars.env" \
     --entrypoint /bin/runironic-inspector \
     -v "$IRONIC_DATA_DIR:/shared" "${IRONIC_INSPECTOR_IMAGE}"
