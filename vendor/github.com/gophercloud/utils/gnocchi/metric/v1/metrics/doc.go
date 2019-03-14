/*
Package metrics provides the ability to retrieve metrics through the Gnocchi API.

Example of Listing metrics

	listOpts := metrics.ListOpts{
		Limit: 25,
	}

	allPages, err := metrics.List(gnocchiClient, listOpts).AllPages()
	if err != nil {
		panic(err)
	}

	allMetrics, err := metrics.ExtractMetrics(allPages)
	if err != nil {
		panic(err)
	}

	for _, metric := range allMetrics {
		fmt.Printf("%+v\n", metric)
	}

Example of Getting a metric

	metricID = "9e5a6441-1044-4181-b66e-34e180753040"
	metric, err := metrics.Get(gnocchiClient, metricID).Extract()
	if err != nil {
		panic(err)
	}

Example of Creating a metric and link it to an existing archive policy

	createOpts := metrics.CreateOpts{
		ArchivePolicyName: "low",
		Name: "network.incoming.packets.rate",
		Unit: "packet/s",
	}
	metric, err := metrics.Create(gnocchiClient, createOpts).Extract()
	if err != nil {
		panic(err)
	}

Example of Creating a metric without an archive policy, assuming that Gnocchi has the needed
archive policy rule and can assign the policy automatically

	createOpts := metrics.CreateOpts{
		ResourceID: "1f3a0724-1807-4bd1-81f9-ee18c8ff6ccc",
		Name: "memory.usage",
		Unit: "MB",
	}
	metric, err := metrics.Create(gnocchiClient, createOpts).Extract()
	if err != nil {
		panic(err)
	}

Example of Deleting a Gnocchi metric

	metricID := "01b2953e-de74-448a-a305-c84440697933"
	err := metrics.Delete(gnocchiClient, metricID).ExtractErr()
	if err != nil {
		panic(err)
	}
*/
package metrics
