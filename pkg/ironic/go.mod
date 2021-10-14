module github.com/metal3-io/baremetal-operator/pkg/ironic

go 1.16

require (
	github.com/go-logr/logr v0.4.0
	github.com/gophercloud/gophercloud v0.18.0
	github.com/metal3-io/baremetal-operator/apis v0.0.0
	github.com/pkg/errors v0.9.1
	github.com/stretchr/testify v1.7.0
	go.etcd.io/etcd v0.5.0-alpha.5.0.20200910180754-dd1b699fc489
	sigs.k8s.io/controller-runtime v0.9.7
)

replace github.com/metal3-io/baremetal-operator/apis => ../../apis
