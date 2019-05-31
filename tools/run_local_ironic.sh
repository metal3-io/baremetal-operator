#!/bin/bash

set -ex

IRONIC_IMAGE=${IRONIC_IMAGE:-"quay.io/metal3-io/ironic:master"}
IRONIC_INSPECTOR_IMAGE=${IRONIC_INSPECTOR_IMAGE:-"quay.io/metal3-io/ironic-inspector"}
IRONIC_DATA_DIR="$PWD/ironic"

sudo podman pull $IRONIC_IMAGE
sudo podman pull $IRONIC_INSPECTOR_IMAGE

mkdir -p "$IRONIC_DATA_DIR/html/images"
pushd $IRONIC_DATA_DIR/html/images

# The images directory should contain images and an associated md5sum.
#   - image.qcow2
#   - image.qcow2.md5sum

for name in ironic ironic-inspector dnsmasq httpd mariadb; do
    sudo podman ps | grep -w "$name$" && sudo podman kill $name
    sudo podman ps --all | grep -w "$name$" && sudo podman rm $name -f
done

# Remove existing pod
if  sudo podman pod exists ironic-pod ; then
    sudo podman pod rm ironic-pod -f
fi

# set password for mariadb
mariadb_password=$(echo $(date;hostname)|sha256sum |cut -c-20)

# Create pod
sudo podman pod create -n ironic-pod

# Start dnsmasq, http, mariadb, and ironic containers using same image

# See this file for env vars you can set, like IP, DHCP_RANGE, INTERFACE
# https://github.com/metal3-io/ironic/blob/master/rundnsmasq.sh
sudo podman run -d --net host --privileged --name dnsmasq  --pod ironic-pod \
     -v $IRONIC_DATA_DIR:/shared --entrypoint /bin/rundnsmasq ${IRONIC_IMAGE}

# For available env vars, see:
# https://github.com/metal3-io/ironic/blob/master/runhttpd.sh
sudo podman run -d --net host --privileged --name httpd --pod ironic-pod \
     -v $IRONIC_DATA_DIR:/shared --entrypoint /bin/runhttpd ${IRONIC_IMAGE}

# https://github.com/metal3-io/ironic/blob/master/runmariadb.sh
sudo podman run -d --net host --privileged --name mariadb --pod ironic-pod \
     -v $IRONIC_DATA_DIR:/shared --entrypoint /bin/runmariadb \
     --env MARIADB_PASSWORD=$mariadb_password ${IRONIC_IMAGE}

# See this file for additional env vars you may want to pass, like IP and INTERFACE
# https://github.com/metal3-io/ironic/blob/master/runironic.sh
sudo podman run -d --net host --privileged --name ironic --pod ironic-pod \
     --env MARIADB_PASSWORD=$mariadb_password \
     -v $IRONIC_DATA_DIR:/shared ${IRONIC_IMAGE}

# Start Ironic Inspector
sudo podman run -d --net host --privileged --name ironic-inspector --pod ironic-pod \
     -v $IRONIC_DATA_DIR:/shared "${IRONIC_INSPECTOR_IMAGE}"
