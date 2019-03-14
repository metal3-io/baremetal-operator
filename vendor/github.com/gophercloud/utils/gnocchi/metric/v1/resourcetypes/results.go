package resourcetypes

import (
	"encoding/json"

	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/pagination"
)

type commonResult struct {
	gophercloud.Result
}

// Extract is a function that accepts a result and extracts a Gnocchi resource type.
func (r commonResult) Extract() (*ResourceType, error) {
	var s *ResourceType
	err := r.ExtractInto(&s)
	return s, err
}

// GetResult represents the result of a get operation. Call its Extract
// method to interpret it as a Gnocchi resource type.
type GetResult struct {
	commonResult
}

// CreateResult represents the result of a create operation. Call its Extract
// method to interpret it as a Gnocchi resource type.
type CreateResult struct {
	commonResult
}

// UpdateResult represents the result of an update operation. Call its Extract
// method to interpret it as a Gnocchi resource type.
type UpdateResult struct {
	commonResult
}

// DeleteResult represents the result of a delete operation. Call its
// ExtractErr method to determine if the request succeeded or failed.
type DeleteResult struct {
	gophercloud.ErrResult
}

// ResourceType represents custom Gnocchi resource type.
type ResourceType struct {
	// Attributes is a collection of keys and values of different resource types.
	Attributes map[string]Attribute `json:"-"`

	// Name is a human-readable resource type identifier.
	Name string `json:"name"`

	// State represents current status of a resource type.
	State string `json:"state"`
}

// Attribute represents single attribute of a Gnocchi resource type.
type Attribute struct {
	// Type is an attribute type.
	Type string `json:"type"`

	// Details represents different attribute fields.
	Details map[string]interface{}
}

// UnmarshalJSON helps to unmarshal ResourceType fields into needed values.
func (r *ResourceType) UnmarshalJSON(b []byte) error {
	type tmp ResourceType
	var s struct {
		tmp
		Attributes map[string]interface{} `json:"attributes"`
	}
	err := json.Unmarshal(b, &s)
	if err != nil {
		return err
	}
	*r = ResourceType(s.tmp)

	if s.Attributes == nil {
		return nil
	}

	// Populate attributes from the JSON map structure.
	attributes := make(map[string]Attribute)
	for attributeName, attributeValues := range s.Attributes {
		attribute := new(Attribute)
		attribute.Details = make(map[string]interface{})

		attributeValuesMap, ok := attributeValues.(map[string]interface{})
		if !ok {
			// Got some strange resource type attribute representation, skip it.
			continue
		}

		// Populate extra and type attribute values.
		for k, v := range attributeValuesMap {
			if k == "type" {
				if attributeType, ok := v.(string); ok {
					attribute.Type = attributeType
				}
			} else {
				attribute.Details[k] = v
			}
		}
		attributes[attributeName] = *attribute
	}

	r.Attributes = attributes

	return err
}

// ResourceTypePage abstracts the raw results of making a List() request against
// the Gnocchi API.
//
// As Gnocchi API may freely alter the response bodies of structures
// returned to the client, you may only safely access the data provided through
// the ExtractResources call.
type ResourceTypePage struct {
	pagination.SinglePageBase
}

// IsEmpty checks whether a ResourceTypePage struct is empty.
func (r ResourceTypePage) IsEmpty() (bool, error) {
	is, err := ExtractResourceTypes(r)
	return len(is) == 0, err
}

// ExtractResourceTypes interprets the results of a single page from a List() call,
// producing a slice of ResourceType structs.
func ExtractResourceTypes(r pagination.Page) ([]ResourceType, error) {
	var s []ResourceType
	err := (r.(ResourceTypePage)).ExtractInto(&s)
	if err != nil {
		return nil, err
	}

	return s, err
}
