#!/bin/bash
export assignedIP="$PROVISIONING_IP/$PROVISIONING_CIDR"
export interface=$PROVISIONING_INTERFACE

sed  -i "s~INTERFACE~$interface~g" /etc/keepalived/keepalived.conf
sed  -i "s~CHANGEIP~$assignedIP~g" /etc/keepalived/keepalived.conf

while true;do
        /etc/init.d/keepalived start
	if (ip a show "$interface" |grep "inet $assignedIP"); then
           echo "waiting for VIP to be assigned on interface $interface"
        else
           exit 0
        fi
        sleep 1
done
