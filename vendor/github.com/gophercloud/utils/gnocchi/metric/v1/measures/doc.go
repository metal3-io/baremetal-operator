/*
Package measures provides the ability to retrieve measures through the Gnocchi API.

Example of Listing measures of a known metric

	startTime := time.Date(2018, 1, 4, 10, 0, 0, 0, time.UTC)
	metricID := "9e5a6441-1044-4181-b66e-34e180753040"
	listOpts := measures.ListOpts{
		Resample: "2h",
		Granularity: "1h",
		Start: &startTime,
	}
	allPages, err := measures.List(gnocchiClient, metricID, listOpts).AllPages()
	if err != nil {
		panic(err)
	}

	allMeasures, err := measures.ExtractMeasures(allPages)
	if err != nil {
		panic(err)
	}

	for _, measure := range allMeasures {
		fmt.Printf("%+v\n", measure)
	}

Example of Creating measures inside a single metric

	createOpts := measures.CreateOpts{
		Measures: []measures.MeasureOpts{
			{
				Timestamp: time.Date(2018, 1, 18, 12, 31, 0, 0, time.UTC),
				Value:     101.2,
			},
			{
				Timestamp: time.Date(2018, 1, 18, 14, 32, 0, 0, time.UTC),
				Value:     102,
			},
		},
	}
	metricID := "9e5a6441-1044-4181-b66e-34e180753040"
	if err := measures.Create(gnocchiClient, metricID, createOpts).ExtractErr(); err != nil {
		panic(err)
	}

Example of Creating measures inside different metrics via metric ID references in a single request

	currentTimestamp := time.Now().UTC()
	pastHourTimestamp := currentTimestamp.Add(-1 * time.Hour)
	createOpts := measures.BatchCreateMetricsOpts{
		{
			ID: "777a01d6-4694-49cb-b86a-5ba9fd4e609e",
			Measures: []measures.MeasureOpts{
				{
					Timestamp: &currentTimestamp,
					Value:     200,
				},
				{
					Timestamp: &pastHourTimestamp,
					Value:     300,
				},
			},
		},
		{
			ID: "6dbc97c5-bfdf-47a2-b184-02e7fa348d21",
			Measures: []measures.MeasureOpts{
				{
					Timestamp: &currentTimestamp,
					Value:     111,
				},
				{
					Timestamp: &pastHourTimestamp,
					Value:     222,
				},
			},
		},
	}
	if err := measures.BatchCreateMetrics(gnocchiClient, createOpts).ExtractErr(); err != nil {
		panic(err)
	}

Example of Creating measures inside different metrics via metric names and resource IDs references of that metrics in a single request

	currentTimestamp := time.Now().UTC()
	pastHourTimestamp := currentTimestamp.Add(-1 * time.Hour)
	createOpts := measures.BatchCreateResourcesMetricsOpts{
		CreateMetrics: true,
		BatchResourcesMetrics: []measures.BatchResourcesMetricsOpts{
			{
				ResourceID: "75274f99-faf6-4112-a6d5-2794cb07c789",
				ResourcesMetrics: []measures.ResourcesMetricsOpts{
					{
						MetricName:        "network.incoming.bytes.rate",
						ArchivePolicyName: "high",
						Unit:              "B/s",
						Measures: []measures.MeasureOpts{
							{
								Timestamp: &currentTimestamp,
								Value:     1562.82,
							},
							{
								Timestamp: &pastHourTimestamp,
								Value:     768.1,
							},
						},
					},
					{
						MetricName:        "network.outgoing.bytes.rate",
						ArchivePolicyName: "high",
						Unit:              "B/s",
						Measures: []measures.MeasureOpts{
							{
								Timestamp: &currentTimestamp,
								Value:     273,
							},
							{
								Timestamp: &pastHourTimestamp,
								Value:     3141.14,
							},
						},
					},
				},
			},
			{
				ResourceID: "23d5d3f7-9dfa-4f73-b72b-8b0b0063ec55",
				ResourcesMetrics: []measures.ResourcesMetricsOpts{
					{
						MetricName:        "disk.write.bytes.rate",
						ArchivePolicyName: "low",
						Unit:              "B/s",
						Measures: []measures.MeasureOpts{
							{
								Timestamp: &currentTimestamp,
								Value:     1237,
							},
							{
								Timestamp: &pastHourTimestamp,
								Value:     132.12,
							},
						},
					},
				},
			},
		},
	}
	if err := measures.BatchCreateResourcesMetrics(gnocchiClient, createOpts).ExtractErr(); err != nil {
		panic(err)
	}
*/
package measures
