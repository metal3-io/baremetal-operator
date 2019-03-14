package testing

import (
	"time"

	"github.com/gophercloud/utils/gnocchi/metric/v1/measures"
)

// MeasuresListResult represents a raw server response from a server to a List call.
const MeasuresListResult = `
[
    [
        "2018-01-10T12:00:00+00:00",
        3600.0,
        15.0
    ],
    [
        "2018-01-10T13:00:00+00:00",
        3600.0,
        10.0
    ],
    [
        "2018-01-10T14:00:00+00:00",
        3600.0,
        20.0
    ]
]
`

// ListMeasuresExpected represents an expected repsonse from a List request.
var ListMeasuresExpected = []measures.Measure{
	{
		Timestamp:   time.Date(2018, 1, 10, 12, 0, 0, 0, time.UTC),
		Granularity: 3600.0,
		Value:       15.0,
	},
	{
		Timestamp:   time.Date(2018, 1, 10, 13, 0, 0, 0, time.UTC),
		Granularity: 3600.0,
		Value:       10.0,
	},
	{
		Timestamp:   time.Date(2018, 1, 10, 14, 0, 0, 0, time.UTC),
		Granularity: 3600.0,
		Value:       20.0,
	},
}

// MeasuresCreateRequest represents a request to create measures for a single metric.
const MeasuresCreateRequest = `
[
    {
        "timestamp": "2018-01-18T12:31:00",
        "value": 101.2
    },
    {
        "timestamp": "2018-01-18T14:32:00",
        "value": 102
    }
]
`

// MeasuresBatchCreateMetricsRequest represents a request to create measures for a single metric.
const MeasuresBatchCreateMetricsRequest = `
{
    "777a01d6-4694-49cb-b86a-5ba9fd4e609e": [
        {
            "timestamp": "2018-01-10T01:00:00",
            "value": 200
        },
        {
            "timestamp": "2018-01-10T02:45:00",
            "value": 300
        }
    ],
    "6dbc97c5-bfdf-47a2-b184-02e7fa348d21": [
        {
            "timestamp": "2018-01-10T01:00:00",
            "value": 111
        },
        {
            "timestamp": "2018-01-10T02:45:00",
            "value": 222
        }
    ]
}
`

// MeasuresBatchCreateResourcesMetricsRequest represents a request to create measures for a single metric.
const MeasuresBatchCreateResourcesMetricsRequest = `
{
    "75274f99-faf6-4112-a6d5-2794cb07c789": {
        "network.incoming.bytes.rate": {
			"archive_policy_name": "high",
			"unit": "B/s",
            "measures": [
                {
                    "timestamp": "2018-01-20T12:30:00",
                    "value": 1562.82
                },
                {
                    "timestamp": "2018-01-20T13:15:00",
                    "value": 768.1
                }
            ]
        },
        "network.outgoing.bytes.rate": {
            "archive_policy_name": "high",
            "unit": "B/s",
            "measures": [
                {
                    "timestamp": "2018-01-20T12:30:00",
                    "value": 273
                },
                {
                    "timestamp": "2018-01-20T13:15:00",
                    "value": 3141.14
                }
            ]
        }
    },
    "23d5d3f7-9dfa-4f73-b72b-8b0b0063ec55": {
        "disk.write.bytes.rate": {
			"archive_policy_name": "low",
			"unit": "B/s",
            "measures": [
                {
                    "timestamp": "2018-01-20T12:30:00",
                    "value": 1237
                },
                {
                    "timestamp": "2018-01-20T13:15:00",
                    "value": 132.12
                }
            ]
        }
    }
}
`
