#!/bin/bash

set -eu

if [ -z "${1:-}" ] || [ -z "${2:-}" ] || [ -z "${3:-}" ] || [ -z "${4:-}" ] || [ -z "${5:-}" ]; then
    echo "Usage : deploy.sh <deploy-BMO> <deploy-Ironic> <deploy-TLS> <deploy-Basic-Auth> <deploy-Keepalived>"
    echo ""
    echo "       deploy-BMO: deploy BareMetal Operator : \"true\" or \"false\""
    echo "       deploy-Ironic: deploy Ironic : \"true\" or \"false\""
    echo "       deploy-TLS: deploy with TLS enabled : \"true\" or \"false\""
    echo "       deploy-Basic-Auth: deploy with Basic Auth enabled : \"true\" or \"false\""
    echo "       deploy-Keepalived: deploy with Keepalived for ironic : \"true\" or \"false\""
    exit 1
fi

DEPLOY_BMO="${1,,}"
DEPLOY_IRONIC="${2,,}"
DEPLOY_TLS="${3,,}"
DEPLOY_BASIC_AUTH="${4,,}"
DEPLOY_KEEPALIVED="${5,,}"
IRONIC_HOST="${IRONIC_HOST}"
IRONIC_HOST_IP="${IRONIC_HOST_IP}"
MARIADB_HOST="${MARIADB_HOST:-"mariaDB"}"
MARIADB_HOST_IP="${MARIADB_HOST_IP:-"127.0.0.1"}"
KUBECTL_ARGS="${KUBECTL_ARGS:-""}"
KUSTOMIZE="go run sigs.k8s.io/kustomize/kustomize/v3"

SCRIPTDIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )/.." && pwd )"

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
IRONIC_CERTS_DIR="${IRONIC_CERTS_DIR:-"${IRONIC_DATA_DIR}certs/"}"

sudo mkdir -p "${IRONIC_DATA_DIR}"
sudo chown -R "${USER}:$(id -gn)" "${IRONIC_DATA_DIR}"
mkdir -p "${IRONIC_AUTH_DIR}"
mkdir -p "${IRONIC_CERTS_DIR}"

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
        envsubst < "${SCRIPTDIR}/ironic-deployment/basic-auth/ironic-rpc-auth-config-tpl" > \
        "${IRONIC_SCENARIO}/ironic-rpc-auth-config"

        echo "HTTP_BASIC_HTPASSWD=$(htpasswd -n -b -B "${IRONIC_USERNAME}" "${IRONIC_PASSWORD}")" > \
        "${IRONIC_SCENARIO}/ironic-htpasswd"
        echo "HTTP_BASIC_HTPASSWD=$(htpasswd -n -b -B "${IRONIC_INSPECTOR_USERNAME}" \
        "${IRONIC_INSPECTOR_PASSWORD}")" > "${IRONIC_SCENARIO}/ironic-inspector-htpasswd"
    fi
fi

if [ "${DEPLOY_TLS}" == "true" ]; then
    IRONIC_CAKEY_FILE="${IRONIC_CAKEY_FILE:-"${IRONIC_DATA_DIR}certs/ca.key"}"
    IRONIC_CACERT_FILE="${IRONIC_CACERT_FILE:-"${IRONIC_DATA_DIR}certs/ca.crt"}"
    IRONIC_CERT_FILE="${IRONIC_CERT_FILE:-"${IRONIC_DATA_DIR}certs/ironic.crt"}"
    IRONIC_KEY_FILE="${IRONIC_KEY_FILE:-"${IRONIC_DATA_DIR}certs/ironic.key"}"

    IRONIC_INSPECTOR_CACERT_FILE="${IRONIC_INSPECTOR_CACERT_FILE:-"${IRONIC_CACERT_FILE}"}"
    IRONIC_INSPECTOR_CAKEY_FILE="${IRONIC_INSPECTOR_CAKEY_FILE:-"${IRONIC_CAKEY_FILE}"}"
    IRONIC_INSPECTOR_CERT_FILE="${IRONIC_INSPECTOR_CERT_FILE:-"${IRONIC_DATA_DIR}certs/ironic-inspector.crt"}"
    IRONIC_INSPECTOR_KEY_FILE="${IRONIC_INSPECTOR_KEY_FILE:-"${IRONIC_DATA_DIR}certs/ironic-inspector.key"}"

    MARIADB_CACERT_FILE="${MARIADB_CACERT_FILE:-"${IRONIC_CACERT_FILE}"}"
    MARIADB_CAKEY_FILE="${MARIADB_CAKEY_FILE:-"${IRONIC_CAKEY_FILE}"}"
    MARIADB_CERT_FILE="${MARIADB_CERT_FILE:-"${IRONIC_DATA_DIR}certs/mariadb.crt"}"
    MARIADB_KEY_FILE="${MARIADB_KEY_FILE:-"${IRONIC_DATA_DIR}certs/mariadb.key"}"

    if [ ! -f "${IRONIC_CAKEY_FILE}" ]; then
        openssl genrsa -out "${IRONIC_CAKEY_FILE}" 2048
    fi
    if [ ! -f "${IRONIC_INSPECTOR_CAKEY_FILE}" ]; then
        openssl genrsa -out "${IRONIC_INSPECTOR_CAKEY_FILE}" 2048
    fi
    if [ ! -f "${MARIADB_CAKEY_FILE}" ]; then
        openssl genrsa -out "${MARIADB_CAKEY_FILE}" 2048
    fi

    if [ ! -f "${IRONIC_CACERT_FILE}" ]; then
        openssl req -x509 -new -nodes -key "${IRONIC_CAKEY_FILE}" -sha256 -days 1825 -out "${IRONIC_CACERT_FILE}" -subj /CN="ironic CA"/
    fi
    if [ ! -f "${IRONIC_INSPECTOR_CACERT_FILE}" ]; then
        openssl req -x509 -new -nodes -key "${IRONIC_INSPECTOR_CAKEY_FILE}" -sha256 -days 1825 -out "${IRONIC_INSPECTOR_CACERT_FILE}" -subj /CN="ironic inspector CA"/
    fi
    if [ ! -f "${MARIADB_CACERT_FILE}" ]; then
        openssl req -x509 -new -nodes -key "${MARIADB_CAKEY_FILE}" -sha256 -days 1825 -out "${MARIADB_CACERT_FILE}" -subj /CN="mariadb CA"/
    fi

    if [ ! -f "${IRONIC_KEY_FILE}" ]; then
        openssl genrsa -out "${IRONIC_KEY_FILE}" 2048
    fi
    if [ ! -f "${IRONIC_INSPECTOR_KEY_FILE}" ]; then
        openssl genrsa -out "${IRONIC_INSPECTOR_KEY_FILE}" 2048
    fi
    if [ ! -f "${MARIADB_KEY_FILE}" ]; then
        openssl genrsa -out "${MARIADB_KEY_FILE}" 2048
    fi

    if [ ! -f "${IRONIC_CERT_FILE}" ]; then
        openssl req -new -key "${IRONIC_KEY_FILE}" -out /tmp/ironic.csr -subj /CN="${IRONIC_HOST}"/
        openssl x509 -req -in /tmp/ironic.csr -CA "${IRONIC_CACERT_FILE}" -CAkey "${IRONIC_CAKEY_FILE}" -CAcreateserial -out "${IRONIC_CERT_FILE}" -days 825 -sha256 -extfile <(printf "subjectAltName=IP:%s" "${IRONIC_HOST_IP}")
    fi
    if [ ! -f "${IRONIC_INSPECTOR_CERT_FILE}" ]; then
        openssl req -new -key "${IRONIC_INSPECTOR_KEY_FILE}" -out /tmp/ironic.csr -subj /CN="${IRONIC_HOST}"/
        openssl x509 -req -in /tmp/ironic.csr -CA "${IRONIC_INSPECTOR_CACERT_FILE}" -CAkey "${IRONIC_INSPECTOR_CAKEY_FILE}" -CAcreateserial -out "${IRONIC_INSPECTOR_CERT_FILE}" -days 825 -sha256 -extfile <(printf "subjectAltName=IP:%s" "${IRONIC_HOST_IP}")
    fi
    if [ ! -f "${MARIADB_CERT_FILE}" ]; then
        openssl req -new -key "${MARIADB_KEY_FILE}" -out /tmp/mariadb.csr -subj /CN="${MARIADB_HOST}"/
        openssl x509 -req -in /tmp/mariadb.csr -CA "${MARIADB_CACERT_FILE}" -CAkey "${MARIADB_CAKEY_FILE}" -CAcreateserial -out "${MARIADB_CERT_FILE}" -days 825 -sha256 -extfile <(printf "subjectAltName=IP:%s" "${MARIADB_HOST_IP}")
    fi


    if [ "${DEPLOY_BMO}" == "true" ]; then
        cp "${IRONIC_CACERT_FILE}" "${SCRIPTDIR}/config/tls/ca.crt"
        [ "${IRONIC_CACERT_FILE}" == "${IRONIC_INSPECTOR_CACERT_FILE}" ] || \
        cat "${IRONIC_INSPECTOR_CACERT_FILE}" >> "${SCRIPTDIR}/config/tls/ca.crt"
    fi

    if [ "${DEPLOY_IRONIC}" == "true" ]; then
        if [ "${DEPLOY_KEEPALIVED}" == "true" ]; then
            IRONIC_TLS_SCENARIO="${SCRIPTDIR}/ironic-deployment/tls/keepalived"
        else
            IRONIC_TLS_SCENARIO="${SCRIPTDIR}/ironic-deployment/tls/default"
        fi
        # Ensure that the MariaDB key file allow a non-owned user to read.
        chmod 604 "${MARIADB_KEY_FILE}"
        cp "${IRONIC_CACERT_FILE}" "${IRONIC_TLS_SCENARIO}/ironic-ca.crt"
        cp "${IRONIC_INSPECTOR_CACERT_FILE}" "${IRONIC_TLS_SCENARIO}/ironic-inspector-ca.crt"
        cp "${MARIADB_CACERT_FILE}" "${IRONIC_TLS_SCENARIO}/mariadb-ca.crt"
        cp "${IRONIC_CERT_FILE}" "${IRONIC_TLS_SCENARIO}/ironic.crt"
        cp "${IRONIC_INSPECTOR_CERT_FILE}" "${IRONIC_TLS_SCENARIO}/ironic-inspector.crt"
        cp "${MARIADB_CERT_FILE}" "${IRONIC_TLS_SCENARIO}/mariadb.crt"
        cp "${IRONIC_KEY_FILE}" "${IRONIC_TLS_SCENARIO}/ironic.key"
        cp "${IRONIC_INSPECTOR_KEY_FILE}" "${IRONIC_TLS_SCENARIO}/ironic-inspector.key"
        cp "${MARIADB_KEY_FILE}" "${IRONIC_TLS_SCENARIO}/mariadb.key"
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
    # shellcheck disable=SC2086
    ${KUSTOMIZE} build "${IRONIC_SCENARIO}" | kubectl apply ${KUBECTL_ARGS} -f -
    popd
fi

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
        rm "${IRONIC_SCENARIO}/ironic-rpc-auth-config"

        rm "${IRONIC_SCENARIO}/ironic-htpasswd"
        rm "${IRONIC_SCENARIO}/ironic-inspector-htpasswd"
    fi
fi

if [ "${DEPLOY_TLS}" == "true" ]; then
    if [ "${DEPLOY_BMO}" == "true" ]; then
        rm "${SCRIPTDIR}/config/tls/ca.crt"
    fi

    if [ "${DEPLOY_IRONIC}" == "true" ]; then
        rm "${IRONIC_TLS_SCENARIO}/ironic-ca.crt"
        rm "${IRONIC_TLS_SCENARIO}/ironic-inspector-ca.crt"
        rm "${IRONIC_TLS_SCENARIO}/ironic.crt"
        rm "${IRONIC_TLS_SCENARIO}/ironic.key"
        rm "${IRONIC_TLS_SCENARIO}/ironic-inspector.crt"
        rm "${IRONIC_TLS_SCENARIO}/ironic-inspector.key"
        rm "${IRONIC_TLS_SCENARIO}/mariadb-ca.crt"
        rm "${IRONIC_TLS_SCENARIO}/mariadb.crt"
        rm "${IRONIC_TLS_SCENARIO}/mariadb.key"
    fi
fi
