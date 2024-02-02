#!/usr/bin/env bash

set -eux

NAME="${1:?}"
VBMC_PORT="${2:?}"

# Add the BareMetalHost VM to VBMC
docker exec vbmc vbmc add "${NAME}" --port "${VBMC_PORT}" --libvirt-uri "qemu:///system"
docker exec vbmc vbmc start "${NAME}"
docker exec vbmc vbmc list
