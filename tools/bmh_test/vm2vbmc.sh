#!/usr/bin/env bash

set -eux

NAME="${1:?}"
VBMC_PORT="${2:?}"
VBMC_ADDRESS="${3:-"::"}"

# Add the BareMetalHost VM to VBMC
docker exec vbmctl-vbmc vbmc add "${NAME}" --port "${VBMC_PORT}" --address "${VBMC_ADDRESS}" --libvirt-uri "qemu:///system"
docker exec vbmctl-vbmc vbmc start "${NAME}"
docker exec vbmctl-vbmc vbmc list
