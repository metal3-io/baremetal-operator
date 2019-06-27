/*
Package status provides the ability to retrieve Gnocchi status through the Gnocchi API.

Example of Getting status

	details := true

	getOpts := status.GetOpts{
		Details: &details,
	}

	gnocchiStatus, err := status.Get(client, getOpts).Extract()
	if err != nil {
		panic(err)
	}

	fmt.Printf("%+v\n", gnocchiStatus)
*/
package status
