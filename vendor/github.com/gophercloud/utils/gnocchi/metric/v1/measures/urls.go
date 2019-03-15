package measures

import "github.com/gophercloud/gophercloud"

const (
	resourcePath                    = "metric"
	batchCreateMetricsPath          = "batch/metrics"
	batchCreateResourcesMetricsPath = "batch/resources/metrics"
)

func resourceURL(c *gophercloud.ServiceClient, metricID string) string {
	return c.ServiceURL(resourcePath, metricID, "measures")
}

func listURL(c *gophercloud.ServiceClient, metricID string) string {
	return resourceURL(c, metricID)
}

func createURL(c *gophercloud.ServiceClient, metricID string) string {
	return resourceURL(c, metricID)
}

func batchCreateMetricsURL(c *gophercloud.ServiceClient) string {
	return c.ServiceURL(batchCreateMetricsPath, "measures")
}

func batchCreateResourcesMetricsURL(c *gophercloud.ServiceClient) string {
	return c.ServiceURL(batchCreateResourcesMetricsPath, "measures")
}
