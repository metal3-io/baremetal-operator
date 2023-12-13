package e2e

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"

	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"

	capm3_e2e "github.com/metal3-io/cluster-api-provider-metal3/test/e2e"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/cluster-api/test/framework"

	"sigs.k8s.io/cluster-api/test/framework/clusterctl"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/kustomize/api/krusty"
	"sigs.k8s.io/kustomize/kyaml/filesys"
)

// LoadImageBehavior indicates the behavior when loading an image.
type LoadImageBehavior string

const (
	// MustLoadImage causes a load operation to fail if the image cannot be
	// loaded.
	MustLoadImage LoadImageBehavior = "mustLoad"

	// TryLoadImage causes any errors that occur when loading an image to be
	// ignored.
	TryLoadImage LoadImageBehavior = "tryLoad"
)

type PowerState string

const (
	PoweredOn  PowerState = "on"
	PoweredOff PowerState = "off"
)

// Config defines the configuration of an e2e test environment.
type Config struct {
	// Images is a list of container images to load into the Kind cluster.
	// Note that this not relevant when using an existing cluster.
	Images []clusterctl.ContainerImage `json:"images,omitempty"`

	// Variables to be used in the tests.
	Variables map[string]string `json:"variables,omitempty"`

	// Intervals to be used for long operations during tests.
	Intervals map[string][]string `json:"intervals,omitempty"`
}

// LoadE2EConfig loads the configuration for the e2e test environment.
func LoadE2EConfig(configPath string) *Config {
	configData, err := os.ReadFile(configPath) //#nosec
	Expect(err).ToNot(HaveOccurred(), "Failed to read the e2e test config file")
	Expect(configData).ToNot(BeEmpty(), "The e2e test config file should not be empty")

	config := &Config{}
	Expect(yaml.Unmarshal(configData, config)).To(Succeed(), "Failed to parse the e2e test config file")

	config.Defaults()
	Expect(config.Validate()).To(Succeed(), "The e2e test config file is not valid")

	return config
}

// Defaults assigns default values to the object. More specifically:
// - Images gets LoadBehavior = MustLoadImage if not otherwise specified.
func (c *Config) Defaults() {
	imageReplacer := strings.NewReplacer("{OS}", runtime.GOOS, "{ARCH}", runtime.GOARCH)
	for i := range c.Images {
		containerImage := &c.Images[i]
		containerImage.Name = imageReplacer.Replace(containerImage.Name)
		if containerImage.LoadBehavior == "" {
			containerImage.LoadBehavior = clusterctl.MustLoadImage
		}
	}
}

// Validate validates the configuration. More specifically:
// - Image should have name and loadBehavior be one of [mustload, tryload].
// - Intervals should be valid ginkgo intervals.
func (c *Config) Validate() error {

	// Image should have name and loadBehavior be one of [mustload, tryload].
	for i, containerImage := range c.Images {
		if containerImage.Name == "" {
			return errors.Errorf("Container image is missing name: Images[%d].Name=%q", i, containerImage.Name)
		}
		switch containerImage.LoadBehavior {
		case clusterctl.MustLoadImage, clusterctl.TryLoadImage:
			// Valid
		default:
			return errors.Errorf("Invalid load behavior: Images[%d].LoadBehavior=%q", i, containerImage.LoadBehavior)
		}
	}

	// Intervals should be valid ginkgo intervals.
	for k, intervals := range c.Intervals {
		switch len(intervals) {
		case 0:
			return errors.Errorf("Invalid interval: Intervals[%s]=%q", k, intervals)
		case 1, 2:
		default:
			return errors.Errorf("Invalid interval: Intervals[%s]=%q", k, intervals)
		}
		for _, i := range intervals {
			if _, err := time.ParseDuration(i); err != nil {
				return errors.Errorf("Invalid interval: Intervals[%s]=%q", k, intervals)
			}
		}
	}
	return nil
}

// GetIntervals returns the intervals to be applied to a Eventually operation.
// It searches for [spec]/[key] intervals first, and if it is not found, it searches
// for default/[key]. If also the default/[key] intervals are not found,
// ginkgo DefaultEventuallyTimeout and DefaultEventuallyPollingInterval are used.
func (c *Config) GetIntervals(spec, key string) []interface{} {
	intervals, ok := c.Intervals[fmt.Sprintf("%s/%s", spec, key)]
	if !ok {
		if intervals, ok = c.Intervals[fmt.Sprintf("default/%s", key)]; !ok {
			return nil
		}
	}
	intervalsInterfaces := make([]interface{}, len(intervals))
	for i := range intervals {
		intervalsInterfaces[i] = intervals[i]
	}
	return intervalsInterfaces
}

// GetVariable returns a variable from environment variables or from the e2e config file.
func (c *Config) GetVariable(varName string) string {
	if value, ok := os.LookupEnv(varName); ok {
		return value
	}

	value, ok := c.Variables[varName]
	Expect(ok).To(BeTrue(), fmt.Sprintf("Configuration variable '%s' not found", varName))
	return value
}

func isUndesiredState(currentState metal3api.ProvisioningState, undesiredStates []metal3api.ProvisioningState) bool {
	if undesiredStates == nil {
		return false
	}

	for _, state := range undesiredStates {
		if (state == "" && currentState == "") || currentState == state {
			return true
		}
	}
	return false
}

type WaitForBmhInProvisioningStateInput struct {
	Client          client.Client
	Bmh             metal3api.BareMetalHost
	State           metal3api.ProvisioningState
	UndesiredStates []metal3api.ProvisioningState
}

func WaitForBmhInProvisioningState(ctx context.Context, input WaitForBmhInProvisioningStateInput, intervals ...interface{}) {
	Eventually(func(g Gomega) {
		bmh := metal3api.BareMetalHost{}
		key := types.NamespacedName{Namespace: input.Bmh.Namespace, Name: input.Bmh.Name}
		g.Expect(input.Client.Get(ctx, key, &bmh)).To(Succeed())

		currentStatus := bmh.Status.Provisioning.State

		// Check if the current state matches any of the undesired states
		if isUndesiredState(currentStatus, input.UndesiredStates) {
			StopTrying(fmt.Sprintf("BMH is in an unexpected state: %s", currentStatus)).Now()
		}

		g.Expect(currentStatus).To(Equal(input.State))
	}, intervals...).Should(Succeed())
}

// WaitForBmhDeletedInput is the input for WaitForBmhDeleted.
type WaitForBmhDeletedInput struct {
	Client          client.Client
	BmhName         string
	Namespace       string
	UndesiredStates []metal3api.ProvisioningState
}

// WaitForBmhDeleted waits until the BMH object has been deleted.
func WaitForBmhDeleted(ctx context.Context, input WaitForBmhDeletedInput, intervals ...interface{}) {
	Eventually(func(g Gomega) bool {
		bmh := &metal3api.BareMetalHost{}
		key := types.NamespacedName{Namespace: input.Namespace, Name: input.BmhName}
		err := input.Client.Get(ctx, key, bmh)

		// If BMH is not found, it's considered deleted, which is the desired outcome.
		if apierrors.IsNotFound(err) {
			return true
		}
		g.Expect(err).NotTo(HaveOccurred())

		currentStatus := bmh.Status.Provisioning.State

		// If the BMH is found, check for undesired states.
		if isUndesiredState(currentStatus, input.UndesiredStates) {
			StopTrying(fmt.Sprintf("BMH is in an unexpected state: %s", currentStatus)).Now()
		}

		return false
	}, intervals...).Should(BeTrue(), fmt.Sprintf("BMH %s in namespace %s should be deleted", input.BmhName, input.Namespace))
}

// WaitForNamespaceDeletedInput is the input for WaitForNamespaceDeleted.
type WaitForNamespaceDeletedInput struct {
	Getter    framework.Getter
	Namespace corev1.Namespace
}

// WaitForNamespaceDeleted waits until the namespace object has been deleted.
func WaitForNamespaceDeleted(ctx context.Context, input WaitForNamespaceDeletedInput, intervals ...interface{}) {
	Eventually(func() bool {
		namespace := &corev1.Namespace{}
		key := client.ObjectKey{
			Name: input.Namespace.Name,
		}
		return apierrors.IsNotFound(input.Getter.Get(ctx, key, namespace))
	}, intervals...).Should(BeTrue())
}

func cleanup(ctx context.Context, clusterProxy framework.ClusterProxy, namespace *corev1.Namespace, cancelWatches context.CancelFunc, intervals ...interface{}) {
	framework.DeleteNamespace(ctx, framework.DeleteNamespaceInput{
		Deleter: clusterProxy.GetClient(),
		Name:    namespace.Name,
	})
	WaitForNamespaceDeleted(ctx, WaitForNamespaceDeletedInput{
		Getter:    clusterProxy.GetClient(),
		Namespace: *namespace,
	}, intervals...)
	cancelWatches()
}

type WaitForBmhInPowerStateInput struct {
	Client client.Client
	Bmh    metal3api.BareMetalHost
	State  PowerState
}

func WaitForBmhInPowerState(ctx context.Context, input WaitForBmhInPowerStateInput, intervals ...interface{}) {
	Eventually(func(g Gomega) {
		bmh := metal3api.BareMetalHost{}
		key := types.NamespacedName{Namespace: input.Bmh.Namespace, Name: input.Bmh.Name}
		g.Expect(input.Client.Get(ctx, key, &bmh)).To(Succeed())
		g.Expect(bmh.Status.PoweredOn).To(Equal(input.State == PoweredOn))
	}, intervals...).Should(Succeed())
}

func buildKustomizeManifest(source string) ([]byte, error) {
	kustomizer := krusty.MakeKustomizer(krusty.MakeDefaultOptions())
	fSys := filesys.MakeFsOnDisk()
	resources, err := kustomizer.Run(fSys, source)
	if err != nil {
		return nil, err
	}
	return resources.AsYaml()
}

func CreateSecret(ctx context.Context, client client.Client, secretNamespace, secretName string, data map[string]string) {
	secret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: secretNamespace,
		},
		StringData: data,
	}

	Expect(client.Create(ctx, &secret)).NotTo(HaveOccurred(), fmt.Sprintf("Failed to create secret '%s/%s'", secretNamespace, secretName))
}

func executeSSHCommand(client *ssh.Client, command string) (string, error) {
	session, err := client.NewSession()
	if err != nil {
		return "", fmt.Errorf("failed to create SSH session: %v", err)
	}
	defer session.Close()

	output, err := session.CombinedOutput(command)
	if err != nil {
		return "", fmt.Errorf("failed to execute command '%s': %v", command, err)
	}

	return string(output), nil
}

// HasRootOnDisk parses the output from 'df -h' and checks if the root filesystem is on a disk (as opposed to tmpfs).
func HasRootOnDisk(output string) bool {
	lines := strings.Split(output, "\n")

	for _, line := range lines[1:] { // Skip header line
		if line == "" {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 6 {
			continue // Skip malformed lines
		}

		if fields[5] == "/" && !strings.Contains(fields[0], "tmpfs") {
			return true // Found a non-tmpfs root filesystem
		}
	}

	return false
}

// IsBootedFromDisk checks if the system, accessed via the provided ssh.Client, is booted
// from a disk. It executes the 'df -h' command on the remote system to analyze the filesystem
// layout. In the case of a disk boot, the output includes a disk-based root filesystem
// (e.g., '/dev/vda1'). Conversely, in the case of a Live-ISO boot, the primary filesystems
// are memory-based (tmpfs).
func IsBootedFromDisk(client *ssh.Client) (bool, error) {
	cmd := "df -h"
	output, err := executeSSHCommand(client, cmd)
	if err != nil {
		return false, fmt.Errorf("error executing 'df -h': %w", err)
	}

	bootedFromDisk := HasRootOnDisk(output)
	if bootedFromDisk {
		capm3_e2e.Logf("System is booted from a disk.")
	} else {
		capm3_e2e.Logf("System is booted from a live ISO.")
	}

	return bootedFromDisk, nil
}

func EstablishSSHConnection(e2eConfig *Config, auth ssh.AuthMethod, user, address string) *ssh.Client {
	config := &ssh.ClientConfig{
		User:            user,
		Auth:            []ssh.AuthMethod{auth},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // #nosec G106
	}

	var client *ssh.Client
	var err error
	Eventually(func() error {
		client, err = ssh.Dial("tcp", address, config)
		return err
	}, e2eConfig.GetIntervals("default", "wait-connect-ssh")...).Should(Succeed(), "Failed to establish SSH connection")

	return client
}

// createCirrosInstanceAndHostnameUserdata creates a Kubernetes secret intended for cloud-init usage.
// This userdata is utilized during BMH's initialization and setup.
func createCirrosInstanceAndHostnameUserdata(ctx context.Context, client client.Client, namespace string, secretName string, sshPubKeyPath string) {
	sshPubKeyData, err := os.ReadFile(sshPubKeyPath) // #nosec G304
	Expect(err).NotTo(HaveOccurred(), "Failed to read SSH public key file")

	userDataContent := fmt.Sprintf(`#!/bin/sh
mkdir /root/.ssh
mkdir /home/cirros/.ssh
chmod 700 /root/.ssh
chmod 700 /home/cirros/.ssh
chown cirros /home/cirros/.ssh
echo "%s" >> /home/cirros/.ssh/authorized_keys
echo "%s" >> /root/.ssh/authorized_keys`, sshPubKeyData, sshPubKeyData)

	CreateSecret(ctx, client, namespace, secretName, map[string]string{"userData": userDataContent})
}

// PerformSSHBootCheck performs an SSH check to verify the node's boot source.
// The `expectedBootMode` parameter should be "disk" or "memory".
// The `auth` parameter is an ssh.AuthMethod for authentication.
func PerformSSHBootCheck(e2eConfig *Config, expectedBootMode string, auth ssh.AuthMethod) {
	ip := e2eConfig.GetVariable("IP_BMO_E2E_0")
	sshPort := e2eConfig.GetVariable("SSH_PORT")
	address := fmt.Sprintf("%s:%s", ip, sshPort)
	user := e2eConfig.GetVariable("CIRROS_USERNAME")

	client := EstablishSSHConnection(e2eConfig, auth, user, address)
	defer func() {
		if client != nil {
			client.Close()
		}
	}()

	bootedFromDisk, err := IsBootedFromDisk(client)
	Expect(err).NotTo(HaveOccurred(), "Error in verifying boot mode")

	// Compare actual boot source with expected
	isExpectedBootMode := (expectedBootMode == "disk" && bootedFromDisk) ||
		(expectedBootMode == "memory" && !bootedFromDisk)
	Expect(isExpectedBootMode).To(BeTrue(), fmt.Sprintf("Expected booting from %s, but found different mode", expectedBootMode))
}
