package measures

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/gophercloud/utils/gnocchi"

	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/pagination"
)

// CreateResult represents the result of a create operation. Call its
// ExtractErr method to determine if the request succeeded or failed.
type CreateResult struct {
	gophercloud.ErrResult
}

// BatchCreateMetricsResult represents the result of a batch create metrics operation. Call its
// ExtractErr method to determine if the request succeeded or failed.
type BatchCreateMetricsResult struct {
	gophercloud.ErrResult
}

// BatchCreateResourcesMetricsResult represents the result of a batch create via resource IDs operation.
// Call its ExtractErr method to determine if the request succeeded or failed.
type BatchCreateResourcesMetricsResult struct {
	gophercloud.ErrResult
}

// Measure is an datapoint thats is composed with a timestamp and a value.
type Measure struct {
	// Timestamp represents a timestamp of when measure was pushed into the Gnocchi.
	Timestamp time.Time `json:"-"`

	// Granularity is a level of precision that is kept when aggregating data.
	Granularity float64 `json:"-"`

	// Value represents a value of data that was pushed into the Gnocchi.
	Value float64 `json:"-"`
}

/*
UnmarshalJSON helps to unmarshal response from reading Gnocchi measures.

Gnocchi APIv1 returns measures in a such format:

[
    [
        "2017-01-08T10:00:00+00:00",
        300.0,
        146.0
    ],
    [
        "2017-01-08T10:05:00+00:00",
        300.0,
        58.0
    ]
]

Helper unmarshals every nested array into the Measure type.
*/
func (r *Measure) UnmarshalJSON(b []byte) error {
	var measuresSlice []interface{}
	err := json.Unmarshal(b, &measuresSlice)
	if err != nil {
		return err
	}

	// We need to check that a measure contains all needed data.
	if len(measuresSlice) != 3 {
		errMsg := fmt.Sprintf("got an invalid measure: %v", measuresSlice)
		return fmt.Errorf(errMsg)
	}

	type tmp Measure
	var s struct {
		tmp
	}
	*r = Measure(s.tmp)

	// Populate a measure's timestamp.
	var timeStamp string
	var ok bool
	if timeStamp, ok = measuresSlice[0].(string); !ok {
		errMsg := fmt.Sprintf("got an invalid timestamp of a measure %v: %v", measuresSlice, measuresSlice[0])
		return fmt.Errorf(errMsg)
	}
	r.Timestamp, err = time.Parse(gnocchi.RFC3339NanoTimezone, timeStamp)
	if err != nil {
		return err
	}

	// Populate a measure's granularity.
	if r.Granularity, ok = measuresSlice[1].(float64); !ok {
		errMsg := fmt.Sprintf("got an invalid granularity of a measure %v: %v", measuresSlice, measuresSlice[1])
		return fmt.Errorf(errMsg)
	}

	// Populate a measure's value.
	if r.Value = measuresSlice[2].(float64); !ok {
		errMsg := fmt.Sprintf("got an invalid value of a measure %v: %v", measuresSlice, measuresSlice[2])
		return fmt.Errorf(errMsg)
	}

	return nil
}

// MeasurePage is the page returned by a pager when traversing over a collection
// of measures.
type MeasurePage struct {
	pagination.SinglePageBase
}

// IsEmpty checks whether a MeasurePage struct is empty.
func (r MeasurePage) IsEmpty() (bool, error) {
	is, err := ExtractMeasures(r)
	return len(is) == 0, err
}

// ExtractMeasures interprets the results of a single page from a List() call,
// producing a slice of Measures structs.
func ExtractMeasures(r pagination.Page) ([]Measure, error) {
	var s []Measure

	err := (r.(MeasurePage)).ExtractInto(&s)
	if err != nil {
		return nil, err
	}

	return s, err
}
