package main

import (
	"flag"
	"fmt"
	"os"
	"runtime"

	"github.com/metal3-io/baremetal-operator/pkg/apis"
	"github.com/metal3-io/baremetal-operator/pkg/controller"
	"github.com/metal3-io/baremetal-operator/pkg/version"
	"github.com/operator-framework/operator-sdk/pkg/k8sutil"
	sdkVersion "github.com/operator-framework/operator-sdk/version"

	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"

	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var (
	log            = logf.Log.WithName("cmd")
	watchNamespace string
)

func printVersion() {
	log.Info(fmt.Sprintf("Go Version: %s", runtime.Version()))
	log.Info(fmt.Sprintf("Go OS/Arch: %s/%s", runtime.GOOS, runtime.GOARCH))
	log.Info(fmt.Sprintf("Version of operator-sdk: %v", sdkVersion.Version))
	log.Info(fmt.Sprintf("Component version: %s", version.String))
}

func main() {
	var err error
	devLogging := flag.Bool("dev", false, "enable dev logging")
	metricsAddr := flag.String("metrics-addr", "127.0.0.1:8085", "The address the metric endpoint binds to.")
	flag.StringVar(&watchNamespace, "namespace", "", "Namespace that the controller watches to reconcile BMO objects.")
	flag.Parse()

	// The logger instantiated here can be changed to any logger
	// implementing the logr.Logger interface. This logger will
	// be propagated through the whole operator, generating
	// uniform and structured logs.
	logf.SetLogger(logf.ZapLogger(*devLogging))

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

	// Get a config to talk to the apiserver
	cfg, err := config.GetConfig()
	if err != nil {
		log.Error(err, "")
		os.Exit(1)
	}

	log.Info(fmt.Sprintf("gather metrics at http://%s/metrics", *metricsAddr))
	opts := manager.Options{
		LeaderElection:          true,
		LeaderElectionID:        "baremetal-operator",
		LeaderElectionNamespace: watchNamespace,
		Namespace:               watchNamespace,
		MetricsBindAddress:      *metricsAddr,
	}

	// Create a new Cmd to provide shared dependencies and start components
	mgr, err := manager.New(cfg, opts)
	if err != nil {
		log.Error(err, "")
		os.Exit(1)
	}

	log.Info("Registering Components.")

	// Setup Scheme for all resources
	if err := apis.AddToScheme(mgr.GetScheme()); err != nil {
		log.Error(err, "")
		os.Exit(1)
	}

	// Setup all Controllers
	if err := controller.AddToManager(mgr); err != nil {
		log.Error(err, "")
		os.Exit(1)
	}

	log.Info("Starting the Cmd.")

	// Start the Cmd
	if err := mgr.Start(signals.SetupSignalHandler()); err != nil {
		log.Error(err, "Manager exited non-zero")
		os.Exit(1)
	}
}
