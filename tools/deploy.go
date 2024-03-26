package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/clientcmd"
)

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
	cmd := exec.Command("htpasswd", "-n", "-b", "-B", username, password)
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(output)), nil
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

// readOrCreateFile reads the content of a file, or creates it with random content if it doesn't exist.
func readOrCreateFile(path string, length int) (string, error) {
	content, err := os.ReadFile(path)
	if err == nil {
		return string(content), nil
	}

	generatedString, err := GenerateRandomString(length)
	if err != nil {
		return "", err
	}
	err = os.WriteFile(path, []byte(generatedString), 0600)
	if err != nil {
		return "", fmt.Errorf("failed to write file %s: %v", path, err)
	}

	return generatedString, nil
}

// getEnvOrFile reads an environment variable; if not present, reads from or creates a file.
func getEnvOrFile(envName, filePath string, length int) (string, error) {
	val, exists := os.LookupEnv(envName)
	if exists && val != "" {
		return val, nil
	}

	return readOrCreateFile(filePath, length)
}

// execCommand execute commands and capture their output.
func execCommand(command string, args ...string) {
	cmd := exec.Command(command, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		log.Fatalf("Failed to execute command: %v, stdout: %s, stderr: %s", err, stdout.String(), stderr.String())
	}
}

// substituteAndWriteEnvVars reads the template, substitutes environment variable placeholders
// and writes the result to the destination file.
func substituteAndWriteEnvVars(templateFilePath, destFilePath string) error {
	templateContent, err := os.ReadFile(templateFilePath)
	if err != nil {
		return fmt.Errorf("failed to read template file: %w", err)
	}

	substitutedContent := os.ExpandEnv(string(templateContent))

	err = os.WriteFile(destFilePath, []byte(substitutedContent), 0600)
	if err != nil {
		return fmt.Errorf("failed to write substituted content to file: %w", err)
	}

	return nil
}

// changeDirTemporarily changes the current working directory and returns a function to revert to the original directory.
func changeDirTemporarily(newDir string) (revertFunc func(), err error) {
	originalDir, err := os.Getwd()
	if err != nil {
		return nil, err
	}

	err = os.Chdir(newDir)
	if err != nil {
		return nil, err
	}

	return func() {
		err := os.Chdir(originalDir)
		if err != nil {
			log.Fatalf("Failed to revert to original directory: %v", err)
		}
	}, nil
}

// setupIronicDeployment runs kustomize commands to generate a Kubernetes manifest for deploying Ironic.
// It allows enabling different components like basic auth, TLS, keepalived and mariadb via flags.
func setupIronicDeployment(deployBasicAuthFlag, deployTLSFlag, deployKeepalivedFlag, deployMariadbFlag bool, tempIronicOverlay, kustomizePath string) {
	revertFunc, err := changeDirTemporarily(tempIronicOverlay)
	if err != nil {
		log.Fatalf("Failed to change directory %s: %v", tempIronicOverlay, err)
	}
	defer revertFunc()

	execCommand(kustomizePath, "create", "--resources=../../../config/namespace", "--namespace=baremetal-operator-system", "--nameprefix=baremetal-operator-")

	if deployBasicAuthFlag {
		execCommand(kustomizePath, "edit", "add", "secret", "ironic-htpasswd", "--from-env-file=ironic-htpasswd")
		execCommand(kustomizePath, "edit", "add", "secret", "ironic-auth-config", "--from-file=auth-config=ironic-auth-config")

		if deployTLSFlag {
			// Basic-auth + TLS is special since TLS also means reverse proxy, which affects basic-auth.
			// Therefore we have an overlay that we use as base for this case.
			execCommand(kustomizePath, "edit", "add", "resource", "../../overlays/basic-auth_tls")
		} else {
			execCommand(kustomizePath, "edit", "add", "resource", "../../base")
			execCommand(kustomizePath, "edit", "add", "component", "../../components/basic-auth")
		}
	} else if deployTLSFlag {
		execCommand(kustomizePath, "edit", "add", "component", "../../components/tls")
	}

	if deployKeepalivedFlag {
		execCommand(kustomizePath, "edit", "add", "component", "../../components/keepalived")
	}

	if deployMariadbFlag {
		execCommand(kustomizePath, "edit", "add", "component", "../../components/mariadb")
	}
}

// setupBMODeployment configures a Baremetal Operator deployment by running
// kustomize commands on the provided overlay directory. It enables
// basic auth and TLS if the respective flags are set.
func setupBMODeployment(deployBasicAuthFlag, deployTLSFlag bool, tempBMOOverlay, kustomizePath string) {
	revertFunc, err := changeDirTemporarily(tempBMOOverlay)
	if err != nil {
		log.Fatalf("Failed to change directory %s: %v", tempBMOOverlay, err)
	}
	defer revertFunc()

	execCommand(kustomizePath, "create", "--resources=../../base,../../namespace", "--namespace=baremetal-operator-system")

	if deployBasicAuthFlag {
		execCommand(kustomizePath, "edit", "add", "component", "../../components/basic-auth")
		execCommand(kustomizePath, "edit", "add", "secret", "ironic-credentials", "--from-file=username=ironic-username", "--from-file=password=ironic-password")
	}

	if deployTLSFlag {
		execCommand(kustomizePath, "edit", "add", "component", "../../components/tls")
	}
}

// updateOrAppendKeyValueInFile updates value for given key in file.
// If key exists, updates value. If not, appends to end of file.
func updateOrAppendKeyValueInFile(filePath, key, value string) error {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read file %s: %v", filePath, err)
	}

	lines := strings.Split(string(content), "\n")
	updatedLines := []string{}
	found := false

	for _, line := range lines {
		if strings.HasPrefix(line, key+"=") {
			updatedLines = append(updatedLines, fmt.Sprintf("%s=%s", key, value))
			found = true
		} else {
			updatedLines = append(updatedLines, line)
		}
	}

	if !found {
		updatedLines = append(updatedLines, fmt.Sprintf("%s=%s", key, value))
	}

	err = os.WriteFile(filePath, []byte(strings.Join(updatedLines, "\n")), 0600)
	if err != nil {
		return fmt.Errorf("failed to write updated lines to file %s: %v", filePath, err)
	}

	return nil
}

// Replaces placeholders with a new value in the file.
// Splits on the placeholder to get parts, joins with the new value,
// and writes updated content back to the file.
func replaceAllPlaceholdersInFile(filePath, placeholder, newValue string) error {
	input, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read file %s: %v", filePath, err)
	}

	content := string(input)

	parts := strings.Split(content, placeholder)
	updatedContent := strings.Join(parts, newValue)

	err = os.WriteFile(filePath, []byte(updatedContent), 0600)
	if err != nil {
		return fmt.Errorf("failed to write to file %s: %v", filePath, err)
	}

	return nil
}

// splitYAMLDocuments splits the provided YAML content into individual
// documents. It uses a yaml.Decoder to parse the content and extract each document.
func splitYAMLDocuments(yamlContent []byte) ([]*yaml.Node, error) {
	var docs []*yaml.Node

	decoder := yaml.NewDecoder(bytes.NewReader(yamlContent))
	for {
		var doc yaml.Node
		err := decoder.Decode(&doc)
		if err == io.EOF {
			break
		} else if err != nil {
			return nil, err
		}
		docs = append(docs, &doc)
	}

	return docs, nil
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

// applyYAML processes and applies a YAML file to the Kubernetes cluster. It reads the specified YAML file,
// splits it into individual resource definitions (if the file contains multiple documents), and then creates
// or updates each resource in the cluster accordingly. This function handles the initialization of Kubernetes
// client configurations, dynamic client, and discovery client to interact with the cluster's API.
// It leverages a RESTMapper to resolve GroupVersionKind (GVK) to resources and manage them dynamically.
func applyYAML(yamlFilePath string) error {
	kubeconfigPath, err := getKubeconfigPath()
	if err != nil {
		return fmt.Errorf("failed to get kubeconfig: %w", err)
	}

	config, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return fmt.Errorf("failed to build kubeconfig: %w", err)
	}

	dynClient, err := dynamic.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("failed to create dynamic client: %w", err)
	}

	discoveryClient, err := discovery.NewDiscoveryClientForConfig(config)
	if err != nil {
		return fmt.Errorf("failed to create discovery client: %w", err)
	}

	grp, err := restmapper.GetAPIGroupResources(discoveryClient)
	if err != nil {
		return fmt.Errorf("failed to get API Group Resources: %w", err)
	}
	mapper := restmapper.NewDiscoveryRESTMapper(grp)

	yamlContent, err := os.ReadFile(filepath.Clean(yamlFilePath))
	if err != nil {
		return fmt.Errorf("failed to read YAML file: %w", err)
	}

	documents, err := splitYAMLDocuments(yamlContent)
	if err != nil {
		return fmt.Errorf("failed to split YAML file: %w", err)
	}

	for _, doc := range documents {
		docBytes, err := yaml.Marshal(doc)
		if err != nil {
			return fmt.Errorf("failed to marshal YAML document: %w", err)
		}

		if strings.TrimSpace(string(docBytes)) == "" {
			continue
		}

		obj := &unstructured.Unstructured{}
		decUnstructured := scheme.Codecs.UniversalDeserializer().Decode
		_, gvk, err := decUnstructured(docBytes, nil, obj)
		if err != nil {
			return fmt.Errorf("error decoding YAML document: %w", err)
		}

		mapping, err := mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
		if err != nil {
			return fmt.Errorf("error finding mapping for GVK %s: %w", gvk.String(), err)
		}

		resourceClient := dynClient.Resource(mapping.Resource).Namespace(obj.GetNamespace())
		err = applyResource(resourceClient, obj)
		if err != nil {
			return err
		}
	}

	return nil
}

// applyResource creates or updates a given resource.
func applyResource(resourceClient dynamic.ResourceInterface, obj *unstructured.Unstructured) error {
	ctx := context.Background()
	name := obj.GetName()

	existingObj, err := resourceClient.Get(ctx, name, metav1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		fmt.Printf("Creating %s %q\n", obj.GetKind(), name)
		_, err := resourceClient.Create(ctx, obj, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("error creating %s %q: %w", obj.GetKind(), name, err)
		}
	} else if err == nil {
		fmt.Printf("Updating %s %q\n", obj.GetKind(), name)
		obj.SetResourceVersion(existingObj.GetResourceVersion())
		_, err := resourceClient.Update(ctx, obj, metav1.UpdateOptions{})
		if err != nil {
			return fmt.Errorf("error updating %s %q: %w", obj.GetKind(), name, err)
		}
	} else {
		return fmt.Errorf("error getting %s %q: %w", obj.GetKind(), name, err)
	}

	return nil
}

// execCommandToFile executes the given command and writes its combined stdout
// and stderr output to the provided file path.
func execCommandToFile(cmd *exec.Cmd, outputPath string) error {
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("command execution failed: %w", err)
	}

	err = os.WriteFile(outputPath, output, 0600)
	if err != nil {
		return fmt.Errorf("writing command output to file failed: %w", err)
	}

	return nil
}

// deployWithKustomizeAndApply first generates the YAML configuration by running Kustomize build on the overlay directory,
// then outputs the generated YAML to a temp file within the overlay dir, and then applies
// the configuration to the Kubernetes cluster specified in the kubeconfig file.
func deployWithKustomizeAndApply(kustomizePath, overlayPath string) error {
	yamlOutputPath := filepath.Join(overlayPath, "output.yaml")

	defer func() {
		if removeErr := os.Remove(yamlOutputPath); removeErr != nil {
			log.Printf("Warning: Failed to remove temporary YAML file %s: %v", yamlOutputPath, removeErr)
		}
	}()

	// Generate YAML with kustomize
	cmd := exec.Command(kustomizePath, "build", overlayPath)
	err := execCommandToFile(cmd, yamlOutputPath)
	if err != nil {
		return fmt.Errorf("failed to generate YAML with kustomize: %v", err)
	}

	err = applyYAML(yamlOutputPath)
	if err != nil {
		return fmt.Errorf("failed to apply YAML: %v", err)
	}

	return nil
}

// copyFile copies the file at src to dst. It opens both files,
// copies the contents from src to dst, and handles closing both files.
func copyFile(src, dst string) error {
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

// deployBMO generates the YAML for the Bare Metal Operator using Kustomize
// and applies it to the Kubernetes cluster.
func deployBMO(tempBMOOverlay, scriptDir, kustomizePath string) error {
	revertFunc, err := changeDirTemporarily(tempBMOOverlay)
	if err != nil {
		log.Fatalf("Failed to change directory %s: %v", tempBMOOverlay, err)
	}
	defer revertFunc()

	// This is to keep the current behavior of using the ironic.env file for the configmap
	ironicEnvSrc := filepath.Join(scriptDir, "config", "default", "ironic.env")
	ironicEnvDst := filepath.Join(tempBMOOverlay, "ironic.env")
	copyFile(ironicEnvSrc, ironicEnvDst)

	execCommand(kustomizePath, "edit", "add", "configmap", "ironic", "--behavior=create", "--from-env-file=ironic.env")

	err = deployWithKustomizeAndApply(kustomizePath, tempBMOOverlay)
	if err != nil {
		return err
	}

	return nil
}

// deployIronic generates the YAML for Ironic using Kustomize and applies it to the Kubernetes cluster.
func deployIronic(tempIronicOverlay, scriptDir, kustomizePath string, deployKeepalivedFlag bool) error {
	revertFunc, err := changeDirTemporarily(tempIronicOverlay)
	if err != nil {
		log.Fatalf("Failed to change directory %s: %v", tempIronicOverlay, err)
	}
	defer revertFunc()

	// Copy the configmap content from either the keepalived or default kustomization
	// and edit based on environment.
	var ironicBMOConfigmapSource string
	if deployKeepalivedFlag {
		ironicBMOConfigmapSource = filepath.Join(scriptDir, "ironic-deployment", "components", "keepalived", "ironic_bmo_configmap.env")
	} else {
		ironicBMOConfigmapSource = filepath.Join(scriptDir, "ironic-deployment", "default", "ironic_bmo_configmap.env")
	}
	ironicBMOConfigmap := filepath.Join(tempIronicOverlay, "ironic_bmo_configmap.env")
	copyFile(ironicBMOConfigmapSource, ironicBMOConfigmap)

	restartContainerCertificateUpdated := GetEnvOrDefault("RESTART_CONTAINER_CERTIFICATE_UPDATED", "false")
	err = updateOrAppendKeyValueInFile(ironicBMOConfigmap, "RESTART_CONTAINER_CERTIFICATE_UPDATED", restartContainerCertificateUpdated)
	if err != nil {
		return err
	}

	ironicHostIP, exists := os.LookupEnv("IRONIC_HOST_IP")
	if !exists {
		return fmt.Errorf("failed to determine IRONIC_HOST_IP")
	}
	ironicCertPath := filepath.Join(scriptDir, "ironic-deployment", "components", "tls", "certificate.yaml")
	err = replaceAllPlaceholdersInFile(ironicCertPath, "IRONIC_HOST_IP", ironicHostIP)
	if err != nil {
		return err
	}

	mariaDBHostIP := GetEnvOrDefault("MARIADB_HOST_IP", "127.0.0.1")
	mariaDBCertPath := filepath.Join(scriptDir, "ironic-deployment", "components", "mariadb", "certificate.yaml")
	err = replaceAllPlaceholdersInFile(mariaDBCertPath, "MARIADB_HOST_IP", mariaDBHostIP)
	if err != nil {
		return err
	}

	// The keepalived component has its own configmap,
	// but we are overriding depending on environment here so we must replace it.
	if deployKeepalivedFlag {
		execCommand(kustomizePath, "edit", "add", "configmap", "ironic-bmo-configmap", "--behavior=replace", "--from-env-file=ironic_bmo_configmap.env")
	} else {
		execCommand(kustomizePath, "edit", "add", "configmap", "ironic-bmo-configmap", "--behavior=create", "--from-env-file=ironic_bmo_configmap.env")
	}

	err = deployWithKustomizeAndApply(kustomizePath, tempIronicOverlay)
	if err != nil {
		return err
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

var (
	deployBMOFlag         bool
	deployIronicFlag      bool
	deployTLSFlag         bool
	deployWithoutAuthFlag bool
	deployKeepalivedFlag  bool
	deployMariadbFlag     bool
	showHelpFlag          bool
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

	if showHelpFlag {
		usage()
		return
	}

	if !deployBasicAuthFlag {
		fmt.Println("WARNING: Deploying without authentication is not recommended")
	}

	if deployMariadbFlag && !deployTLSFlag {
		fmt.Println("ERROR: Deploying Ironic with MariaDB without TLS is not supported.")
		usage()
		os.Exit(1)
	}

	if !deployBMOFlag && !deployIronicFlag {
		fmt.Println("ERROR: At least one of -b (BMO) or -i (Ironic) must be specified for deployment.")
		usage()
		os.Exit(1)
	}

	exePath, err := os.Executable()
	if err != nil {
		log.Fatalf("Failed to determine executable path: %v", err)
	}
	scriptDir := filepath.Join(filepath.Dir(exePath), "..")

	ironicBasicAuthComponent := filepath.Join(scriptDir, "ironic-deployment", "components", "basic-auth")
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

	kustomizePath := filepath.Join(scriptDir, "tools", "bin", "kustomize")
	execCommand("make", "-C", scriptDir, kustomizePath)

	// Generate credentials as needed
	ironicDataDir := GetEnvOrDefault("IRONIC_DATA_DIR", "/opt/metal3/ironic/")
	ironicAuthDir := filepath.Join(ironicDataDir, "auth")

	// If usernames and passwords are unset, read them from file or generate them
	if deployBasicAuthFlag {
		ironicUsername, err := getEnvOrFile("IRONIC_USERNAME", filepath.Join(ironicAuthDir, "ironic-username"), 12)
		if err != nil {
			log.Fatalf("Error retrieving Ironic username: %v", err)
		}
		ironicPassword, err := getEnvOrFile("IRONIC_PASSWORD", filepath.Join(ironicAuthDir, "ironic-password"), 12)
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
			err := substituteAndWriteEnvVars(
				filepath.Join(ironicBasicAuthComponent, "ironic-auth-config-tpl"),
				filepath.Join(tempIronicOverlay, "ironic-auth-config"),
			)
			if err != nil {
				log.Fatalf("Failed to process ironic-auth-config template: %v", err)
			}

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

	if deployBMOFlag {
		setupBMODeployment(deployBasicAuthFlag, deployTLSFlag, tempBMOOverlay, kustomizePath)

		err = deployBMO(tempBMOOverlay, scriptDir, kustomizePath)
		if err != nil {
			log.Fatalf("Failed to deploy BMO: %v", err)
		}
	}

	if deployIronicFlag {
		setupIronicDeployment(deployBasicAuthFlag, deployTLSFlag, deployKeepalivedFlag, deployMariadbFlag, tempIronicOverlay, kustomizePath)

		err = deployIronic(tempIronicOverlay, scriptDir, kustomizePath, deployKeepalivedFlag)
		if err != nil {
			log.Fatalf("Failed to deploy Ironic: %v", err)
		}
	}

	cleanup(deployBasicAuthFlag, deployBMOFlag, deployIronicFlag, tempBMOOverlay, tempIronicOverlay)
}
