//go:build vbmctl
// +build vbmctl

package libvirt

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	vbmctlapi "github.com/metal3-io/baremetal-operator/test/vbmctl/pkg/api"
)

// touch creates an empty file in dir and returns its path.
func touch(t *testing.T, dir, name string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatalf("failed to create %s: %v", path, err)
	}
	return path
}

func TestFindOVMF(t *testing.T) {
	dir := t.TempDir()
	plainCode := touch(t, dir, "OVMF_CODE.fd")
	plainVars := touch(t, dir, "OVMF_VARS.fd")
	secbootCode := touch(t, dir, "OVMF_CODE.secboot.fd")
	onlyCode := touch(t, dir, "ONLY_CODE.fd")
	missing := filepath.Join(dir, "missing.fd")

	tests := []struct {
		name       string
		candidates [][2]string
		wantCode   string
		wantVars   string
	}{
		{
			name:       "no candidates",
			candidates: nil,
		},
		{
			name:       "no candidate exists",
			candidates: [][2]string{{missing, missing}},
		},
		{
			name:       "skips pair with missing vars",
			candidates: [][2]string{{onlyCode, missing}},
		},
		{
			name:       "skips pair with missing code",
			candidates: [][2]string{{missing, plainVars}},
		},
		{
			name:       "returns the first existing pair",
			candidates: [][2]string{{missing, missing}, {plainCode, plainVars}, {secbootCode, plainVars}},
			wantCode:   plainCode,
			wantVars:   plainVars,
		},
		{
			name:       "falls back to secboot code when plain is missing",
			candidates: [][2]string{{missing, plainVars}, {secbootCode, plainVars}},
			wantCode:   secbootCode,
			wantVars:   plainVars,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, vars := findOVMF(tt.candidates)
			if code != tt.wantCode || vars != tt.wantVars {
				t.Errorf("findOVMF() = (%q, %q), want (%q, %q)", code, vars, tt.wantCode, tt.wantVars)
			}
		})
	}
}

func TestOVMFCandidatesPlainBeforeSecboot(t *testing.T) {
	seenSecboot := false
	for _, c := range ovmfCandidates {
		if strings.Contains(c[0], "secboot") {
			seenSecboot = true
		} else if seenSecboot {
			t.Errorf("plain candidate %q is listed after a secboot candidate", c[0])
		}
	}
}

func TestRenderVMFirmware(t *testing.T) {
	dir := t.TempDir()
	code := touch(t, dir, "OVMF_CODE.fd")
	vars := touch(t, dir, "OVMF_VARS.fd")

	tests := []struct {
		name       string
		candidates [][2]string
		want       []string
		notWant    []string
	}{
		{
			name:       "pins firmware when found",
			candidates: [][2]string{{code, vars}},
			want: []string{
				"<os>",
				"<loader readonly='yes' type='pflash'>" + code + "</loader>",
				"<nvram template='" + vars + "'/>",
			},
			notWant: []string{`firmware="efi"`, "<firmware>", "secure-boot"},
		},
		{
			name:       "lets libvirt select firmware when none is found",
			candidates: nil,
			want: []string{
				`<os firmware="efi">`,
				`<feature enabled="no" name="secure-boot"/>`,
			},
			notWant: []string{"<loader", "<nvram"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			renderer, err := NewTemplateRenderer()
			if err != nil {
				t.Fatalf("failed to create template renderer: %v", err)
			}
			renderer.ovmfCandidates = tt.candidates

			xml, err := renderer.RenderVM(vbmctlapi.VMConfig{Name: "test-vm"}, "/pool")
			if err != nil {
				t.Fatalf("RenderVM() failed: %v", err)
			}
			for _, w := range tt.want {
				if !strings.Contains(xml, w) {
					t.Errorf("rendered XML does not contain %q:\n%s", w, xml)
				}
			}
			for _, nw := range tt.notWant {
				if strings.Contains(xml, nw) {
					t.Errorf("rendered XML unexpectedly contains %q:\n%s", nw, xml)
				}
			}
			for _, w := range []string{"<type arch='x86_64' machine='q35'>hvm</type>", "<boot dev='hd'/>"} {
				if !strings.Contains(xml, w) {
					t.Errorf("rendered XML does not contain %q:\n%s", w, xml)
				}
			}
		})
	}
}
