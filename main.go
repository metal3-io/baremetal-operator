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
	"runtime"

	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	metal3iov1alpha1 "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	metal3iocontroller "github.com/metal3-io/baremetal-operator/controllers/metal3.io"
	"github.com/metal3-io/baremetal-operator/pkg/bmc"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner/demo"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner/empty"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner/fixture"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner/ironic"
	"github.com/metal3-io/baremetal-operator/pkg/version"
	// +kubebuilder:scaffold:imports
)

var (
	scheme     = k8sruntime.NewScheme()
	setupLog   = ctrl.Log.WithName("setup")
	healthAddr string
)

func init() {
	_ = clientgoscheme.AddToScheme(scheme)

	_ = metal3iov1alpha1.AddToScheme(scheme)
	// +kubebuilder:scaffold:scheme
}

func printVersion() {
	setupLog.Info(fmt.Sprintf("Go Version: %s", runtime.Version()))
	setupLog.Info(fmt.Sprintf("Go OS/Arch: %s/%s", runtime.GOOS, runtime.GOARCH))
	setupLog.Info(fmt.Sprintf("baremetal-operator version: %s", version.String))
}

func setupChecks(mgr ctrl.Manager) {
	if err := mgr.AddReadyzCheck("ping", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to create ready check")
		os.Exit(1)
	}

	if err := mgr.AddHealthzCheck("ping", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to create health check")
		os.Exit(1)
	}
}

func main() {
	var watchNamespace string
	var metricsAddr string
	var enableLeaderElection bool
	var devLogging bool
	var runInTestMode bool
	var runInDemoMode bool

	// From CAPI point of view, BMO should be able to watch all namespaces
	// in case of a deployment that is not multi-tenant. If the deployment
	// is for multi-tenancy, then the BMO should watch only the provided
	// namespace.
	flag.StringVar(&watchNamespace, "namespace", os.Getenv("WATCH_NAMESPACE"),
		"Namespace that the controller watches to reconcile host resources.")
	flag.StringVar(&metricsAddr, "metrics-addr", "127.0.0.1:8085",
		"The address the metric endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "enable-leader-election", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.BoolVar(&devLogging, "dev", false, "enable developer logging")
	flag.BoolVar(&runInTestMode, "test-mode", false, "disable ironic communication")
	flag.BoolVar(&runInDemoMode, "demo-mode", false,
		"use the demo provisioner to set host states")
	flag.StringVar(&healthAddr, "health-addr", ":9440",
		"The address the health endpoint binds to.")
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseDevMode(devLogging)))

	printVersion()

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                  scheme,
		MetricsBindAddress:      metricsAddr,
		Port:                    0, // Add flag with default of 9443 when adding webhooks
		LeaderElection:          enableLeaderElection,
		LeaderElectionID:        "baremetal-operator",
		LeaderElectionNamespace: watchNamespace,
		Namespace:               watchNamespace,
		HealthProbeBindAddress:  healthAddr,
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	provisionerFactory := func(host metal3iov1alpha1.BareMetalHost, bmcCreds bmc.Credentials, publish provisioner.EventPublisher) (provisioner.Provisioner, error) {
		isUnmanaged := host.Spec.ExternallyProvisioned && !host.HasBMCDetails()

		hostCopy := host.DeepCopy()

		if runInTestMode {
			ctrl.Log.Info("using test provisioner")
			fix := fixture.Fixture{}
			return fix.New(*hostCopy, bmcCreds, publish)
		} else if runInDemoMode {
			ctrl.Log.Info("using demo provisioner")
			return demo.New(*hostCopy, bmcCreds, publish)
		} else if isUnmanaged {
			ctrl.Log.Info("using empty provisioner")
			return empty.New(*hostCopy, bmcCreds, publish)
		}
		ironic.LogStartup()
		return ironic.New(*hostCopy, bmcCreds, publish)
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

	setupChecks(mgr)

	// +kubebuilder:scaffold:builder

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
