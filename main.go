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

	"github.com/google/martian/log"
	metal3iov1alpha1 "github.com/metal3-io/baremetal-operator/api/v1alpha1"
	"github.com/metal3-io/baremetal-operator/controllers"
	"github.com/metal3-io/baremetal-operator/provisioner"
	"github.com/metal3-io/baremetal-operator/provisioner/demo"
	"github.com/metal3-io/baremetal-operator/provisioner/fixture"
	"github.com/metal3-io/baremetal-operator/provisioner/ironic"
	"github.com/metal3-io/baremetal-operator/version"
	"github.com/operator-framework/operator-sdk/pkg/k8sutil"
	// +kubebuilder:scaffold:imports
)

var (
	scheme         = runtime.NewScheme()
	setupLog       = ctrl.Log.WithName("setup")
	watchNamespace string
)

func init() {
	_ = clientgoscheme.AddToScheme(scheme)

	_ = metal3iov1alpha1.AddToScheme(scheme)
	// +kubebuilder:scaffold:scheme
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var runInTestMode bool
	var runInDemoMode bool

	devLogging := flag.Bool("dev", false, "enable dev logging")
	flag.StringVar(&metricsAddr, "metrics-addr", ":8085", "The address the metric endpoint binds to.")
	flag.StringVar(&watchNamespace, "namespace", "", "Namespace that the controller watches to reconcile BMO objects.")
	flag.BoolVar(&enableLeaderElection, "enable-leader-election", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.BoolVar(&runInTestMode, "test-mode", false, "disable ironic communication")
	flag.BoolVar(&runInDemoMode, "demo-mode", false,
		"use the demo provisioner to set host states")
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseDevMode(&devLogging)))

	printVersion()

	// From CAPI point of view, BMO should be able to watch all namespaces
	// in case of a deployment that is not multi-tenant. If the deployment
	// is for multi-tenancy, then the BMO should watch only the provided
	// namespace.
	if watchNamespace == "" {
		watchNamespace, err = k8sutil.GetWatchNamespace()
		if err != nil {
			watchNamespace = ""
		}
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                  scheme,
		MetricsBindAddress:      metricsAddr,
		Port:                    9443,
		LeaderElection:          enableLeaderElection,
		LeaderElectionID:        "baremetal-operator",
		LeaderElectionNamespace: watchNamespace,
		Namespace:               watchNamespace,
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	var provisionerFactory provisioner.Factory
	switch {
	case runInTestMode:
		setupLog.Info("USING TEST MODE")
		provisionerFactory = fixture.New
	case runInDemoMode:
		setupLog.Info("USING DEMO MODE")
		provisionerFactory = demo.New
	default:
		provisionerFactory = ironic.New
		ironic.LogStartup()
	}
	if err = (&controllers.BareMetalHostReconciler{
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

func printVersion() {
	log.Info(fmt.Sprintf("Go Version: %s", runtime.Version()))
	log.Info(fmt.Sprintf("Go OS/Arch: %s/%s", runtime.GOOS, runtime.GOARCH))
	log.Info(fmt.Sprintf("Version of operator-sdk: %v", sdkVersion.Version))
	log.Info(fmt.Sprintf("Component version: %s", version.String))
}
