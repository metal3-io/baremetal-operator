module github.com/metal3-io/baremetal-operator

go 1.16

require (
	github.com/go-logr/logr v0.3.0
	github.com/golangci/golangci-lint v1.32.0
	github.com/gophercloud/gophercloud v0.12.0
	github.com/pkg/errors v0.9.1
	github.com/prometheus/client_golang v1.7.1
	github.com/stretchr/testify v1.6.1
	go.etcd.io/etcd v0.5.0-alpha.5.0.20200819165624-17cef6e3e9d5
	k8s.io/api v0.20.1
	k8s.io/apimachinery v0.20.1
	k8s.io/client-go v0.20.1
	k8s.io/utils v0.0.0-20201110183641-67b214c5f920
	sigs.k8s.io/controller-runtime v0.7.0
	sigs.k8s.io/controller-tools v0.4.1
	sigs.k8s.io/kustomize/kustomize/v3 v3.8.5
	sigs.k8s.io/yaml v1.2.0
)
