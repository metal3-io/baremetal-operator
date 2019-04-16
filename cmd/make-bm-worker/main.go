package main

import (
	"encoding/base64"
	"flag"
	"fmt"
	"os"
	"strings"
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
apiVersion: metalkube.org/v1alpha1
kind: BareMetalHost
metadata:
  name: {{ .Name }}
spec:
  online: true
  bmc:
    address: {{ .BMCAddress }}
    credentialsName: {{ .Name }}-bmc-secret
{{- if .WithMachine }}
  machineRef:
    name: {{ .Machine }}
    namespace: {{ .MachineNamespace }}
{{- end }}
`

// TemplateArgs holds the arguments to pass to the template.
type TemplateArgs struct {
	Name             string
	BMCAddress       string
	EncodedUsername  string
	EncodedPassword  string
	WithMachine      bool
	Machine          string
	MachineNamespace string
}

func encodeToSecret(input string) string {
	return base64.StdEncoding.EncodeToString([]byte(input))
}

func main() {
	var username = flag.String("user", "", "username for BMC")
	var password = flag.String("password", "", "password for BMC")
	var bmcAddress = flag.String("address", "", "address URL for BMC")
	var verbose = flag.Bool("v", false, "turn on verbose output")
	var machine = flag.String(
		"machine", "", "specify name of a related, existing, machine to link")
	var machineNamespace = flag.String(
		"machine-namespace", "", "specify namespace of a related, existing, machine to link")

	flag.Parse()

	hostName := flag.Arg(0)
	if hostName == "" {
		fmt.Fprintf(os.Stderr, "Missing name argument\n")
		os.Exit(1)
	}
	if *username == "" {
		fmt.Fprintf(os.Stderr, "Missing -user argument\n")
		os.Exit(1)
	}
	if *password == "" {
		fmt.Fprintf(os.Stderr, "Missing -password argument\n")
		os.Exit(1)
	}
	if *bmcAddress == "" {
		fmt.Fprintf(os.Stderr, "Missing -address argument\n")
		os.Exit(1)
	}

	args := TemplateArgs{
		Name:             strings.Replace(hostName, "_", "-", -1),
		BMCAddress:       *bmcAddress,
		EncodedUsername:  encodeToSecret(*username),
		EncodedPassword:  encodeToSecret(*password),
		Machine:          strings.TrimSpace(*machine),
		MachineNamespace: strings.TrimSpace(*machineNamespace),
	}
	if args.Machine != "" {
		args.WithMachine = true
	}
	if *verbose {
		fmt.Fprintf(os.Stderr, "%v", args)
	}

	t := template.Must(template.New("yaml_out").Parse(templateBody))
	err := t.Execute(os.Stdout, args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %s\n", err)
	}
}
