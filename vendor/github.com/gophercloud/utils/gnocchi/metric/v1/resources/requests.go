package resources

import (
	"time"

	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/pagination"
	"github.com/gophercloud/utils/gnocchi"
)

// ListOptsBuilder allows extensions to add additional parameters to the
// List request.
type ListOptsBuilder interface {
	ToResourceListQuery() (string, error)
}

// ListOpts allows the limiting and sorting of paginated collections through
// the Gnocchi API.
type ListOpts struct {
	// Details allows to list resources with all attributes.
	Details bool `q:"details"`

	// Limit allows to limits count of resources in the response.
	Limit int `q:"limit"`

	// Marker is used for pagination.
	Marker string `q:"marker"`

	// SortKey allows to sort resources in the response by key.
	SortKey string `q:"sort_key"`

	// SortDir allows to set the direction of sorting.
	// Can be `asc` or `desc`.
	SortDir string `q:"sort_dir"`
}

// ToResourceListQuery formats a ListOpts into a query string.
func (opts ListOpts) ToResourceListQuery() (string, error) {
	q, err := gophercloud.BuildQueryString(opts)
	return q.String(), err
}

// List returns a Pager which allows you to iterate over a collection of
// resources. It accepts a ListOpts struct, which allows you to limit and sort
// the returned collection for a greater efficiency.
func List(c *gophercloud.ServiceClient, opts ListOptsBuilder, resourceType string) pagination.Pager {
	url := listURL(c, resourceType)
	if opts != nil {
		query, err := opts.ToResourceListQuery()
		if err != nil {
			return pagination.Pager{Err: err}
		}
		url += query
	}
	return pagination.NewPager(c, url, func(r pagination.PageResult) pagination.Page {
		return ResourcePage{pagination.SinglePageBase(r)}
	})
}

// Get retrieves a specific Gnocchi resource based on its type and ID.
func Get(c *gophercloud.ServiceClient, resourceType string, resourceID string) (r GetResult) {
	_, r.Err = c.Get(getURL(c, resourceType, resourceID), &r.Body, nil)
	return
}

// CreateOptsBuilder allows to add additional parameters to the
// Create request.
type CreateOptsBuilder interface {
	ToResourceCreateMap() (map[string]interface{}, error)
}

// CreateOpts specifies parameters of a new Gnocchi resource.
type CreateOpts struct {
	// ID uniquely identifies the Gnocchi resource.
	ID string `json:"id" required:"true"`

	// Metrics field can be used to link existing metrics in the resource
	// or to create metrics with the resource at the same time to save
	// some requests.
	Metrics map[string]interface{} `json:"metrics,omitempty"`

	// ProjectID is the Identity project of the resource.
	ProjectID string `json:"project_id,omitempty"`

	// UserID is the Identity user of the resource.
	UserID string `json:"user_id,omitempty"`

	// StartedAt is a resource creation timestamp.
	StartedAt *time.Time `json:"-"`

	// EndedAt is a timestamp of when the resource has ended.
	EndedAt *time.Time `json:"-"`

	// ExtraAttributes is a collection of keys and values that can be found in resources
	// of different resource types.
	ExtraAttributes map[string]interface{} `json:"-"`
}

// ToResourceCreateMap constructs a request body from CreateOpts.
func (opts CreateOpts) ToResourceCreateMap() (map[string]interface{}, error) {
	b, err := gophercloud.BuildRequestBody(opts, "")
	if err != nil {
		return nil, err
	}

	if opts.StartedAt != nil {
		b["started_at"] = opts.StartedAt.Format(gnocchi.RFC3339NanoTimezone)
	}

	if opts.EndedAt != nil {
		b["ended_at"] = opts.EndedAt.Format(gnocchi.RFC3339NanoTimezone)
	}

	if opts.ExtraAttributes != nil {
		for key, value := range opts.ExtraAttributes {
			b[key] = value
		}
	}

	return b, nil
}

// Create requests the creation of a new Gnocchi resource on the server.
func Create(client *gophercloud.ServiceClient, resourceType string, opts CreateOptsBuilder) (r CreateResult) {
	b, err := opts.ToResourceCreateMap()
	if err != nil {
		r.Err = err
		return
	}
	_, r.Err = client.Post(createURL(client, resourceType), b, &r.Body, &gophercloud.RequestOpts{
		OkCodes: []int{201},
	})
	return
}

// UpdateOptsBuilder allows extensions to add additional parameters to the
// Update request.
type UpdateOptsBuilder interface {
	ToResourceUpdateMap() (map[string]interface{}, error)
}

// UpdateOpts represents options used to update a network.
type UpdateOpts struct {
	// Metrics field can be used to link existing metrics in the resource
	// or to create metrics and update the resource at the same time to save
	// some requests.
	Metrics *map[string]interface{} `json:"metrics,omitempty"`

	// ProjectID is the Identity project of the resource.
	ProjectID string `json:"project_id,omitempty"`

	// UserID is the Identity user of the resource.
	UserID string `json:"user_id,omitempty"`

	// StartedAt is a resource creation timestamp.
	StartedAt *time.Time `json:"-"`

	// EndedAt is a timestamp of when the resource has ended.
	EndedAt *time.Time `json:"-"`

	// ExtraAttributes is a collection of keys and values that can be found in resources
	// of different resource types.
	ExtraAttributes map[string]interface{} `json:"-"`
}

// ToResourceUpdateMap builds a request body from UpdateOpts.
func (opts UpdateOpts) ToResourceUpdateMap() (map[string]interface{}, error) {
	b, err := gophercloud.BuildRequestBody(opts, "")
	if err != nil {
		return nil, err
	}

	if opts.StartedAt != nil {
		b["started_at"] = opts.StartedAt.Format(gnocchi.RFC3339NanoTimezone)
	}

	if opts.EndedAt != nil {
		b["ended_at"] = opts.EndedAt.Format(gnocchi.RFC3339NanoTimezone)
	}

	if opts.ExtraAttributes != nil {
		for key, value := range opts.ExtraAttributes {
			b[key] = value
		}
	}

	return b, nil
}

// Update accepts a UpdateOpts struct and updates an existing Gnocchi resource using the
// values provided.
func Update(c *gophercloud.ServiceClient, resourceType, resourceID string, opts UpdateOptsBuilder) (r UpdateResult) {
	b, err := opts.ToResourceUpdateMap()
	if err != nil {
		r.Err = err
		return
	}
	_, r.Err = c.Patch(updateURL(c, resourceType, resourceID), b, &r.Body, &gophercloud.RequestOpts{
		OkCodes: []int{200},
	})
	return
}

// Delete accepts a unique ID and deletes the Gnocchi resource associated with it.
func Delete(c *gophercloud.ServiceClient, resourceType, resourceID string) (r DeleteResult) {
	requestOpts := &gophercloud.RequestOpts{
		MoreHeaders: map[string]string{
			"Accept": "application/json, */*",
		},
	}
	_, r.Err = c.Delete(deleteURL(c, resourceType, resourceID), requestOpts)
	return
}
