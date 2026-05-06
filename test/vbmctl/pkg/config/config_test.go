package config

import (
	"os"
	"path/filepath"
	"testing"

	vbmctlapi "github.com/metal3-io/baremetal-operator/test/vbmctl/pkg/api"
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
  imageServer:
    image: "test/image-server:latest"
    port: 81
    containerPort: 8081
    dataDir: "/var/lib/vbmctl/images-test"
    containerDataDir: "/usr/share/nginx/html"
    containerName: "vbmctl-image-server-test"
  bmcEmulator:
    type: "sushy-tools"
    configFile: "vbmc-emulator-file"
    image: "test/bmc-emulator:latest"
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

	if cfg.Spec.ImageServer == nil {
		t.Fatal("expected image server config, got nil")
	}

	if cfg.Spec.ImageServer.Image != "test/image-server:latest" {
		t.Errorf("expected image server image 'test/image-server:latest', got %s", cfg.Spec.ImageServer.Image)
	}

	if cfg.Spec.ImageServer.Port != 81 {
		t.Errorf("expected image server port 81, got %d", cfg.Spec.ImageServer.Port)
	}

	if cfg.Spec.ImageServer.ContainerPort != 8081 {
		t.Errorf("expected image server container port 8081, got %d", cfg.Spec.ImageServer.ContainerPort)
	}

	if cfg.Spec.ImageServer.DataDir != "/var/lib/vbmctl/images-test" {
		t.Errorf("expected image server data dir '/var/lib/vbmctl/images-test', got %s", cfg.Spec.ImageServer.DataDir)
	}

	if cfg.Spec.ImageServer.ContainerDataDir != "/usr/share/nginx/html" {
		t.Errorf("expected image server container data dir '/usr/share/nginx/html', got %s", cfg.Spec.ImageServer.ContainerDataDir)
	}

	if cfg.Spec.ImageServer.ContainerName != "vbmctl-image-server-test" {
		t.Errorf("expected image server container name 'vbmctl-image-server-test', got %s", cfg.Spec.ImageServer.ContainerName)
	}

	if cfg.Spec.BMCEmulator == nil {
		t.Fatal("expected BMC emulator config, got nil")
	}

	if cfg.Spec.BMCEmulator.Type != "sushy-tools" {
		t.Errorf("expected BMC emulator type 'sushy-tools', got %s", cfg.Spec.BMCEmulator.Type)
	}

	if cfg.Spec.BMCEmulator.ConfigFile != "vbmc-emulator-file" {
		t.Errorf("expected BMC emulator config file 'vbmc-emulator-file', got %s", cfg.Spec.BMCEmulator.ConfigFile)
	}

	if cfg.Spec.BMCEmulator.Image != "test/bmc-emulator:latest" {
		t.Errorf("expected BMC emulator image 'test/bmc-emulator:latest', got %s", cfg.Spec.BMCEmulator.Image)
	}
}

func TestLoadAndSave(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test-config.yaml")

	// Create a config
	cfg := Default()
	cfg.Spec.VMs = []vbmctlapi.VMConfig{
		{Name: "saved-vm", Memory: 2048, VCPUs: 1},
	}
	cfg.Spec.ImageServer = &vbmctlapi.ImageServerConfig{
		Image:            "test/image-server:latest",
		Port:             81,
		ContainerPort:    8081,
		DataDir:          "/var/lib/vbmctl/images-test",
		ContainerDataDir: "/usr/share/nginx/html",
		ContainerName:    "vbmctl-image-server",
	}
	cfg.Spec.BMCEmulator = &vbmctlapi.BMCEmulatorConfig{
		Type:       BMCEmulatorTypeSushyTools,
		ConfigFile: "vbmc-emulator-file",
		Image:      "test/bmc-emulator:latest",
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

	if loadedCfg.Spec.ImageServer == nil {
		t.Fatal("expected image server config, got nil")
	}

	if loadedCfg.Spec.ImageServer.Image != "test/image-server:latest" {
		t.Errorf("expected image server image 'test/image-server:latest', got %s", loadedCfg.Spec.ImageServer.Image)
	}

	if loadedCfg.Spec.ImageServer.DataDir != "/var/lib/vbmctl/images-test" {
		t.Errorf("expected image server data dir '/var/lib/vbmctl/images-test', got %s", loadedCfg.Spec.ImageServer.DataDir)
	}

	if loadedCfg.Spec.ImageServer.ContainerDataDir != "/usr/share/nginx/html" {
		t.Errorf("expected image server container data dir '/usr/share/nginx/html', got %s", loadedCfg.Spec.ImageServer.ContainerDataDir)
	}

	if loadedCfg.Spec.ImageServer.ContainerName != "vbmctl-image-server" {
		t.Errorf("expected image server container name 'vbmctl-image-server', got %s", loadedCfg.Spec.ImageServer.ContainerName)
	}

	if loadedCfg.Spec.ImageServer.Port != 81 {
		t.Errorf("expected image server port %d, got %d", 81, loadedCfg.Spec.ImageServer.Port)
	}

	if loadedCfg.Spec.ImageServer.ContainerPort != 8081 {
		t.Errorf("expected image server container port %d, got %d", 8081, loadedCfg.Spec.ImageServer.ContainerPort)
	}

	if loadedCfg.Spec.BMCEmulator == nil {
		t.Fatal("expected BMC emulator config, got nil")
	}

	if loadedCfg.Spec.BMCEmulator.Type != BMCEmulatorTypeSushyTools {
		t.Errorf("expected BMC emulator type '%s', got %s", BMCEmulatorTypeSushyTools, loadedCfg.Spec.BMCEmulator.Type)
	}

	if loadedCfg.Spec.BMCEmulator.ConfigFile != "vbmc-emulator-file" {
		t.Errorf("expected BMC emulator config file 'vbmc-emulator-file', got %s", loadedCfg.Spec.BMCEmulator.ConfigFile)
	}

	if loadedCfg.Spec.BMCEmulator.Image != "test/bmc-emulator:latest" {
		t.Errorf("expected BMC emulator image 'test/bmc-emulator:latest', got %s", loadedCfg.Spec.BMCEmulator.Image)
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
				c.Spec.VMs = []vbmctlapi.VMConfig{{Memory: 1024}}
			},
			wantErr: true,
		},
		{
			name: "valid image server config",
			modify: func(c *Config) {
				c.Spec.ImageServer = &vbmctlapi.ImageServerConfig{
					Image:            "test/image-server:latest",
					Port:             80,
					ContainerPort:    8080,
					DataDir:          "/var/lib/vbmctl/images-test",
					ContainerDataDir: "/var/lib/vbmctl/images-test",
					ContainerName:    "vbmctl-image-server",
				}
			},
			wantErr: false,
		},
		{
			name: "invalid image server config - missing port",
			modify: func(c *Config) {
				c.Spec.ImageServer = &vbmctlapi.ImageServerConfig{
					Image:            "test/image-server:latest",
					ContainerPort:    8080,
					DataDir:          "/var/lib/vbmctl/images-test",
					ContainerDataDir: "/var/lib/vbmctl/images-test",
					ContainerName:    "vbmctl-image-server",
				}
			},
			wantErr: true,
		},
		{
			name: "invalid image server config - missing container port",
			modify: func(c *Config) {
				c.Spec.ImageServer = &vbmctlapi.ImageServerConfig{
					Image:            "test/image-server:latest",
					Port:             80,
					DataDir:          "/var/lib/vbmctl/images-test",
					ContainerDataDir: "/var/lib/vbmctl/images-test",
					ContainerName:    "vbmctl-image-server",
				}
			},
			wantErr: true,
		},
		{
			name: "invalid image server config - missing data dir",
			modify: func(c *Config) {
				c.Spec.ImageServer = &vbmctlapi.ImageServerConfig{
					Image:            "test/image-server:latest",
					Port:             80,
					ContainerPort:    8080,
					ContainerDataDir: "/var/lib/vbmctl/images-test",
					ContainerName:    "vbmctl-image-server",
				}
			},
			wantErr: true,
		},
		{
			name: "invalid image server config - missing container data dir",
			modify: func(c *Config) {
				c.Spec.ImageServer = &vbmctlapi.ImageServerConfig{
					Image:         "test/image-server:latest",
					Port:          80,
					ContainerPort: 8080,
					DataDir:       "/var/lib/vbmctl/images-test",
					ContainerName: "vbmctl-image-server",
				}
			},
			wantErr: true,
		},
		{
			name: "invalid image server config - missing image",
			modify: func(c *Config) {
				c.Spec.ImageServer = &vbmctlapi.ImageServerConfig{
					Port:             80,
					ContainerPort:    8080,
					DataDir:          "/var/lib/vbmctl/images-test",
					ContainerDataDir: "/var/lib/vbmctl/images-test",
					ContainerName:    "vbmctl-image-server",
				}
			},
			wantErr: true,
		},
		{
			name: "invalid image server config - missing container name",
			modify: func(c *Config) {
				c.Spec.ImageServer = &vbmctlapi.ImageServerConfig{
					Image:            "test/image-server:latest",
					Port:             80,
					ContainerPort:    8080,
					DataDir:          "/var/lib/vbmctl/images-test",
					ContainerDataDir: "/var/lib/vbmctl/images-test",
				}
			},
			wantErr: true,
		},
		{
			name: "valid BMC emulator config (sushy-tools)",
			modify: func(c *Config) {
				c.Spec.BMCEmulator = &vbmctlapi.BMCEmulatorConfig{
					Type:       BMCEmulatorTypeSushyTools,
					ConfigFile: "vbmc-emulator-file",
					Image:      "test/bmc-emulator:latest",
				}
			},
			wantErr: false,
		},
		{
			name: "valid BMC emulator config (vbmc)",
			modify: func(c *Config) {
				c.Spec.BMCEmulator = &vbmctlapi.BMCEmulatorConfig{
					Type:  BMCEmulatorTypeVBMC,
					Image: "test/bmc-emulator:latest",
				}
			},
			wantErr: false,
		},
		{
			name: "invalid BMC emulator config - missing type",
			modify: func(c *Config) {
				c.Spec.BMCEmulator = &vbmctlapi.BMCEmulatorConfig{
					ConfigFile: "vbmc-emulator-file",
					Image:      "test/bmc-emulator:latest",
				}
			},
			wantErr: true,
		},
		{
			name: "invalid BMC emulator config - missing config file",
			modify: func(c *Config) {
				c.Spec.BMCEmulator = &vbmctlapi.BMCEmulatorConfig{
					Type:  BMCEmulatorTypeSushyTools,
					Image: "test/bmc-emulator:latest",
				}
			},
			wantErr: true,
		},

		{
			name: "invalid BMC emulator config - missing image",
			modify: func(c *Config) {
				c.Spec.BMCEmulator = &vbmctlapi.BMCEmulatorConfig{
					Type:       BMCEmulatorTypeVBMC,
					ConfigFile: "vbmc-emulator-file",
				}
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
	cfg.Spec.BMCEmulator = &vbmctlapi.BMCEmulatorConfig{}
	cfg.ApplyDefaults()

	if cfg.Spec.Libvirt.URI != DefaultLibvirtURI {
		t.Errorf("expected Libvirt URI %s, got %s", DefaultLibvirtURI, cfg.Spec.Libvirt.URI)
	}

	if cfg.Spec.Pool.Name != DefaultPoolName {
		t.Errorf("expected Pool Name %s, got %s", DefaultPoolName, cfg.Spec.Pool.Name)
	}

	if cfg.Spec.BMCEmulator.Type != DefaultBMCEmulatorType {
		t.Errorf("expected BMC Emulator Type %s, got %s", DefaultBMCEmulatorType, cfg.Spec.BMCEmulator.Type)
	}

	// If type is unknown, image default should not be applied
	unknownCfg := &Config{}
	unknownCfg.Spec.BMCEmulator = &vbmctlapi.BMCEmulatorConfig{
		Type: "unknown-type",
	}
	unknownCfg.ApplyDefaults()
	if unknownCfg.Spec.BMCEmulator.Image != "" {
		t.Errorf("expected BMC Emulator Image <empty>, got %s", unknownCfg.Spec.BMCEmulator.Image)
	}

	// If type is vbmc, vbmc image default should be applied
	vbmcCfg := &Config{}
	vbmcCfg.Spec.BMCEmulator = &vbmctlapi.BMCEmulatorConfig{
		Type: BMCEmulatorTypeVBMC,
	}
	vbmcCfg.ApplyDefaults()
	if vbmcCfg.Spec.BMCEmulator.Image != DefaultBMCEmulatorVBMCImage {
		t.Errorf("expected BMC Emulator Image %s, got %s", DefaultBMCEmulatorVBMCImage, vbmcCfg.Spec.BMCEmulator.Image)
	}

	// Config file default is only applied for sushy-tools type. Also verify that the image default for sushy-tools is applied.
	sushyCfg := &Config{}
	sushyCfg.Spec.BMCEmulator = &vbmctlapi.BMCEmulatorConfig{
		Type: BMCEmulatorTypeSushyTools,
	}
	sushyCfg.ApplyDefaults()
	if sushyCfg.Spec.BMCEmulator.ConfigFile != DefaultBMCEmulatorSushyToolsConfigFile {
		t.Errorf("expected BMC Emulator Config File %s, got %s", DefaultBMCEmulatorSushyToolsConfigFile, sushyCfg.Spec.BMCEmulator.ConfigFile)
	}
	if sushyCfg.Spec.BMCEmulator.Image != DefaultBMCEmulatorSushyToolsImage {
		t.Errorf("expected BMC Emulator Image %s, got %s", DefaultBMCEmulatorSushyToolsImage, sushyCfg.Spec.BMCEmulator.Image)
	}
}

func TestWithVMs(t *testing.T) {
	cfg := Default()
	vms := []vbmctlapi.VMConfig{
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
