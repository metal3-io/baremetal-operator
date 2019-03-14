package resourcetypes

import "github.com/gophercloud/gophercloud"

const resourcePath = "resource_type"

func rootURL(c *gophercloud.ServiceClient) string {
	return c.ServiceURL(resourcePath)
}

func resourceURL(c *gophercloud.ServiceClient, resourceTypeName string) string {
	return c.ServiceURL(resourcePath, resourceTypeName)
}

func listURL(c *gophercloud.ServiceClient) string {
	return rootURL(c)
}

func getURL(c *gophercloud.ServiceClient, resourceTypeName string) string {
	return resourceURL(c, resourceTypeName)
}

func createURL(c *gophercloud.ServiceClient) string {
	return rootURL(c)
}

func updateURL(c *gophercloud.ServiceClient, resourceTypeName string) string {
	return resourceURL(c, resourceTypeName)
}

func deleteURL(c *gophercloud.ServiceClient, resourceTypeName string) string {
	return resourceURL(c, resourceTypeName)
}
