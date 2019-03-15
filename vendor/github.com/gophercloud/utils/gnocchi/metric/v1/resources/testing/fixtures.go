package testing

import (
	"time"

	"github.com/gophercloud/utils/gnocchi/metric/v1/resources"
)

// ResourceListResult represents raw server response from a server to a list call.
const ResourceListResult = `[
    {
        "created_by_project_id": "3d40ca37723449118987b9f288f4ae84",
        "created_by_user_id": "fdcfb420c09645e69e177a0bb1950884",
        "creator": "fdcfb420c09645e69e177a0bb1950884:3d40ca37723449118987b9f288f4ae84",
        "display_name": "MyInstance00",
        "flavor_name": "2CPU4G",
        "host": "compute010",
        "ended_at": null,
        "id": "1f3a0724-1807-4bd1-81f9-ee18c8ff6ccc",
        "metrics": {
            "cpu.delta": "2df1515e-6325-4d49-af0d-1052f6462fe4",
            "memory.usage": "777a01d6-4694-49cb-b86a-5ba9fd4e609e"
        },
        "original_resource_id": "1f3a0724-1807-4bd1-81f9-ee18c8ff6ccc",
        "project_id": "4154f08883334e0494c41155c33c0fc9",
        "revision_end": null,
        "revision_start": "2018-01-02T11:39:33.942419+00:00",
        "started_at": "2018-01-02T11:39:33.942391+00:00",
        "type": "compute_instance",
        "user_id": "bd5874d666624b24a9f01c128871e4ac"
    },
    {
        "created_by_project_id": "3d40ca37723449118987b9f288f4ae84",
        "created_by_user_id": "fdcfb420c09645e69e177a0bb1950884",
        "creator": "fdcfb420c09645e69e177a0bb1950884:3d40ca37723449118987b9f288f4ae84",
        "disk_device_name": "sdb",
        "ended_at": null,
        "id": "789a7f65-977d-40f4-beed-f717100125f5",
        "metrics": {
            "disk.read.bytes.rate": "ed1bb76f-6ccc-4ad2-994c-dbb19ddccbae",
            "disk.write.bytes.rate": "0a2da84d-4753-43f5-a65f-0f8d44d2766c"
        },
        "original_resource_id": "789a7f65-977d-40f4-beed-f717100125f5",
        "project_id": "4154f08883334e0494c41155c33c0fc9",
        "revision_end": null,
        "revision_start": "2018-01-03T11:44:31.155773+00:00",
        "started_at": "2018-01-03T11:44:31.155732+00:00",
        "type": "compute_instance_disk",
        "user_id": "bd5874d666624b24a9f01c128871e4ac"
    }
]`

// Resource1 is an expected representation of a first resource from the ResourceListResult.
var Resource1 = resources.Resource{
	CreatedByProjectID: "3d40ca37723449118987b9f288f4ae84",
	CreatedByUserID:    "fdcfb420c09645e69e177a0bb1950884",
	Creator:            "fdcfb420c09645e69e177a0bb1950884:3d40ca37723449118987b9f288f4ae84",
	ID:                 "1f3a0724-1807-4bd1-81f9-ee18c8ff6ccc",
	Metrics: map[string]string{
		"cpu.delta":    "2df1515e-6325-4d49-af0d-1052f6462fe4",
		"memory.usage": "777a01d6-4694-49cb-b86a-5ba9fd4e609e",
	},
	OriginalResourceID: "1f3a0724-1807-4bd1-81f9-ee18c8ff6ccc",
	ProjectID:          "4154f08883334e0494c41155c33c0fc9",
	RevisionStart:      time.Date(2018, 1, 2, 11, 39, 33, 942419000, time.UTC),
	RevisionEnd:        time.Time{},
	StartedAt:          time.Date(2018, 1, 2, 11, 39, 33, 942391000, time.UTC),
	EndedAt:            time.Time{},
	Type:               "compute_instance",
	UserID:             "bd5874d666624b24a9f01c128871e4ac",
	ExtraAttributes: map[string]interface{}{
		"display_name": "MyInstance00",
		"flavor_name":  "2CPU4G",
		"host":         "compute010",
	},
}

// Resource2 is an expected representation of a second resource from the ResourceListResult.
var Resource2 = resources.Resource{
	CreatedByProjectID: "3d40ca37723449118987b9f288f4ae84",
	CreatedByUserID:    "fdcfb420c09645e69e177a0bb1950884",
	Creator:            "fdcfb420c09645e69e177a0bb1950884:3d40ca37723449118987b9f288f4ae84",
	ID:                 "789a7f65-977d-40f4-beed-f717100125f5",
	Metrics: map[string]string{
		"disk.read.bytes.rate":  "ed1bb76f-6ccc-4ad2-994c-dbb19ddccbae",
		"disk.write.bytes.rate": "0a2da84d-4753-43f5-a65f-0f8d44d2766c",
	},
	OriginalResourceID: "789a7f65-977d-40f4-beed-f717100125f5",
	ProjectID:          "4154f08883334e0494c41155c33c0fc9",
	RevisionStart:      time.Date(2018, 1, 3, 11, 44, 31, 155773000, time.UTC),
	RevisionEnd:        time.Time{},
	StartedAt:          time.Date(2018, 1, 3, 11, 44, 31, 155732000, time.UTC),
	EndedAt:            time.Time{},
	Type:               "compute_instance_disk",
	UserID:             "bd5874d666624b24a9f01c128871e4ac",
	ExtraAttributes: map[string]interface{}{
		"disk_device_name": "sdb",
	},
}

// ResourceGetResult represents raw server response from a server to a get requrest.
const ResourceGetResult = `
{
    "created_by_project_id": "3d40ca37723449118987b9f288f4ae84",
    "created_by_user_id": "fdcfb420c09645e69e177a0bb1950884",
    "creator": "fdcfb420c09645e69e177a0bb1950884:3d40ca37723449118987b9f288f4ae84",
    "iface_name": "eth0",
    "ended_at": null,
    "id": "75274f99-faf6-4112-a6d5-2794cb07c789",
    "metrics": {
        "network.incoming.bytes.rate": "01b2953e-de74-448a-a305-c84440697933",
        "network.outgoing.bytes.rate": "4ac0041b-3bf7-441d-a95a-d3e2f1691158",
        "network.incoming.packets.rate": "5a64328e-8a7c-4c6a-99df-2e6d17440142",
        "network.outgoing.packets.rate": "dc9f3198-155b-4b88-a92c-58a3853ce2b2"
    },
    "original_resource_id": "75274f99-faf6-4112-a6d5-2794cb07c789",
    "project_id": "4154f08883334e0494c41155c33c0fc9",
    "revision_end": null,
    "revision_start": "2018-01-01T11:44:31.742031+00:00",
    "started_at": "2018-01-01T11:44:31.742011+00:00",
    "type": "compute_instance_network",
    "user_id": "bd5874d666624b24a9f01c128871e4ac"
}
`

// ResourceCreateWithoutMetricsRequest represents a request to create a resource without metrics.
const ResourceCreateWithoutMetricsRequest = `
{
    "id": "23d5d3f7-9dfa-4f73-b72b-8b0b0063ec55",
    "project_id": "4154f088-8333-4e04-94c4-1155c33c0fc9",
    "user_id": "bd5874d6-6662-4b24-a9f01c128871e4ac"
}
`

// ResourceCreateWithoutMetricsResult represents a raw server responce to the ResourceCreateNoMetricsRequest.
const ResourceCreateWithoutMetricsResult = `
{
    "created_by_project_id": "3d40ca37-7234-4911-8987b9f288f4ae84",
    "created_by_user_id": "fdcfb420-c096-45e6-9e177a0bb1950884",
    "creator": "fdcfb420-c096-45e6-9e177a0bb1950884:3d40ca37-7234-4911-8987b9f288f4ae84",
    "ended_at": null,
    "id": "23d5d3f7-9dfa-4f73-b72b-8b0b0063ec55",
    "metrics": {},
    "original_resource_id": "23d5d3f7-9dfa-4f73-b72b-8b0b0063ec55",
    "project_id": "4154f088-8333-4e04-94c4-1155c33c0fc9",
    "revision_end": null,
    "revision_start": "2018-01-03T11:44:31.155773+00:00",
    "started_at": "2018-01-03T11:44:31.155732+00:00",
    "type": "generic",
    "user_id": "bd5874d6-6662-4b24-a9f01c128871e4ac"
}
`

// ResourceCreateLinkMetricsRequest represents a request to create a resource with linked metrics.
const ResourceCreateLinkMetricsRequest = `
{
    "id": "23d5d3f7-9dfa-4f73-b72b-8b0b0063ec55",
    "project_id": "4154f088-8333-4e04-94c4-1155c33c0fc9",
    "user_id": "bd5874d6-6662-4b24-a9f01c128871e4ac",
    "started_at": "2018-01-02T23:23:34+00:00",
    "ended_at": "2018-01-04T10:00:12+00:00",
    "metrics": {
        "network.incoming.bytes.rate": "01b2953e-de74-448a-a305-c84440697933",
        "network.outgoing.bytes.rate": "dc9f3198-155b-4b88-a92c-58a3853ce2b2"
    }
}
`

// ResourceCreateLinkMetricsResult represents a raw server responce to the ResourceCreateLinkMetricsRequest.
const ResourceCreateLinkMetricsResult = `
{
    "created_by_project_id": "3d40ca37-7234-4911-8987b9f288f4ae84",
    "created_by_user_id": "fdcfb420-c096-45e6-9e177a0bb1950884",
    "creator": "fdcfb420-c096-45e6-9e177a0bb1950884:3d40ca37-7234-4911-8987b9f288f4ae84",
    "id": "23d5d3f7-9dfa-4f73-b72b-8b0b0063ec55",
    "metrics": {
        "network.incoming.bytes.rate": "01b2953e-de74-448a-a305-c84440697933",
        "network.outgoing.bytes.rate": "dc9f3198-155b-4b88-a92c-58a3853ce2b2"
    },
    "original_resource_id": "23d5d3f7-9dfa-4f73-b72b-8b0b0063ec55",
    "project_id": "4154f088-8333-4e04-94c4-1155c33c0fc9",
    "revision_end": null,
    "revision_start": "2018-01-02T23:23:34.155813+00:00",
    "ended_at": "2018-01-04T10:00:12+00:00",
    "started_at": "2018-01-02T23:23:34+00:00",
    "type": "compute_instance_network",
    "user_id": "bd5874d6-6662-4b24-a9f01c128871e4ac"
}
`

// ResourceCreateWithMetricsRequest represents a request to simultaneously create a resource with metrics.
const ResourceCreateWithMetricsRequest = `
{
    "id": "23d5d3f7-9dfa-4f73-b72b-8b0b0063ec55",
    "project_id": "4154f088-8333-4e04-94c4-1155c33c0fc9",
    "user_id": "bd5874d6-6662-4b24-a9f01c128871e4ac",
    "ended_at": "2018-01-09T20:00:00+00:00",
    "metrics": {
        "disk.write.bytes.rate": {
            "archive_policy_name": "high"
        }
    }
}
`

// ResourceCreateWithMetricsResult represents a raw server responce to the ResourceCreateWithMetricsRequest.
const ResourceCreateWithMetricsResult = `
{
    "created_by_project_id": "3d40ca37-7234-4911-8987b9f288f4ae84",
    "created_by_user_id": "fdcfb420-c096-45e6-9e177a0bb1950884",
    "creator": "fdcfb420-c096-45e6-9e177a0bb1950884:3d40ca37-7234-4911-8987b9f288f4ae84",
    "id": "23d5d3f7-9dfa-4f73-b72b-8b0b0063ec55",
    "metrics": {
        "disk.write.bytes.rate": "0a2da84d-4753-43f5-a65f-0f8d44d2766c"
    },
    "original_resource_id": "23d5d3f7-9dfa-4f73-b72b-8b0b0063ec55",
    "project_id": "4154f088-8333-4e04-94c4-1155c33c0fc9",
    "revision_end": null,
    "revision_start": "2018-01-02T23:23:34.155813+00:00",
    "ended_at": "2018-01-09T20:00:00+00:00",
    "started_at": "2018-01-02T23:23:34.155773+00:00",
    "type": "compute_instance_disk",
    "user_id": "bd5874d6-6662-4b24-a9f01c128871e4ac"
}
`

// ResourceUpdateLinkMetricsRequest represents a request to update a resource and link some existing metrics.
const ResourceUpdateLinkMetricsRequest = `
{
    "ended_at":"2018-01-14T13:00:00+00:00",
    "metrics": {
        "network.incoming.bytes.rate": "01b2953e-de74-448a-a305-c84440697933"
    }
}
`

// ResourceUpdateLinkMetricsResponse represents a raw server responce to the ResourceUpdateLinkMetricsRequest.
const ResourceUpdateLinkMetricsResponse = `
{
    "created_by_project_id": "3d40ca37-7234-4911-8987b9f288f4ae84",
    "created_by_user_id": "fdcfb420-c096-45e6-9e177a0bb1950884",
    "creator": "fdcfb420-c096-45e6-9e177a0bb1950884:3d40ca37-7234-4911-8987b9f288f4ae84",
    "ended_at": "2018-01-14T13:00:00+00:00",
    "id": "23d5d3f7-9dfa-4f73-b72b-8b0b0063ec55",
    "project_id": "4154f088-8333-4e04-94c4-1155c33c0fc9",
    "original_resource_id": "23d5d3f7-9dfa-4f73-b72b-8b0b0063ec55",
    "revision_end": null,
    "metrics": {
        "network.incoming.bytes.rate": "01b2953e-de74-448a-a305-c84440697933"
    },
    "revision_start": "2018-01-12T13:44:34.742031+00:00",
    "started_at": "2018-01-12T13:44:34.742011+00:00",
    "type": "compute_instance_network",
    "user_id": "bd5874d6-6662-4b24-a9f01c128871e4ac"
}
`

// ResourceUpdateCreateMetricsRequest represents a request to update a resource and link some existing metrics.
const ResourceUpdateCreateMetricsRequest = `
{
    "started_at":"2018-01-12T11:00:00+00:00",
    "metrics": {
        "disk.read.bytes.rate": {
            "archive_policy_name": "low"
        }
    }
}
`

// ResourceUpdateCreateMetricsResponse represents a raw server responce to the ResourceUpdateLinkMetricsRequest.
const ResourceUpdateCreateMetricsResponse = `
{
    "created_by_project_id": "3d40ca37-7234-4911-8987b9f288f4ae84",
    "created_by_user_id": "fdcfb420-c096-45e6-9e177a0bb1950884",
    "creator": "fdcfb420-c096-45e6-9e177a0bb1950884:3d40ca37-7234-4911-8987b9f288f4ae84",
    "ended_at": null,
    "id": "23d5d3f7-9dfa-4f73-b72b-8b0b0063ec55",
    "project_id": "4154f088-8333-4e04-94c4-1155c33c0fc9",
    "original_resource_id": "23d5d3f7-9dfa-4f73-b72b-8b0b0063ec55",
    "revision_end": null,
    "metrics": {
        "disk.read.bytes.rate": "ed1bb76f-6ccc-4ad2-994c-dbb19ddccbae"
    },
    "revision_start": "2018-01-12T12:00:34.742031+00:00",
    "started_at": "2018-01-12T11:00:00+00:00",
    "type": "compute_instance_disk",
    "user_id": "bd5874d6-6662-4b24-a9f01c128871e4ac"
}
`
