package main

import (
	"context"
	"embed"
	"flag"
	"fmt"
	"log"
	"os"
)

var (
	deployBMOFlag         bool
	bmoOverlay            string
	deployIronicFlag      bool
	ironicOverlay         string
	deployTLSFlag         bool
	deployWithoutAuthFlag bool
	deployBasicAuthFlag   bool
	deployKeepalivedFlag  bool
	deployMariadbFlag     bool
	showHelpFlag          bool
	err                   error
	defaultMap            map[string]string
	ironicEnvFile         string
	configMapEnvFile      string

	//go:embed templates/*.tpl
	templateFiles embed.FS
)

func init() {
	flag.BoolVar(&deployBMOFlag, "b", false, "Deploy BMO")
	flag.StringVar(&bmoOverlay, "bmo-overlay", "", "Kustomization overlay to install BMO")
	flag.BoolVar(&deployIronicFlag, "i", false, "Deploy Ironic")
	flag.StringVar(&ironicOverlay, "ironic-overlay", "", "Kustomization overlay to install Ironic")
	flag.StringVar(&ironicEnvFile, "ironic-envfile", "", "ironic.env file that will be consumed by the BMO overlay")
	flag.StringVar(&configMapEnvFile, "ironic-bmo-config-map-envfile", "", "ironic_bmo_configmap.env file that will be consumed by the Ironic overlay")
	flag.BoolVar(&deployTLSFlag, "t", false, "Deploy with TLS enabled")
	flag.BoolVar(&deployWithoutAuthFlag, "n", false, "Deploy with authentication disabled")
	flag.BoolVar(&deployKeepalivedFlag, "k", false, "Deploy with keepalived")
	flag.BoolVar(&deployMariadbFlag, "m", false, "Deploy with mariadb (requires TLS enabled)")
	flag.BoolVar(&showHelpFlag, "h", false, "Show help message")

	deployBasicAuthFlag = !deployWithoutAuthFlag
	defaultMap = map[string]string{
		"IRONIC_HOST_IP":                        "",
		"IRONIC_DATA_DIR":                       "/tmp/metal3/ironic/",
		"MARIADB_HOST_IP":                       "127.0.0.1",
		"RESTART_CONTAINER_CERTIFICATE_UPDATED": "false",
	}

	flag.Usage = func() {
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, `
NOTES:
- Environment Variable "IRONIC_HOST_IP" must be set with a valid IP to install Ironic.
- If environment variable "BMOPATH" is set, exists and not empty, Ironic and BMO will be installed based on the kustomization in that directory.
- If "deploy-cli" is not built (i.e. run with "go run *.go"), BMOPATH will be inferred according to the relative path from the "deploy-cli" directory.


AVAILABLE ENVIRONMENT VARIABLES:
- IRONIC_USERNAME
- IRONIC_PASSWORD
`)
		for key, val := range defaultMap {
			fmt.Fprintf(os.Stderr, "- %s, Default: %s\n", key, val)
		}
	}
}

func main() {
	flag.Parse()
	dc := DeployContext{
		Context:                   context.Background(),
		BMOPath:                   getBMOPath(),
		KubeconfigPath:            GetKubeconfigPath(),
		DeployBasicAuth:           deployBasicAuthFlag,
		DeployTLS:                 deployTLSFlag,
		DeployKeepAlived:          deployKeepalivedFlag,
		DeployMariadb:             deployMariadbFlag,
		BMOOverlay:                bmoOverlay,
		IronicOverlay:             ironicOverlay,
		IronicEnvFile:             ironicEnvFile,
		IronicBMOConfigMapEnvFile: configMapEnvFile,
		TemplateFiles:             templateFiles,
		DefaultMap:                &defaultMap,
	}

	dc.RestartContainerCertificateUpdated, err = dc.GetEnvOrDefault("RESTART_CONTAINER_CERTIFICATE_UPDATED")
	if err != nil {
		log.Fatal(err)
	}
	dc.IronicHostIP, err = dc.GetEnvOrDefault("IRONIC_HOST_IP")
	if err != nil {
		log.Fatal(err)
	}
	dc.MariaDBHostIP, err = dc.GetEnvOrDefault("MARIADB_HOST_IP")
	if err != nil {
		log.Fatal(err)
	}

	if showHelpFlag {
		flag.Usage()
		return
	}

	if deployMariadbFlag && !deployTLSFlag {
		log.Println("ERROR: Deploying Ironic with MariaDB without TLS is not supported.")
		flag.Usage()
		os.Exit(1)
	}

	if !deployBMOFlag && !deployIronicFlag {
		log.Println("ERROR: At least one of -b (BMO) or -i (Ironic) must be specified for deployment.")
		flag.Usage()
		os.Exit(1)
	}

	if err := dc.determineIronicAuth(); err != nil {
		log.Fatalf("Error retrieving Ironic username/password: %v", err)
	}

	if deployIronicFlag {
		if dc.IronicHostIP == "" {
			log.Fatalf("ERROR: Environment Variable IRONIC_HOST_IP is required to install ironic")
		}
		err = dc.deployIronic()
		if err != nil {
			log.Fatalf("Failed to deploy Ironic: %v", err)
		}
	}

	if deployBMOFlag {
		err = dc.deployBMO()
		if err != nil {
			log.Fatalf("Failed to deploy BMO: %v", err)
		}
	}
}
