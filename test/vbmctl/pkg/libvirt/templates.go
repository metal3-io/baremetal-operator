//go:build vbmctl
// +build vbmctl

package libvirt

import (
	"bytes"
	"embed"
	"fmt"
	"text/template"

	vbmctlapi "github.com/metal3-io/baremetal-operator/test/vbmctl/pkg/api"
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

// VolumeAttachmentData contains data for a volume attachment in VM templates.
type VolumeAttachmentData struct {
	Name   string
	Path   string
	Device string // e.g., "vda", "vdb"
	Bus    string // e.g., "virtio"
}

// RenderVM renders the VM XML template with the given data.
func (r *TemplateRenderer) RenderVM(cfg vbmctlapi.VMConfig, poolPath string) (string, error) {
	// Apply defaults
	cfg = cfg.Defaults()

	// Convert memory from MB to KiB
	memoryKiB := cfg.Memory * kiBPerMiB

	// Check and convert volumes to attachments
	devices := []string{"vda", "vdb", "vdc", "vdd", "vde", "vdf", "vdg", "vdh"}
	if len(cfg.Volumes) > len(devices) {
		return "", fmt.Errorf("configuration has %d volumes but only %d are supported", len(cfg.Volumes), len(devices))
	}
	volumes := make([]VolumeAttachmentData, len(cfg.Volumes))
	for i, vol := range cfg.Volumes {
		volumes[i] = VolumeAttachmentData{
			Name:   vol.Name,
			Path:   fmt.Sprintf("%s/%s-%s.qcow2", poolPath, cfg.Name, vol.Name),
			Device: devices[i],
			Bus:    "virtio",
		}
	}

	data := struct {
		Name     string
		Memory   int // in KiB
		VCPUs    int
		PoolPath string
		Networks []vbmctlapi.NetworkAttachment
		Volumes  []VolumeAttachmentData
	}{
		Name:     cfg.Name,
		Memory:   memoryKiB,
		VCPUs:    cfg.VCPUs,
		PoolPath: poolPath,
		Networks: cfg.Networks,
		Volumes:  volumes,
	}
	return r.render("VM.xml.tpl", data)
}

// RenderPool renders the storage pool XML template with the given data.
func (r *TemplateRenderer) RenderPool(cfg vbmctlapi.PoolConfig) (string, error) {
	return r.render("pool.xml.tpl", cfg)
}

// RenderVolume renders the volume XML template with the given data.
func (r *TemplateRenderer) RenderVolume(cfg vbmctlapi.VolumeConfig) (string, error) {
	return r.render("volume.xml.tpl", cfg)
}

// RenderDHCPHost renders XML for a DHCP host entry.
func (r *TemplateRenderer) RenderDHCPHost(net vbmctlapi.NetworkAttachment, hostName string) (string, error) {
	data := struct {
		MACAddress string
		Name       string
		IPAddress  string
	}{
		MACAddress: net.MACAddress,
		Name:       hostName,
		IPAddress:  net.IPAddress,
	}
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
