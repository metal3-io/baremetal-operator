#!/usr/bin/env bash

[[ -f bin/kubebuilder ]] && exit 0

version=2.2.0
arch=amd64

mkdir -p ./bin
curl -L -O "https://github.com/kubernetes-sigs/kubebuilder/releases/download/v${version}/kubebuilder_${version}_linux_${arch}.tar.gz"

tar -zxvf kubebuilder_${version}_linux_${arch}.tar.gz
mv kubebuilder_${version}_linux_${arch}/bin/* bin

rm kubebuilder_${version}_linux_${arch}.tar.gz
rm -r kubebuilder_${version}_linux_${arch}
