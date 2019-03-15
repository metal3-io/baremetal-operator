package metrics

import (
	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/pagination"
)

// ListOptsBuilder allows extensions to add additional parameters to the
// List request.
type ListOptsBuilder interface {
	ToMetricListQuery() (string, error)
}

// ListOpts allows the limiting and sorting of paginated collections through
// the Gnocchi API.
type ListOpts struct {
	// Limit allows to limits count of metrics in the response.
	Limit int `q:"limit"`

	// Marker is used for pagination.
	Marker string `q:"marker"`

	// SortKey allows to sort metrics in the response by key.
	SortKey string `q:"sort_key"`

	// SortDir allows to set the direction of sorting.
	// Can be `asc` or `desc`.
	SortDir string `q:"sort_dir"`

	// Creator shows who created the metric.
	// Usually it contains concatenated string with values from
	// "created_by_user_id" and "created_by_project_id" fields.
	Creator string `json:"creator"`

	// ProjectID is the Identity project of the metric.
	ProjectID string `json:"project_id"`

	// UserID is the Identity user of the metric.
	UserID string `json:"user_id"`
}

// ToMetricListQuery formats a ListOpts into a query string.
func (opts ListOpts) ToMetricListQuery() (string, error) {
	q, err := gophercloud.BuildQueryString(opts)
	return q.String(), err
}

// List returns a Pager which allows you to iterate over a collection of
// metrics. It accepts a ListOpts struct, which allows you to limit and sort
// the returned collection for a greater efficiency.
func List(c *gophercloud.ServiceClient, opts ListOptsBuilder) pagination.Pager {
	url := listURL(c)
	if opts != nil {
		query, err := opts.ToMetricListQuery()
		if err != nil {
			return pagination.Pager{Err: err}
		}
		url += query
	}
	return pagination.NewPager(c, url, func(r pagination.PageResult) pagination.Page {
		return MetricPage{pagination.SinglePageBase(r)}
	})
}

// Get retrieves a specific Gnocchi metric based on its id.
func Get(c *gophercloud.ServiceClient, metricID string) (r GetResult) {
	_, r.Err = c.Get(getURL(c, metricID), &r.Body, nil)
	return
}

// CreateOptsBuilder allows to add additional parameters to the
// Create request.
type CreateOptsBuilder interface {
	ToMetricCreateMap() (map[string]interface{}, error)
}

// CreateOpts specifies parameters of a new Gnocchi metric.
type CreateOpts struct {
	// ArchivePolicyName is a name of the Gnocchi archive policy that describes
	// the aggregate storage policy of a metric.
	// You can omit it in the request if your Gnocchi installation has the needed
	// archive policy rule to assign an archive policy by a metric's name.
	ArchivePolicyName string `json:"archive_policy_name,omitempty"`

	// Name is a human-readable name for the Gnocchi metric.
	// You must provide it if you are also providing a ResourceID in the request.
	Name string `json:"name,omitempty"`

	// ResourceID identifies the associated Gnocchi resource of the metric.
	ResourceID string `json:"resource_id,omitempty"`

	// Unit is a unit of measurement for measures of that Gnocchi metric.
	Unit string `json:"unit,omitempty"`
}

// ToMetricCreateMap constructs a request body from CreateOpts.
func (opts CreateOpts) ToMetricCreateMap() (map[string]interface{}, error) {
	b, err := gophercloud.BuildRequestBody(opts, "")
	if err != nil {
		return nil, err
	}

	return b, nil
}

// Create requests the creation of a new Gnocchi metric on the server.
func Create(client *gophercloud.ServiceClient, opts CreateOptsBuilder) (r CreateResult) {
	b, err := opts.ToMetricCreateMap()
	if err != nil {
		r.Err = err
		return
	}
	_, r.Err = client.Post(createURL(client), b, &r.Body, &gophercloud.RequestOpts{
		OkCodes: []int{201},
	})
	return
}

// Delete accepts a unique ID and deletes the Gnocchi metric associated with it.
func Delete(c *gophercloud.ServiceClient, metricID string) (r DeleteResult) {
	requestOpts := &gophercloud.RequestOpts{
		MoreHeaders: map[string]string{
			"Accept": "application/json, */*",
		},
	}
	_, r.Err = c.Delete(deleteURL(c, metricID), requestOpts)
	return
}
