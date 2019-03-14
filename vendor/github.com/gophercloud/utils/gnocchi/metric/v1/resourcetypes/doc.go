/*
Package resourcetypes provides ability to manage resource types through the Gnocchi API.

Example of Listing resource types

  allPages, err := resourcetypes.List(client).AllPages()
  if err != nil {
    panic(err)
	}

  allResourceTypes, err := resourcetypes.ExtractResourceTypes(allPages)
  if err != nil {
    panic(err)
	}

  for _, resourceType := range allResourceTypes {
    fmt.Printf("%+v\n", resourceType)
  }

Example of Getting a resource type

  resourceTypeName := "compute_instance"
  resourceType, err := resourcetypes.Get(gnocchiClient, resourceTypeName).Extract()
  if err != nil {
    panic(err)
  }

Example of Creating a resource type

  resourceTypeOpts := resourcetypes.CreateOpts{
    Name: "compute_instance_network",
    Attributes: map[string]resourcetypes.AttributeOpts{
      "port_name": resourcetypes.AttributeOpts{
        Type: "string",
        Details: map[string]interface{}{
          "max_length": 128,
          "required":   false,
        },
      },
      "port_id": resourcetypes.AttributeOpts{
        Type: "uuid",
        Details: map[string]interface{}{
          "required": true,
        },
      },
    },
  }
  resourceType, err := resourcetypes.Create(gnocchiClient, resourceTypeOpts).Extract()
  if err != nil {
    panic(err)
  }

Example of Updating a resource type

  enabledAttributeOptions := resourcetypes.AttributeOpts{
    Details: map[string]interface{}{
      "required": true,
      "options": map[string]interface{}{
        "fill": true,
      },
    },
    Type: "bool",
  }
  parendIDAttributeOptions := resourcetypes.AttributeOpts{
    Details: map[string]interface{}{
      "required": false,
    },
    Type: "uuid",
  }
  resourceTypeOpts := resourcetypes.UpdateOpts{
    Attributes: []resourcetypes.AttributeUpdateOpts{
      {
        Name:      "enabled",
        Operation: resourcetypes.AttributeAdd,
        Value:     &enabledAttributeOptions,
      },
      {
        Name:      "parent_id",
        Operation: resourcetypes.AttributeAdd,
        Value:     &parendIDAttributeOptions,
      },
      {
        Name:      "domain_id",
        Operation: resourcetypes.AttributeRemove,
      },
    },
  }
  resourceType, err := resourcetypes.Update(gnocchiClient, resourceTypeOpts).Extract()
  if err != nil {
    panic(err)
  }

Example of Deleting a resource type

  err := resourcetypes.Delete(gnocchiClient, resourceType).ExtractErr()
  if err != nil {
    panic(err)
  }
*/
package resourcetypes
