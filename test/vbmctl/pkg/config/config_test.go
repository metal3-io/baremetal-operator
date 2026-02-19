package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/metal3-io/baremetal-operator/test/vbmctl/pkg/api"
)

func TestDefault(t *testing.T) {
	cfg := Default()

	if cfg.APIVersion != "vbmctl.metal3.io/v1alpha1" {
		t.Errorf("expected APIVersion 'vbmctl.metal3.io/v1alpha1', got %s", cfg.APIVersion)
	}

	if cfg.Kind != "Config" {
		t.Errorf("expected Kind 'Config', got %s", cfg.Kind)
	}

	if cfg.Spec.Libvirt.URI != DefaultLibvirtURI {
		t.Errorf("expected Libvirt URI %s, got %s", DefaultLibvirtURI, cfg.Spec.Libvirt.URI)
	}

	if cfg.Spec.Pool.Name != DefaultPoolName {
		t.Errorf("expected Pool Name %s, got %s", DefaultPoolName, cfg.Spec.Pool.Name)
	}
}

func TestParse(t *testing.T) {
	yamlData := []byte(`
apiVersion: vbmctl.metal3.io/v1alpha1
kind: Config
spec:
  libvirt:
    uri: "qemu+ssh://user@host/system"
  pool:
    name: "test-pool"
    path: "/tmp/test-pool"
  vms:
    - name: "test-vm"
      memory: 8192
      vcpus: 4
`)

	cfg, err := Parse(yamlData)
	if err != nil {
		t.Fatalf("failed to parse config: %v", err)
	}

	if cfg.Spec.Libvirt.URI != "qemu+ssh://user@host/system" {
		t.Errorf("expected Libvirt URI 'qemu+ssh://user@host/system', got %s", cfg.Spec.Libvirt.URI)
	}

	if cfg.Spec.Pool.Name != "test-pool" {
		t.Errorf("expected Pool Name 'test-pool', got %s", cfg.Spec.Pool.Name)
	}

	if len(cfg.Spec.VMs) != 1 {
		t.Fatalf("expected 1 VM, got %d", len(cfg.Spec.VMs))
	}

	if cfg.Spec.VMs[0].Name != "test-vm" {
		t.Errorf("expected VM name 'test-vm', got %s", cfg.Spec.VMs[0].Name)
	}

	if cfg.Spec.VMs[0].Memory != 8192 {
		t.Errorf("expected VM memory 8192, got %d", cfg.Spec.VMs[0].Memory)
	}
}

func TestLoadAndSave(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test-config.yaml")

	// Create a config
	cfg := Default()
	cfg.Spec.VMs = []api.VMConfig{
		{Name: "saved-vm", Memory: 2048, VCPUs: 1},
	}

	// Save it
	if err := cfg.Save(configPath); err != nil {
		t.Fatalf("failed to save config: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Fatal("config file was not created")
	}

	// Load it back
	loadedCfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	if len(loadedCfg.Spec.VMs) != 1 {
		t.Fatalf("expected 1 VM, got %d", len(loadedCfg.Spec.VMs))
	}

	if loadedCfg.Spec.VMs[0].Name != "saved-vm" {
		t.Errorf("expected VM name 'saved-vm', got %s", loadedCfg.Spec.VMs[0].Name)
	}
}

func TestLoadOrDefault(t *testing.T) {
	// Test with non-existent file - should return default
	cfg, err := LoadOrDefault("/non/existent/path.yaml")
	if err != nil {
		t.Fatalf("failed to load or default: %v", err)
	}

	if cfg.Spec.Libvirt.URI != DefaultLibvirtURI {
		t.Errorf("expected default Libvirt URI, got %s", cfg.Spec.Libvirt.URI)
	}

	// Test with empty path - should return default
	cfg, err = LoadOrDefault("")
	if err != nil {
		t.Fatalf("failed to load or default: %v", err)
	}

	if cfg.Spec.Libvirt.URI != DefaultLibvirtURI {
		t.Errorf("expected default Libvirt URI, got %s", cfg.Spec.Libvirt.URI)
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		modify  func(*Config)
		wantErr bool
	}{
		{
			name:    "valid default config",
			modify:  func(c *Config) {},
			wantErr: false,
		},
		{
			name: "empty libvirt URI",
			modify: func(c *Config) {
				c.Spec.Libvirt.URI = ""
			},
			wantErr: true,
		},
		{
			name: "empty pool name",
			modify: func(c *Config) {
				c.Spec.Pool.Name = ""
			},
			wantErr: true,
		},
		{
			name: "empty pool path",
			modify: func(c *Config) {
				c.Spec.Pool.Path = ""
			},
			wantErr: true,
		},
		{
			name: "VM without name",
			modify: func(c *Config) {
				c.Spec.VMs = []api.VMConfig{{Memory: 1024}}
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Default()
			tt.modify(cfg)
			err := cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestApplyDefaults(t *testing.T) {
	cfg := &Config{}
	cfg.ApplyDefaults()

	if cfg.Spec.Libvirt.URI != DefaultLibvirtURI {
		t.Errorf("expected Libvirt URI %s, got %s", DefaultLibvirtURI, cfg.Spec.Libvirt.URI)
	}

	if cfg.Spec.Pool.Name != DefaultPoolName {
		t.Errorf("expected Pool Name %s, got %s", DefaultPoolName, cfg.Spec.Pool.Name)
	}
}

func TestWithVMs(t *testing.T) {
	cfg := Default()
	vms := []api.VMConfig{
		{Name: "vm1", Memory: 1024},
		{Name: "vm2", Memory: 2048},
	}

	newCfg := cfg.WithVMs(vms...)

	if len(newCfg.Spec.VMs) != 2 {
		t.Fatalf("expected 2 VMs, got %d", len(newCfg.Spec.VMs))
	}

	if newCfg.Spec.VMs[0].Name != "vm1" {
		t.Errorf("expected VM name 'vm1', got %s", newCfg.Spec.VMs[0].Name)
	}

	// Original should be unchanged
	if len(cfg.Spec.VMs) != 0 {
		t.Errorf("original config was modified")
	}
}
