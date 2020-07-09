#!/bin/bash -xe

# Move out of any reasonable go working directory so we don't end up
# modifying go.mod or go.sum
cd /tmp

go get -u golang.org/x/lint/golint
go get -u github.com/securego/gosec/cmd/gosec

KUBEBUILDER_VERSION="2.3.1"
os=$(go env GOOS)
arch=$(go env GOARCH)
curl -L "https://go.kubebuilder.io/dl/${KUBEBUILDER_VERSION}/${os}/${arch}" | tar -xz -C /tmp/
sudo mv "/tmp/kubebuilder_${KUBEBUILDER_VERSION}_${os}_${arch}" /usr/local/kubebuilder
