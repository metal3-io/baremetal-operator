// Package config provides configuration types and loading for vbmctl.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/metal3-io/baremetal-operator/test/vbmctl/pkg/api"
	"gopkg.in/yaml.v2"
)

const (
	// DefaultConfigFileName is the default name for the config file.
	DefaultConfigFileName = "vbmctl.yaml"

	// DefaultLibvirtURI is the default libvirt connection URI.
	DefaultLibvirtURI = "qemu:///system"

	// DefaultNetworkName is the default network name.
	DefaultNetworkName = "baremetal-e2e"

	// DefaultPoolName is the default storage pool name.
	DefaultPoolName = "baremetal-e2e"

	// DefaultPoolPath is the default storage pool path.
	DefaultPoolPath = "/tmp/pool_oo"

	// DefaultVMMemory is the default VM memory in MB.
	DefaultVMMemory = 4096

	// DefaultVMVCPUs is the default number of VM vCPUs.
	DefaultVMVCPUs = 2

	// DefaultVolumeSize is the default volume size in GB.
	DefaultVolumeSize = 20

	// dirPermissions is the default permission for directories.
	dirPermissions = 0750

	// filePermissions is the default permission for config files.
	filePermissions = 0600
)

// Config is the top-level configuration for vbmctl.
type Config struct {
	// APIVersion is the API version of the config format.
	APIVersion string `json:"apiVersion,omitempty" yaml:"apiVersion,omitempty"`

	// Kind is the type of config (should be "Config").
	Kind string `json:"kind,omitempty" yaml:"kind,omitempty"`

	// Spec contains the configuration specification.
	Spec Spec `json:"spec" yaml:"spec"`
}

// Spec contains the main configuration options.
type Spec struct {
	// Libvirt contains libvirt connection settings.
	Libvirt LibvirtConfig `json:"libvirt,omitempty" yaml:"libvirt,omitempty"`

	// Pool contains storage pool configuration.
	Pool api.PoolConfig `json:"pool,omitempty" yaml:"pool,omitempty"`

	// VMs is a list of VM configurations to create.
	VMs []api.VMConfig `json:"vms,omitempty" yaml:"vms,omitempty"`
}

// LibvirtConfig contains libvirt connection settings.
type LibvirtConfig struct {
	// URI is the libvirt connection URI.
	// Defaults to "qemu:///system".
	URI string `json:"uri,omitempty" yaml:"uri,omitempty"`
}

// Default returns a Config with sensible defaults.
func Default() *Config {
	return &Config{
		APIVersion: "vbmctl.metal3.io/v1alpha1",
		Kind:       "Config",
		Spec: Spec{
			Libvirt: LibvirtConfig{
				URI: DefaultLibvirtURI,
			},
			Pool: api.PoolConfig{
				Name: DefaultPoolName,
				Path: DefaultPoolPath,
			},
		},
	}
}

// Load reads a configuration from the specified file path.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path) //#nosec G304
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	return Parse(data)
}

// Parse parses configuration from YAML data.
func Parse(data []byte) (*Config, error) {
	cfg := Default()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	return cfg, nil
}

// LoadOrDefault loads a configuration from the specified path,
// or returns the default configuration if the file doesn't exist.
func LoadOrDefault(path string) (*Config, error) {
	if path == "" {
		return Default(), nil
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		return Default(), nil
	}

	return Load(path)
}

// FindConfigFile searches for a config file in standard locations.
// It checks in order:
//  1. Current directory
//  2. $HOME/.vbmctl/
//  3. /etc/vbmctl/
//
// Returns empty string if no config file is found.
func FindConfigFile() string {
	locations := []string{
		DefaultConfigFileName,
	}

	// Add home directory location
	if home, err := os.UserHomeDir(); err == nil {
		locations = append(locations, filepath.Join(home, ".vbmctl", DefaultConfigFileName))
	}

	// Add system-wide location
	locations = append(locations, filepath.Join(string(filepath.Separator), "etc", "vbmctl", DefaultConfigFileName))

	for _, loc := range locations {
		if _, err := os.Stat(loc); err == nil {
			return loc
		}
	}

	return ""
}

// Save writes the configuration to the specified file path.
func (c *Config) Save(path string) error {
	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// Ensure parent directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, dirPermissions); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	if err := os.WriteFile(path, data, filePermissions); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// Validate checks the configuration for errors.
func (c *Config) Validate() error {
	if c.Spec.Libvirt.URI == "" {
		return errors.New("libvirt URI is required")
	}

	// Validate pool config
	if c.Spec.Pool.Name == "" {
		return errors.New("pool name is required")
	}
	if c.Spec.Pool.Path == "" {
		return errors.New("pool path is required")
	}

	// Validate VM configs
	for i, vm := range c.Spec.VMs {
		if vm.Name == "" {
			return fmt.Errorf("VM at index %d has no name", i)
		}
	}

	return nil
}

// ApplyDefaults applies default values to the configuration.
func (c *Config) ApplyDefaults() {
	// Apply libvirt defaults
	if c.Spec.Libvirt.URI == "" {
		c.Spec.Libvirt.URI = DefaultLibvirtURI
	}

	// Apply pool defaults
	if c.Spec.Pool.Name == "" {
		c.Spec.Pool.Name = DefaultPoolName
	}
	if c.Spec.Pool.Path == "" {
		c.Spec.Pool.Path = DefaultPoolPath
	}

	// Apply VM defaults
	for i := range c.Spec.VMs {
		c.Spec.VMs[i] = c.Spec.VMs[i].Defaults()
	}
}

// WithVMs returns a copy of the config with the specified VMs.
func (c *Config) WithVMs(vms ...api.VMConfig) *Config {
	cfg := *c
	cfg.Spec.VMs = vms
	return &cfg
}
