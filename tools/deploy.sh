#!/bin/bash

set -eu

function usage {
    echo "Usage : deploy.sh [-b -i -t -n -k -m]"
    echo ""
    echo "       -b: deploy BMO"
    echo "       -i: deploy Ironic"
    echo "       -t: deploy with TLS enabled"
    echo "       -n: deploy without authentication"
    echo "       -k: deploy with keepalived"
    echo "       -m: deploy with mariadb (requires TLS enabled)"
}

DEPLOY_BMO=false
DEPLOY_IRONIC=false
DEPLOY_TLS=false
DEPLOY_BASIC_AUTH=true
DEPLOY_KEEPALIVED=false
DEPLOY_MARIADB=false

while getopts ":hbitnkm" options; do
    case "${options}" in
        h)
            usage
            exit 0
            ;;
        b)
            DEPLOY_BMO=true
            ;;
        i)
            DEPLOY_IRONIC=true
            ;;
        t)
            DEPLOY_TLS=true
            ;;
        n)
            echo "WARNING: Deploying without authentication is not recommended"
            DEPLOY_BASIC_AUTH=false
            ;;
        k)
            DEPLOY_KEEPALIVED=true
            ;;
        m)
            DEPLOY_MARIADB=true
            ;;
        :)
            echo "ERROR: -${OPTARG} requires an argument"
            usage
            exit 1
            ;;
        *)
            usage
            exit 1
            ;;
    esac
done

# Backward compatibility
shift $(( OPTIND - 1 ))
if [ $# -gt 0 ]; then
    echo "WARNING: positional arguments are deprecated, run deploy.sh -h for information"
fi

if [ -n "${1:-}" ]; then
    DEPLOY_BMO=$1
fi

if [ -n "${2:-}" ]; then
    DEPLOY_IRONIC=$2
fi

if [ -n "${3:-}" ]; then
    DEPLOY_TLS=$3
fi

if [ -n "${4:-}" ]; then
    DEPLOY_BASIC_AUTH=$4
fi

if [ -n "${5:-}" ]; then
    DEPLOY_KEEPALIVED=$5
fi

if [[ "${DEPLOY_BMO}" == "false" ]] && [[ "${DEPLOY_IRONIC}" == "false" ]]; then
    echo "ERROR: nothing to deploy"
    usage
    exit 1
fi

if [[ "${DEPLOY_MARIADB}" == "true" ]] && [[ "${DEPLOY_TLS}" == "false" ]]; then
    echo "ERROR: Deploying Ironic with MariaDB without TLS is not supported."
    usage
    exit 1
fi

MARIADB_HOST_IP="${MARIADB_HOST_IP:-"127.0.0.1"}"
KUBECTL_ARGS="${KUBECTL_ARGS:-""}"
RESTART_CONTAINER_CERTIFICATE_UPDATED=${RESTART_CONTAINER_CERTIFICATE_UPDATED:-"false"}
export NAMEPREFIX=${NAMEPREFIX:-"baremetal-operator"}

SCRIPTDIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )/.." && pwd )"

TEMP_BMO_OVERLAY="${SCRIPTDIR}/config/overlays/temp"
TEMP_IRONIC_OVERLAY="${SCRIPTDIR}/ironic-deployment/overlays/temp"
rm -rf "${TEMP_BMO_OVERLAY}"
rm -rf "${TEMP_IRONIC_OVERLAY}"
mkdir -p "${TEMP_BMO_OVERLAY}"
mkdir -p "${TEMP_IRONIC_OVERLAY}"

KUSTOMIZE="${SCRIPTDIR}/tools/bin/kustomize"
make -C "$(dirname "$0")/.." "${KUSTOMIZE}"

#
# Generate credentials as needed
#

IRONIC_DATA_DIR="${IRONIC_DATA_DIR:-/opt/metal3/ironic/}"
IRONIC_AUTH_DIR="${IRONIC_AUTH_DIR:-"${IRONIC_DATA_DIR}auth/"}"

sudo mkdir -p "${IRONIC_DATA_DIR}"
sudo chown -R "${USER}:$(id -gn)" "${IRONIC_DATA_DIR}"
mkdir -p "${IRONIC_AUTH_DIR}"

# If usernames and passwords are unset, read them from file or generate them
if [[ "${DEPLOY_BASIC_AUTH}" == "true" ]]; then
    if [ -z "${IRONIC_USERNAME:-}" ]; then
        if [ ! -f "${IRONIC_AUTH_DIR}ironic-username" ]; then
            IRONIC_USERNAME="$(tr -dc 'a-zA-Z0-9' < /dev/urandom | fold -w 12 | head -n 1)"
            echo "$IRONIC_USERNAME" > "${IRONIC_AUTH_DIR}ironic-username"
        else
            IRONIC_USERNAME="$(cat "${IRONIC_AUTH_DIR}ironic-username")"
        fi
    fi
    if [ -z "${IRONIC_PASSWORD:-}" ]; then
        if [ ! -f "${IRONIC_AUTH_DIR}ironic-password" ]; then
            IRONIC_PASSWORD="$(tr -dc 'a-zA-Z0-9' < /dev/urandom | fold -w 12 | head -n 1)"
            echo "$IRONIC_PASSWORD" > "${IRONIC_AUTH_DIR}ironic-password"
        else
            IRONIC_PASSWORD="$(cat "${IRONIC_AUTH_DIR}ironic-password")"
        fi
    fi

    if [[ "${DEPLOY_BMO}" == "true" ]]; then
        echo "${IRONIC_USERNAME}" > "${TEMP_BMO_OVERLAY}/ironic-username"
        echo "${IRONIC_PASSWORD}" > "${TEMP_BMO_OVERLAY}/ironic-password"
    fi

    if [[ "${DEPLOY_IRONIC}" == "true" ]]; then
        echo "IRONIC_HTPASSWD=$(htpasswd -n -b -B "${IRONIC_USERNAME}" "${IRONIC_PASSWORD}")" > \
        "${TEMP_IRONIC_OVERLAY}/ironic-htpasswd"
    fi
fi

#
# Ironic
#

if [[ "${DEPLOY_IRONIC}" == "true" ]]; then
    # Create a temporary overlay where we can make changes.
    pushd "${TEMP_IRONIC_OVERLAY}"
    ${KUSTOMIZE} create --resources=../../../config/namespace \
    --namespace=baremetal-operator-system --nameprefix=baremetal-operator-

    if [ "${DEPLOY_BASIC_AUTH}" == "true" ]; then
        ${KUSTOMIZE} edit add secret ironic-htpasswd --from-env-file=ironic-htpasswd

        if [[ "${DEPLOY_TLS}" == "true" ]]; then
            # Basic-auth + TLS is special since TLS also means reverse proxy, which affects basic-auth.
            # Therefore we have an overlay that we use as base for this case.
            ${KUSTOMIZE} edit add resource ../../overlays/basic-auth_tls
        else
            ${KUSTOMIZE} edit add resource ../../base
            ${KUSTOMIZE} edit add component ../../components/basic-auth
        fi
    else
        if [[ "${DEPLOY_TLS}" == "true" ]]; then
            ${KUSTOMIZE} edit add component ../../components/tls
        fi
    fi

    if [[ "${DEPLOY_KEEPALIVED}" == "true" ]]; then
        ${KUSTOMIZE} edit add component ../../components/keepalived
    fi

    if [[ "${DEPLOY_MARIADB}" == "true" ]]; then
        ${KUSTOMIZE} edit add component ../../components/mariadb
    fi
    popd
fi

#
# BMO
#

if [[ "${DEPLOY_BMO}" == "true" ]]; then
    # Create a temporary overlay where we can make changes.
    pushd "${TEMP_BMO_OVERLAY}"
    ${KUSTOMIZE} create --resources=../../base,../../namespace \
    --namespace=baremetal-operator-system

    if [ "${DEPLOY_BASIC_AUTH}" == "true" ]; then
        ${KUSTOMIZE} edit add component ../../components/basic-auth
        # These files are created below
        ${KUSTOMIZE} edit add secret ironic-credentials \
            --from-file=username=ironic-username --from-file=password=ironic-password
    fi

    if [[ "${DEPLOY_TLS}" == "true" ]]; then
        ${KUSTOMIZE} edit add component ../../components/tls
    fi
    popd
fi

#
# Deploy
#

if [[ "${DEPLOY_BMO}" == "true" ]]; then
    pushd "${TEMP_BMO_OVERLAY}"
    # This is to keep the current behavior of using the ironic.env file for the configmap
    cp "${SCRIPTDIR}/config/default/ironic.env" "${TEMP_BMO_OVERLAY}/ironic.env"
    ${KUSTOMIZE} edit add configmap ironic --behavior=create --from-env-file=ironic.env
    # shellcheck disable=SC2086
    ${KUSTOMIZE} build "${TEMP_BMO_OVERLAY}" | kubectl apply ${KUBECTL_ARGS} -f -
    popd
fi

if [[ "${DEPLOY_IRONIC}" == "true" ]]; then
    pushd "${TEMP_IRONIC_OVERLAY}"
    # Copy the configmap content from either the keepalived or default kustomization
    # and edit based on environment.
    if [[ "${DEPLOY_KEEPALIVED}" == "true" ]]; then
        IRONIC_BMO_CONFIGMAP_SOURCE="${SCRIPTDIR}/ironic-deployment/components/keepalived/ironic_bmo_configmap.env"
    else
        IRONIC_BMO_CONFIGMAP_SOURCE="${SCRIPTDIR}/ironic-deployment/default/ironic_bmo_configmap.env"
    fi
    IRONIC_BMO_CONFIGMAP="${TEMP_IRONIC_OVERLAY}/ironic_bmo_configmap.env"
    cp "${IRONIC_BMO_CONFIGMAP_SOURCE}" "${IRONIC_BMO_CONFIGMAP}"
    if grep -q "RESTART_CONTAINER_CERTIFICATE_UPDATED" "${IRONIC_BMO_CONFIGMAP}" ; then
        sed "s/\(RESTART_CONTAINER_CERTIFICATE_UPDATED\).*/\1=${RESTART_CONTAINER_CERTIFICATE_UPDATED}/" -i "${IRONIC_BMO_CONFIGMAP}"
    else
        echo "RESTART_CONTAINER_CERTIFICATE_UPDATED=${RESTART_CONTAINER_CERTIFICATE_UPDATED}" >> "${IRONIC_BMO_CONFIGMAP}"
    fi
    sed -i "s/IRONIC_HOST_IP/${IRONIC_HOST_IP}/g" "${SCRIPTDIR}/ironic-deployment/components/tls/certificate.yaml"
    sed -i "s/MARIADB_HOST_IP/${MARIADB_HOST_IP}/g" "${SCRIPTDIR}/ironic-deployment/components/mariadb/certificate.yaml"
    # The keepalived component has its own configmap,
    # but we are overriding depending on environment here so we must replace it.
    if [[ "${DEPLOY_KEEPALIVED}" == "true" ]]; then
        ${KUSTOMIZE} edit add configmap ironic-bmo-configmap --behavior=replace --from-env-file=ironic_bmo_configmap.env
    else
        ${KUSTOMIZE} edit add configmap ironic-bmo-configmap --behavior=create --from-env-file=ironic_bmo_configmap.env
    fi
    # shellcheck disable=SC2086
    ${KUSTOMIZE} build "${TEMP_IRONIC_OVERLAY}" | kubectl apply ${KUBECTL_ARGS} -f -
    popd
fi

#
# Cleanup
#

if [[ "${DEPLOY_BASIC_AUTH}" == "true" ]]; then
    if [[ "${DEPLOY_BMO}" == "true" ]]; then
        rm "${TEMP_BMO_OVERLAY}/ironic-username"
        rm "${TEMP_BMO_OVERLAY}/ironic-password"
        rm -f "${TEMP_BMO_OVERLAY}/ironic-inspector-username"
        rm -f "${TEMP_BMO_OVERLAY}/ironic-inspector-password"
    fi

    if [[ "${DEPLOY_IRONIC}" == "true" ]]; then
        rm "${TEMP_IRONIC_OVERLAY}/ironic-htpasswd"

        rm -f "${TEMP_IRONIC_OVERLAY}/ironic-auth-config"
        rm -f "${TEMP_IRONIC_OVERLAY}/ironic-inspector-auth-config"
        rm -f "${TEMP_IRONIC_OVERLAY}/ironic-inspector-htpasswd"
    fi
fi
