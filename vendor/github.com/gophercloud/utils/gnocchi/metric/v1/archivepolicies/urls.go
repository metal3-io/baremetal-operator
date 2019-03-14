package archivepolicies

import "github.com/gophercloud/gophercloud"

const resourcePath = "archive_policy"

func resourceURL(c *gophercloud.ServiceClient, archivePolicyName string) string {
	return c.ServiceURL(resourcePath, archivePolicyName)
}

func rootURL(c *gophercloud.ServiceClient) string {
	return c.ServiceURL(resourcePath)
}

func listURL(c *gophercloud.ServiceClient) string {
	return rootURL(c)
}

func getURL(c *gophercloud.ServiceClient, archivePolicyName string) string {
	return resourceURL(c, archivePolicyName)
}

func createURL(c *gophercloud.ServiceClient) string {
	return rootURL(c)
}

func updateURL(c *gophercloud.ServiceClient, archivePolicyName string) string {
	return resourceURL(c, archivePolicyName)
}

func deleteURL(c *gophercloud.ServiceClient, archivePolicyName string) string {
	return resourceURL(c, archivePolicyName)
}
