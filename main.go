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
	"crypto/tls"
	"flag"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	"go.uber.org/zap/zapcore"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	cliflag "k8s.io/component-base/cli/flag"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
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

// Constants for TLS versions.
const (
	TLSVersion12 = "TLS12"
	TLSVersion13 = "TLS13"
)

type TLSOptions struct {
	TLSMaxVersion   string
	TLSMinVersion   string
	TLSCipherSuites string
}

var (
	scheme               = k8sruntime.NewScheme()
	setupLog             = ctrl.Log.WithName("setup")
	healthAddr           string
	tlsOptions           = TLSOptions{}
	tlsSupportedVersions = []string{TLSVersion12, TLSVersion13}
)

const leaderElectionID = "baremetal-operator"

func init() {
	_ = clientgoscheme.AddToScheme(scheme)

	_ = metal3api.AddToScheme(scheme)
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
	var bmh webhook.Validator = &metal3api.BareMetalHost{}
	if err := ctrl.NewWebhookManagedBy(mgr).
		For(bmh).
		Complete(); err != nil {
		setupLog.Error(err, "unable to create webhook", "webhook", "BareMetalHost")
		os.Exit(1)
	}

	var bmces webhook.Validator = &metal3api.BMCEventSubscription{}
	if err := ctrl.NewWebhookManagedBy(mgr).
		For(bmces).
		Complete(); err != nil {
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
	var restConfigQPS float64
	var restConfigBurst int
	var controllerConcurrency int

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
	// NOTE (honza): Downstream only: disabling webhooks until CBO supports them
	flag.IntVar(&webhookPort, "webhook-port", 0,
		"Webhook Server port (set to 0 to disable)")
	flag.Float64Var(&restConfigQPS, "kube-api-qps", 20,
		"Maximum queries per second from the controller client to the Kubernetes API server. Default 20")
	flag.IntVar(&restConfigBurst, "kube-api-burst", 30,
		"Maximum number of queries that should be allowed in one burst from the controller client to the Kubernetes API server. Default 30")
	flag.StringVar(&tlsOptions.TLSMinVersion, "tls-min-version", TLSVersion12,
		"The minimum TLS version in use by the webhook server.\n"+
			fmt.Sprintf("Possible values are %s.", strings.Join(tlsSupportedVersions, ", ")),
	)
	flag.StringVar(&tlsOptions.TLSMaxVersion, "tls-max-version", TLSVersion13,
		"The maximum TLS version in use by the webhook server.\n"+
			fmt.Sprintf("Possible values are %s.", strings.Join(tlsSupportedVersions, ", ")),
	)

	tlsCipherPreferredValues := cliflag.PreferredTLSCipherNames()
	tlsCipherInsecureValues := cliflag.InsecureTLSCipherNames()
	flag.StringVar(&tlsOptions.TLSCipherSuites, "tls-cipher-suites", "",
		"Comma-separated list of cipher suites for the webhook server. "+
			"If omitted, the default Go cipher suites will be used. \n"+
			"Preferred values: "+strings.Join(tlsCipherPreferredValues, ", ")+". \n"+
			"Insecure values: "+strings.Join(tlsCipherInsecureValues, ", ")+".")
	flag.IntVar(&controllerConcurrency, "controller-concurrency", 0,
		"Number of CRs of each type to process simultaneously")
	flag.Parse()

	logOpts := zap.Options{}
	if devLogging {
		logOpts.Development = true
		logOpts.TimeEncoder = zapcore.ISO8601TimeEncoder
	} else {
		logOpts.TimeEncoder = zapcore.EpochTimeEncoder
	}
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&logOpts)))

	printVersion()

	enableWebhook := webhookPort != 0

	leaderElectionNamespace := os.Getenv("POD_NAMESPACE")
	if leaderElectionNamespace == "" {
		leaderElectionNamespace = watchNamespace
	}
	tlsOptionOverrides, err := GetTLSOptionOverrideFuncs(tlsOptions)
	if err != nil {
		setupLog.Error(err, "unable to add TLS settings to the webhook server")
		os.Exit(1)
	}
	restConfig := ctrl.GetConfigOrDie()
	restConfig.QPS = float32(restConfigQPS)
	restConfig.Burst = restConfigBurst

	var watchNamespaces map[string]cache.Config
	if watchNamespace != "" {
		watchNamespaces = map[string]cache.Config{
			watchNamespace: {},
		}
	}

	ctrlOpts := ctrl.Options{
		Scheme:  scheme,
		Metrics: metricsserver.Options{BindAddress: metricsBindAddr},
		WebhookServer: webhook.NewServer(webhook.Options{
			Port:    webhookPort,
			TLSOpts: tlsOptionOverrides,
		}),
		LeaderElection:          enableLeaderElection,
		LeaderElectionID:        leaderElectionID,
		LeaderElectionNamespace: leaderElectionNamespace,
		HealthProbeBindAddress:  healthAddr,
		Cache: cache.Options{
			ByObject:          secretutils.AddSecretSelector(nil),
			DefaultNamespaces: watchNamespaces,
		},
	}

	mgr, err := ctrl.NewManager(restConfig, ctrlOpts)
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
		provLog := zap.New(zap.UseFlagOptions(&logOpts)).WithName("provisioner")
		provisionerFactory = ironic.NewProvisionerFactory(provLog, preprovImgEnable)
	}

	maxConcurrency, err := getMaxConcurrentReconciles(controllerConcurrency)
	if err != nil {
		setupLog.Error(err, "unable to create controllers")
		os.Exit(1)
	}

	if err = (&metal3iocontroller.BareMetalHostReconciler{
		Client:             mgr.GetClient(),
		Log:                ctrl.Log.WithName("controllers").WithName("BareMetalHost"),
		ProvisionerFactory: provisionerFactory,
		APIReader:          mgr.GetAPIReader(),
	}).SetupWithManager(mgr, preprovImgEnable, maxConcurrency); err != nil {
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
			if err = (&imgReconciler).SetupWithManager(mgr, maxConcurrency); err != nil {
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
	}).SetupWithManager(mgr, maxConcurrency); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "HostFirmwareSettings")
		os.Exit(1)
	}

	if err = (&metal3iocontroller.BMCEventSubscriptionReconciler{
		Client:             mgr.GetClient(),
		Log:                ctrl.Log.WithName("controllers").WithName("BMCEventSubscription"),
		ProvisionerFactory: provisionerFactory,
	}).SetupWithManager(mgr, maxConcurrency); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "BMCEventSubscription")
		os.Exit(1)
	}

	if err = (&metal3iocontroller.HostFirmwareComponentsReconciler{
		Client:             mgr.GetClient(),
		Log:                ctrl.Log.WithName("controllers").WithName("HostFirmwareComponents"),
		ProvisionerFactory: provisionerFactory,
	}).SetupWithManager(mgr, maxConcurrency); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "HostFirmwareComponents")
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

// GetTLSOptionOverrideFuncs returns a list of TLS configuration overrides to be used
// by the webhook server.
func GetTLSOptionOverrideFuncs(options TLSOptions) ([]func(*tls.Config), error) {
	var tlsOptions []func(config *tls.Config)

	tlsMinVersion, err := GetTLSVersion(options.TLSMinVersion)
	if err != nil {
		return nil, err
	}

	tlsMaxVersion, err := GetTLSVersion(options.TLSMaxVersion)
	if err != nil {
		return nil, err
	}

	if tlsMaxVersion != 0 && tlsMinVersion > tlsMaxVersion {
		return nil, fmt.Errorf("TLS version flag min version (%s) is greater than max version (%s)",
			options.TLSMinVersion, options.TLSMaxVersion)
	}

	tlsOptions = append(tlsOptions, func(cfg *tls.Config) {
		cfg.MinVersion = tlsMinVersion
	})

	tlsOptions = append(tlsOptions, func(cfg *tls.Config) {
		cfg.MaxVersion = tlsMaxVersion
	})
	// Cipher suites should not be set if empty.
	if tlsMinVersion >= tls.VersionTLS13 &&
		options.TLSCipherSuites != "" {
		setupLog.Info("warning: Cipher suites should not be set for TLS version 1.3. Ignoring ciphers")
		options.TLSCipherSuites = ""
	}

	if options.TLSCipherSuites != "" {
		tlsCipherSuites := strings.Split(options.TLSCipherSuites, ",")
		suites, err := cliflag.TLSCipherSuites(tlsCipherSuites)
		if err != nil {
			return nil, err
		}

		insecureCipherValues := cliflag.InsecureTLSCipherNames()
		for _, cipher := range tlsCipherSuites {
			for _, insecureCipherName := range insecureCipherValues {
				if insecureCipherName == cipher {
					setupLog.Info(fmt.Sprintf("warning: use of insecure cipher '%s' detected.", cipher))
				}
			}
		}
		tlsOptions = append(tlsOptions, func(cfg *tls.Config) {
			cfg.CipherSuites = suites
		})
	}

	return tlsOptions, nil
}

// GetTLSVersion returns the corresponding tls.Version or error.
func GetTLSVersion(version string) (uint16, error) {
	var v uint16

	switch version {
	case TLSVersion12:
		v = tls.VersionTLS12
	case TLSVersion13:
		v = tls.VersionTLS13
	default:
		return 0, fmt.Errorf("unexpected TLS version %q (must be one of: %s)", version, strings.Join(tlsSupportedVersions, ", "))
	}
	return v, nil
}

func getMaxConcurrentReconciles(controllerConcurrency int) (int, error) {
	if controllerConcurrency > 0 {
		ctrl.Log.Info(fmt.Sprintf("controller concurrency will be set to %d according to command line flag", controllerConcurrency))
		return controllerConcurrency, nil
	} else if controllerConcurrency < 0 {
		return 0, fmt.Errorf("controller concurrency value: %d is invalid", controllerConcurrency)
	}

	// controller-concurrency value is 0 i.e. no values passed via the flag
	// maxConcurrentReconcile value would be set based on env var or number of CPUs.
	maxConcurrentReconciles := runtime.NumCPU()
	if maxConcurrentReconciles > 8 {
		maxConcurrentReconciles = 8
	}
	if maxConcurrentReconciles < 2 {
		maxConcurrentReconciles = 2
	}
	if mcrEnv, ok := os.LookupEnv("BMO_CONCURRENCY"); ok {
		mcr, err := strconv.Atoi(mcrEnv)
		if err != nil {
			return 0, errors.Wrap(err, fmt.Sprintf("BMO_CONCURRENCY value: %s is invalid", mcrEnv))
		}
		if mcr > 0 {
			ctrl.Log.Info(fmt.Sprintf("BMO_CONCURRENCY of %d is set via an environment variable", mcr))
			maxConcurrentReconciles = mcr
		} else {
			ctrl.Log.Info(fmt.Sprintf("Invalid BMO_CONCURRENCY value. Operator Concurrency will be set to a default value of %d", maxConcurrentReconciles))
		}
	} else {
		ctrl.Log.Info(fmt.Sprintf("Operator Concurrency will be set to a default value of %d", maxConcurrentReconciles))
	}
	return maxConcurrentReconciles, nil
}
