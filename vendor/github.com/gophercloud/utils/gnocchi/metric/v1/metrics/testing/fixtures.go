package testing

import (
	"github.com/gophercloud/utils/gnocchi/metric/v1/archivepolicies"
	"github.com/gophercloud/utils/gnocchi/metric/v1/metrics"
)

// MetricsListResult represents a raw server response from a server to a list call.
const MetricsListResult = `[
    {
        "archive_policy": {
            "aggregation_methods": [
                "max",
                "min"
            ],
            "back_window": 0,
            "definition": [
                {
                    "granularity": "1:00:00",
                    "points": 2304,
                    "timespan": "96 days, 0:00:00"
                },
                {
                    "granularity": "0:05:00",
                    "points": 9216,
                    "timespan": "32 days, 0:00:00"
                },
                {
                    "granularity": "1 day, 0:00:00",
                    "points": 400,
                    "timespan": "400 days, 0:00:00"
                }
            ],
            "name": "precise"
        },
        "created_by_project_id": "e9dc821ca664406e981820a477e9a761",
        "created_by_user_id": "a23c5b98d42d4df3b961e54d5167eb6d",
        "creator": "a23c5b98d42d4df3b961e54d5167eb6d:e9dc821ca664406e981820a477e9a761",
        "id": "777a01d6-4694-49cb-b86a-5ba9fd4e609e",
        "name": "memory.usage",
        "resource_id": "1f3a0724-1807-4bd1-81f9-ee18c8ff6ccc",
        "unit": "MB"
    },
    {
        "archive_policy": {
            "aggregation_methods": [
                "mean",
                "sum"
            ],
            "back_window": 12,
            "definition": [
                {
                    "granularity": "1:00:00",
                    "points": 2160,
                    "timespan": "90 days, 0:00:00"
                },
                {
                    "granularity": "1 day, 0:00:00",
                    "points": 200,
                    "timespan": "200 days, 0:00:00"
                }
            ],
            "name": "not_so_precise"
        },
        "created_by_project_id": "c6b68a6b413648b0a0eb191bf3222f4d",
        "created_by_user_id": "cb072aacdb494419aeeba5f1c62d1a65",
        "creator": "cb072aacdb494419aeeba5f1c62d1a65:c6b68a6b413648b0a0eb191bf3222f4d",
        "id": "6dbc97c5-bfdf-47a2-b184-02e7fa348d21",
        "name": "cpu.delta",
        "resource_id": "c5dc0c47-f43c-425c-a82f-44d61ee91175",
        "unit": "ns"
    }
]`

// Metric1 is an expected representation of a first metric from the MetricsListResult.
var Metric1 = metrics.Metric{
	ArchivePolicy: archivepolicies.ArchivePolicy{
		AggregationMethods: []string{
			"max",
			"min",
		},
		BackWindow: 0,
		Definition: []archivepolicies.ArchivePolicyDefinition{
			{
				Granularity: "1:00:00",
				Points:      2304,
				TimeSpan:    "96 days, 0:00:00",
			},
			{
				Granularity: "0:05:00",
				Points:      9216,
				TimeSpan:    "32 days, 0:00:00",
			},
			{
				Granularity: "1 day, 0:00:00",
				Points:      400,
				TimeSpan:    "400 days, 0:00:00",
			},
		},
		Name: "precise",
	},
	CreatedByProjectID: "e9dc821ca664406e981820a477e9a761",
	CreatedByUserID:    "a23c5b98d42d4df3b961e54d5167eb6d",
	Creator:            "a23c5b98d42d4df3b961e54d5167eb6d:e9dc821ca664406e981820a477e9a761",
	ID:                 "777a01d6-4694-49cb-b86a-5ba9fd4e609e",
	Name:               "memory.usage",
	ResourceID:         "1f3a0724-1807-4bd1-81f9-ee18c8ff6ccc",
	Unit:               "MB",
}

// Metric2 is an expected representation of a second metric from the MetricsListResult.
var Metric2 = metrics.Metric{
	ArchivePolicy: archivepolicies.ArchivePolicy{
		AggregationMethods: []string{
			"mean",
			"sum",
		},
		BackWindow: 12,
		Definition: []archivepolicies.ArchivePolicyDefinition{
			{
				Granularity: "1:00:00",
				Points:      2160,
				TimeSpan:    "90 days, 0:00:00",
			},
			{
				Granularity: "1 day, 0:00:00",
				Points:      200,
				TimeSpan:    "200 days, 0:00:00",
			},
		},
		Name: "not_so_precise",
	},
	CreatedByProjectID: "c6b68a6b413648b0a0eb191bf3222f4d",
	CreatedByUserID:    "cb072aacdb494419aeeba5f1c62d1a65",
	Creator:            "cb072aacdb494419aeeba5f1c62d1a65:c6b68a6b413648b0a0eb191bf3222f4d",
	ID:                 "6dbc97c5-bfdf-47a2-b184-02e7fa348d21",
	Name:               "cpu.delta",
	ResourceID:         "c5dc0c47-f43c-425c-a82f-44d61ee91175",
	Unit:               "ns",
}

// MetricGetResult represents a raw server response from a server to a get request.
const MetricGetResult = `
{
    "archive_policy": {
        "aggregation_methods": [
            "mean",
            "sum"
        ],
        "back_window": 12,
        "definition": [
            {
                "granularity": "1:00:00",
                "points": 2160,
                "timespan": "90 days, 0:00:00"
            },
            {
                "granularity": "1 day, 0:00:00",
                "points": 200,
                "timespan": "200 days, 0:00:00"
            }
        ],
        "name": "not_so_precise"
    },
    "created_by_project_id": "c6b68a6b413648b0a0eb191bf3222f4d",
    "created_by_user_id": "cb072aacdb494419aeeba5f1c62d1a65",
    "creator": "cb072aacdb494419aeeba5f1c62d1a65:c6b68a6b413648b0a0eb191bf3222f4d",
    "id": "0ddf61cf-3747-4f75-bf13-13c28ff03ae3",
    "name": "network.incoming.packets.rate",
    "resource": {
        "created_by_project_id": "c6b68a6b413648b0a0eb191bf3222f4d",
        "created_by_user_id": "cb072aacdb494419aeeba5f1c62d1a65",
        "creator": "cb072aacdb494419aeeba5f1c62d1a65:c6b68a6b413648b0a0eb191bf3222f4d",
        "ended_at": null,
        "id": "75274f99-faf6-4112-a6d5-2794cb07c789",
        "original_resource_id": "75274f99-faf6-4112-a6d5-2794cb07c789",
        "project_id": "4154f08883334e0494c41155c33c0fc9",
        "revision_end": null,
        "revision_start": "2018-01-08T00:59:33.767815+00:00",
        "started_at": "2018-01-08T00:59:33.767795+00:00",
        "type": "compute_instance_network",
        "user_id": "bd5874d666624b24a9f01c128871e4ac"
    },
    "unit": "packet/s"
}
`

// MetricCreateRequest represents a request to create a metric.
const MetricCreateRequest = `
{
    "archive_policy_name": "high",
    "name": "network.incoming.bytes.rate",
    "resource_id": "23d5d3f7-9dfa-4f73-b72b-8b0b0063ec55",
    "unit": "B/s"
}
`

// MetricCreateResponse represents a raw server responce to the MetricCreateRequest.
const MetricCreateResponse = `
{
    "archive_policy_name": "high",
    "created_by_project_id": "3d40ca37-7234-4911-8987b9f288f4ae84",
    "created_by_user_id": "fdcfb420-c096-45e6-9e177a0bb1950884",
    "creator": "fdcfb420-c096-45e6-9e177a0bb1950884:3d40ca37-7234-4911-8987b9f288f4ae84",
    "id": "01b2953e-de74-448a-a305-c84440697933",
    "name": "network.incoming.bytes.rate",
    "resource_id": "23d5d3f7-9dfa-4f73-b72b-8b0b0063ec55",
    "unit": "B/s"
}
`
