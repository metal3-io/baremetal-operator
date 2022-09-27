#!/bin/bash

set -ex

SCRIPTDIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )/.." && pwd )"

IRONIC_IMAGE=${IRONIC_IMAGE:-"quay.io/metal3-io/ironic:main"}
IRONIC_KEEPALIVED_IMAGE=${IRONIC_KEEPALIVED_IMAGE:-"quay.io/metal3-io/keepalived"}
IPA_DOWNLOADER_IMAGE=${IPA_DOWNLOADER_IMAGE:-"quay.io/metal3-io/ironic-ipa-downloader:main"}
MARIADB_IMAGE=${MARIADB_IMAGE:-"quay.io/metal3-io/mariadb:main"}

IPA_BASEURI=${IPA_BASEURI:-}
IRONIC_DATA_DIR=${IRONIC_DATA_DIR:-"/opt/metal3-dev-env/ironic"}
CONTAINER_RUNTIME="${CONTAINER_RUNTIME:-podman}"
HTTP_PORT=${HTTP_PORT:-"6180"}
PROVISIONING_IP="${PROVISIONING_IP:-"172.22.0.1"}"
CLUSTER_PROVISIONING_IP="${CLUSTER_PROVISIONING_IP:-"172.22.0.2"}"
# ironicendpoint is used in the CI setup
if ip link show ironicendpoint > /dev/null; then
    PROVISIONING_INTERFACE="${PROVISIONING_INTERFACE:-ironicendpoint}"
else
    PROVISIONING_INTERFACE="${PROVISIONING_INTERFACE:-}"
fi
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

# Variables used to configure IPA handling
IPA_DOWNLOAD_ENABLED="${IPA_DOWNLOAD_ENABLED:-true}"
USE_LOCAL_IPA="${USE_LOCAL_IPA:-false}"
LOCAL_IPA_PATH="${LOCAL_IPA_PATH:-/tmp/dib}"

HTTP_PROXY="${HTTP_PROXY:-}"
HTTPS_PROXY="${HTTPS_PROXY:-}"
NO_PROXY="${NO_PROXY:-}"

# Ensure that the MariaDB key file allow a non-owned user to read.
if [ -n "${MARIADB_KEY_FILE}" ]
then
  chmod 604 "${MARIADB_KEY_FILE}"
fi

sudo mkdir -p "${IRONIC_DATA_DIR}/auth"

if [ "$IRONIC_TLS_SETUP" = "true" ]; then
    sudo mkdir -p "${IRONIC_DATA_DIR}/tls"

    if [ -z "$IRONIC_CERT_FILE" ]; then
        IRONIC_CERT_FILE="${IRONIC_DATA_DIR}/tls/ironic.crt"
        IRONIC_KEY_FILE="${IRONIC_DATA_DIR}/tls/ironic.key"
        IRONIC_CACERT_FILE="${IRONIC_CERT_FILE}"
        sudo openssl req -x509 -newkey rsa:4096 -nodes -days 365 -subj "/CN=ironic" \
            -addext "subjectAltName = IP:${CLUSTER_PROVISIONING_IP},IP:${PROVISIONING_IP}" \
            -out "${IRONIC_CERT_FILE}" -keyout "${IRONIC_KEY_FILE}"
    fi

    if [ -z "$IRONIC_INSPECTOR_CERT_FILE" ]; then
        IRONIC_INSPECTOR_CERT_FILE="${IRONIC_DATA_DIR}/tls/inspector.crt"
        IRONIC_INSPECTOR_KEY_FILE="${IRONIC_DATA_DIR}/tls/inspector.key"
        IRONIC_INSPECTOR_CACERT_FILE="${IRONIC_CERT_FILE}"
        sudo openssl req -x509 -newkey rsa:4096 -nodes -days 365 -subj "/CN=ironic" \
            -addext "subjectAltName = IP:${CLUSTER_PROVISIONING_IP},IP:${PROVISIONING_IP}" \
            -out "${IRONIC_INSPECTOR_CERT_FILE}" -keyout "${IRONIC_INSPECTOR_KEY_FILE}"
    fi

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
IRONIC_REVERSE_PROXY_SETUP=${IRONIC_REVERSE_PROXY_SETUP:-"true"}
INSPECTOR_REVERSE_PROXY_SETUP=${INSPECTOR_REVERSE_PROXY_SETUP:-"true"}
IRONIC_USE_MARIADB=${IRONIC_USE_MARIADB:-"false"}
if [[ $IRONIC_TLS_SETUP == *false* ]]
then
     # No reverse proxy for Ironic if TLS is not used
     IRONIC_REVERSE_PROXY_SETUP="false"
     INSPECTOR_REVERSE_PROXY_SETUP="false"
fi
IRONIC_INSPECTOR_VLAN_INTERFACES=${IRONIC_INSPECTOR_VLAN_INTERFACES:-"all"}

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
IRONIC_TLS_SETUP=${IRONIC_TLS_SETUP}
IRONIC_INSPECTOR_TLS_SETUP=${IRONIC_TLS_SETUP}
IRONIC_REVERSE_PROXY_SETUP=${IRONIC_REVERSE_PROXY_SETUP}
INSPECTOR_REVERSE_PROXY_SETUP=${INSPECTOR_REVERSE_PROXY_SETUP}
IRONIC_INSPECTOR_VLAN_INTERFACES=${IRONIC_INSPECTOR_VLAN_INTERFACES}
IPA_BASEURI=${IPA_BASEURI}
IRONIC_USE_MARIADB=${IRONIC_USE_MARIADB}
HTTP_PROXY=${HTTP_PROXY}
HTTPS_PROXY=${HTTPS_PROXY}
NO_PROXY=${NO_PROXY}
EOF

if [ "$IRONIC_TLS_SETUP" == "true" ] && [ -n "$IRONIC_CA_CERT_B64" ]; then
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
sudo "${CONTAINER_RUNTIME}" pull "$IRONIC_KEEPALIVED_IMAGE"
if [ "$IRONIC_USE_MARIADB" = "true" ]; then
    sudo "${CONTAINER_RUNTIME}" pull "$MARIADB_IMAGE"
fi

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
     BASIC_AUTH_MOUNTS="-v ${IRONIC_DATA_DIR}/auth/ironic-auth-config:/auth/ironic/auth-config"
     IRONIC_HTPASSWD="$(htpasswd -n -b -B "${IRONIC_USERNAME}" "${IRONIC_PASSWORD}")"
     IRONIC_HTPASSWD="--env HTTP_BASIC_HTPASSWD=${IRONIC_HTPASSWD} --env IRONIC_HTPASSWD=${IRONIC_HTPASSWD}"
fi
IRONIC_INSPECTOR_HTPASSWD=""
if [ -n "$IRONIC_INSPECTOR_USERNAME" ]; then
     envsubst < "${SCRIPTDIR}/ironic-deployment/basic-auth/ironic-inspector-auth-config-tpl" > \
        "${IRONIC_DATA_DIR}/auth/ironic-inspector-auth-config"
     BASIC_AUTH_MOUNTS="${BASIC_AUTH_MOUNTS} -v ${IRONIC_DATA_DIR}/auth/ironic-inspector-auth-config:/auth/ironic-inspector/auth-config"
     IRONIC_INSPECTOR_HTPASSWD="$(htpasswd -n -b -B "${IRONIC_INSPECTOR_USERNAME}" "${IRONIC_INSPECTOR_PASSWORD}")"
     IRONIC_INSPECTOR_HTPASSWD="--env HTTP_BASIC_HTPASSWD=${IRONIC_INSPECTOR_HTPASSWD} --env INSPECTOR_HTPASSWD=${IRONIC_INSPECTOR_HTPASSWD}"
fi

sudo mkdir -p "$IRONIC_DATA_DIR/html/images"
# Locally supplied IPA images are imported here when the environment variables are set accordingly.
# Name of the IPA archive is expected to be "ironic-python-agent.tar" at all times.
if ${USE_LOCAL_IPA} && ! ${IPA_DOWNLOAD_ENABLED}; then
    sudo cp "${LOCAL_IPA_PATH}/ironic-python-agent.tar" "$IRONIC_DATA_DIR/html/images"
    sudo tar --extract --file "$IRONIC_DATA_DIR/html/images/ironic-python-agent.tar" \
        --directory "$IRONIC_DATA_DIR/html/images"
fi

# The images directory should contain images and an associated md5sum.
#   - image.qcow2
#   - image.qcow2.md5sum
# By default, image directory points to dir having needed images when metal3-dev-env environment in use.
# In other cases user has to store images beforehand.

"$SCRIPTDIR/tools/remove_local_ironic.sh"

if [ "$IRONIC_USE_MARIADB" = "true" ]; then
    # set password for mariadb
    mariadb_password=$(echo "$(date;hostname)"|sha256sum |cut -c-20)
    IRONIC_MARIADB_PASSWORD="--env MARIADB_PASSWORD=$mariadb_password"
else
    IRONIC_MARIADB_PASSWORD=
fi

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
if ${IPA_DOWNLOAD_ENABLED}; then
  # shellcheck disable=SC2086
  sudo "${CONTAINER_RUNTIME}" run -d --net host --privileged --name ipa-downloader \
    ${POD} --env-file "${IRONIC_DATA_DIR}/ironic-vars.env" \
    -v "$IRONIC_DATA_DIR:/shared" "${IPA_DOWNLOADER_IMAGE}" /usr/local/bin/get-resource.sh

  sudo "${CONTAINER_RUNTIME}" wait ipa-downloader
fi

# Start dnsmasq, http, mariadb, and ironic containers using same image

# See this file for env vars you can set, like IP, DHCP_RANGE, INTERFACE
# https://github.com/metal3-io/ironic-image/blob/main/scripts/rundnsmasq
# shellcheck disable=SC2086
sudo "${CONTAINER_RUNTIME}" run -d --net host --privileged --name dnsmasq \
     ${POD} --env-file "${IRONIC_DATA_DIR}/ironic-vars.env" \
     -v "$IRONIC_DATA_DIR:/shared" --entrypoint /bin/rundnsmasq "${IRONIC_IMAGE}"

# See this file for env vars you can set, like IP, DHCP_RANGE, INTERFACE
# https://github.com/metal3-io/ironic-image/blob/main/scripts/runhttpd
# shellcheck disable=SC2086
sudo "${CONTAINER_RUNTIME}" run -d --net host --privileged --name httpd \
     ${POD} ${CERTS_MOUNTS} ${BASIC_AUTH_MOUNTS} ${IRONIC_HTPASSWD} \
     --env-file "${IRONIC_DATA_DIR}/ironic-vars.env" \
     -v "$IRONIC_DATA_DIR:/shared" --entrypoint /bin/runhttpd "${IRONIC_IMAGE}"

if [ "$IRONIC_USE_MARIADB" = "true" ]; then
    # https://github.com/metal3-io/mariadb-image/blob/main/runmariadb
    # shellcheck disable=SC2086
    sudo "${CONTAINER_RUNTIME}" run -d --net host --privileged --name mariadb \
         ${POD} ${CERTS_MOUNTS} --env-file "${IRONIC_DATA_DIR}/ironic-vars.env" \
         -v "$IRONIC_DATA_DIR:/shared" \
         --env "MARIADB_PASSWORD=$mariadb_password" "${MARIADB_IMAGE}"
fi

# See this file for additional env vars you may want to pass, like IP and INTERFACE
# https://github.com/metal3-io/ironic-image/blob/main/scripts/runironic
# shellcheck disable=SC2086
sudo "${CONTAINER_RUNTIME}" run -d --net host --privileged --name ironic \
     ${POD} ${CERTS_MOUNTS} ${BASIC_AUTH_MOUNTS} ${IRONIC_HTPASSWD} \
     --env-file "${IRONIC_DATA_DIR}/ironic-vars.env" \
     ${IRONIC_MARIADB_PASSWORD} --entrypoint /bin/runironic \
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
     -v "$IRONIC_DATA_DIR:/shared" "${IRONIC_IMAGE}"
