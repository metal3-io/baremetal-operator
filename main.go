/*


Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"flag"
	"fmt"
	"os"

	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	metal3iov1alpha1 "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	metal3iocontroller "github.com/metal3-io/baremetal-operator/controllers/metal3.io"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner/demo"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner/fixture"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner/ironic"
	// +kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	_ = clientgoscheme.AddToScheme(scheme)

	_ = metal3iov1alpha1.AddToScheme(scheme)
	// +kubebuilder:scaffold:scheme
}

func main() {
	var devLogging bool
	var runInTestMode bool
	var runInDemoMode bool
	var metricsAddr string
	var watchNamespace string

	flag.BoolVar(&devLogging, "dev", false, "enable developer logging")
	flag.StringVar(&metricsAddr, "metrics-addr", "127.0.0.1:8085",
		"The address the metric endpoint binds to.")
	// From CAPI point of view, BMO should be able to watch all namespaces
	// in case of a deployment that is not multi-tenant. If the deployment
	// is for multi-tenancy, then the BMO should watch only the provided
	// namespace.
	flag.StringVar(&watchNamespace, "namespace", "",
		"Namespace that the controller watches to reconcile BMO objects.")
	flag.BoolVar(&runInTestMode, "test-mode", false, "disable ironic communication")
	flag.BoolVar(&runInDemoMode, "demo-mode", false,
		"use the demo provisioner to set host states")
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseDevMode(devLogging)))

	ctrl.Log.Info(fmt.Sprintf("gather metrics at http://%s/metrics", metricsAddr))
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                  scheme,
		MetricsBindAddress:      metricsAddr,
		Port:                    9443,
		LeaderElection:          true,
		LeaderElectionID:        "baremetal=operator",
		LeaderElectionNamespace: watchNamespace,
		Namespace:               watchNamespace,
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	var provisionerFactory provisioner.Factory
	if runInTestMode {
		ctrl.Log.Info("USING TEST MODE")
		provisionerFactory = fixture.New
	} else if runInDemoMode {
		ctrl.Log.Info("USING DEMO MODE")
		provisionerFactory = demo.New
	} else {
		provisionerFactory = ironic.New
		ironic.LogStartup()
	}

	if err = (&metal3iocontroller.BareMetalHostReconciler{
		Client:             mgr.GetClient(),
		Log:                ctrl.Log.WithName("controllers").WithName("BareMetalHost"),
		Scheme:             mgr.GetScheme(),
		ProvisionerFactory: provisionerFactory,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "BareMetalHost")
		os.Exit(1)
	}
	// +kubebuilder:scaffold:builder

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
