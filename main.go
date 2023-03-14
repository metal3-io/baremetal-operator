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

	"go.uber.org/zap/zapcore"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	metal3iov1alpha1 "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	metal3iocontroller "github.com/metal3-io/baremetal-operator/controllers/metal3.io"
	"github.com/metal3-io/baremetal-operator/pkg/imageprovider"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner/demo"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner/fixture"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner/ironic"
	"github.com/metal3-io/baremetal-operator/pkg/secretutils"
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
}

func printVersion() {
	setupLog.Info(fmt.Sprintf("Go Version: %s", runtime.Version()))
	setupLog.Info(fmt.Sprintf("Go OS/Arch: %s/%s", runtime.GOOS, runtime.GOARCH))
	setupLog.Info(fmt.Sprintf("Git commit: %s", version.Commit))
	setupLog.Info(fmt.Sprintf("Build time: %s", version.BuildTime))
	setupLog.Info(fmt.Sprintf("Component: %s", version.String))
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

func setupWebhooks(mgr ctrl.Manager) {
	if err := (&metal3iov1alpha1.BareMetalHost{}).SetupWebhookWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create webhook", "webhook", "BareMetalHost")
		os.Exit(1)
	}

	if err := (&metal3iov1alpha1.BMCEventSubscription{}).SetupWebhookWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create webhook", "webhook", "BMCEventSubscription")
		os.Exit(1)
	}
}

func main() {
	var watchNamespace string
	var metricsBindAddr string
	var enableLeaderElection bool
	var preprovImgEnable bool
	var devLogging bool
	var runInTestMode bool
	var runInDemoMode bool
	var webhookPort int
	var baremetalhostConcurrency int
	var preprovisioningimageConcurrency int
	var hostfirmwaresettingsConcurrency int
	var bmceventsubscriptionConcurrency int

	// From CAPI point of view, BMO should be able to watch all namespaces
	// in case of a deployment that is not multi-tenant. If the deployment
	// is for multi-tenancy, then the BMO should watch only the provided
	// namespace.
	flag.StringVar(&watchNamespace, "namespace", os.Getenv("WATCH_NAMESPACE"),
		"Namespace that the controller watches to reconcile host resources.")
	flag.StringVar(&metricsBindAddr, "metrics-addr", "127.0.0.1:8085",
		"The address the metric endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "enable-leader-election", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.BoolVar(&preprovImgEnable, "build-preprov-image", false, "enable integration with the PreprovisioningImage API")
	flag.BoolVar(&devLogging, "dev", false, "enable developer logging")
	flag.BoolVar(&runInTestMode, "test-mode", false, "disable ironic communication")
	flag.BoolVar(&runInDemoMode, "demo-mode", false,
		"use the demo provisioner to set host states")
	flag.StringVar(&healthAddr, "health-addr", ":9440",
		"The address the health endpoint binds to.")
	flag.IntVar(&webhookPort, "webhook-port", 9443,
		"Webhook Server port (set to 0 to disable)")
	flag.IntVar(&baremetalhostConcurrency, "baremetalhost-concurrency", 10,
		"Number of baremetalhosts to process simultaneously")
	flag.IntVar(&preprovisioningimageConcurrency, "preprovisioningimage-concurrency", 10,
		"Number of preprovisioningimages to process simultaneously")
	flag.IntVar(&hostfirmwaresettingsConcurrency, "hostfirmwaresettings-concurrency", 10,
		"Number of hostfirmwaresettings to process simultaneously")
	flag.IntVar(&bmceventsubscriptionConcurrency, "bmceventsubscription-concurrency", 10,
		"Number of bmceventsubscriptions to process simultaneously")

	opts := zap.Options{
		Development: devLogging,
		TimeEncoder: zapcore.ISO8601TimeEncoder,
	}
	opts.BindFlags(flag.CommandLine)

	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	printVersion()

	enableWebhook := webhookPort != 0

	leaderElectionNamespace := os.Getenv("POD_NAMESPACE")
	if leaderElectionNamespace == "" {
		leaderElectionNamespace = watchNamespace
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                  scheme,
		MetricsBindAddress:      metricsBindAddr,
		Port:                    webhookPort,
		LeaderElection:          enableLeaderElection,
		LeaderElectionID:        "baremetal-operator",
		LeaderElectionNamespace: leaderElectionNamespace,
		Namespace:               watchNamespace,
		HealthProbeBindAddress:  healthAddr,

		NewCache: cache.BuilderWithOptions(cache.Options{
			SelectorsByObject: secretutils.AddSecretSelector(nil),
		}),
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	var provisionerFactory provisioner.Factory
	if runInTestMode {
		ctrl.Log.Info("using test provisioner")
		provisionerFactory = &fixture.Fixture{}
	} else if runInDemoMode {
		ctrl.Log.Info("using demo provisioner")
		provisionerFactory = &demo.Demo{}
	} else {
		provisionerFactory = ironic.NewProvisionerFactory(preprovImgEnable)
	}

	if err = (&metal3iocontroller.BareMetalHostReconciler{
		Client:             mgr.GetClient(),
		Log:                ctrl.Log.WithName("controllers").WithName("BareMetalHost"),
		ProvisionerFactory: provisionerFactory,
		APIReader:          mgr.GetAPIReader(),
	}).SetupWithManager(mgr, preprovImgEnable, concurrency(baremetalhostConcurrency)); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "BareMetalHost")
		os.Exit(1)
	}

	if preprovImgEnable {
		imgReconciler := metal3iocontroller.PreprovisioningImageReconciler{
			Client:        mgr.GetClient(),
			Log:           ctrl.Log.WithName("controllers").WithName("PreprovisioningImage"),
			APIReader:     mgr.GetAPIReader(),
			Scheme:        mgr.GetScheme(),
			ImageProvider: imageprovider.NewDefaultImageProvider(),
		}
		if imgReconciler.CanStart() {
			if err = (&imgReconciler).SetupWithManager(mgr, concurrency(preprovisioningimageConcurrency)); err != nil {
				setupLog.Error(err, "unable to create controller", "controller", "PreprovisioningImage")
				os.Exit(1)
			}
		}
	}
	// +kubebuilder:scaffold:builder

	if err = (&metal3iocontroller.HostFirmwareSettingsReconciler{
		Client:             mgr.GetClient(),
		Log:                ctrl.Log.WithName("controllers").WithName("HostFirmwareSettings"),
		ProvisionerFactory: provisionerFactory,
	}).SetupWithManager(mgr, concurrency(hostfirmwaresettingsConcurrency)); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "HostFirmwareSettings")
		os.Exit(1)
	}

	if err = (&metal3iocontroller.BMCEventSubscriptionReconciler{
		Client:             mgr.GetClient(),
		Log:                ctrl.Log.WithName("controllers").WithName("BMCEventSubscription"),
		ProvisionerFactory: provisionerFactory,
	}).SetupWithManager(mgr, concurrency(bmceventsubscriptionConcurrency)); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "BMCEventSubscription")
		os.Exit(1)
	}

	setupChecks(mgr)

	if enableWebhook {
		setupWebhooks(mgr)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

func concurrency(c int) controller.Options {
	return controller.Options{MaxConcurrentReconciles: c}
}
