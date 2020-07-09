#!/bin/bash -xe

# Move out of any reasonable go working directory so we don't end up
# modifying go.mod or go.sum
cd /tmp

go get -u golang.org/x/lint/golint
go get -u github.com/securego/gosec/cmd/gosec
