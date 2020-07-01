#!/bin/bash -xe

"${GOPATH}/src/k8s.io/code-generator/generate-groups.sh" \
         client \
         github.com/metal3-io/baremetal-operator/pkg \
         github.com/metal3-io/baremetal-operator/pkg/apis \
         metal3:v1alpha1
