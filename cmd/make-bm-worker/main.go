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
	var disableCertificateVerification = flag.Bool("disableCertificateVerification", false, "will skip certificate validation when true")
	var hardwareProfile = flag.String("hardwareprofile", "", "hardwareProfile to be used")
	var macAddress = flag.String("boot-mac", "", "boot-mac for bootMACAddress")
	var bootMode = flag.String("boot-mode", "", "boot-mode for host (UEFI, UEFISecureBoot or legacy)")
	var verbose = flag.Bool("v", false, "turn on verbose output")
	var consumer = flag.String(
		"consumer", "", "specify name of a related, existing, consumer to link")
	var consumerNamespace = flag.String(
		"consumer-namespace", "", "specify namespace of a related, existing, consumer to link")
	var automatedCleaningMode = flag.String(
		"automatedCleaningMode", "", "automatic cleaning mode for host (metadata or disabled)")
	var imageURL = flag.String("image-url", "", "url for the image")
	var imageChecksum = flag.String("image-checksum", "", "checksum for the image")
	var imageChecksumType = flag.String(
		"image-checksum-type", "", "checksum algorithm for the image (md5, sha256 or sha512)")
	var imageFormat = flag.String(
		"image-format", "", "format of the image (raw, qcow2, vdi, vmdk, or live-iso)")

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

	if *bootMode != "" && *bootMode != "UEFI" && *bootMode != "UEFISecureBoot" && *bootMode != "legacy" {
		fmt.Fprintf(os.Stderr, "Invalid boot mode %q, use \"UEFI\", \"UEFISecureBoot\" or \"legacy\"\n", *bootMode)
		os.Exit(1)
	}

	if *automatedCleaningMode != "" && *automatedCleaningMode != "metadata" && *automatedCleaningMode != "disabled" {
		fmt.Fprintf(os.Stderr, "Invalid automatic cleaning mode %q, use \"metadata\" or \"disabled\"\n", *automatedCleaningMode)
		os.Exit(1)
	}

	switch *imageChecksumType {
	case "", "md5", "sha256", "sha512", "auto":
	default:
		fmt.Fprintf(os.Stderr, "Invalid image checksum type %q, use \"md5\", \"sha256\", \"sha512\" or \"auto\"\n", *imageChecksumType)
		os.Exit(1)
	}

	switch *imageFormat {
	case "", "raw", "qcow2", "vdi", "vmdk", "live-iso":
	default:
		fmt.Fprintf(os.Stderr, "Invalid image format %q, use \"raw\", \"qcow2\", \"vdi\", \"vmdk\" or \"live-iso\"\n", *imageFormat)
		os.Exit(1)
	}

	template := templates.Template{
		Name:                           strings.Replace(hostName, "_", "-", -1),
		BMCAddress:                     *bmcAddress,
		DisableCertificateVerification: *disableCertificateVerification,
		Username:                       *username,
		Password:                       *password,
		HardwareProfile:                *hardwareProfile,
		BootMacAddress:                 *macAddress,
		Consumer:                       strings.TrimSpace(*consumer),
		ConsumerNamespace:              strings.TrimSpace(*consumerNamespace),
		AutomatedCleaningMode:          *automatedCleaningMode,
		ImageURL:                       *imageURL,
		ImageChecksum:                  *imageChecksum,
		ImageChecksumType:              *imageChecksumType,
		ImageFormat:                    *imageFormat,
	}
	if bootMode != nil {
		template.BootMode = *bootMode
	}
	if *verbose {
		fmt.Fprintf(os.Stderr, "%v", template)
	}

	result, err := template.Render()
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %s\n", err)
		os.Exit(1)
	}

	fmt.Fprint(os.Stdout, result)
}
