package main

import (
	"embed"
	"flag"
	"log"
	"os"
	"path/filepath"
)

var (
	deployBMOFlag         bool
	deployIronicFlag      bool
	deployTLSFlag         bool
	deployWithoutAuthFlag bool
	deployKeepalivedFlag  bool
	deployMariadbFlag     bool
	showHelpFlag          bool

	//go:embed templates/*.tpl
	templateFiles embed.FS
)

func main() {
	flag.BoolVar(&deployBMOFlag, "b", false, "Deploy BMO")
	flag.BoolVar(&deployIronicFlag, "i", false, "Deploy Ironic")
	flag.BoolVar(&deployTLSFlag, "t", false, "Deploy with TLS enabled")
	flag.BoolVar(&deployWithoutAuthFlag, "n", false, "Deploy with authentication disabled")
	flag.BoolVar(&deployKeepalivedFlag, "k", false, "Deploy with keepalived")
	flag.BoolVar(&deployMariadbFlag, "m", false, "Deploy with mariadb (requires TLS enabled)")
	flag.BoolVar(&showHelpFlag, "h", false, "Show help message")

	flag.Usage = usage
	flag.Parse()

	deployBasicAuthFlag := !deployWithoutAuthFlag

	data := DeployContext{
		DeployBasicAuth:                    deployBasicAuthFlag,
		DeployTLS:                          deployTLSFlag,
		DeployKeepAlived:                   deployKeepalivedFlag,
		DeployMariadb:                      deployMariadbFlag,
		RestartContainerCertificateUpdated: GetEnvOrDefault("RESTART_CONTAINER_CERTIFICATE_UPDATED", "false"),
		IronicHostIP:                       GetEnvOrDefault("IRONIC_HOST_IP", ""),
		MariaDBHostIP:                      GetEnvOrDefault("MARIADB_HOST_IP", "127.0.0.1"),
		TemplateFiles:                      templateFiles,
	}

	if showHelpFlag {
		usage()
		return
	}

	if !deployBasicAuthFlag {
		log.Println("WARNING: Deploying without authentication is not recommended")
	}

	if deployMariadbFlag && !deployTLSFlag {
		log.Println("ERROR: Deploying Ironic with MariaDB without TLS is not supported.")
		usage()
		os.Exit(1)
	}

	if !deployBMOFlag && !deployIronicFlag {
		log.Println("ERROR: At least one of -b (BMO) or -i (Ironic) must be specified for deployment.")
		usage()
		os.Exit(1)
	}

	exePath, err := os.Executable()
	if err != nil {
		log.Fatalf("Failed to determine executable path: %v", err)
	}
	scriptDir := filepath.Join(filepath.Dir(exePath), "..", "..")

	tempBMOOverlay := filepath.Join(scriptDir, "config", "overlays", "temp")
	tempIronicOverlay := filepath.Join(scriptDir, "ironic-deployment", "overlays", "temp")

	// Cleaning up directories before making new ones
	err = os.RemoveAll(tempBMOOverlay)
	if err != nil {
		log.Fatalf("Failed to remove temp BMO overlay: %v", err)
	}
	err = os.RemoveAll(tempIronicOverlay)
	if err != nil {
		log.Fatalf("Failed to remove temp Ironic overlay: %v", err)
	}

	// Create temporary overlays where we can make changes.
	err = os.MkdirAll(tempBMOOverlay, 0755)
	if err != nil {
		log.Fatalf("Failed to create temp BMO overlay directory: %v", err)
	}
	err = os.MkdirAll(tempIronicOverlay, 0755)
	if err != nil {
		log.Fatalf("Failed to create temp Ironic overlay directory: %v", err)
	}

	// Generate credentials as needed
	ironicDataDir := GetEnvOrDefault("IRONIC_DATA_DIR", "/tmp/metal3/ironic/")
	ironicAuthDir := filepath.Join(ironicDataDir, "auth")
	err = os.MkdirAll(ironicAuthDir, 0755)
	if err != nil {
		log.Fatalf("Failed to create temp BMO overlay directory: %v", err)
	}

	// If usernames and passwords are unset, read them from file or generate them
	if deployBasicAuthFlag {
		ironicUsername, err := getEnvOrFileContent("IRONIC_USERNAME", filepath.Join(ironicAuthDir, "ironic-username"), 12)
		if err != nil {
			log.Fatalf("Error retrieving Ironic username: %v", err)
		}
		ironicPassword, err := getEnvOrFileContent("IRONIC_PASSWORD", filepath.Join(ironicAuthDir, "ironic-password"), 12)
		if err != nil {
			log.Fatalf("Error retrieving Ironic password: %v", err)
		}

		if deployBMOFlag {
			tempBMOOverlayPath := filepath.Join(tempBMOOverlay, "ironic-username")
			err = os.WriteFile(tempBMOOverlayPath, []byte(ironicUsername), 0600)
			if err != nil {
				log.Fatalf("Failed to write BMO overlay file: %v", err)
			}
			tempBMOOverlayPath = filepath.Join(tempBMOOverlay, "ironic-password")
			err = os.WriteFile(tempBMOOverlayPath, []byte(ironicPassword), 0600)
			if err != nil {
				log.Fatalf("Failed to write BMO overlay file: %v", err)
			}
		}

		if deployIronicFlag {
			ironicHtpasswd, err := GenerateHtpasswd(ironicUsername, ironicPassword)
			if err != nil {
				log.Fatalf("Failed to generate ironic htpasswd: %v", err)
			}

			htpasswdPath := filepath.Join(tempIronicOverlay, "ironic-htpasswd")
			err = os.WriteFile(htpasswdPath, []byte("IRONIC_HTPASSWD="+ironicHtpasswd), 0600)
			if err != nil {
				log.Fatalf("Failed to write ironic htpasswd file: %v", err)
			}
		}
	}

	if deployIronicFlag {
		err = deployIronic(&data, tempIronicOverlay)
		if err != nil {
			log.Fatalf("Failed to deploy Ironic: %v", err)
		}

	}

	if deployBMOFlag {
		err = deployBMO(&data, tempBMOOverlay)
		if err != nil {
			log.Fatalf("Failed to deploy BMO: %v", err)
		}
	}

	cleanup(deployBasicAuthFlag, deployBMOFlag, deployIronicFlag, tempBMOOverlay, tempIronicOverlay)
}
