package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/metal3-io/baremetal-operator/cmd/make-bm-worker/templates"
)

func main() {
	var username = flag.String("user", "", "username for BMC")
	var password = flag.String("password", "", "password for BMC")
	var bmcAddress = flag.String("address", "", "address URL for BMC")
	var hardwareProfile = flag.String("hardwareprofile", "", "hardwareProfile to be used")
	var macAddress = flag.String("boot-mac", "", "boot-mac for bootMACAddress")
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

	template := templates.Template{
		Name:             strings.Replace(hostName, "_", "-", -1),
		BMCAddress:       *bmcAddress,
		Username:         *username,
		Password:         *password,
		HardwareProfile:  *hardwareProfile,
		BootMacAddress:    *macAddress,
		Machine:          strings.TrimSpace(*machine),
		MachineNamespace: strings.TrimSpace(*machineNamespace),
	}
	if *verbose {
		fmt.Fprintf(os.Stderr, "%v", template)
	}

	result, err := template.Render()
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %s\n", err)
		os.Exit(1)
	} else {
		fmt.Fprintf(os.Stdout, result)
	}
}

