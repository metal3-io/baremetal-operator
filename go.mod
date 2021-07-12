module github.com/metal3-io/baremetal-operator

go 1.16

require (
	github.com/go-logr/logr v0.4.0
	github.com/gophercloud/gophercloud v0.18.0
	github.com/metal3-io/baremetal-operator/apis v0.0.0
	github.com/metal3-io/baremetal-operator/ironic v0.0.0
	github.com/onsi/ginkgo v1.16.4 // indirect
	github.com/pkg/errors v0.9.1
	github.com/prometheus/client_golang v1.11.0
	github.com/stretchr/testify v1.7.0
	k8s.io/api v0.21.4
	k8s.io/apimachinery v0.21.4
	k8s.io/client-go v0.21.4
	k8s.io/utils v0.0.0-20210802155522-efc7438f0176
	sigs.k8s.io/controller-runtime v0.9.7
	sigs.k8s.io/yaml v1.2.0
)

replace github.com/metal3-io/baremetal-operator/apis => ./apis

replace github.com/metal3-io/baremetal-operator/ironic => ./pkg/ironic
