package v1

import (
	"testing"
	"time"

	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/acceptance/tools"
	"github.com/gophercloud/utils/gnocchi/metric/v1/resources"
	"github.com/hashicorp/go-uuid"
)

// CreateGenericResource will create a Gnocchi resource with a generic type.
// An error will be returned if the resource could not be created.
func CreateGenericResource(t *testing.T, client *gophercloud.ServiceClient) (*resources.Resource, error) {
	id, err := uuid.GenerateUUID()
	if err != nil {
		return nil, err
	}

	randomDay := tools.RandomInt(1, 100)
	now := time.Now().UTC().AddDate(0, 0, -randomDay)
	metricName := tools.RandomString("TESTACCT-", 8)
	createOpts := resources.CreateOpts{
		ID:        id,
		StartedAt: &now,
		Metrics: map[string]interface{}{
			metricName: map[string]string{
				"archive_policy_name": "medium",
			},
		},
	}
	resourceType := "generic"
	t.Logf("Attempting to create a generic Gnocchi resource")

	resource, err := resources.Create(client, resourceType, createOpts).Extract()
	if err != nil {
		return nil, err
	}

	t.Logf("Successfully created the generic Gnocchi resource.")
	return resource, nil
}

// DeleteResource will delete a Gnocchi resource with specified type and ID.
// A fatal error will occur if the delete was not successful.
func DeleteResource(t *testing.T, client *gophercloud.ServiceClient, resourceType, resourceID string) {
	t.Logf("Attempting to delete the Gnocchi resource: %s", resourceID)

	err := resources.Delete(client, resourceType, resourceID).ExtractErr()
	if err != nil {
		t.Fatalf("Unable to delete the Gnocchi resource %s: %v", resourceID, err)
	}

	t.Logf("Deleted the Gnocchi resource: %s", resourceID)
}

// CreateResourcesToBatchMeasures will create Gnocchi resources with metrics to test batch measures requests and
// return a map with references of resource IDs and metric names.
// An error will be returned if resources or metrics could not be created.
func CreateResourcesToBatchMeasures(t *testing.T, client *gophercloud.ServiceClient) (map[string][]string, error) {
	// Prepare metric names.
	firstMetricName := tools.RandomString("TESTACCT-", 8)
	secondMetricName := tools.RandomString("TESTACCT-", 8)
	thirdMetricName := tools.RandomString("TESTACCT-", 8)

	// Prepare the first resource.
	firstResourceID, err := uuid.GenerateUUID()
	if err != nil {
		return nil, err
	}
	firstRandomDay := tools.RandomInt(1, 100)
	firstStartTimestamp := time.Now().UTC().AddDate(0, 0, -firstRandomDay)
	firstResourceCreateOpts := resources.CreateOpts{
		ID:        firstResourceID,
		StartedAt: &firstStartTimestamp,
		Metrics: map[string]interface{}{
			firstMetricName: map[string]string{
				"archive_policy_name": "medium",
			},
			secondMetricName: map[string]string{
				"archive_policy_name": "low",
			},
		},
	}
	firstResourceType := "generic"

	t.Logf("Attempting to create a generic Gnocchi resource")
	firstResource, err := resources.Create(client, firstResourceType, firstResourceCreateOpts).Extract()
	if err != nil {
		return nil, err
	}

	t.Logf("Successfully created the generic Gnocchi resource.")
	tools.PrintResource(t, firstResource)

	// Prepare the second resource.
	secondResourceID, err := uuid.GenerateUUID()
	if err != nil {
		return nil, err
	}
	secondRandomDay := tools.RandomInt(1, 100)
	secondStartTimestamp := time.Now().UTC().AddDate(0, 0, -secondRandomDay)
	secondResourceCreateOpts := resources.CreateOpts{
		ID:        secondResourceID,
		StartedAt: &secondStartTimestamp,
		Metrics: map[string]interface{}{
			thirdMetricName: map[string]string{
				"archive_policy_name": "low",
			},
		},
	}
	secondResourceType := "generic"

	t.Logf("Attempting to create a generic Gnocchi resource")
	secondResource, err := resources.Create(client, secondResourceType, secondResourceCreateOpts).Extract()
	if err != nil {
		return nil, err
	}

	t.Logf("Successfully created the generic Gnocchi resource.")
	tools.PrintResource(t, secondResource)

	resourcesReferenceMap := map[string][]string{
		firstResource.ID: []string{
			firstMetricName,
			secondMetricName,
		},
		secondResource.ID: []string{
			thirdMetricName,
		},
	}
	return resourcesReferenceMap, nil
}
