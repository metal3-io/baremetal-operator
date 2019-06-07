package templates

import (
	"bytes"
	"encoding/base64"
	"text/template"
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
  online: true
{{- if .HardwareProfile }}
  hardwareProfile: {{ .HardwareProfile }}
{{- end }}
{{- if .BootMacAddress }}
  bootMACAddress: {{ .BootMacAddress }}
{{- end }}
  bmc:
    address: {{ .BMCAddress }}
    credentialsName: {{ .Name }}-bmc-secret
{{- if .Machine }}
  machineRef:
    name: {{ .Machine }}
    namespace: {{ .MachineNamespace }}
{{- end }}
`

// Template holds the arguments to pass to the template.
type Template struct {
	Name             string
	BMCAddress       string
	Username         string
	Password         string
	HardwareProfile  string
	BootMacAddress   string
	Machine          string
	MachineNamespace string
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
	tmpl := template.Must(template.New("yaml_out").Parse(templateBody))
	err := tmpl.Execute(buf, t)
	return buf.String(), err
}

