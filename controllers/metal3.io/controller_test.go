package controllers

import (
	"k8s.io/client-go/kubernetes/scheme"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	logz "sigs.k8s.io/controller-runtime/pkg/log/zap"

	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
)

func init() {
	logf.SetLogger(logz.New(logz.UseDevMode(true)))
	// Register our package types with the global scheme
	metal3api.AddToScheme(scheme.Scheme)
}
