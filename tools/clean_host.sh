#!/bin/bash -x

num=$1

openstack baremetal node list
sudo virsh list --all
oc get baremetalhosts

openstack baremetal node maintenance set "openshift-worker-${num}"
openstack baremetal node delete "openshift-worker-${num}"
sudo virsh shutdown "openshift_worker_${num}"
oc delete baremetalhost "openshift-worker-${num}"
