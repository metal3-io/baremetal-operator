/*
Package archivepolicies provides the ability to retrieve archive policies
through the Gnocchi API.

Example of Listing archive policies

	allPages, err := archivepolicies.List(gnocchiClient).AllPages()
	if err != nil {
		panic(err)
	}

	allArchivePolicies, err := archivepolicies.ExtractArchivePolicies(allPages)
	if err != nil {
		panic(err)
	}

	for _, archivePolicy := range allArchivePolicies {
		fmt.Printf("%+v\n", archivePolicy)
	}

Example of Getting an archive policy

	archivePolicyName = "my_policy"
	archivePolicy, err := archivepolicies.Get(gnocchiClient, archivePolicyName).Extract()
	if err != nil {
	  panic(err)
	}

Example of Creating an archive policy

  createOpts := archivepolicies.CreateOpts{
    BackWindow: 31,
    AggregationMethods: []string{
      "sum",
      "mean",
      "count",
    },
    Definition: []archivepolicies.ArchivePolicyDefinitionOpts{
      {
        Granularity: "1:00:00",
        TimeSpan:    "90 days, 0:00:00",
      },
      {
        Granularity: "1 day, 0:00:00",
        TimeSpan:    "100 days, 0:00:00",
      },
    },
    Name: "test_policy",
  }
  archivePolicy, err := archivepolicies.Create(gnocchiClient, createOpts).Extract()
  if err != nil {
    panic(err)
  }

Example of Updating an archive policy

  updateOpts := archivepolicies.UpdateOpts{
    Definition: []archivepolicies.ArchivePolicyDefinitionOpts{
      {
        Granularity: "12:00:00",
        TimeSpan:    "30 days, 0:00:00",
      },
      {
        Granularity: "1 day, 0:00:00",
        TimeSpan:    "90 days, 0:00:00",
      },
    },
  }
  archivePolicy, err := archivepolicies.Update(gnocchiClient, "test_policy", updateOpts).Extract()
  if err != nil {
    panic(err)
  }

Example of Deleting a Gnocchi archive policy

  err := archivepolicies.Delete(gnocchiClient, "test_policy").ExtractErr()
  if err != nil {
    panic(err)
  }
*/
package archivepolicies
