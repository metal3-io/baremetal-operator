#!/usr/bin/bash

set -eux

export assignedIP="${PROVISIONING_IP}/32"
export interface="${PROVISIONING_INTERFACE}"

sed -i "s~INTERFACE~${interface}~g" /etc/keepalived/keepalived.conf
sed -i "s~CHANGEIP~${assignedIP}~g" /etc/keepalived/keepalived.conf

exec /usr/sbin/keepalived -n -l -p /run/keepalived/keepalived.pid -r /run/keepalived/vrrp.pid
