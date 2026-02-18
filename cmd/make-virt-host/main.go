package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"flag"
	"log"
	"os"
	"os/exec"
	"strings"

	"github.com/google/safetext/yamltemplate"
)

const (
	instanceImageSource      = "http://172.22.0.1/images/rhcos-ootpa-latest.qcow2"
	instanceImageChecksumURL = instanceImageSource + ".md5sum"
)

/* PARTIAL EXAMPLE XML OUTPUT:
   <interface type='bridge'>
     <mac address='00:1a:74:74:e5:cb'/>
     <source bridge='provisioning'/>
     <target dev='vnet10'/>
     <model type='virtio'/>
     <alias name='net0'/>
     <address type='pci' domain='0x0000' bus='0x00' slot='0x03' function='0x0'/>
   </interface>
*/

// MAC is a hardware address for a NIC.
type MAC struct {
	XMLName xml.Name `xml:"mac"`
	Address string   `xml:"address,attr"`
}

// Source is the network to which the interface is attached.
type Source struct {
	XMLName xml.Name `xml:"source"`
	Bridge  string   `xml:"bridge,attr"`
}

// Interface is one NIC.
type Interface struct {
	XMLName xml.Name `xml:"interface"`
	MAC     MAC      `xml:"mac"`
	Source  Source   `xml:"source"`
}

// Domain is the main tag for the XML document.
type Domain struct {
	Interfaces []Interface `xml:"devices>interface"`
}

var templateBody = `---
apiVersion: v1
kind: Secret
metadata:
  name: {{ .Domain }}-bmc-secret
type: Opaque
data:
  username: {{ .B64UserName }}
  password: {{ .B64Password }}

---
apiVersion: metal3.io/v1alpha1
kind: BareMetalHost
metadata:
  name: {{ .Domain }}
spec:
  online: true
  bmc:
    address: libvirt://192.168.122.1:{{ .BMCPort }}/
    credentialsName: {{ .Domain }}-bmc-secret
  bootMACAddress: {{ .MAC }}
{{- if .WithImage }}
  userData:
    namespace: openshift-machine-api
    name: worker-user-data
  image:
    url: "{{ .ImageSourceURL }}"
    checksum: "{{ .Checksum }}"
{{- end }}{{ if .Consumer }}
  consumerRef:
    name: {{ .Consumer }}
    namespace: {{ .ConsumerNamespace }}
{{ end }}
`

// TemplateArgs holds the arguments to pass to the template.
type TemplateArgs struct {
	Domain            string
	B64UserName       string
	B64Password       string
	MAC               string
	BMCPort           int
	Checksum          string
	ImageSourceURL    string
	Consumer          string
	ConsumerNamespace string
	WithImage         bool
}

/*
vbmc list -f json -c 'Domain name' -c Port
[
  {
    "Port": 6230,
    "Domain name": "openshift_master_0"
  }, ...
]
*/

// VBMC holds the parameters for describing a virtual machine
// controller.
type VBMC struct {
	//nolint:tagliatelle
	Port int `json:"Port"`
	//nolint:tagliatelle
	Name string `json:"Domain name"`
}

func main() {
	var provisionNet = flag.String(
		"provision-net", "provisioning", "use the MAC on this network")
	var consumer = flag.String(
		"consumer", "", "specify name of a related, existing, consumer to link")
	var consumerNamespace = flag.String(
		"consumer-namespace", "", "specify namespace of a related, existing, consumer to link")
	var verbose = flag.Bool("v", false, "turn on verbose output")
	var withImage = flag.Bool("image", false, "include image settings for immediate provisioning")
	var desiredMAC string
	var userName = flag.String(
		"user", "admin", "Specify an username for vBMC")
	var password = flag.String(
		"password", "password", "Specify password for vBMC")

	flag.Parse()

	virshDomain := flag.Arg(0)
	if virshDomain == "" {
		log.Fatalln("Missing domain argument")
	}

	if *verbose {
		log.Printf("net: %s domain: %s\n", *provisionNet, virshDomain)
	}

	// Figure out the MAC for the VM
	virshOut, err := exec.CommandContext(context.Background(), "sudo", "virsh", "dumpxml", virshDomain).Output() // #nosec
	if err != nil {
		log.Fatalf("ERROR: Could not get details of domain %s: %s\n",
			virshDomain, err)
	}

	domainResult := Domain{}
	err = xml.Unmarshal(virshOut, &domainResult)
	if err != nil {
		log.Fatalf("ERROR: Could not unmarshal details of domain %s: %s\n",
			virshDomain, err)
	}

	if *verbose {
		log.Printf("%v\n", domainResult)
	}

	for _, iface := range domainResult.Interfaces {
		if *verbose {
			log.Printf("%v\n", iface)
		}
		if iface.Source.Bridge == *provisionNet {
			desiredMAC = iface.MAC.Address
		}
	}

	// Base64 encoding for user and password
	b64UserName := base64.StdEncoding.EncodeToString([]byte(*userName))
	b64Password := base64.StdEncoding.EncodeToString([]byte(*password))

	if *verbose {
		log.Printf("Using MAC: %s\n", desiredMAC)
	}
	if desiredMAC == "" {
		log.Fatalf("Could not find MAC for %s on network %s\n",
			virshDomain, *provisionNet)
	}

	vbmcOut, err := exec.CommandContext(context.Background(),
		"vbmc", "list", "-f", "json", "-c", "Domain name", "-c", "Port",
	).Output()
	if err != nil {
		log.Fatalf("ERROR: Could not get details of vbmc: %s\n", err)
	}

	var vbmcResult []VBMC
	err = json.Unmarshal(vbmcOut, &vbmcResult)
	if err != nil {
		log.Fatalf("ERROR: Could not unmarshal details of vbmc: %s\n", err)
	}

	nameToPort := make(map[string]int)
	for _, vbmc := range vbmcResult {
		if *verbose {
			log.Printf("VBMC: %s: %d\n", vbmc.Name, vbmc.Port)
		}
		nameToPort[vbmc.Name] = vbmc.Port
	}

	args := TemplateArgs{
		Domain:            strings.ReplaceAll(virshDomain, "_", "-"),
		B64UserName:       b64UserName,
		B64Password:       b64Password,
		MAC:               desiredMAC,
		BMCPort:           nameToPort[virshDomain],
		WithImage:         *withImage,
		Checksum:          instanceImageChecksumURL,
		ImageSourceURL:    strings.TrimSpace(instanceImageSource),
		Consumer:          strings.TrimSpace(*consumer),
		ConsumerNamespace: strings.TrimSpace(*consumerNamespace),
	}
	t := yamltemplate.Must(yamltemplate.New("yaml_out").Parse(templateBody))
	err = t.Execute(os.Stdout, args)
	if err != nil {
		log.Printf("ERROR: %s\n", err)
	}
}
