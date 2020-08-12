#!/bin/bash

set -ex

SCRIPTPATH="$(dirname "$(readlink -f "${0}")")"

IRONIC_IMAGE=${IRONIC_IMAGE:-"quay.io/metal3-io/ironic:master"}
IRONIC_INSPECTOR_IMAGE=${IRONIC_INSPECTOR_IMAGE:-"quay.io/metal3-io/ironic-inspector"}
IRONIC_KEEPALIVED_IMAGE=${IRONIC_KEEPALIVED_IMAGE:-"quay.io/metal3-io/keepalived"}
IPA_DOWNLOADER_IMAGE=${IPA_DOWNLOADER_IMAGE:-"quay.io/metal3-io/ironic-ipa-downloader:master"}
IRONIC_DATA_DIR=${IRONIC_DATA_DIR:-"/opt/metal3-dev-env/ironic"}
CONTAINER_RUNTIME="${CONTAINER_RUNTIME:-podman}"

sudo "${CONTAINER_RUNTIME}" pull "$IRONIC_IMAGE"
sudo "${CONTAINER_RUNTIME}" pull "$IRONIC_INSPECTOR_IMAGE"
sudo "${CONTAINER_RUNTIME}" pull "$IRONIC_KEEPALIVED_IMAGE"

mkdir -p "$IRONIC_DATA_DIR/html/images"
pushd "$IRONIC_DATA_DIR/html/images"

# The images directory should contain images and an associated md5sum.
#   - image.qcow2
#   - image.qcow2.md5sum
# By default, image directory points to dir having needed images when metal3-dev-env environment in use.
# In other cases user has to store images beforehand.

for name in ironic ironic-inspector dnsmasq httpd mariadb ipa-downloader ironic-endpoint-keepalived; do
    sudo "${CONTAINER_RUNTIME}" ps | grep -w "$name$" && sudo "${CONTAINER_RUNTIME}" kill "$name"
    sudo "${CONTAINER_RUNTIME}" ps --all | grep -w "$name$" && sudo "${CONTAINER_RUNTIME}" rm "$name" -f
done

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
     ${POD} --env-file "${SCRIPTPATH}/../deploy/ironic_ci.env" \
     -v "$IRONIC_DATA_DIR:/shared" "${IPA_DOWNLOADER_IMAGE}" /usr/local/bin/get-resource.sh

sudo "${CONTAINER_RUNTIME}" wait ipa-downloader

# Start dnsmasq, http, mariadb, and ironic containers using same image

# See this file for env vars you can set, like IP, DHCP_RANGE, INTERFACE
# https://github.com/metal3-io/ironic/blob/master/rundnsmasq.sh
# shellcheck disable=SC2086
sudo "${CONTAINER_RUNTIME}" run -d --net host --privileged --name dnsmasq \
     ${POD} --env-file "${SCRIPTPATH}/../deploy/ironic_ci.env" \
     -v "$IRONIC_DATA_DIR:/shared" --entrypoint /bin/rundnsmasq "${IRONIC_IMAGE}"

# For available env vars, see:
# https://github.com/metal3-io/ironic/blob/master/runhttpd.sh
# shellcheck disable=SC2086
sudo "${CONTAINER_RUNTIME}" run -d --net host --privileged --name httpd \
     ${POD} --env-file "${SCRIPTPATH}/../deploy/ironic_ci.env" \
     -v "$IRONIC_DATA_DIR:/shared" --entrypoint /bin/runhttpd "${IRONIC_IMAGE}"

# https://github.com/metal3-io/ironic/blob/master/runmariadb.sh
# shellcheck disable=SC2086
sudo "${CONTAINER_RUNTIME}" run -d --net host --privileged --name mariadb \
     ${POD} --env-file "${SCRIPTPATH}/../deploy/ironic_ci.env" \
     -v "$IRONIC_DATA_DIR:/shared" --entrypoint /bin/runmariadb \
     --env "MARIADB_PASSWORD=$mariadb_password" "${IRONIC_IMAGE}"

# See this file for additional env vars you may want to pass, like IP and INTERFACE
# https://github.com/metal3-io/ironic/blob/master/runironic.sh
# shellcheck disable=SC2086
sudo "${CONTAINER_RUNTIME}" run -d --net host --privileged --name ironic \
     ${POD} --env-file "${SCRIPTPATH}/../deploy/ironic_ci.env" \
     --env "MARIADB_PASSWORD=$mariadb_password" \
     -v "$IRONIC_DATA_DIR:/shared" "${IRONIC_IMAGE}"

# Start ironic-endpoint-keepalived
# shellcheck disable=SC2086
sudo "${CONTAINER_RUNTIME}" run -d --net host --privileged --name ironic-endpoint-keepalived \
    ${POD} --env-file "${SCRIPTPATH}/../deploy/ironic_ci.env" \
    -v "$IRONIC_DATA_DIR:/shared" "${IRONIC_KEEPALIVED_IMAGE}"

# Start Ironic Inspector
# shellcheck disable=SC2086
sudo "${CONTAINER_RUNTIME}" run -d --net host --privileged --name ironic-inspector \
     ${POD} --env-file "${SCRIPTPATH}/../deploy/ironic_ci.env" \
     -v "$IRONIC_DATA_DIR:/shared" "${IRONIC_INSPECTOR_IMAGE}"
