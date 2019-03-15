/*
Package resources provides the ability to retrieve resources through the Gnocchi API.

Example of Listing resources

	resourceType: "instance",
	listOpts := resources.ListOpts{
		Details: True,
	}

	allPages, err := resources.List(gnocchiClient, listOpts, resourceType).AllPages()
	if err != nil {
		panic(err)
	}

	allResources, err := resources.ExtractResources(allPages)
	if err != nil {
		panic(err)
	}

	for _, resource := range allResources {
		fmt.Printf("%+v\n", resource)
	}

Example of Getting a resource

	resourceID = "23d5d3f7-9dfa-4f73-b72b-8b0b0063ec55"
	resourceType = "generic"
	resource, err := resources.Get(gnocchiClient, resourceType, resourceID).Extract()
	if err != nil {
		panic(err)
	}

Example of Creating a resource without a metric

	createOpts := resources.CreateOpts{
		ID: "23d5d3f7-9dfa-4f73-b72b-8b0b0063ec55",
	}
	resourceType = "generic"
	resource, err := resources.Create(gnocchiClient, resourceType, createOpts).Extract()
	if err != nil {
		panic(err)
	}

Example of Creating a resource with links to some existing metrics with a starting timestamp of the resource

	startedAt := time.Date(2018, 1, 4, 10, 0, 0, 0, time.UTC)
	createOpts := resources.CreateOpts{
		ID: "23d5d3f7-9dfa-4f73-b72b-8b0b0063ec55",
		ProjectID: "4154f088-8333-4e04-94c4-1155c33c0fc9",
		StartedAt: &startedAt,
		Metrics: map[string]interface{}{
			"disk.read.bytes.rate": "ed1bb76f-6ccc-4ad2-994c-dbb19ddccbae",
			"disk.write.bytes.rate": "0a2da84d-4753-43f5-a65f-0f8d44d2766c",
		},
	}
	resourceType = "compute_instance_disk"
	resource, err := resources.Create(gnocchiClient, resourceType, createOpts).Extract()
	if err != nil {
		panic(err)
	}

Example of Creating a resource and a metric a the same time

	createOpts := resources.CreateOpts{
		ID: "23d5d3f7-9dfa-4f73-b72b-8b0b0063ec55",
		ProjectID: "4154f088-8333-4e04-94c4-1155c33c0fc9",
		UserID: "bd5874d6-6662-4b24-a9f01c128871e4ac",
		Metrics: map[string]interface{}{
			"cpu.delta": map[string]string{
				"archive_policy_name": "medium",
			},
		},
	}
	resourceType = "compute_instance"
	resource, err := resources.Create(gnocchiClient, resourceType, createOpts).Extract()
	if err != nil {
		panic(err)
	}

Example of Updating a resource

	updateOpts := resources.UpdateOpts{
		ProjectID: "4154f08883334e0494c41155c33c0fc9",
	}
	resourceType = "generic"
	resourceID = "23d5d3f7-9dfa-4f73-b72b-8b0b0063ec55"
	resource, err := resources.Update(gnocchiClient, resourceType, resourceID, updateOpts).Extract()
	if err != nil {
		panic(err)
	}

Example of Updating a resource and associating an existing metric to it

	endedAt := time.Date(2018, 1, 16, 12, 0, 0, 0, time.UTC)
	metrics := map[string]interface{}{
			"disk.write.bytes.rate": "0a2da84d-4753-43f5-a65f-0f8d44d2766c",
	}
	updateOpts := resources.UpdateOpts{
		EndedAt: &endedAt,
		Metrics: &metrics,
	}
	resourceType = "compute_instance_disk"
	resourceID = "23d5d3f7-9dfa-4f73-b72b-8b0b0063ec55"
	resource, err := resources.Update(gnocchiClient, resourceType, resourceID, updateOpts).Extract()
	if err != nil {
		panic(err)
	}

Example of Updating a resource and creating an associated metric at the same time

	metrics := map[string]interface{}{
		"cpu.delta": map[string]string{
			"archive_policy_name": "medium",
		},
	}
	updateOpts := resources.UpdateOpts{
		Metrics: &metrics,
	}
	resourceType = "compute_instance"
	resourceID = "23d5d3f7-9dfa-4f73-b72b-8b0b0063ec55"
	resource, err := resources.Update(gnocchiClient, resourceType, resourceID, updateOpts).Extract()
	if err != nil {
		panic(err)
	}

Example of Deleting a Gnocchi resource

	resourceID := "23d5d3f7-9dfa-4f73-b72b-8b0b0063ec55"
	err := resources.Delete(gnocchiClient, resourceType, resourceID).ExtractErr()
	if err != nil {
		panic(err)
	}
*/
package resources
