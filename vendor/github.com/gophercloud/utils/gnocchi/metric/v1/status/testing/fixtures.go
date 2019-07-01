package testing

import "github.com/gophercloud/utils/gnocchi/metric/v1/status"

// StatusGetWithDetailsResult represents raw server response with all status attributes
// from a server to a get request.
const StatusGetWithDetailsResult = `
{
    "metricd": {
        "processors": [
            "node-stat1.27.ce1da3c9-6c8c-490d-b256-3ba1e2bceb7b",
            "node-stat1.10.9d9a99b2-f0ac-496b-36f3-115b84304a84",
            "node-stat1.23.915e39c0-4002-489c-87bd-9055033d440c"
        ]
    },
    "storage": {
        "measures_to_process": {
            "002583fd-edc7-47c1-a253-b0575b7ebfe8": 1,
            "0028fe48-c7a9-4515-91ac-c17b4eade4d4": 1,
            "002d65fe-56bc-4f52-8c71-da87cd82c436": 23,
            "003ead5d-657a-4ac0-be0d-d0ab3be9590e": 4,
            "009c537v-58f4-4dc6-ba9c-e931d3e57565": 1,
            "009cc1eg-2337-4171-930e-75cd54bf6de1": 2
        },
        "summary": {
            "measures": 32,
            "metrics": 6
        }
    }
}
`

// StatusGetWithoutDetailsResult represents raw server response without details
// from a server to a get request.
const StatusGetWithoutDetailsResult = `
{
    "metricd": {
        "processors": [
            "node-stat1.27.ce1da3c9-6c8c-490d-b256-3ba1e2bceb7b",
            "node-stat1.10.9d9a99b2-f0ac-496b-36f3-115b84304a84",
            "node-stat1.23.915e39c0-4002-489c-87bd-9055033d440c"
        ]
    },
    "storage": {
        "summary": {
            "measures": 32,
            "metrics": 6
        }
    }
}
`

// GetStatusWithDetailsExpected represents an expected response with all status
// attributes from a get request.
var GetStatusWithDetailsExpected = status.Status{
	Metricd: status.Metricd{
		Processors: []string{
			"node-stat1.27.ce1da3c9-6c8c-490d-b256-3ba1e2bceb7b",
			"node-stat1.10.9d9a99b2-f0ac-496b-36f3-115b84304a84",
			"node-stat1.23.915e39c0-4002-489c-87bd-9055033d440c",
		},
	},
	Storage: status.Storage{
		MeasuresToProcess: map[string]int{
			"002583fd-edc7-47c1-a253-b0575b7ebfe8": 1,
			"0028fe48-c7a9-4515-91ac-c17b4eade4d4": 1,
			"002d65fe-56bc-4f52-8c71-da87cd82c436": 23,
			"003ead5d-657a-4ac0-be0d-d0ab3be9590e": 4,
			"009c537v-58f4-4dc6-ba9c-e931d3e57565": 1,
			"009cc1eg-2337-4171-930e-75cd54bf6de1": 2,
		},
		Summary: status.Summary{
			Measures: 32,
			Metrics:  6,
		},
	},
}

// GetStatusWithoutDetailsExpected represents an expected response without details
// from a get request.
var GetStatusWithoutDetailsExpected = status.Status{
	Metricd: status.Metricd{
		Processors: []string{
			"node-stat1.27.ce1da3c9-6c8c-490d-b256-3ba1e2bceb7b",
			"node-stat1.10.9d9a99b2-f0ac-496b-36f3-115b84304a84",
			"node-stat1.23.915e39c0-4002-489c-87bd-9055033d440c",
		},
	},
	Storage: status.Storage{
		Summary: status.Summary{
			Measures: 32,
			Metrics:  6,
		},
	},
}
