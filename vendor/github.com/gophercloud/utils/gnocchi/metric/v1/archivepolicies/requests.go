package archivepolicies

import (
	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/pagination"
)

// List makes a request against the Gnocchi API to list archive policies.
func List(client *gophercloud.ServiceClient) pagination.Pager {
	return pagination.NewPager(client, listURL(client), func(r pagination.PageResult) pagination.Page {
		return ArchivePolicyPage{pagination.SinglePageBase(r)}
	})
}

// Get retrieves a specific Gnocchi archive policy based on its name.
func Get(c *gophercloud.ServiceClient, archivePolicyName string) (r GetResult) {
	_, r.Err = c.Get(getURL(c, archivePolicyName), &r.Body, nil)
	return
}

// CreateOptsBuilder allows extensions to add additional parameters to the Create request.
type CreateOptsBuilder interface {
	ToArchivePolicyCreateMap() (map[string]interface{}, error)
}

// CreateOpts specifies parameters of a new Archive Policy.
type CreateOpts struct {
	// AggregationMethods is a list of functions used to aggregate
	// multiple measures into an aggregate.
	AggregationMethods []string `json:"aggregation_methods,omitempty"`

	// BackWindow configures number of coarsest periods to keep.
	// It allows to process measures that are older
	// than the last timestamp period boundary.
	BackWindow int `json:"back_window,omitempty"`

	// Definition is a list of parameters that configures
	// archive policy precision and timespan.
	Definition []ArchivePolicyDefinitionOpts `json:"definition"`

	// Name is a name of an archive policy.
	Name string `json:"name"`
}

// ArchivePolicyDefinitionOpts represents definition of how metrics of new or
// updated Archive Policy will be saved with the selected archive policy.
// It configures precision and timespan.
type ArchivePolicyDefinitionOpts struct {
	// Granularity is the level of  precision that must be kept when aggregating data.
	Granularity string `json:"granularity"`

	// Points is a given aggregates or samples in the lifespan of a time series.
	// Time series is a list of aggregates ordered by time.
	// It can be omitted to allow Gnocchi server to calculate it automatically.
	Points *int `json:"points,omitempty"`

	// TimeSpan is the time period for which a metric keeps its aggregates.
	TimeSpan string `json:"timespan"`
}

// ToArchivePolicyCreateMap constructs a request body from CreateOpts.
func (opts CreateOpts) ToArchivePolicyCreateMap() (map[string]interface{}, error) {
	return gophercloud.BuildRequestBody(opts, "")
}

// Create requests the creation of a new Gnocchi archive policy on the server.
func Create(client *gophercloud.ServiceClient, opts CreateOptsBuilder) (r CreateResult) {
	b, err := opts.ToArchivePolicyCreateMap()
	if err != nil {
		r.Err = err
		return
	}
	_, r.Err = client.Post(createURL(client), b, &r.Body, &gophercloud.RequestOpts{
		OkCodes: []int{201},
	})

	return
}

// UpdateOptsBuilder allows extensions to add additional parameters to the Update request.
type UpdateOptsBuilder interface {
	ToArchivePolicyUpdateMap() (map[string]interface{}, error)
}

// UpdateOpts represents options used to update an archive policy.
type UpdateOpts struct {
	// Definition is a list of parameters that configures
	// archive policy precision and timespan.
	Definition []ArchivePolicyDefinitionOpts `json:"definition"`
}

// ToArchivePolicyUpdateMap constructs a request body from UpdateOpts.
func (opts UpdateOpts) ToArchivePolicyUpdateMap() (map[string]interface{}, error) {
	return gophercloud.BuildRequestBody(opts, "")
}

// Update accepts a UpdateOpts and updates an existing Gnocchi archive policy using the values provided.
func Update(client *gophercloud.ServiceClient, archivePolicyName string, opts UpdateOptsBuilder) (r UpdateResult) {
	b, err := opts.ToArchivePolicyUpdateMap()
	if err != nil {
		r.Err = err
		return
	}
	_, r.Err = client.Patch(updateURL(client, archivePolicyName), b, &r.Body, &gophercloud.RequestOpts{
		OkCodes: []int{200},
	})

	return
}

// Delete accepts a Gnocchi archive policy by its name.
func Delete(c *gophercloud.ServiceClient, archivePolicyName string) (r DeleteResult) {
	requestOpts := &gophercloud.RequestOpts{
		MoreHeaders: map[string]string{
			"Accept": "application/json, */*",
		},
	}
	_, r.Err = c.Delete(deleteURL(c, archivePolicyName), requestOpts)
	return
}
