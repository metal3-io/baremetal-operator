package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"

	"embed"
	"golang.org/x/crypto/bcrypt"
	"regexp"
	testexec "sigs.k8s.io/cluster-api/test/framework/exec"
	"sigs.k8s.io/kustomize/api/krusty"
	"sigs.k8s.io/kustomize/kyaml/filesys"
	"strings"
	"text/template"
)

// DeployContext defines the context of the deploy run
type DeployContext struct {
	Context context.Context
	// OPTIONAL: Path to BMO repository.
	BMOPath string
	// Path to Kubeconfig file
	KubeconfigPath string
	// Whether to deploy with basic auth
	DeployBasicAuth bool
	// Whether to deploy with TLS
	DeployTLS bool
	// Whether to deploy KeepAlived
	DeployKeepAlived bool
	// Whether to deploy Mariadb
	DeployMariadb bool
	// string represents whether to deploy Ironic with RestartContainerCertificateUpdated
	RestartContainerCertificateUpdated string
	// kustomization overlays
	BMOOverlay string
	// ironic.env file location
	IronicEnvFile string
	IronicOverlay string
	// ironic_bmo_configmap.env file location
	IronicBMOConfigMapEnvFile string
	// Endpoint for Ironic
	IronicHostIP string
	// Username and Password for Ironic authentication
	IronicUsername string
	IronicPassword string
	// Endpoint for Mariadb
	MariaDBHostIP string
	// Templates to render files using in deployments
	TemplateFiles embed.FS
	// Default environment variables map
	DefaultMap *map[string]string
}

// determineIronicAuth determines the username and password configured for ironic
// authentication, following the order:
// - `IRONIC_USERNAME` and `IRONIC_PASSWORD` env var
// - `ironic-username` and `ironic-password` files content
// - Random string
func (d *DeployContext) determineIronicAuth() error {
	if !d.DeployTLS {
		log.Println("WARNING: Deploying without authentication is not recommended")
		return nil
	}
	ironicDataDir, err := d.GetEnvOrDefault("IRONIC_DATA_DIR")
	if err != nil {
		return err
	}
	ironicAuthDir := filepath.Join(ironicDataDir, "auth")
	ironicUsernameFile := filepath.Join(ironicAuthDir, "ironic-username")

	if err := os.MkdirAll(ironicAuthDir, 0755); err != nil {
		return err
	}

	ironicUsername, err := getEnvOrFileContent("IRONIC_USERNAME", ironicUsernameFile)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if ironicUsername == "" {
		ironicUsername, err = GenerateRandomString(12)
		if err != nil {
			return err
		}
	}
	if err = os.WriteFile(ironicUsernameFile, []byte(ironicUsername), 0600); err != nil {
		return err
	}

	ironicPasswordFile := filepath.Join(ironicAuthDir, "ironic-password")
	ironicPassword, err := getEnvOrFileContent("IRONIC_PASSWORD", ironicPasswordFile)
	if err != nil || ironicPassword == "" {
		ironicPassword, err = GenerateRandomString(12)
		if err != nil {
			return err
		}
	}
	if err = os.WriteFile(ironicPasswordFile, []byte(ironicPassword), 0600); err != nil {
		return err
	}

	d.IronicUsername = ironicUsername
	d.IronicPassword = ironicPassword

	return nil
}

// deployIronic configures the kustomize overlay for ironic
// based on the configuration, then install ironic with that overlay
func (d *DeployContext) deployIronic() error {
	var err error
	ironicDataDir, err := d.GetEnvOrDefault("IRONIC_DATA_DIR")
	if err != nil {
		return err
	}
	configMapFileName := "ironic_bmo_configmap.env"
	configMapInDataDir := filepath.Join(ironicDataDir, configMapFileName)

	if _, err := os.Stat(configMapInDataDir); err == nil && d.IronicBMOConfigMapEnvFile == "" {
		log.Printf("Detected ironic_bmo_configmap.env file in IRONIC_DATA_DIR: %s. Taken into use.", configMapInDataDir)
		d.IronicBMOConfigMapEnvFile = configMapInDataDir
	}

	if d.IronicOverlay == "" {
		var ironicOverlay, ironicKustomizeTpl string
		if d.BMOPath != "" {
			ironicOverlay = filepath.Join(d.BMOPath, "ironic-deployment", "overlays", "temp")
			ironicKustomizeTpl = "templates/ironic-kustomize-bmopath.tpl"
			err = EnsureCleanDirectory(ironicOverlay, 0755)
		} else {
			ironicOverlay, err = MakeRandomDirectory("/tmp/ironic-overlay-", 0755)
			ironicKustomizeTpl = "templates/ironic-kustomize.tpl"
		}

		if err != nil {
			return err
		}

		kustomizeFile := filepath.Join(ironicOverlay, "kustomization.yaml")

		if err := RenderEmbedTemplateToFile(d.TemplateFiles, ironicKustomizeTpl, kustomizeFile, d); err != nil {
			return err
		}

		ironicBMOConfigMapTpl := "templates/ironic_bmo_configmap_env.tpl"
		ironicBMOConfigMapOutput := filepath.Join(ironicOverlay, configMapFileName)

		if err := RenderEmbedTemplateToFile(d.TemplateFiles, ironicBMOConfigMapTpl, ironicBMOConfigMapOutput, d); err != nil {
			return err
		}

		d.IronicOverlay = ironicOverlay
	}

	ironicOverlay := d.IronicOverlay
	log.Printf("Installing ironic with kustomize overlay: %s", ironicOverlay)

	if d.IronicBMOConfigMapEnvFile != "" {
		log.Printf("Using custom ironic_bmo_configmap.env file: %s", d.IronicBMOConfigMapEnvFile)
		if err := CopyFile(d.IronicBMOConfigMapEnvFile, filepath.Join(ironicOverlay, "ironic_bmo_configmap.env")); err != nil {
			return err
		}
	}

	username, password := d.IronicUsername, d.IronicPassword

	if username != "" && password != "" {
		ironicHtpasswd, err := GenerateHtpasswd(username, password)
		if err != nil {
			return err
		}
		htpasswdPath := filepath.Join(d.IronicOverlay, "ironic-htpasswd")
		if err = os.WriteFile(htpasswdPath, []byte(ironicHtpasswd), 0600); err != nil {
			return err
		}
	}

	return BuildAndApplyKustomization(d.Context, d.KubeconfigPath, d.IronicOverlay)
}

// deployBMO generates the YAML for the Bare Metal Operator using Kustomize
// and applies it to the Kubernetes cluster.
func (d *DeployContext) deployBMO() error {
	var err error
	ironicDataDir, err := d.GetEnvOrDefault("IRONIC_DATA_DIR")
	if err != nil {
		return err
	}
	ironicEnvFileName := "ironic.env"
	ironicEnvInDataDir := filepath.Join(ironicDataDir, ironicEnvFileName)

	if _, err := os.Stat(ironicEnvInDataDir); err == nil && d.IronicEnvFile == "" {
		log.Printf("Detected ironic.env file in IRONIC_DATA_DIR: %s. Taken into use.", ironicEnvInDataDir)
		d.IronicEnvFile = ironicEnvInDataDir
	}

	if d.BMOOverlay == "" {
		var bmoOverlay, kustomizeTpl string
		if d.BMOPath != "" {
			bmoOverlay = filepath.Join(d.BMOPath, "config", "overlays", "temp")
			kustomizeTpl = "templates/bmo-kustomize-bmopath.tpl"
			err = EnsureCleanDirectory(bmoOverlay, 0755)
		} else {
			bmoOverlay, err = MakeRandomDirectory("/tmp/bmo-overlay-", 0755)
			kustomizeTpl = "templates/bmo-kustomize.tpl"
		}
		if err != nil {
			return err
		}
		kustomizeFile := filepath.Join(bmoOverlay, "kustomization.yaml")
		if err := RenderEmbedTemplateToFile(d.TemplateFiles, kustomizeTpl, kustomizeFile, d); err != nil {
			return err
		}
		ironicEnvTpl := "templates/ironic.env.tpl"
		ironicEnvFile := filepath.Join(bmoOverlay, "ironic.env")
		if err := RenderEmbedTemplateToFile(d.TemplateFiles, ironicEnvTpl, ironicEnvFile, d); err != nil {
			return err
		}
		d.BMOOverlay = bmoOverlay
	}

	log.Printf("Installing BMO with kustomize overlay: %s", d.BMOOverlay)
	bmoOverlay := d.BMOOverlay

	if d.IronicEnvFile != "" {
		log.Printf("Using custom ironic.env file: %s", d.IronicEnvFile)
		if err := CopyFile(d.IronicEnvFile, filepath.Join(bmoOverlay, "ironic.env")); err != nil {
			return err
		}
	}

	username, password := d.IronicUsername, d.IronicPassword
	if username != "" && password != "" {
		usernameFile := filepath.Join(bmoOverlay, "ironic-username")
		if err := os.WriteFile(usernameFile, []byte(username), 0600); err != nil {
			return err
		}
		passwordFile := filepath.Join(bmoOverlay, "ironic-password")
		if err := os.WriteFile(passwordFile, []byte(password), 0600); err != nil {
			return err
		}
	}

	return BuildAndApplyKustomization(d.Context, d.KubeconfigPath, d.BMOOverlay)
}

// cleanup removes temporary files created for basic auth credentials during deployment.
func (d *DeployContext) cleanup() error {
	tempFiles := []string{
		filepath.Join(d.BMOOverlay, "ironic-username"),
		filepath.Join(d.BMOOverlay, "ironic-password"),
		filepath.Join(d.IronicOverlay, "ironic-auth-config"),
		filepath.Join(d.IronicOverlay, "ironic-htpasswd"),
	}
	for _, tempFile := range tempFiles {
		if err := os.Remove(tempFile); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

// GetEnvOrDefault returns the value of the environment variable key if it exists
// and is non-empty. Otherwise it returns the provided value of the same key
// in the DeployContext DefaultMap
func (d *DeployContext) GetEnvOrDefault(key string) (string, error) {
	value, exists := os.LookupEnv(key)
	if exists && value != "" {
		log.Printf("[%s] Using value from environment variable", key)
		return value, nil
	}

	value, exists = (*d.DefaultMap)[key]
	if exists {
		return value, nil
	}

	return "", fmt.Errorf("value of %s not defined and has no default", key)
}

// CopyFile copies the file at src to dst. It opens both files,
// copies the contents from src to dst, and handles closing both files.
func CopyFile(src, dst string) error {
	inputFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("failed to open source file %s: %v", src, err)
	}
	defer inputFile.Close()

	outputFile, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("failed to create destination file %s: %v", dst, err)
	}
	defer outputFile.Close()

	_, err = io.Copy(outputFile, inputFile)
	if err != nil {
		return fmt.Errorf("failed to copy from %s to %s: %v", src, dst, err)
	}

	return nil
}

// GenerateHtpasswd generates a htpasswd entry for the given username and password.
func GenerateHtpasswd(username, password string) (string, error) {
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("%s:%s", username, string(hashedPassword)), nil
}

// GenerateRandomString generates random string of given length
// using crypto/rand and base64 encoding.
func GenerateRandomString(length int) (string, error) {
	b := make([]byte, length)
	_, err := rand.Read(b)
	if err != nil {
		return "", fmt.Errorf("failed to generate random string: %v", err)
	}

	return base64.RawURLEncoding.EncodeToString(b)[:length], nil
}

// getEnvOrFileContent checks for an environment variable;
// if the env var is not present, reads the content of a file and return
func getEnvOrFileContent(varName, filePath string) (string, error) {
	val, exists := os.LookupEnv(varName)
	if exists && val != "" {
		log.Printf("[%s] Using value from environment variable", varName)
		return val, nil
	}
	content, err := os.ReadFile(filePath)
	if err != nil {
		return "", err
	}

	return string(content), nil
}

// EnsureCleanDirectory makes sure that the specified directory is created
// with provided permission and is empty
func EnsureCleanDirectory(name string, perm os.FileMode) error {
	if err := os.RemoveAll(name); err != nil {
		return err
	}
	if err := os.MkdirAll(name, perm); err != nil {
		return err
	}
	return nil
}

// MakeRandomDirectory generates a new directory whose name starts with the
// provided prefix and ends with a random string
func MakeRandomDirectory(prefix string, perm os.FileMode) (string, error) {
	randomStr, err := GenerateRandomString(6)
	if err != nil {
		return "", err
	}
	randomDir := prefix + randomStr
	err = EnsureCleanDirectory(randomDir, perm)
	return randomDir, err
}

// RenderEmbedTemplateToFile reads in a go-template, renders it with supporting data
// and then write the result to an output file
func RenderEmbedTemplateToFile(templateFiles embed.FS, inputFile, outputFile string, data interface{}) error {
	tmpl, err := template.ParseFS(templateFiles, inputFile)
	if err != nil {
		return err
	}
	f, err := os.Create(outputFile)
	if err != nil {
		return err
	}
	defer f.Close()

	if err = tmpl.Execute(f, data); err != nil {
		return err
	}

	return nil
}

// GetKubeconfigPath returns the path to the kubeconfig file.
func GetKubeconfigPath() string {
	// Check KUBECTL_ARGS env var
	kubectlArgs, exists := os.LookupEnv("KUBECTL_ARGS")
	if exists {
		re := regexp.MustCompile(`--kubeconfig=([^\s]+)`)
		match := re.FindStringSubmatch(kubectlArgs)
		if len(match) > 1 {
			return match[1]
		}
	}
	kubeconfigPath := os.Getenv("KUBECONFIG")
	if kubeconfigPath == "" {
		kubeconfigPath = os.Getenv("HOME") + "/.kube/config"
	}
	return kubeconfigPath
}

// getBMOPath determines the abs path of the BMO repo, using
// `BMOPATH` env var; if it's not set or does not exist, detect
// BMO repo from the exe path of the script. If the script was built,
// return an empty string.
func getBMOPath() string {
	bmoPath, exists := os.LookupEnv("BMOPATH")
	if exists {
		log.Printf("BMOPATH env var exists. Using existing BMO repo at %s.", bmoPath)
		return bmoPath
	}
	exePath, err := os.Executable()
	if err != nil {
		return ""
	}
	if strings.Contains(exePath, "go-build") {
		repo := filepath.Join(exePath, "..", "..")
		bmoPath, err := filepath.Abs(repo)
		if err != nil {
			return ""
		}
		log.Printf("Detected script running with `go run`. Using BMO repo path determined from the script location: %s", bmoPath)
		return bmoPath
	}
	return ""
}

// NOTE: The following functions are almost identical to the same functions in BMO e2e
// They can be removed and imported from BMO E2E after the next BMO release
func BuildKustomizeManifest(source string) ([]byte, error) {
	kustomizer := krusty.MakeKustomizer(krusty.MakeDefaultOptions())
	fSys := filesys.MakeFsOnDisk()
	resources, err := kustomizer.Run(fSys, source)
	if err != nil {
		return nil, err
	}
	return resources.AsYaml()
}

// BuildAndApplyKustomization builds the provided kustomization
// and apply it to the cluster provided by clusterProxy.
func BuildAndApplyKustomization(ctx context.Context, kubeconfigPath string, kustomization string) error {
	var err error
	manifest, err := BuildKustomizeManifest(kustomization)
	if err != nil {
		return err
	}

	if err = testexec.KubectlApply(ctx, kubeconfigPath, manifest); err != nil {
		return err
	}
	return nil
}
