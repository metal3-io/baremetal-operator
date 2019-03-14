package v1

import (
	"testing"
	"time"

	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/acceptance/tools"
	"github.com/gophercloud/utils/gnocchi/metric/v1/measures"
)

// CreateMeasures will create measures inside a single Gnocchi metric. An error will be returned if the
// measures could not be created.
func CreateMeasures(t *testing.T, client *gophercloud.ServiceClient, metricID string) error {
	currentTimestamp := time.Now().UTC()
	pastHourTimestamp := currentTimestamp.Add(-1 * time.Hour)
	currentValue := float64(tools.RandomInt(100, 200))
	pastHourValue := float64(tools.RandomInt(500, 600))
	measuresToCreate := []measures.MeasureOpts{
		{
			Timestamp: &currentTimestamp,
			Value:     currentValue,
		},
		{
			Timestamp: &pastHourTimestamp,
			Value:     pastHourValue,
		},
	}
	createOpts := measures.CreateOpts{
		Measures: measuresToCreate,
	}

	t.Logf("Attempting to create measures inside a Gnocchi metric %s", metricID)
	if err := measures.Create(client, metricID, createOpts).ExtractErr(); err != nil {
		return err
	}

	t.Logf("Successfully created measures inside the Gnocchi metric %s", metricID)
	return nil
}

// MeasuresBatchCreateMetrics will create measures inside different metrics via batch request.
// An error will be returned if measures could not be created.
func MeasuresBatchCreateMetrics(t *testing.T, client *gophercloud.ServiceClient, metricIDs ...string) error {
	currentTimestamp := time.Now().UTC()
	pastHourTimestamp := currentTimestamp.Add(-1 * time.Hour)
	currentValue := float64(tools.RandomInt(100, 200))
	pastHourValue := float64(tools.RandomInt(500, 600))
	createOpts := make([]measures.MetricOpts, len(metricIDs))

	// Populate batch options with provided metric IDs and generated values.
	for i, m := range metricIDs {
		createOpts[i] = measures.MetricOpts{
			ID: m,
			Measures: []measures.MeasureOpts{
				{
					Timestamp: &currentTimestamp,
					Value:     currentValue,
				},
				{
					Timestamp: &pastHourTimestamp,
					Value:     pastHourValue,
				},
			},
		}
	}

	t.Logf("Attempting to create measures inside Gnocchi metrics via batch request")
	if err := measures.BatchCreateMetrics(client, createOpts).ExtractErr(); err != nil {
		return err
	}

	t.Logf("Successfully created measures inside Gnocchi metrics")
	return nil
}

// MeasuresBatchCreateResourcesMetrics will create measures inside different metrics via batch request to resource IDs.
// The batchResourcesMetrics arguments is a mapping between resource IDs and corresponding metric names.
// An error will be returned if measures could not be created.
func MeasuresBatchCreateResourcesMetrics(t *testing.T, client *gophercloud.ServiceClient, batchResourcesMetrics map[string][]string) error {
	currentTimestamp := time.Now().UTC()
	pastHourTimestamp := currentTimestamp.Add(-1 * time.Hour)
	currentValue := float64(tools.RandomInt(100, 200))
	pastHourValue := float64(tools.RandomInt(500, 600))

	// measureSet is a set of measures for an every metric.
	measureSet := []measures.MeasureOpts{
		{
			Timestamp: &currentTimestamp,
			Value:     currentValue,
		},
		{
			Timestamp: &pastHourTimestamp,
			Value:     pastHourValue,
		},
	}

	// batchResourcesMetricsOpts is an internal slice representation of measures.BatchResourcesMetricsOpts stucts.
	batchResourcesMetricsOpts := make([]measures.BatchResourcesMetricsOpts, 0)

	for resourceID, metricNames := range batchResourcesMetrics {
		// resourcesMetricsOpts is an internal slice representation of measures.ResourcesMetricsOpts structs.
		resourcesMetricsOpts := make([]measures.ResourcesMetricsOpts, 0)

		// Populate batch options for each metric of a resource.
		for _, metricName := range metricNames {
			resourcesMetricsOpts = append(resourcesMetricsOpts, measures.ResourcesMetricsOpts{
				MetricName: metricName,
				Measures:   measureSet,
			})
		}

		// Save batch options of a resource.
		batchResourcesMetricsOpts = append(batchResourcesMetricsOpts, measures.BatchResourcesMetricsOpts{
			ResourceID:       resourceID,
			ResourcesMetrics: resourcesMetricsOpts,
		})
	}

	createOpts := measures.BatchCreateResourcesMetricsOpts{
		CreateMetrics:         true,
		BatchResourcesMetrics: batchResourcesMetricsOpts,
	}

	t.Logf("Attempting to create measures inside Gnocchi metrics via batch request with resource IDs")
	if err := measures.BatchCreateResourcesMetrics(client, createOpts).ExtractErr(); err != nil {
		return err
	}

	t.Logf("Successfully created measures inside Gnocchi metrics")
	return nil
}
