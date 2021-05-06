module github.com/metal3-io/baremetal-operator

go 1.16

require (
	github.com/go-logr/logr v0.4.0
	github.com/gophercloud/gophercloud v0.16.0
	github.com/metal3-io/baremetal-operator/apis v0.0.0
	github.com/pkg/errors v0.9.1
	github.com/prometheus/client_golang v1.7.1
	github.com/stretchr/testify v1.7.0
	go.etcd.io/etcd v0.5.0-alpha.5.0.20200910180754-dd1b699fc489
	k8s.io/api v0.20.2
	k8s.io/apimachinery v0.20.2
	k8s.io/client-go v0.20.2
	k8s.io/utils v0.0.0-20210111153108-fddb29f9d009
	sigs.k8s.io/controller-runtime v0.7.0
	sigs.k8s.io/controller-tools v0.4.1
	sigs.k8s.io/kustomize/kustomize/v3 v3.8.5
	sigs.k8s.io/yaml v1.2.0
)

replace github.com/metal3-io/baremetal-operator/apis => ./apis
