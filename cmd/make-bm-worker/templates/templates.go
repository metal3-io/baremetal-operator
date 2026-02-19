package templates

import (
	"bytes"
	"encoding/base64"

	"github.com/google/safetext/yamltemplate"
)

var templateBody = `---
apiVersion: v1
kind: Secret
metadata:
  name: {{ .Name }}-bmc-secret
type: Opaque
data:
  username: {{ .EncodedUsername }}
  password: {{ .EncodedPassword }}

---
apiVersion: metal3.io/v1alpha1
kind: BareMetalHost
metadata:
  name: {{ .Name }}
spec:
{{- if .AutomatedCleaningMode }}
  automatedCleaningMode: {{ .AutomatedCleaningMode }}
{{- end }}
  online: true
{{- if .HardwareProfile }}
  hardwareProfile: {{ .HardwareProfile }}
{{- end }}
{{- if .BootMacAddress }}
  bootMACAddress: {{ .BootMacAddress }}
{{- end }}
{{- if .BootMode }}
  bootMode: {{ .BootMode }}
{{- end }}
  bmc:
    address: {{ .BMCAddress }}
    credentialsName: {{ .Name }}-bmc-secret
{{- if .DisableCertificateVerification }}
    disableCertificateVerification: true
{{- end}}
{{- if .Consumer }}
  consumerRef:
    name: {{ .Consumer }}
{{- if .ConsumerNamespace }}
    namespace: {{ .ConsumerNamespace }}
{{- end }}
{{- end }}
{{- if .ImageURL }}
  image:
{{- if .ImageChecksum }}
    checksum: {{ .ImageChecksum}}
{{- end}}
{{- if .ImageChecksumType }}
    checksumType: {{ .ImageChecksumType}}
{{- end}}
{{- if .ImageFormat }}
    format: {{ .ImageFormat}}
{{- end}}
    url: {{ .ImageURL}}
{{- end}}
`

// Template holds the arguments to pass to the template.
type Template struct {
	Name                           string
	BMCAddress                     string
	DisableCertificateVerification bool
	Username                       string
	Password                       string //nolint:gosec
	HardwareProfile                string
	BootMacAddress                 string
	BootMode                       string
	Consumer                       string
	ConsumerNamespace              string
	AutomatedCleaningMode          string
	ImageURL                       string
	ImageChecksum                  string
	ImageChecksumType              string
	ImageFormat                    string
}

// EncodedUsername returns the username in the format needed to store
// it in a Secret.
func (t Template) EncodedUsername() string {
	return encodeToSecret(t.Username)
}

// EncodedPassword returns the password in the format needed to store
// it in a Secret.
func (t Template) EncodedPassword() string {
	return encodeToSecret(t.Password)
}

func encodeToSecret(input string) string {
	return base64.StdEncoding.EncodeToString([]byte(input))
}

// Render returns the string from the template or an error if there
// was a problem rendering it.
func (t Template) Render() (string, error) {
	buf := new(bytes.Buffer)
	tmpl := yamltemplate.Must(yamltemplate.New("yaml_out").Parse(templateBody))
	err := tmpl.Execute(buf, t)
	return buf.String(), err
}
