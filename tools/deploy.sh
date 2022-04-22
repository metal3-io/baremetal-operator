#!/bin/bash

set -eu

function usage {
    echo "Usage : deploy.sh [-b -i -t -n -k]"
    echo ""
    echo "       -b: deploy BMO"
    echo "       -i: deploy Ironic"
    echo "       -t: deploy with TLS enabled"
    echo "       -n: deploy without authentication"
    echo "       -k: deploy with keepalived"
}

DEPLOY_BMO=false
DEPLOY_IRONIC=false
DEPLOY_TLS=false
DEPLOY_BASIC_AUTH=true
DEPLOY_KEEPALIVED=false

while getopts ":hbitnk" options; do
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

if [ "$DEPLOY_BMO" == "false" ] && [ "$DEPLOY_IRONIC" == "false" ]; then
    echo "ERROR: nothing to deploy"
    usage
    exit 1
fi

MARIADB_HOST_IP="${MARIADB_HOST_IP:-"127.0.0.1"}"
KUBECTL_ARGS="${KUBECTL_ARGS:-""}"
RESTART_CONTAINER_CERTIFICATE_UPDATED=${RESTART_CONTAINER_CERTIFICATE_UPDATED:-"false"}
export NAMEPREFIX=${NAMEPREFIX:-"baremetal-operator"}

SCRIPTDIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )/.." && pwd )"

IRONIC_DEPLOY_FILES="${SCRIPTDIR}/ironic-deployment/basic-auth/default/auth.yaml \
	${SCRIPTDIR}/ironic-deployment/basic-auth/default/kustomization.yaml \
	${SCRIPTDIR}/ironic-deployment/basic-auth/keepalived/auth.yaml \
	${SCRIPTDIR}/ironic-deployment/basic-auth/keepalived/kustomization.yaml \
	${SCRIPTDIR}/ironic-deployment/basic-auth/tls/default/auth.yaml \
	${SCRIPTDIR}/ironic-deployment/basic-auth/tls/default/kustomization.yaml \
	${SCRIPTDIR}/ironic-deployment/basic-auth/tls/keepalived/auth.yaml \
	${SCRIPTDIR}/ironic-deployment/basic-auth/tls/keepalived/kustomization.yaml \
	${SCRIPTDIR}/ironic-deployment/certmanager/certificate.yaml \
	${SCRIPTDIR}/ironic-deployment/default/kustomization.yaml \
	${SCRIPTDIR}/ironic-deployment/ironic/ironic.yaml \
	${SCRIPTDIR}/ironic-deployment/keepalived/keepalived_patch.yaml \
	${SCRIPTDIR}/ironic-deployment/keepalived/kustomization.yaml \
	${SCRIPTDIR}/ironic-deployment/tls/default/kustomization.yaml \
	${SCRIPTDIR}/ironic-deployment/tls/default/tls.yaml \
	${SCRIPTDIR}/ironic-deployment/tls/keepalived/kustomization.yaml \
	${SCRIPTDIR}/ironic-deployment/tls/keepalived/tls.yaml"

KUSTOMIZE="tools/bin/kustomize"
make -C "$(dirname "$0")/.." "${KUSTOMIZE}"

for DEPLOY_FILE in ${IRONIC_DEPLOY_FILES}; do
  cp "$DEPLOY_FILE" "$DEPLOY_FILE".bak
  # shellcheck disable=SC2094
  envsubst <"$DEPLOY_FILE".bak> "$DEPLOY_FILE"
done

if [ "${DEPLOY_BASIC_AUTH}" == "true" ]; then
    BMO_SCENARIO="${SCRIPTDIR}/config/basic-auth"
    IRONIC_SCENARIO="${SCRIPTDIR}/ironic-deployment/basic-auth"
else
    BMO_SCENARIO="${SCRIPTDIR}/config"
    IRONIC_SCENARIO="${SCRIPTDIR}/ironic-deployment"
fi

if [ "${DEPLOY_TLS}" == "true" ]; then
    BMO_SCENARIO="${BMO_SCENARIO}/tls"
    IRONIC_SCENARIO="${IRONIC_SCENARIO}/tls"
elif [ "${DEPLOY_BASIC_AUTH}" == "true" ]; then
    BMO_SCENARIO="${BMO_SCENARIO}/default"
fi

if [ "${DEPLOY_KEEPALIVED}" == "true" ]; then
    IRONIC_SCENARIO="${IRONIC_SCENARIO}/keepalived"
else
    IRONIC_SCENARIO="${IRONIC_SCENARIO}/default"
fi

IRONIC_DATA_DIR="${IRONIC_DATA_DIR:-/opt/metal3/ironic/}"
IRONIC_AUTH_DIR="${IRONIC_AUTH_DIR:-"${IRONIC_DATA_DIR}auth/"}"

sudo mkdir -p "${IRONIC_DATA_DIR}"
sudo chown -R "${USER}:$(id -gn)" "${IRONIC_DATA_DIR}"
mkdir -p "${IRONIC_AUTH_DIR}"

#If usernames and passwords are unset, read them from file or generate them
if [ "${DEPLOY_BASIC_AUTH}" == "true" ]; then
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
    if [ -z "${IRONIC_INSPECTOR_USERNAME:-}" ]; then
        if [ ! -f "${IRONIC_AUTH_DIR}ironic-inspector-username" ]; then
            IRONIC_INSPECTOR_USERNAME="$(tr -dc 'a-zA-Z0-9' < /dev/urandom | fold -w 12 | head -n 1)"
            echo "$IRONIC_INSPECTOR_USERNAME" > "${IRONIC_AUTH_DIR}ironic-inspector-username"
        else
            IRONIC_INSPECTOR_USERNAME="$(cat "${IRONIC_AUTH_DIR}ironic-inspector-username")"
        fi
    fi
    if [ -z "${IRONIC_INSPECTOR_PASSWORD:-}" ]; then
        if [ ! -f "${IRONIC_AUTH_DIR}ironic-inspector-password" ]; then
            IRONIC_INSPECTOR_PASSWORD="$(tr -dc 'a-zA-Z0-9' < /dev/urandom | fold -w 12 | head -n 1)"
            echo "$IRONIC_INSPECTOR_PASSWORD" > "${IRONIC_AUTH_DIR}ironic-inspector-password"
        else
            IRONIC_INSPECTOR_PASSWORD="$(cat "${IRONIC_AUTH_DIR}ironic-inspector-password")"
        fi
    fi

    if [ "${DEPLOY_BMO}" == "true" ]; then
        echo "${IRONIC_USERNAME}" > "${BMO_SCENARIO}/ironic-username"
        echo "${IRONIC_PASSWORD}" > "${BMO_SCENARIO}/ironic-password"

        echo "${IRONIC_INSPECTOR_USERNAME}" > "${BMO_SCENARIO}/ironic-inspector-username"
        echo "${IRONIC_INSPECTOR_PASSWORD}" > "${BMO_SCENARIO}/ironic-inspector-password"
    fi

    if [ "${DEPLOY_IRONIC}" == "true" ]; then
        envsubst < "${SCRIPTDIR}/ironic-deployment/basic-auth/ironic-auth-config-tpl" > \
        "${IRONIC_SCENARIO}/ironic-auth-config"
        envsubst < "${SCRIPTDIR}/ironic-deployment/basic-auth/ironic-inspector-auth-config-tpl" > \
        "${IRONIC_SCENARIO}/ironic-inspector-auth-config"

        echo "IRONIC_HTPASSWD=$(htpasswd -n -b -B "${IRONIC_USERNAME}" "${IRONIC_PASSWORD}")" > \
        "${IRONIC_SCENARIO}/ironic-htpasswd"
        echo "INSPECTOR_HTPASSWD=$(htpasswd -n -b -B "${IRONIC_INSPECTOR_USERNAME}" \
        "${IRONIC_INSPECTOR_PASSWORD}")" > "${IRONIC_SCENARIO}/ironic-inspector-htpasswd"
    fi
fi

if [ "${DEPLOY_BMO}" == "true" ]; then
    pushd "${SCRIPTDIR}"
    # shellcheck disable=SC2086
    ${KUSTOMIZE} build "${BMO_SCENARIO}" | kubectl apply ${KUBECTL_ARGS} -f -
    popd
fi

if [ "${DEPLOY_IRONIC}" == "true" ]; then
    pushd "${SCRIPTDIR}"
    if [[ "${DEPLOY_KEEPALIVED}" == "true" ]]; then
      IRONIC_BMO_CONFIGMAP="${SCRIPTDIR}/ironic-deployment/keepalived/ironic_bmo_configmap.env"
    else
      IRONIC_BMO_CONFIGMAP="${SCRIPTDIR}/ironic-deployment/default/ironic_bmo_configmap.env"
    fi
    cp "${IRONIC_BMO_CONFIGMAP}" /tmp/ironic_bmo_configmap.env
    if grep -q "INSPECTOR_REVERSE_PROXY_SETUP" "${IRONIC_BMO_CONFIGMAP}" ; then
      sed "s/\(INSPECTOR_REVERSE_PROXY_SETUP\).*/\1=${DEPLOY_TLS}/" -i "${IRONIC_BMO_CONFIGMAP}"
    else
      echo "INSPECTOR_REVERSE_PROXY_SETUP=${DEPLOY_TLS}" >> "${IRONIC_BMO_CONFIGMAP}"
    fi
    if grep -q "RESTART_CONTAINER_CERTIFICATE_UPDATED" "${IRONIC_BMO_CONFIGMAP}" ; then
      sed "s/\(RESTART_CONTAINER_CERTIFICATE_UPDATED\).*/\1=${RESTART_CONTAINER_CERTIFICATE_UPDATED}/" -i "${IRONIC_BMO_CONFIGMAP}"
    else
      echo "RESTART_CONTAINER_CERTIFICATE_UPDATED=${RESTART_CONTAINER_CERTIFICATE_UPDATED}" >> "${IRONIC_BMO_CONFIGMAP}"
    fi
    IRONIC_CERTIFICATE_FILE="${SCRIPTDIR}/ironic-deployment/certmanager/certificate.yaml"
    sed -i "s/IRONIC_HOST_IP/${IRONIC_HOST_IP}/g; s/MARIADB_HOST_IP/${MARIADB_HOST_IP}/g" "${IRONIC_CERTIFICATE_FILE}"
    kubectl create ns "${NAMEPREFIX}-system" || true
    # shellcheck disable=SC2086
    ${KUSTOMIZE} build "${IRONIC_SCENARIO}" | kubectl apply ${KUBECTL_ARGS} -f -
    mv /tmp/ironic_bmo_configmap.env "${IRONIC_BMO_CONFIGMAP}"
    popd
fi

# Move back the original IRONIC_DEPLOY_FILES
 for DEPLOY_FILE in ${IRONIC_DEPLOY_FILES}; do
    mv "$DEPLOY_FILE".bak "$DEPLOY_FILE"
 done

if [ "${DEPLOY_BASIC_AUTH}" == "true" ]; then
    if [ "${DEPLOY_BMO}" == "true" ]; then
        rm "${BMO_SCENARIO}/ironic-username"
        rm "${BMO_SCENARIO}/ironic-password"
        rm "${BMO_SCENARIO}/ironic-inspector-username"
        rm "${BMO_SCENARIO}/ironic-inspector-password"
    fi

    if [ "${DEPLOY_IRONIC}" == "true" ]; then
        rm "${IRONIC_SCENARIO}/ironic-auth-config"
        rm "${IRONIC_SCENARIO}/ironic-inspector-auth-config"

        rm "${IRONIC_SCENARIO}/ironic-htpasswd"
        rm "${IRONIC_SCENARIO}/ironic-inspector-htpasswd"
    fi
fi
