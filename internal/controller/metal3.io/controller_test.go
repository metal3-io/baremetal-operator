package controllers

import (
	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	"k8s.io/client-go/kubernetes/scheme"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	logz "sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func init() {
	logf.SetLogger(logz.New(logz.UseDevMode(true)))
	// Register our package types with the global scheme
	err := metal3api.AddToScheme(scheme.Scheme)
	if err != nil {
		logf.Log.Error(err, "Cannot Add scheme into metal3api")
	}
}
