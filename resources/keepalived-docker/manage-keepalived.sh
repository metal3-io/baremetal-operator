#!/usr/bin/bash

set -eux
CUSTOM_CONF_DIR="${CUSTOM_CONF_DIR:-/conf}"
CUSTOM_DATA_DIR="${CUSTOM_DATA_DIR:-/data}"
KEEPALIVED_DEFAULT_CONF='/etc/keepalived/keepalived.conf'
KEEPALIVED_CONF_DIR="${CUSTOM_CONF_DIR}/keepalived"
KEEPALIVED_CONF="${KEEPALIVED_CONF_DIR}/keepalived.conf"
KEEPALIVED_DATA_DIR="${CUSTOM_DATA_DIR}/keepalived"
mkdir -p "${KEEPALIVED_CONF_DIR}" "${KEEPALIVED_DATA_DIR}"
cp "${KEEPALIVED_DEFAULT_CONF}" "${KEEPALIVED_CONF}"

export assignedIP="${PROVISIONING_IP}/32"
export interface="${PROVISIONING_INTERFACE}"

sed -i "s~INTERFACE~${interface}~g" "${KEEPALIVED_CONF}"
sed -i "s~CHANGEIP~${assignedIP}~g" "${KEEPALIVED_CONF}"

exec /usr/sbin/keepalived --dont-fork --log-console \
    --pid="${KEEPALIVED_DATA_DIR}/keepalived.pid" \
    --vrrp_pid="${KEEPALIVED_DATA_DIR}/vrrp.pid" \
    --use-file="${KEEPALIVED_CONF}"
