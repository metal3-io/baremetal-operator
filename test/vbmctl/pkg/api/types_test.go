package vbmctlapi

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v2"
)

func TestVMConfigDefaults(t *testing.T) {
	tests := []struct {
		name     string
		input    VMConfig
		expected VMConfig
	}{
		{
			name:  "empty config gets defaults",
			input: VMConfig{Name: "test"},
			expected: VMConfig{
				Name:   "test",
				Memory: 4096,
				VCPUs:  2,
			},
		},
		{
			name: "custom values preserved",
			input: VMConfig{
				Name:   "test",
				Memory: 8192,
				VCPUs:  4,
			},
			expected: VMConfig{
				Name:   "test",
				Memory: 8192,
				VCPUs:  4,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.input.Defaults()

			if result.Name != tt.expected.Name {
				t.Errorf("Name: got %s, want %s", result.Name, tt.expected.Name)
			}
			if result.Memory != tt.expected.Memory {
				t.Errorf("Memory: got %d, want %d", result.Memory, tt.expected.Memory)
			}
			if result.VCPUs != tt.expected.VCPUs {
				t.Errorf("VCPUs: got %d, want %d", result.VCPUs, tt.expected.VCPUs)
			}
		})
	}
}

func TestBMCEmulatorConfigMarshalYAMLUsesLegacyFlatShapeForValues(t *testing.T) {
	cfg := BMCEmulatorConfig{
		Type: BMCEmulatorTypeSushyTools,
		SushyToolsConfig: SushyToolsConfig{
			ConfigFile:    "/etc/sushy-tools.conf",
			ListenAddress: "192.0.2.10",
			ListenPort:    8001,
			StoragePool:   "pool-1",
			LibvirtURI:    "qemu:///system",
		},
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatalf("yaml.Marshal returned error: %v", err)
	}

	output := string(data)
	for _, expected := range []string{
		"type: sushy-tools",
		"configFile: /etc/sushy-tools.conf",
		"listenAddress: 192.0.2.10",
		"listenPort: 8001",
		"storagePool: pool-1",
		"libvirtUri: qemu:///system",
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("expected marshaled YAML to contain %q, got:\n%s", expected, output)
		}
	}

	if strings.Contains(output, "sushyToolsConfig:") {
		t.Fatalf("expected legacy flat YAML shape, got:\n%s", output)
	}
}

func TestVolumeConfigDefaults(t *testing.T) {
	tests := []struct {
		name     string
		input    VolumeConfig
		expected VolumeConfig
	}{
		{
			name:  "empty config gets defaults",
			input: VolumeConfig{Name: "vol1"},
			expected: VolumeConfig{
				Name: "vol1",
				Size: 20,
			},
		},
		{
			name: "custom values preserved",
			input: VolumeConfig{
				Name: "vol1",
				Size: 50,
			},
			expected: VolumeConfig{
				Name: "vol1",
				Size: 50,
			},
		},
		{
			name: "size of zero gets default",
			input: VolumeConfig{
				Name: "vol1",
				Size: 0,
			},
			expected: VolumeConfig{
				Name: "vol1",
				Size: 20,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.input.Defaults()

			if result.Size != tt.expected.Size {
				t.Errorf("Size: got %d, want %d", result.Size, tt.expected.Size)
			}
		})
	}
}
