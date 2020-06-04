// get-hardware-details is a tool that can be used to convert raw Ironic introspection data into the HardwareDetails
// type used by Metal3.
package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/gophercloud/gophercloud/openstack/baremetalintrospection/noauth"
	"github.com/gophercloud/gophercloud/openstack/baremetalintrospection/v1/introspection"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner/ironic/hardwaredetails"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Println("Usage: get-hardware-details <inspector URI> <node UUID>")
		return
	}

	inspector, err := noauth.NewBareMetalIntrospectionNoAuth(
		noauth.EndpointOpts{
			IronicInspectorEndpoint: os.Args[1],
		})
	if err != nil {
		fmt.Printf("could not get inspector client: %s", err)
		os.Exit(1)
	}

	introData := introspection.GetIntrospectionData(inspector, os.Args[2])
	data, err := introData.Extract()
	if err != nil {
		fmt.Printf("could not get introspection data: %s", err)
		os.Exit(1)
	}

	json, err := json.MarshalIndent(hardwaredetails.GetHardwareDetails(data), "", "\t")
	if err != nil {
		fmt.Printf("could not convert introspection data: %s", err)
		os.Exit(1)
	}

	fmt.Println(string(json))
}
