package e2e

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"

	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"

	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"

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
	Expect(ok).To(BeTrue())
	return value
}

type WaitForBmhInProvisioningStateInput struct {
	Client client.Client
	Bmh    metal3api.BareMetalHost
	State  metal3api.ProvisioningState
}

func WaitForBmhInProvisioningState(ctx context.Context, input WaitForBmhInProvisioningStateInput, intervals ...interface{}) {
	Eventually(func(g Gomega) {
		bmh := metal3api.BareMetalHost{}
		key := types.NamespacedName{Namespace: input.Bmh.Namespace, Name: input.Bmh.Name}
		g.Expect(input.Client.Get(ctx, key, &bmh)).To(Succeed())
		g.Expect(bmh.Status.Provisioning.State).To(Equal(input.State))
	}, intervals...).Should(Succeed())
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

func StringPtr(s string) *string {
	return &s
}
