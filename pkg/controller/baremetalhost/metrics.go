package baremetalhost

import (
	"github.com/prometheus/client_golang/prometheus"

	"sigs.k8s.io/controller-runtime/pkg/metrics"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metal3v1alpha1 "github.com/metal3-io/baremetal-operator/pkg/apis/metal3/v1alpha1"
)

const (
	labelHostNamespace = "namespace"
	labelHostName      = "host"
	labelPowerOnOff    = "on_off"
)

var reconcileCounters = prometheus.NewCounterVec(prometheus.CounterOpts{
	Name: "metal3_reconcile_total",
	Help: "The number of times hosts have been reconciled",
}, []string{labelHostNamespace, labelHostName})
var reconcileErrorCounter = prometheus.NewCounter(prometheus.CounterOpts{
	Name: "metal3_reconcile_error_total",
	Help: "The number of times the operator has failed to reconcile a host",
})

var powerChangeAttempts = prometheus.NewCounterVec(prometheus.CounterOpts{
	Name: "metal3_operation_power_change_total",
	Help: "Number of times a host has been powered on or off",
}, []string{labelHostNamespace, labelHostName, labelPowerOnOff})

var slowOperationBuckets = []float64{30, 90, 180, 360, 720, 1440}

var stateTime = map[metal3v1alpha1.ProvisioningState]*prometheus.HistogramVec{
	metal3v1alpha1.StateRegistering: prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name: "metal3_operation_register_duration_seconds",
		Help: "Length of time per registration per host",
	}, []string{labelHostNamespace, labelHostName}),
	metal3v1alpha1.StateInspecting: prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "metal3_operation_inspect_duration_seconds",
		Help:    "Length of time per hardware inspection per host",
		Buckets: slowOperationBuckets,
	}, []string{labelHostNamespace, labelHostName}),
	metal3v1alpha1.StateProvisioning: prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "metal3_operation_provision_duration_seconds",
		Help:    "Length of time per hardware provision operation per host",
		Buckets: slowOperationBuckets,
	}, []string{labelHostNamespace, labelHostName}),
	metal3v1alpha1.StateDeprovisioning: prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "metal3_operation_deprovision_duration_seconds",
		Help:    "Length of time per hardware deprovision operation per host",
		Buckets: slowOperationBuckets,
	}, []string{labelHostNamespace, labelHostName}),
}

func init() {
	metrics.Registry.MustRegister(
		reconcileCounters,
		reconcileErrorCounter,
		powerChangeAttempts)

	for _, collector := range stateTime {
		metrics.Registry.MustRegister(collector)
	}
}

func hostMetricLabels(request reconcile.Request) prometheus.Labels {
	return prometheus.Labels{
		labelHostNamespace: request.Namespace,
		labelHostName:      request.Name,
	}
}
