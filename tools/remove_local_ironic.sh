#!/bin/bash

set -xe

# This script removes local ironic containers. 
# It requires ${CONTAINER_RUNTIME} variable to be defined first

for name in ironic ironic-api ironic-conductor ironic-inspector dnsmasq httpd mariadb ipa-downloader \
    ironic-endpoint-keepalived ironic-log-watch ironic-inspector-log-watch httpd-reverse-proxy ; do
    sudo "${CONTAINER_RUNTIME}" ps | grep -w "$name$" && sudo "${CONTAINER_RUNTIME}" kill "$name"
    sudo "${CONTAINER_RUNTIME}" ps --all | grep -w "$name$" && sudo "${CONTAINER_RUNTIME}" rm "$name" -f
done

set +xe
