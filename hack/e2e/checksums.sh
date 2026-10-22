#!/usr/bin/env bash

# shellcheck disable=SC2034
# All variables in this file are used by scripts that source it.

# This file contains pinned SHA-256 checksums for all binaries and images
# downloaded by the E2E infrastructure scripts. All checksums are verified
# before any extraction or installation.
#
# To update checksums: change the version in this file and run `make update-e2e-checksums`.

# Hardened curl wrapper: HTTPS-only, TLS 1.3, retries, silent with fail-on-error.
safe_curl() {
    curl --proto '=https' --tlsv1.3 -sSfL \
        --retry 3 --retry-delay 5 --max-time 600 \
        "$@"
}

# ------------------------------------------------------------------------------
# Go
# Version is defined in ensure_go.sh as MINIMUM_GO_VERSION.
# Source: https://dl.google.com/go/go${MINIMUM_GO_VERSION}.linux-amd64.tar.gz.sha256
MINIMUM_GO_VERSION="go1.26.6"
GO_SHA256="708effb774be8237570d0add163225abbdfaf4fca28b2611df167beba4feef89"

# ------------------------------------------------------------------------------
# kubectl
# Source: https://dl.k8s.io/release/${MINIMUM_KUBECTL_VERSION}/bin/linux/amd64/kubectl.sha256
MINIMUM_KUBECTL_VERSION="v1.34.1"
KUBECTL_SHA256="7721f265e18709862655affba5343e85e1980639395d5754473dafaadcaa69e3"

# ------------------------------------------------------------------------------
# yq
# Source: https://github.com/mikefarah/yq/releases/download/${YQ_VERSION}/checksums
#         Column determined by checksums_hashes_order (SHA-256 row + 1)
YQ_VERSION="v4.40.5"
YQ_SHA256="bccbf5ce1717ea5cec9662446b8bfa5863747ffb0a49a32e4c8dd23ada5c26fa"

# ------------------------------------------------------------------------------
# cirros (E2E test disk image)
# Source: https://download.cirros-cloud.net/${CIRROS_VERSION}/${IMAGE_FILE}
# MD5 source: https://download.cirros-cloud.net/${CIRROS_VERSION}/MD5SUMS
# NOTE: When CIRROS_MD5 is updated, make sure the IMAGE_CHECKSUM in
# test/e2e/config/*.yaml is updated accordingly.
CIRROS_VERSION="0.6.2"
CIRROS_MD5="c8fc807773e5354afe61636071771906"

IMAGE_FILE="cirros-${CIRROS_VERSION}-x86_64-disk.img"
CIRROS_URI="https://download.cirros-cloud.net/${CIRROS_VERSION}/${IMAGE_FILE}"
# URL and checksum consumed by the E2E tests (via e2eConfig.GetVariable).
export IMAGE_CHECKSUM="${CIRROS_MD5}"

# ------------------------------------------------------------------------------
# SystemRescue ISO (E2E test ISO image)
# Source: https://artifactory.nordix.org/artifactory/metal3/images/sysrescue/systemrescue-${SYSRESCUE_VERSION}-amd64.iso.sha256
SYSRESCUE_VERSION="11.00"
SYSRESCUE_ISO_SHA256="b25579c9e8814eed84ec3260fa10566cf979e1569f857fa8fe15505968b527ed"

ISO_FILE="systemrescue-${SYSRESCUE_VERSION}-amd64.iso"
ISO_URI="https://sourceforge.net/projects/systemrescuecd/files/sysresccd-x86/${SYSRESCUE_VERSION}/${ISO_FILE}/download"

# ------------------------------------------------------------------------------
# sysrescue-customize script (pinned to specific commit)
# COMMIT is the latest commit on the systemrescue-sources main branch that
# modifies airootfs/usr/share/sysrescue/bin/sysrescue-customize, obtained from
# the GitLab repository file metadata API.
# SHA256 is the sha256sum of the raw script content downloaded at that commit
# from the GitLab raw file endpoint.
SYSRESCUE_CUSTOMIZE_COMMIT="aa6dac4bb43382d314fb4bc9cf05f3522541f7cd"
SYSRESCUE_CUSTOMIZE_SHA256="93065ceb8d96520d0c9efbd769fecb9fe912d747fb344ef90ac4d23ab2fc62cb"
