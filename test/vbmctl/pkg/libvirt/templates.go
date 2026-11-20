//go:build vbmctl
// +build vbmctl

package libvirt

import (
	"bytes"
	"embed"
	"fmt"
	"os"
	"strings"
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
		// OVMFCode and OVMFVars pin the UEFI firmware. If empty, libvirt
		// auto-selects firmware.
		OVMFCode string
		OVMFVars string
	}{
		Name:     cfg.Name,
		Memory:   memoryKiB,
		VCPUs:    cfg.VCPUs,
		PoolPath: poolPath,
		Networks: cfg.Networks,
		Volumes:  volumes,
	}
	data.OVMFCode, data.OVMFVars = findOVMF()
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

// RenderNetwork renders the network XML template with the given data.
func (r *TemplateRenderer) RenderNetwork(cfg vbmctlapi.NetworkConfig) (string, error) {
	cfg = cfg.Defaults()
	ipFamily := "ipv4"
	// The address is validated before, it contains either v4 or v6 address
	if strings.Contains(cfg.Address, ":") {
		ipFamily = "ipv6"
	}

	data := struct {
		vbmctlapi.NetworkConfig
		IPFamily string
	}{
		NetworkConfig: cfg,
		IPFamily:      ipFamily,
	}
	return r.render("network.xml.tpl", data)
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

// ovmfCandidates lists known (code, vars) pairs of plain, non-secureboot,
// non-confidential-computing OVMF images across distributions.
var ovmfCandidates = [][2]string{
	{"/usr/share/OVMF/OVMF_CODE_4M.fd", "/usr/share/OVMF/OVMF_VARS_4M.fd"},         // Debian/Ubuntu
	{"/usr/share/OVMF/OVMF_CODE.fd", "/usr/share/OVMF/OVMF_VARS.fd"},               // Debian/Ubuntu (legacy)
	{"/usr/share/edk2/ovmf/OVMF_CODE.fd", "/usr/share/edk2/ovmf/OVMF_VARS.fd"},     // Fedora/CentOS
	{"/usr/share/edk2/x64/OVMF_CODE.4m.fd", "/usr/share/edk2/x64/OVMF_VARS.4m.fd"}, // Arch
}

// findOVMF returns the first existing OVMF code/vars pair. Pinning the loader
// avoids libvirt auto-selecting restricted images (e.g. OVMF.amdsev.fd on
// Ubuntu 26.04) that AppArmor refuses to load.
func findOVMF() (string, string) {
	for _, c := range ovmfCandidates {
		if _, err := os.Stat(c[0]); err != nil {
			continue
		}
		if _, err := os.Stat(c[1]); err != nil {
			continue
		}
		return c[0], c[1]
	}
	return "", ""
}
