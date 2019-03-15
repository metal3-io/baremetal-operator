package resourcetypes

import (
	"fmt"
	"strings"

	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/pagination"
)

// List makes a request against the Gnocchi API to list resource types.
func List(client *gophercloud.ServiceClient) pagination.Pager {
	return pagination.NewPager(client, listURL(client), func(r pagination.PageResult) pagination.Page {
		return ResourceTypePage{pagination.SinglePageBase(r)}
	})
}

// Get retrieves a specific Gnocchi resource type based on its name.
func Get(c *gophercloud.ServiceClient, resourceTypeName string) (r GetResult) {
	_, r.Err = c.Get(getURL(c, resourceTypeName), &r.Body, nil)
	return
}

// CreateOptsBuilder allows to add additional parameters to the Create request.
type CreateOptsBuilder interface {
	ToResourceTypeCreateMap() (map[string]interface{}, error)
}

// AttributeOpts represents options of a single resource type attribute that
// can be created in the Gnocchi.
type AttributeOpts struct {
	// Type is an attribute type.
	Type string `json:"type"`

	// Details represents different attribute fields.
	Details map[string]interface{} `json:"-"`
}

// ToMap is a helper function to convert individual AttributeOpts structure into a sub-map.
func (opts AttributeOpts) ToMap() (map[string]interface{}, error) {
	b, err := gophercloud.BuildRequestBody(opts, "")
	if err != nil {
		return nil, err
	}
	if opts.Details != nil {
		for k, v := range opts.Details {
			b[k] = v
		}
	}
	return b, nil
}

// CreateOpts specifies parameters of a new Gnocchi resource type.
type CreateOpts struct {
	// Attributes is a collection of keys and values of different resource types.
	Attributes map[string]AttributeOpts `json:"-"`

	// Name is a human-readable resource type identifier.
	Name string `json:"name" required:"true"`
}

// ToResourceTypeCreateMap constructs a request body from CreateOpts.
func (opts CreateOpts) ToResourceTypeCreateMap() (map[string]interface{}, error) {
	b, err := gophercloud.BuildRequestBody(opts, "")
	if err != nil {
		return nil, err
	}

	// Create resource type without attributes if they're omitted.
	if opts.Attributes == nil {
		return b, nil
	}

	attributes := make(map[string]interface{}, len(opts.Attributes))
	for k, v := range opts.Attributes {
		attributesMap, err := v.ToMap()
		if err != nil {
			return nil, err
		}
		attributes[k] = attributesMap
	}

	b["attributes"] = attributes
	return b, nil
}

// Create requests the creation of a new Gnocchi resource type on the server.
func Create(client *gophercloud.ServiceClient, opts CreateOptsBuilder) (r CreateResult) {
	b, err := opts.ToResourceTypeCreateMap()
	if err != nil {
		r.Err = err
		return
	}
	_, r.Err = client.Post(createURL(client), b, &r.Body, &gophercloud.RequestOpts{
		OkCodes: []int{201},
	})
	return
}

// AttributeOperation represents a type of operation that can be performed over
// Gnocchi resource type attribute.
type AttributeOperation string

const (
	// AttributeAdd represents Gnocchi resource type attribute add operation.
	AttributeAdd AttributeOperation = "add"

	// AttributeRemove represents Gnocchi resource type attribute remove operation.
	AttributeRemove AttributeOperation = "remove"

	// AttributeCommonPath represents a prefix for every attribute in the Gnocchi
	// resource type.
	AttributeCommonPath = "/attributes"
)

// UpdateOptsBuilder allows to add additional parameters to the Update request.
type UpdateOptsBuilder interface {
	ToResourceTypeUpdateMap() ([]map[string]interface{}, error)
}

// UpdateOpts specifies parameters for a Gnocchi resource type update request.
type UpdateOpts struct {
	// AttributesOperations is a collection of operations that need to be performed
	// over Gnocchi resource type attributes.
	Attributes []AttributeUpdateOpts `json:"-"`
}

// AttributeUpdateOpts represents update options over a single Gnocchi resource
// type attribute.
type AttributeUpdateOpts struct {
	// Name is a human-readable name of an attribute that needs to be added or removed.
	Name string `json:"-" required:"true"`

	// Operation represent action that needs to be performed over the attribute.
	Operation AttributeOperation `json:"-" required:"true"`

	// Value is an attribute options.
	Value *AttributeOpts `json:"-"`
}

// ToResourceTypeUpdateMap constructs a request body from UpdateOpts.
func (opts UpdateOpts) ToResourceTypeUpdateMap() ([]map[string]interface{}, error) {
	if len(opts.Attributes) == 0 {
		return nil, fmt.Errorf("provided Gnocchi resource type UpdateOpts is empty")
	}

	updateOptsMaps := make([]map[string]interface{}, len(opts.Attributes))

	// Populate a map for every attribute.
	for i, attributeUpdateOpts := range opts.Attributes {
		attributeUpdateOptsMap := make(map[string]interface{})

		// Populate attribute value map if provided.
		if attributeUpdateOpts.Value != nil {
			attributeValue, err := attributeUpdateOpts.Value.ToMap()
			if err != nil {
				return nil, err
			}
			attributeUpdateOptsMap["value"] = attributeValue
		}

		// Populate attribute update operation.
		attributeUpdateOptsMap["op"] = attributeUpdateOpts.Operation

		// Populate attribute path from its name.
		attributeUpdateOptsMap["path"] = strings.Join([]string{
			AttributeCommonPath,
			attributeUpdateOpts.Name,
		}, "/")

		updateOptsMaps[i] = attributeUpdateOptsMap
	}

	return updateOptsMaps, nil
}

// Update requests the update operation over existsing Gnocchi resource type.
func Update(client *gophercloud.ServiceClient, resourceTypeName string, opts UpdateOptsBuilder) (r UpdateResult) {
	b, err := opts.ToResourceTypeUpdateMap()
	if err != nil {
		r.Err = err
		return
	}
	_, r.Err = client.Patch(updateURL(client, resourceTypeName), b, &r.Body, &gophercloud.RequestOpts{
		OkCodes: []int{200},
		MoreHeaders: map[string]string{
			"Content-Type": "application/json-patch+json",
		},
	})
	return
}

// Delete accepts a human-readable name and deletes the Gnocchi resource type associated with it.
func Delete(c *gophercloud.ServiceClient, resourceTypeName string) (r DeleteResult) {
	requestOpts := &gophercloud.RequestOpts{
		MoreHeaders: map[string]string{
			"Accept": "application/json, */*",
		},
	}
	_, r.Err = c.Delete(deleteURL(c, resourceTypeName), requestOpts)
	return
}
