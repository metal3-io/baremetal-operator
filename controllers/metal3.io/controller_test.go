package controllers

import (
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	metal3v1alpha1 "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
)

var testScheme = runtime.NewScheme()
var log = ctrl.Log.WithName("controllers").WithName("controller_test")

func init() {
	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))
	// Register our package types with the global scheme
	_ = metal3v1alpha1.AddToScheme(testScheme)
	_ = clientgoscheme.AddToScheme(testScheme)
}
