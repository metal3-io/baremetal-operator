package main

import (
	"encoding/json"
	"encoding/xml"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"text/template"
)

const (
	instanceImageSource      = "http://172.22.0.1/images/redhat-coreos-maipo-latest.qcow2"
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

// MAC is a hardware address for a NIC
type MAC struct {
	XMLName xml.Name `xml:"mac"`
	Address string   `xml:"address,attr"`
}

// Source is the network to which the interface is attached
type Source struct {
	XMLName xml.Name `xml:"source"`
	Bridge  string   `xml:"bridge,attr"`
}

// Interface is one NIC
type Interface struct {
	XMLName xml.Name `xml:"interface"`
	MAC     MAC      `xml:"mac"`
	Source  Source   `xml:"source"`
}

// Domain is the main tag for the XML document
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
  username: YWRtaW4=
  password: cGFzc3dvcmQ=

---
apiVersion: metalkube.org/v1alpha1
kind: BareMetalHost
metadata:
  name: {{ .Domain }}
spec:
  online: true
  bmc:
    address: libvirt://192.168.122.1:{{ .BMCPort }}/
    credentialsName: {{ .Domain }}-bmc-secret
  bootMACAddress: {{ .MAC }}
  userData:
    namespace: openshift-machine-api
    name: worker-user-data
  image:
    url: "http://172.22.0.1/images/redhat-coreos-maipo-latest.qcow2"
    checksum: "{{ .Checksum }}"
`

// TemplateArgs holds the arguments to pass to the template.
type TemplateArgs struct {
	Domain   string
	MAC      string
	BMCPort  int
	Checksum string
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
// controller
type VBMC struct {
	Port int    `json:"Port"`
	Name string `json:"Domain name"`
}

func main() {
	var provisionNet = flag.String(
		"provision-net", "provisioning", "use the MAC on this network")
	var verbose = flag.Bool("v", false, "turn on verbose output")
	var desiredMAC string

	flag.Parse()

	virshDomain := flag.Arg(0)
	if virshDomain == "" {
		fmt.Fprintf(os.Stderr, "Missing domain argument\n")
		os.Exit(1)
	}

	if *verbose {
		fmt.Printf("net: %s domain: %s\n", *provisionNet, virshDomain)
	}

	// Figure out the MAC for the VM
	virshOut, err := exec.Command("sudo", "virsh", "dumpxml", virshDomain).Output()
	if err != nil {
		fmt.Fprintf(os.Stderr,
			"ERROR: Could not get details of domain %s: %s\n",
			virshDomain, err)
		os.Exit(1)
	}

	domainResult := Domain{}
	xml.Unmarshal([]byte(virshOut), &domainResult)
	if *verbose {
		fmt.Printf("%v\n", domainResult)
	}

	for _, iface := range domainResult.Interfaces {
		if *verbose {
			fmt.Printf("%v\n", iface)
		}
		if iface.Source.Bridge == *provisionNet {
			desiredMAC = iface.MAC.Address
		}
	}
	if *verbose {
		fmt.Printf("Using MAC: %s\n", desiredMAC)
	}
	if desiredMAC == "" {
		fmt.Fprintf(os.Stderr, "Could not find MAC for %s on network %s\n",
			virshDomain, *provisionNet)
		os.Exit(1)
	}

	vbmcOut, err := exec.Command(
		"vbmc", "list", "-f", "json", "-c", "Domain name", "-c", "Port",
	).Output()
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: Could not get details of vbmc: %s\n", err)
		os.Exit(1)
	}

	var vbmcResult []VBMC
	json.Unmarshal([]byte(vbmcOut), &vbmcResult)
	nameToPort := make(map[string]int)
	for _, vbmc := range vbmcResult {
		if *verbose {
			fmt.Printf("VBMC: %s: %d\n", vbmc.Name, vbmc.Port)
		}
		nameToPort[vbmc.Name] = vbmc.Port
	}

	resp, err := http.Get(instanceImageChecksumURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: Could not get image checksum: %s\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()
	checksum, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: Could not get image checksum: %s\n", err)
		os.Exit(1)
	}

	args := TemplateArgs{
		Domain:   strings.Replace(virshDomain, "_", "-", -1),
		MAC:      desiredMAC,
		BMCPort:  nameToPort[virshDomain],
		Checksum: strings.TrimSpace(string(checksum)),
	}
	t := template.Must(template.New("yaml_out").Parse(templateBody))
	err = t.Execute(os.Stdout, args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %s\n", err)
	}
}
