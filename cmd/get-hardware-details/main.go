// get-hardware-details is a tool that can be used to convert raw Ironic introspection data into the HardwareDetails
// type used by Metal3.
package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack/baremetalintrospection/httpbasic"
	"github.com/gophercloud/gophercloud/openstack/baremetalintrospection/noauth"
	"github.com/gophercloud/gophercloud/openstack/baremetalintrospection/v1/introspection"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner/ironic/hardwaredetails"
)

func main() {
	authStrategy := os.Getenv("IRONIC_AUTH_STRATEGY")
	if authStrategy == "http_basic" {
		if len(os.Args) != 5 {
			fmt.Println("Usage: get-hardware-details <inspector URI> <inspector User> <inspector Password> <node UUID>")
			return
		}
	} else {
		if len(os.Args) != 3 {
			fmt.Println("Usage: get-hardware-details <inspector URI> <node UUID>")
			return
		}
	}

	var inspector *gophercloud.ServiceClient
	var nodeID string
	if authStrategy == "http_basic" {
		client, err := httpbasic.NewBareMetalIntrospectionHTTPBasic(httpbasic.EndpointOpts{
			IronicInspectorEndpoint:     os.Args[1],
			IronicInspectorUser:         os.Args[2],
			IronicInspectorUserPassword: os.Args[3],
		})
		if err != nil {
			fmt.Printf("could not get inspector client: %s", err)
			os.Exit(1)
		}
		inspector = client
		nodeID = os.Args[4]
	} else {
		client, err := noauth.NewBareMetalIntrospectionNoAuth(
			noauth.EndpointOpts{
				IronicInspectorEndpoint: os.Args[1],
			})
		if err != nil {
			fmt.Printf("could not get inspector client: %s", err)
			os.Exit(1)
		}
		inspector = client
		nodeID = os.Args[2]
	}

	introData := introspection.GetIntrospectionData(inspector, nodeID)
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
