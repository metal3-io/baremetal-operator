#!/bin/bash -x

for host in $(oc get baremetalhosts --no-headers | grep '^demo-' | awk '{print $1}')
do
    oc delete baremetalhost $host
done
