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
	"regexp"
	"strings"

	"embed"

	"golang.org/x/crypto/bcrypt"
	"net/http"
	"sigs.k8s.io/cluster-api/test/framework"
	testexec "sigs.k8s.io/cluster-api/test/framework/exec"
	"text/template"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/kustomize/api/krusty"
	"sigs.k8s.io/kustomize/kyaml/filesys"
)

// DeployContext defines the context of the deploy run
type DeployContext struct {
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
	// Endpoint for Ironic
	IronicHostIP string
	// Endpoint for Mariadb
	MariaDBHostIP string
	// Templates to render files using in deployments
	TemplateFiles embed.FS
}

// GetEnvOrDefault returns the value of the environment variable key if it exists
// and is non-empty. Otherwise it returns the provided default value.
func GetEnvOrDefault(key, defaultValue string) string {
	value, exists := os.LookupEnv(key)
	if exists && value != "" {
		return value
	}

	return defaultValue
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

// DownloadFile downloads a file and stores its content to a specified location on disk
func DownloadFile(url string, filepath string) error {
	// Create the file
	out, err := os.Create(filepath)
	if err != nil {
		return err
	}
	defer out.Close()

	// Get the data
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Write the body to file
	_, err = io.Copy(out, resp.Body)
	return err
}

// EnsureFileExists checks if a file exists at path, creating it with random content if it doesn't.
func EnsureFileExists(varName, path string, length int) (bool, error) {
	_, err := os.Stat(path)
	if os.IsNotExist(err) {
		generatedString, err := GenerateRandomString(length)
		if err != nil {
			return false, err
		}

		err = os.WriteFile(path, []byte(generatedString), 0600)
		if err != nil {
			return false, err
		}
		log.Printf("[%s] Created new file with random content at: %s", varName, path)
		return true, nil
	}

	return false, nil
}

// ReadFileContent reads and returns the content of the file at path.
func ReadFileContent(varName, path string) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("[%s] failed to read file %s: %v", varName, path, err)
	}

	return string(content), nil
}

// getEnvOrFileContent checks for an environment variable; if not present, ensures a file exists and reads it.
func getEnvOrFileContent(varName, filePath string, length int) (string, error) {
	val, exists := os.LookupEnv(varName)
	if exists && val != "" {
		log.Printf("[%s] Using value from environment variable", varName)
		return val, nil
	}

	newlyCreated, err := EnsureFileExists(varName, filePath, length)
	if err != nil {
		return "", err
	}

	if !newlyCreated {
		log.Printf("[%s] Reading content from existing file: %s", varName, filePath)
	}

	content, err := ReadFileContent(varName, filePath)
	if err != nil {
		return "", err
	}

	return content, nil
}

// BuildKustomizeManifest builds a provided kustomize overlays to output, same as `kustomize build`
func BuildKustomizeManifest(source string) ([]byte, error) {
	kustomizer := krusty.MakeKustomizer(krusty.MakeDefaultOptions())
	fSys := filesys.MakeFsOnDisk()
	resources, err := kustomizer.Run(fSys, source)
	if err != nil {
		return nil, err
	}
	return resources.AsYaml()
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

// deployIronic generates the YAML for Ironic using Kustomize and applies it to the Kubernetes cluster.
func deployIronic(data *DeployContext, tempIronicOverlay string) error {
	if data.IronicHostIP == "" {
		return fmt.Errorf("failed to determine IRONIC_HOST_IP")
	}
	ironicKustomizeTpl := "templates/ironic-kustomize.tpl"
	kustomizeFile := filepath.Join(tempIronicOverlay, "kustomization.yaml")

	if err := RenderEmbedTemplateToFile(data.TemplateFiles, ironicKustomizeTpl, kustomizeFile, data); err != nil {
		return err
	}

	ironicBMOConfigMapTpl := "templates/ironic_bmo_configmap_env.tpl"
	ironicBMOConfigMapOutput := filepath.Join(tempIronicOverlay, "ironic_bmo_configmap.env")

	if err := RenderEmbedTemplateToFile(data.TemplateFiles, ironicBMOConfigMapTpl, ironicBMOConfigMapOutput, data); err != nil {
		return err
	}

	return deployWithKustomizeAndApply(tempIronicOverlay)
}

// deployBMO generates the YAML for the Bare Metal Operator using Kustomize
// and applies it to the Kubernetes cluster.
func deployBMO(data *DeployContext, tempBMOOverlay string) error {

	inputFile := "templates/bmo-kustomize.tpl"
	kustomizeFile := filepath.Join(tempBMOOverlay, "kustomization.yaml")
	if err := RenderEmbedTemplateToFile(data.TemplateFiles, inputFile, kustomizeFile, data); err != nil {
		return err
	}
	ironicEnvSrc := "https://raw.githubusercontent.com/metal3-io/baremetal-operator/main/config/default/ironic.env"
	ironicEnvDst := filepath.Join(tempBMOOverlay, "ironic.env")
	if err := DownloadFile(ironicEnvSrc, ironicEnvDst); err != nil {
		return err
	}

	return deployWithKustomizeAndApply(tempBMOOverlay)
}

// getKubeconfigPath determines the kubeconfig path from KUBECTL_ARGS, KUBECONFIG_PATH, or defaults to ~/.kube/config.
func getKubeconfigPath() (string, error) {
	kubectlArgs := os.Getenv("KUBECTL_ARGS")
	kubeconfigPrefix := "--kubeconfig="
	regexPattern := regexp.MustCompile(`^--kubeconfig=[\w/.-]+$`)
	var kubeconfigPath string

	if strings.Contains(kubectlArgs, kubeconfigPrefix) && regexPattern.MatchString(kubectlArgs) {
		kubeconfigPath = strings.TrimPrefix(kubectlArgs, kubeconfigPrefix)
	} else if kubectlArgs != "" {
		return "", fmt.Errorf("error: invalid format in KUBECTL_ARGS. Expected format: '--kubeconfig=/path/to/kubeconfig'")
	}

	if kubeconfigPath == "" {
		kubeconfigPath = os.Getenv("KUBECONFIG_PATH")
		if kubeconfigPath == "" {
			homeDir, err := os.UserHomeDir()
			if err != nil {
				return "", fmt.Errorf("failed to get user home directory: %v", err)
			}
			kubeconfigPath = filepath.Join(homeDir, ".kube", "config")
		}
	}

	// Verify the file exists and is readable
	_, err := os.Stat(kubeconfigPath)
	if os.IsNotExist(err) || err != nil {
		return "", fmt.Errorf("specified kubeconfig file does not exist or is not readable: %s", kubeconfigPath)
	}

	return kubeconfigPath, nil
}

// generateClusterProxy creates a new ClusterProxy instance using the provided kubeconfig path.
func generateClusterProxy(kubeconfigPath string) (*framework.ClusterProxy, error) {
	scheme := runtime.NewScheme()
	framework.TryAddDefaultSchemes(scheme)
	clusterProxy := framework.NewClusterProxy("deploy-cli", kubeconfigPath, scheme)
	if clusterProxy == nil {
		return nil, fmt.Errorf("failed to create cluster proxy")
	}
	return &clusterProxy, nil
}

// deployWithKustomizeAndApply first generates the YAML configuration by running Kustomize build on the overlay directory,
// then outputs the generated YAML to a temp file within the overlay dir, and then applies
// the configuration to the Kubernetes cluster specified in the kubeconfig file.
func deployWithKustomizeAndApply(overlayPath string) error {
	yamlOutput, err := BuildKustomizeManifest(overlayPath)

	if err != nil {
		return fmt.Errorf("failed to apply YAML: %v", err)
	}

	kubeconfigPath, err := getKubeconfigPath()
	if err != nil {
		return fmt.Errorf("failed to apply YAML: %v", err)
	}

	ctx := context.Background()
	if err := testexec.KubectlApply(ctx, kubeconfigPath, yamlOutput); err != nil {
		return fmt.Errorf("failed to apply YAML: %v", err)
	}

	return nil
}

// cleanup removes temporary files created for basic auth credentials during deployment.
func cleanup(deployBasicAuthFlag, deployBMOFlag, deployIronicFlag bool, tempBMOOverlay, tempIronicOverlay string) {
	if deployBasicAuthFlag {
		if deployBMOFlag {
			os.Remove(filepath.Join(tempBMOOverlay, "ironic-username"))
			os.Remove(filepath.Join(tempBMOOverlay, "ironic-password"))
			os.Remove(filepath.Join(tempBMOOverlay, "ironic-inspector-username"))
			os.Remove(filepath.Join(tempBMOOverlay, "ironic-inspector-password"))
		}

		if deployIronicFlag {
			os.Remove(filepath.Join(tempIronicOverlay, "ironic-auth-config"))
			os.Remove(filepath.Join(tempIronicOverlay, "ironic-inspector-auth-config"))
			os.Remove(filepath.Join(tempIronicOverlay, "ironic-htpasswd"))
			os.Remove(filepath.Join(tempIronicOverlay, "ironic-inspector-htpasswd"))
		}
	}
}

func usage() {
	fmt.Println(`Usage : deploy [options]
Options:
	-h:	show this help message
	-b:	deploy BMO
	-i:	deploy Ironic
	-t:	deploy with TLS enabled
	-n:	deploy without authentication
	-k:	deploy with keepalived
	-m:	deploy with mariadb (requires TLS enabled)`)
}
