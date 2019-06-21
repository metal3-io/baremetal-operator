package status

import "github.com/gophercloud/gophercloud"

type commonResult struct {
	gophercloud.Result
}

// Extract is a function that accepts a result and extracts a Gnocchi status.
func (r commonResult) Extract() (*Status, error) {
	var s *Status
	err := r.ExtractInto(&s)
	return s, err
}

// GetResult represents the result of a get operation. Call its Extract
// method to interpret it as a Gnocchi status.
type GetResult struct {
	commonResult
}

// Status represents a Gnocchi status of measurements processing.
type Status struct {
	// Metricd represents all running Gnocchi metricd daemons.
	Metricd Metricd `json:"metricd"`

	// Storage contains Gnocchi storage data of measures backlog.
	Storage Storage `json:"storage"`
}

// Metricd represents all running Gnocchi metricd daemons.
type Metricd struct {
	// Processors represents a list of running Gnocchi metricd processors.
	Processors []string `json:"processors"`
}

// Storage contains Gnocchi storage data of metrics and measures to process.
type Storage struct {
	// MeasuresToProcess represents all metrics having measures to process.
	MeasuresToProcess map[string]int `json:"measures_to_process"`

	// Summary represents total count of metrics and processing measures.
	Summary Summary `json:"summary"`
}

// Summary contains total numbers of metrics and measures to process.
type Summary struct {
	// Measures represents total number of measures to process.
	Measures int `json:"measures"`

	// Metrics represents total number of metric having measures to process.
	Metrics int `json:"metrics"`
}
