package measures

import (
	"fmt"
	"net/url"
	"time"

	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/pagination"
	"github.com/gophercloud/utils/gnocchi"
)

// ListOptsBuilder allows extensions to add additional parameters to the
// List request.
type ListOptsBuilder interface {
	ToMeasureListQuery() (string, error)
}

// ListOpts allows to provide additional options to the Gnocchi measures List request.
type ListOpts struct {
	// Refresh can be used to force any unprocessed measures to be handled in the Gnocchi
	// to ensure that List request returns all aggregates.
	Refresh bool `q:"refresh"`

	// Start is a start of time time range for the measures.
	Start *time.Time

	// Stop is a stop of time time range for the measures.
	Stop *time.Time

	// Aggregation is a needed aggregation method for returned measures.
	// Gnocchi returns "mean" by default.
	Aggregation string `q:"aggregation"`

	// Granularity is a needed time between two series of measures to retreive.
	// Gnocchi will response with all granularities for available measures by default.
	Granularity string `q:"granularity"`

	// Resample allows to select different granularity instead of those that were defined in the
	// archive policy.
	Resample string `q:"resample"`
}

// ToMeasureListQuery formats a ListOpts into a query string.
func (opts ListOpts) ToMeasureListQuery() (string, error) {
	q, err := gophercloud.BuildQueryString(opts)
	params := q.Query()

	if opts.Start != nil {
		params.Add("start", opts.Start.Format(gnocchi.RFC3339NanoNoTimezone))
	}

	if opts.Stop != nil {
		params.Add("stop", opts.Stop.Format(gnocchi.RFC3339NanoNoTimezone))
	}

	q = &url.URL{RawQuery: params.Encode()}
	return q.String(), err
}

// List returns a Pager which allows you to iterate over a collection of
// measures.
// It accepts a ListOpts struct, which allows you to provide options to a Gnocchi measures List request.
func List(c *gophercloud.ServiceClient, metricID string, opts ListOptsBuilder) pagination.Pager {
	url := listURL(c, metricID)
	if opts != nil {
		query, err := opts.ToMeasureListQuery()
		if err != nil {
			return pagination.Pager{Err: err}
		}
		url += query
	}
	return pagination.NewPager(c, url, func(r pagination.PageResult) pagination.Page {
		return MeasurePage{pagination.SinglePageBase(r)}
	})
}

// CreateOptsBuilder is needed to add measures to the Create request.
type CreateOptsBuilder interface {
	ToMeasureCreateMap() (map[string]interface{}, error)
}

// MeasureOpts represents options of a single measure that can be created in the Gnocchi.
type MeasureOpts struct {
	// Timestamp represents a measure creation timestamp.
	Timestamp *time.Time `json:"-" required:"true"`

	// Value represents a measure data value.
	Value float64 `json:"value"`
}

// ToMap is a helper function to convert individual MeasureOpts structure into a sub-map.
func (opts MeasureOpts) ToMap() (map[string]interface{}, error) {
	b, err := gophercloud.BuildRequestBody(opts, "")
	if err != nil {
		return nil, err
	}
	if opts.Timestamp != nil {
		b["timestamp"] = opts.Timestamp.Format(gnocchi.RFC3339NanoNoTimezone)
	}
	return b, nil
}

// CreateOpts specifies a parameters for creating measures for a single metric.
type CreateOpts struct {
	// Measures is a set of measures for a single metric that needs to be created.
	Measures []MeasureOpts
}

// ToMeasureCreateMap constructs a request body from CreateOpts.
func (opts CreateOpts) ToMeasureCreateMap() (map[string]interface{}, error) {
	measures := make([]map[string]interface{}, len(opts.Measures))
	for i, m := range opts.Measures {
		measureMap, err := m.ToMap()
		if err != nil {
			return nil, err
		}
		measures[i] = measureMap
	}
	return map[string]interface{}{"measures": measures}, nil
}

// Create requests the creation of a new measures in the single Gnocchi metric.
func Create(client *gophercloud.ServiceClient, metricID string, opts CreateOptsBuilder) (r CreateResult) {
	b, err := opts.ToMeasureCreateMap()
	if err != nil {
		r.Err = err
		return
	}
	_, r.Err = client.Post(createURL(client, metricID), b["measures"], &r.Body, &gophercloud.RequestOpts{
		OkCodes: []int{202},
		MoreHeaders: map[string]string{
			"Accept": "application/json, */*",
		},
	})
	return
}

// BatchCreateMetricsOptsBuilder is needed to add measures to the BatchCreateMetrics request.
type BatchCreateMetricsOptsBuilder interface {
	ToMeasuresBatchCreateMetricsMap() (map[string]interface{}, error)
}

// BatchCreateMetricsOpts specifies a parameters for creating measures for different metrics in a single request.
type BatchCreateMetricsOpts []MetricOpts

// MetricOpts represents measures of a single metric of the BatchCreateMetrics request.
type MetricOpts struct {
	// ID uniquely identifies the Gnocchi metric.
	ID string

	// Measures is a set of measures for a single metric that needs to be created.
	Measures []MeasureOpts
}

// ToMap is a helper function to convert individual MetricOpts structure into a sub-map.
func (opts MetricOpts) ToMap() (map[string]interface{}, error) {
	// Check provided MetricOpts fields.
	if opts.ID == "" {
		errMsg := "missing input for the MetricOpts 'ID' argument"
		return nil, fmt.Errorf(errMsg)
	}
	if opts.Measures == nil {
		errMsg := "missing input for the MetricOpts 'Measures' argument"
		return nil, fmt.Errorf(errMsg)
	}

	// measures is a slice of measures maps.
	measures := make([]map[string]interface{}, len(opts.Measures))

	// metricOpts is an internal map representation of the MetricOpts struct.
	metricOpts := make(map[string]interface{})

	for i, measure := range opts.Measures {
		measureMap, err := measure.ToMap()
		if err != nil {
			return nil, err
		}
		measures[i] = measureMap
	}
	metricOpts[opts.ID] = measures

	return metricOpts, nil
}

// ToMeasuresBatchCreateMetricsMap constructs a request body from BatchCreateMetricsOpts.
func (opts BatchCreateMetricsOpts) ToMeasuresBatchCreateMetricsMap() (map[string]interface{}, error) {
	// batchCreateMetricsOpts is an internal representation of the BatchCreateMetricsOpts struct.
	batchCreateMetricsOpts := make(map[string]interface{})

	for _, metricOpts := range opts {
		metricOptsMap, err := metricOpts.ToMap()
		if err != nil {
			return nil, err
		}
		for k, v := range metricOptsMap {
			batchCreateMetricsOpts[k] = v
		}
	}

	return map[string]interface{}{"batchCreateMetrics": batchCreateMetricsOpts}, nil
}

// BatchCreateMetrics requests the creation of a new measures for different metrics.
func BatchCreateMetrics(client *gophercloud.ServiceClient, opts BatchCreateMetricsOpts) (r BatchCreateMetricsResult) {
	b, err := opts.ToMeasuresBatchCreateMetricsMap()
	if err != nil {
		r.Err = err
		return
	}

	_, r.Err = client.Post(batchCreateMetricsURL(client), b["batchCreateMetrics"], &r.Body, &gophercloud.RequestOpts{
		OkCodes: []int{202},
		MoreHeaders: map[string]string{
			"Accept": "application/json, */*",
		},
	})
	return
}

// BatchCreateResourcesMetricsOptsBuilder is needed to add measures to the BatchCreateResourcesMetrics request.
type BatchCreateResourcesMetricsOptsBuilder interface {
	// ToMeasuresBatchCreateResourcesMetricsMap builds a request body.
	ToMeasuresBatchCreateResourcesMetricsMap() (map[string]interface{}, error)

	// ToMeasuresBatchCreateResourcesMetricsQuery builds a query string.
	ToMeasuresBatchCreateResourcesMetricsQuery() (string, error)
}

// BatchCreateResourcesMetricsOpts specifies a parameters for creating measures inside metrics via resource IDs and metric names.
type BatchCreateResourcesMetricsOpts struct {
	// CreateMetrics allows request to create metrics that don't exist yet.
	CreateMetrics bool `q:"create_metrics"`

	// BatchResourcesMetrics is a map of resource IDs, metrics names and corresponding measures that needs to be created.
	BatchResourcesMetrics []BatchResourcesMetricsOpts
}

// BatchResourcesMetricsOpts represents parameters of a single resource of the BatchCreateResourcesMetrics request.
type BatchResourcesMetricsOpts struct {
	// ResourceID uniquely identifies the Gnocchi resource.
	ResourceID string

	// ResourcesMetrics specifies metrics whose measures will be updated.
	ResourcesMetrics []ResourcesMetricsOpts
}

// ToMap is a helper function to convert individual BatchResourcesMetricsOpts structure into a sub-map.
func (opts BatchResourcesMetricsOpts) ToMap() (map[string]interface{}, error) {
	// Check provided BatchResourcesMetricsOpts fields.
	if opts.ResourceID == "" {
		errMsg := "missing input for the BatchResourcesMetricsOpts 'ResourceID' argument"
		return nil, fmt.Errorf(errMsg)
	}
	if opts.ResourcesMetrics == nil {
		errMsg := "missing input for the BatchResourcesMetricsOpts 'ResourcesMetrics' argument"
		return nil, fmt.Errorf(errMsg)
	}

	// batchResourcesMetricsOpts is an internal map representation of the BatchResourcesMetricsOpts struct.
	batchResourcesMetricsOpts := make(map[string]interface{})

	// resourcesMetricsMaps is a temporary slice that contains different metrics for a single resource.
	resourcesMetricsMaps := make([]map[string]interface{}, len(opts.ResourcesMetrics))

	// resourcesMetricsOptsMap is a temporary map that contains join of different maps from resourcesMetricsMaps slice.
	resourcesMetricsOptsMap := make(map[string]interface{})

	// Populate the temporary resourcesMetricsMaps slice.
	for i, resourcesMetrics := range opts.ResourcesMetrics {
		resourcesMetricsMap, err := resourcesMetrics.ToMap()
		if err != nil {
			return nil, err
		}
		resourcesMetricsMaps[i] = resourcesMetricsMap
	}

	// Populate the temporary resourcesMetricsOptsMap map.
	for _, resourcesMetricsMap := range resourcesMetricsMaps {
		for k, v := range resourcesMetricsMap {
			resourcesMetricsOptsMap[k] = v
		}
	}

	// Populate the final map batchResourcesMetricsOpts.
	batchResourcesMetricsOpts[opts.ResourceID] = resourcesMetricsOptsMap

	return batchResourcesMetricsOpts, nil
}

// ResourcesMetricsOpts represents measures of a single metric of the resource from BatchResourcesMetricsOpts.
type ResourcesMetricsOpts struct {
	// MetricName is a human-readable name for the Gnocchi metric.
	MetricName string

	// ArchivePolicyName is a name of the Gnocchi archive policy that describes
	// the aggregate storage policy of a metric.
	ArchivePolicyName string

	// Unit is a unit of measurement for measures of that Gnocchi metric.
	Unit string

	// Measures is a set of measures for a single metric that needs to be created.
	Measures []MeasureOpts
}

// ToMap is a helper function to convert individual ResourcesMetricsOpts structure into a sub-map.
func (opts ResourcesMetricsOpts) ToMap() (map[string]interface{}, error) {
	// Check provided ResourcesMetricsOpts fields.
	if opts.MetricName == "" {
		errMsg := "missing input for the ResourcesMetricsOpts 'MetricName' argument"
		return nil, fmt.Errorf(errMsg)
	}
	if opts.Measures == nil {
		errMsg := "missing input for the ResourcesMetricsOpts 'Measures' argument"
		return nil, fmt.Errorf(errMsg)
	}

	// measures is a slice of measures maps.
	measures := make([]map[string]interface{}, len(opts.Measures))

	// resourcesMetricsOpts is an internal map representation of the ResourcesMetricsOpts struct.
	resourcesMetricsOpts := make(map[string]interface{})

	// metricOpts is an internal nested map for each metric in the resourcesMetricsOpts.
	metricOpts := make(map[string]interface{})

	// Populate metricOpts with values from provided opts.
	if opts.ArchivePolicyName != "" {
		metricOpts["archive_policy_name"] = opts.ArchivePolicyName
	}
	if opts.Unit != "" {
		metricOpts["unit"] = opts.Unit
	}
	for i, measure := range opts.Measures {
		measureMap, err := measure.ToMap()
		if err != nil {
			return nil, err
		}
		measures[i] = measureMap
	}
	metricOpts["measures"] = measures

	resourcesMetricsOpts[opts.MetricName] = metricOpts

	return resourcesMetricsOpts, nil
}

// ToMeasuresBatchCreateResourcesMetricsMap constructs a request body from the BatchCreateResourcesMetricsOpts.
func (opts BatchCreateResourcesMetricsOpts) ToMeasuresBatchCreateResourcesMetricsMap() (map[string]interface{}, error) {
	// batchCreateResourcesMetricsOpts is an internal representation of the
	// BatchCreateResourcesMetricsOpts's BatchResourcesMetrics field.
	batchCreateResourcesMetricsOpts := make(map[string]interface{})

	for _, resourceMetricsOpts := range opts.BatchResourcesMetrics {
		resourceMetricsOptsMap, err := resourceMetricsOpts.ToMap()
		if err != nil {
			return nil, err
		}
		for k, v := range resourceMetricsOptsMap {
			batchCreateResourcesMetricsOpts[k] = v
		}
	}

	return map[string]interface{}{"batchCreateResourcesMetrics": batchCreateResourcesMetricsOpts}, nil
}

// ToMeasuresBatchCreateResourcesMetricsQuery formats the BatchCreateResourcesMetricsOpts into a query string.
func (opts BatchCreateResourcesMetricsOpts) ToMeasuresBatchCreateResourcesMetricsQuery() (string, error) {
	q, err := gophercloud.BuildQueryString(opts)
	return q.String(), err
}

// BatchCreateResourcesMetrics requests the creation of new measures inside metrics via resource IDs and metric names.
func BatchCreateResourcesMetrics(client *gophercloud.ServiceClient, opts BatchCreateResourcesMetricsOptsBuilder) (r BatchCreateResourcesMetricsResult) {
	url := batchCreateResourcesMetricsURL(client)
	if opts != nil {
		query, err := opts.ToMeasuresBatchCreateResourcesMetricsQuery()
		if err != nil {
			r.Err = err
			return
		}
		url += query
	}

	b, err := opts.ToMeasuresBatchCreateResourcesMetricsMap()
	if err != nil {
		r.Err = err
		return
	}

	_, r.Err = client.Post(url, b["batchCreateResourcesMetrics"], &r.Body, &gophercloud.RequestOpts{
		OkCodes: []int{202},
		MoreHeaders: map[string]string{
			"Accept": "application/json, */*",
		},
	})
	return
}
