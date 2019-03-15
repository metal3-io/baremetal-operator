package testing

import "github.com/gophercloud/utils/gnocchi/metric/v1/resourcetypes"

// ResourceTypeListResult represents raw server response from a server to a list call.
const ResourceTypeListResult = `[
    {
        "attributes": {},
        "name": "generic",
        "state": "active"
    },
    {
        "attributes": {
            "parent_id": {
                "required": false,
                "type": "uuid"
            }
        },
        "name": "identity_project",
        "state": "active"
    },
    {
        "attributes": {
            "host": {
                "max_length": 128,
                "min_length": 0,
                "required": true,
                "type": "string"
            }
        },
        "name": "compute_instance",
        "state": "active"
    }
]`

// ResourceType1 is an expected representation of a first resource from the ResourceTypeListResult.
var ResourceType1 = resourcetypes.ResourceType{
	Name:       "generic",
	State:      "active",
	Attributes: map[string]resourcetypes.Attribute{},
}

// ResourceType2 is an expected representation of a first resource from the ResourceTypeListResult.
var ResourceType2 = resourcetypes.ResourceType{
	Name:  "identity_project",
	State: "active",
	Attributes: map[string]resourcetypes.Attribute{
		"parent_id": resourcetypes.Attribute{
			Type: "uuid",
			Details: map[string]interface{}{
				"required": false,
			},
		},
	},
}

// ResourceType3 is an expected representation of a first resource from the ResourceTypeListResult.
var ResourceType3 = resourcetypes.ResourceType{
	Name:  "compute_instance",
	State: "active",
	Attributes: map[string]resourcetypes.Attribute{
		"host": resourcetypes.Attribute{
			Type: "string",
			Details: map[string]interface{}{
				"max_length": float64(128),
				"min_length": float64(0),
				"required":   true,
			},
		},
	},
}

// ResourceTypeGetResult represents raw server response from a server to a get request.
const ResourceTypeGetResult = `
{
    "attributes": {
        "host": {
            "min_length": 0,
            "max_length": 255,
            "type": "string",
            "required": true
        },
        "image_ref": {
            "type": "uuid",
            "required": false
        }
    },
    "state": "active",
    "name": "compute_instance"
}
`

// ResourceTypeCreateWithoutAttributesRequest represents a request to create a resource type without attributes.
const ResourceTypeCreateWithoutAttributesRequest = `
{
    "name":"identity_project"
}
`

// ResourceTypeCreateWithoutAttributesResult represents a raw server response to the ResourceTypeCreateWithoutAttributesRequest.
const ResourceTypeCreateWithoutAttributesResult = `
{
    "attributes": {},
    "state": "active",
    "name": "identity_project"
}
`

// ResourceTypeCreateWithAttributesRequest represents a request to create a resource type with attributes.
const ResourceTypeCreateWithAttributesRequest = `
{
    "attributes": {
        "port_name": {
            "max_length": 128,
            "required": false,
            "type": "string"
        },
        "port_id": {
            "required": true,
            "type": "uuid"
        }
    },
    "name": "compute_instance_network"
}
`

// ResourceTypeCreateWithAttributesResult represents a raw server response to the ResourceTypeCreateWithAttributesRequest.
const ResourceTypeCreateWithAttributesResult = `
{
    "attributes": {
        "port_id": {
            "required": true,
            "type": "uuid"
        },
        "port_name": {
            "min_length": 0,
            "max_length": 128,
            "type": "string",
            "required": false
        }
    },
    "state": "active",
    "name": "compute_instance_network"
}
`

// ResourceTypeUpdateRequest represents a request to update a resource type.
const ResourceTypeUpdateRequest = `
[
    {
        "op": "add",
        "path": "/attributes/enabled",
        "value": {
            "options": {
                "fill": true
            },
            "required": true,
            "type": "bool"
        }
    },
    {
        "op": "add",
        "path": "/attributes/parent_id",
        "value": {
            "required": false,
            "type": "uuid"
        }
    },
    {
        "op": "remove",
        "path": "/attributes/domain_id"
    }
]
`

// ResourceTypeUpdateResult represents a raw server response to the ResourceTypeUpdateRequest.
const ResourceTypeUpdateResult = `
{
    "attributes": {
        "enabled": {
            "required": true,
            "type": "bool"
        },
        "parent_id": {
            "type": "uuid",
            "required": false
        },
        "name": {
            "required": true,
            "type": "string",
            "min_length": 0,
            "max_length": 128
        }
    },
    "state": "active",
    "name": "identity_project"
}
`
