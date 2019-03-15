package testing

import "github.com/gophercloud/utils/gnocchi/metric/v1/archivepolicies"

// ArchivePoliciesListResult represents a raw server response from a server to a list call.
const ArchivePoliciesListResult = `
[
    {
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
    {
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
    }
]
`

// ListArchivePoliciesExpected represents an expected repsonse from a List request.
var ListArchivePoliciesExpected = []archivepolicies.ArchivePolicy{
	{
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
	{
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
}

// ArchivePolicyGetResult represents a raw server response from a server to a get request.
const ArchivePolicyGetResult = `
{
    "aggregation_methods": [
        "max",
        "min",
        "mean"
    ],
    "back_window": 128,
    "definition": [
        {
            "granularity": "1:00:00",
            "points": 2160,
            "timespan": "90 days, 0:00:00"
        },
        {
            "granularity": "1 day, 0:00:00",
            "points": 100,
            "timespan": "100 days, 0:00:00"
        }
    ],
    "name": "test_policy"
}
`

// ArchivePolicyCreateRequest represents a raw create request.
const ArchivePolicyCreateRequest = `
{
    "aggregation_methods": [
        "sum",
        "mean",
        "count"
    ],
    "back_window": 31,
    "definition": [
        {
            "granularity": "1:00:00",
            "timespan": "90 days, 0:00:00"
        },
        {
            "granularity": "1 day, 0:00:00",
            "timespan": "100 days, 0:00:00"
        }
    ],
    "name": "test_policy"
}
`

// ArchivePolicyCreateResponse represents a raw server response from a server to a create request.
const ArchivePolicyCreateResponse = `
{
    "aggregation_methods": [
        "sum",
        "mean",
        "count"
    ],
    "back_window": 31,
    "definition": [
        {
            "granularity": "1:00:00",
            "points": 2160,
            "timespan": "90 days, 0:00:00"
        },
        {
            "granularity": "1 day, 0:00:00",
            "points": 100,
            "timespan": "100 days, 0:00:00"
        }
    ],
    "name": "test_policy"
}
`

// ArchivePolicyUpdateRequest represents a raw update request.
const ArchivePolicyUpdateRequest = `
{
    "definition": [
        {
            "timespan": "30 days, 0:00:00",
            "granularity": "12:00:00"
        },
        {
            "timespan": "90 days, 0:00:00",
            "granularity": "1 day, 0:00:00"
        }
    ]
}
`

// ArchivePolicyUpdateResponse represents a raw server response from a server to an update request.
const ArchivePolicyUpdateResponse = `
{
    "definition": [
        {
            "points": 60,
            "timespan": "30 days, 0:00:00",
            "granularity": "12:00:00"
        },
        {
            "points": 90,
            "timespan": "90 days, 0:00:00",
            "granularity": "1 day, 0:00:00"
        }
    ],
    "back_window": 0,
    "name": "test_policy",
    "aggregation_methods": [
        "max"
    ]
}
`
