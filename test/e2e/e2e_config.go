//go:build e2e
// +build e2e

package e2e

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"

	. "github.com/onsi/gomega"
	"gopkg.in/yaml.v2"
	"sigs.k8s.io/cluster-api/test/framework/clusterctl"
	"sigs.k8s.io/kind/pkg/apis/config/v1alpha4"
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

	bmoString    string = "bmo"
	ironicString string = "ironic"
)

// ContainerImage describes an image to load into a cluster.
// This is a copy of clusterctl.ContainerImage with proper yaml tags for YAML unmarshaling.
// See: https://github.com/metal3-io/baremetal-operator/issues/2832
type ContainerImage struct {
	// Name is the fully qualified name of the image.
	Name string `yaml:"name"`

	// LoadBehavior may be used to dictate whether a failed load operation
	// should fail the test run.
	LoadBehavior clusterctl.LoadImageBehavior `yaml:"loadBehavior,omitempty"`
}

type BMOIronicUpgradeInput struct {
	// DeployIronic determines if Ironic should be installed at the beginning of the test.
	// This should be generally set to `true`, but can be `false` in case Ironic is either pre-installed
	// or not required (for e.g. in `fixture` setup)
	DeployIronic bool `yaml:"deployIronic,omitempty"`
	// DeployBMO determines if BMO should be installed at the beginning of the test.
	// This should be generally set to `true`, but can be `false` in case BMO is pre-installed
	DeployBMO bool `yaml:"deployBMO,omitempty"`
	// Path to the Ironic kustomization that should be installed at the beginning of the test.
	// Not used if DeployIronic is false
	InitIronicKustomization string `yaml:"initIronicKustomization,omitempty"`
	// Path to the BMO kustomization that should be installed at the beginning of the test.
	// Not used if DeployBMO is false
	InitBMOKustomization string `yaml:"initBMOKustomization,omitempty"`
	// Name of the entity that should be upgraded and tested. It should be either `bmo` or `ironic`.
	UpgradeEntityName string `yaml:"upgradeEntityName,omitempty"`
	// Path to the kustomization of the entity that should be used in upgrading.
	UpgradeEntityKustomization string `yaml:"upgradeEntityKustomization,omitempty"`
}

// Config defines the configuration of an e2e test environment.
type Config struct {
	// Images is a list of container images to load into the Kind cluster.
	// Note that this not relevant when using an existing cluster.
	// Uses custom ContainerImage type with yaml tags for proper YAML unmarshaling.
	Images []ContainerImage `yaml:"images,omitempty"`

	// Variables to be used in the tests.
	Variables map[string]string `yaml:"variables,omitempty"`

	// Intervals to be used for long operations during tests.
	Intervals map[string][]string `yaml:"intervals,omitempty"`

	// BMOIronicUpgradeSpecs
	BMOIronicUpgradeSpecs []BMOIronicUpgradeInput `yaml:"bmoIronicUpgradeSpecs,omitempty"`

	// Extra port mappings for the kind cluster
	KindExtraPortMappings []v1alpha4.PortMapping `yaml:"kindExtraPortMappings,omitempty"`
}

// LoadE2EConfig loads the configuration for the e2e test environment.
func LoadE2EConfig(configPath string, g *WithT) *Config {
	configData, err := os.ReadFile(configPath) //#nosec
	g.Expect(err).ToNot(HaveOccurred(), "Failed to read the e2e test config file")
	g.Expect(configData).ToNot(BeEmpty(), "The e2e test config file should not be empty")

	config := &Config{}
	g.Expect(yaml.Unmarshal(configData, config)).To(Succeed(), "Failed to parse the e2e test config file")

	config.Defaults()
	g.Expect(config.Validate()).To(Succeed(), "The e2e test config file is not valid")

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

// GetClusterctlImages converts the local ContainerImage slice to clusterctl.ContainerImage
// for use with the CAPI bootstrap framework.
func (c *Config) GetClusterctlImages() []clusterctl.ContainerImage {
	result := make([]clusterctl.ContainerImage, len(c.Images))
	for i, img := range c.Images {
		result[i] = clusterctl.ContainerImage{
			Name:         img.Name,
			LoadBehavior: img.LoadBehavior,
		}
	}
	return result
}

// Validate validates the configuration. More specifically:
// - Image should have name and loadBehavior be one of [mustload, tryload].
// - Intervals should be valid ginkgo intervals.
// - BMOIronicUpgradeSpecs should have valid InitIronicKustomization field if DeployIronic is true.
// - BMOIronicUpgradeSpecs should have valid InitBMOKustomization field if DeployBMO is true.
// - BMOIronicUpgradeSpecs' UpgradeEntityName should be either 'bmo' or 'ironic'.
// - BMOIronicUpgradeSpecs should have valid UpgradeEntityKustomization.
func (c *Config) Validate() error {
	// Image should have name and loadBehavior be one of [mustload, tryload].
	for i, containerImage := range c.Images {
		if containerImage.Name == "" {
			return fmt.Errorf("container image is missing name: Images[%d].Name=%q", i, containerImage.Name)
		}
		switch containerImage.LoadBehavior {
		case clusterctl.MustLoadImage, clusterctl.TryLoadImage:
			// Valid
		default:
			return fmt.Errorf("invalid load behavior: Images[%d].LoadBehavior=%q", i, containerImage.LoadBehavior)
		}
	}

	// Intervals should be valid ginkgo intervals.
	for k, intervals := range c.Intervals {
		switch len(intervals) {
		case 0:
			return fmt.Errorf("invalid interval: Intervals[%s]=%q", k, intervals)
		case 1, 2: //nolint: mnd
		default:
			return fmt.Errorf("invalid interval: Intervals[%s]=%q", k, intervals)
		}
		for _, i := range intervals {
			if _, err := time.ParseDuration(i); err != nil {
				return fmt.Errorf("invalid interval: Intervals[%s]=%q", k, intervals)
			}
		}
	}

	for _, spec := range c.BMOIronicUpgradeSpecs {
		if spec.DeployIronic {
			if spec.InitIronicKustomization == "" {
				return errors.New("ironic kustomization should be provided")
			}
			if _, err := os.Stat(spec.InitIronicKustomization); err != nil {
				return fmt.Errorf("ironic kustomization file not found: %s. Error %w", spec.InitIronicKustomization, err)
			}
		}

		if spec.DeployBMO {
			if spec.InitBMOKustomization == "" {
				return errors.New("BMO kustomization should be provided")
			}
			if _, err := os.Stat(spec.InitBMOKustomization); err != nil {
				return fmt.Errorf("BMO kustomization file not found: %s. Error %w", spec.InitBMOKustomization, err)
			}
		}

		if spec.UpgradeEntityName != bmoString && spec.UpgradeEntityName != "ironic" {
			return errors.New("UpgradeEntityName should be either 'bmo' or 'ironic'")
		}

		if spec.UpgradeEntityKustomization == "" {
			return errors.New("UpgradeEntityKustomization should be provided")
		}
		if _, err := os.Stat(spec.UpgradeEntityKustomization); err != nil {
			return fmt.Errorf("UpgradeEntityKustomization file not found: %s. Error %w", spec.UpgradeEntityKustomization, err)
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
		if intervals, ok = c.Intervals["default/"+key]; !ok {
			return nil
		}
	}
	intervalsInterfaces := make([]interface{}, len(intervals))
	for i := range intervals {
		intervalsInterfaces[i] = intervals[i]
	}
	return intervalsInterfaces
}

// HasVariable checks if a variable is defined in environment variables or in the e2e config file.
func (c *Config) HasVariable(varName string) bool {
	if _, ok := os.LookupEnv(varName); ok {
		return true
	}
	_, ok := c.Variables[varName]
	return ok
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

// GetBoolVariable returns a variable from environment variables or from the e2e config file as boolean.
func (c *Config) GetBoolVariable(varName string) bool {
	value := c.GetVariable(varName)
	falseValues := []string{"", "false", "no"}
	for _, falseVal := range falseValues {
		if strings.EqualFold(value, falseVal) {
			return false
		}
	}
	return true
}
