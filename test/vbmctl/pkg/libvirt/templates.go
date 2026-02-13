//go:build vbmctl
// +build vbmctl

package libvirt

import (
	"bytes"
	"embed"
	"fmt"
	"text/template"

	"github.com/metal3-io/baremetal-operator/test/vbmctl/pkg/api"
)

const (
	// kiBPerMiB is the number of KiB in a MiB.
	kiBPerMiB = 1024
)

//go:embed templates/*.tpl
var templateFiles embed.FS

// TemplateRenderer provides methods for rendering libvirt XML templates.
type TemplateRenderer struct {
	templates *template.Template
}

// NewTemplateRenderer creates a new template renderer with embedded templates.
func NewTemplateRenderer() (*TemplateRenderer, error) {
	tmpl, err := template.ParseFS(templateFiles, "templates/*.tpl")
	if err != nil {
		return nil, fmt.Errorf("failed to parse embedded templates: %w", err)
	}

	return &TemplateRenderer{
		templates: tmpl,
	}, nil
}

// VMTemplateData contains data for rendering VM templates.
type VMTemplateData struct {
	Name     string
	Memory   int // in KiB
	VCPUs    int
	PoolPath string
	Networks []NetworkInterfaceData
	Volumes  []VolumeAttachmentData
}

// NetworkInterfaceData contains data for a network interface in VM templates.
type NetworkInterfaceData struct {
	Network    string
	MACAddress string
}

// VolumeAttachmentData contains data for a volume attachment in VM templates.
type VolumeAttachmentData struct {
	Name   string
	Path   string
	Device string // e.g., "vda", "vdb"
	Bus    string // e.g., "virtio"
}

// PoolTemplateData contains data for rendering storage pool templates.
type PoolTemplateData struct {
	PoolName string
	PoolPath string
}

// VolumeTemplateData contains data for rendering volume templates.
type VolumeTemplateData struct {
	VolumeName         string
	VolumeCapacityInGB int
}

// DHCPHostData contains data for a DHCP host entry.
type DHCPHostData struct {
	MACAddress string
	Name       string
	IPAddress  string
}

// RenderVM renders the VM XML template with the given data.
func (r *TemplateRenderer) RenderVM(data VMTemplateData) (string, error) {
	return r.render("VM.xml.tpl", data)
}

// RenderPool renders the storage pool XML template with the given data.
func (r *TemplateRenderer) RenderPool(data PoolTemplateData) (string, error) {
	return r.render("pool.xml.tpl", data)
}

// RenderVolume renders the volume XML template with the given data.
func (r *TemplateRenderer) RenderVolume(data VolumeTemplateData) (string, error) {
	return r.render("volume.xml.tpl", data)
}

// RenderDHCPHost renders XML for a DHCP host entry.
func (r *TemplateRenderer) RenderDHCPHost(data DHCPHostData) (string, error) {
	tmpl, err := template.New("dhcp-host").Parse(
		"<host mac='{{ .MACAddress }}' name='{{ .Name }}' ip='{{ .IPAddress }}' />",
	)
	if err != nil {
		return "", fmt.Errorf("failed to parse DHCP host template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to render DHCP host template: %w", err)
	}

	return buf.String(), nil
}

// render is a helper function that renders a named template with the given data.
func (r *TemplateRenderer) render(name string, data interface{}) (string, error) {
	var buf bytes.Buffer
	if err := r.templates.ExecuteTemplate(&buf, name, data); err != nil {
		return "", fmt.Errorf("failed to render template %s: %w", name, err)
	}
	return buf.String(), nil
}

// RenderTemplate is a standalone function for rendering templates from the embedded filesystem.
// This is provided for backward compatibility with existing code.
func RenderTemplate(inputFile string, data interface{}) (string, error) {
	tmpl, err := template.ParseFS(templateFiles, inputFile)
	if err != nil {
		return "", fmt.Errorf("failed to parse template %s: %w", inputFile, err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to render template %s: %w", inputFile, err)
	}

	return buf.String(), nil
}

// VMConfigToTemplateData converts a VMConfig to VMTemplateData.
func VMConfigToTemplateData(cfg api.VMConfig, poolPath string) VMTemplateData {
	// Apply defaults
	cfg = cfg.Defaults()

	// Convert memory from MB to KiB
	memoryKiB := cfg.Memory * kiBPerMiB

	// Convert networks
	networks := make([]NetworkInterfaceData, len(cfg.Networks))
	for i, net := range cfg.Networks {
		networks[i] = NetworkInterfaceData{
			Network:    net.Network,
			MACAddress: net.MACAddress,
		}
	}

	// Convert volumes to attachments
	volumes := make([]VolumeAttachmentData, len(cfg.Volumes))
	devices := []string{"vda", "vdb", "vdc", "vdd", "vde", "vdf", "vdg", "vdh"}
	for i, vol := range cfg.Volumes {
		device := "vda"
		if i < len(devices) {
			device = devices[i]
		}
		volumes[i] = VolumeAttachmentData{
			Name:   vol.Name,
			Path:   fmt.Sprintf("%s/%s-%s.qcow2", poolPath, cfg.Name, vol.Name),
			Device: device,
			Bus:    "virtio",
		}
	}

	return VMTemplateData{
		Name:     cfg.Name,
		Memory:   memoryKiB,
		VCPUs:    cfg.VCPUs,
		PoolPath: poolPath,
		Networks: networks,
		Volumes:  volumes,
	}
}

// PoolConfigToTemplateData converts a PoolConfig to PoolTemplateData.
func PoolConfigToTemplateData(cfg api.PoolConfig) PoolTemplateData {
	return PoolTemplateData{
		PoolName: cfg.Name,
		PoolPath: cfg.Path,
	}
}

// VolumeConfigToTemplateData converts a VolumeConfig to VolumeTemplateData.
func VolumeConfigToTemplateData(cfg api.VolumeConfig) VolumeTemplateData {
	cfg = cfg.Defaults()
	return VolumeTemplateData{
		VolumeName:         cfg.Name,
		VolumeCapacityInGB: cfg.Size,
	}
}
