package testing

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	th "github.com/gophercloud/gophercloud/testhelper"
	fake "github.com/gophercloud/gophercloud/testhelper/client"
	"github.com/gophercloud/utils/openstack/objectstorage/v1/objects"
)

const multipartManifest = `
[
    {
        "bytes": 104857600,
        "content_type": "application/swiftclient-segment",
        "hash": "a8b539f420dc1f47a6721cec22efeab3",
        "last_modified": "2018-04-22T01:34:00.000000",
        "name": "/testContainer/testObject/slo/1524360755.633149/289669120/104857600/00000000"
    },
    {
        "bytes": 104857600,
        "content_type": "application/swiftclient-segment",
        "hash": "7fd240a4032a676efd518ffa8601cde1",
        "last_modified": "2018-04-22T01:34:00.000000",
        "name": "/testContainer/testObject/slo/1524360755.633149/289669120/104857600/00000001"
    },
    {
        "bytes": 79953920,
        "content_type": "application/swiftclient-segment",
        "hash": "96414e8a758f1ba7107fd03bc5fc4741",
        "last_modified": "2018-04-22T01:34:00.000000",
        "name": "/testContainer/testObject/slo/1524360755.633149/289669120/104857600/00000002"
    }
]
`

var expectedMultipartManifest = []objects.Manifest{
	{
		Bytes:        104857600,
		ContentType:  "application/swiftclient-segment",
		Hash:         "a8b539f420dc1f47a6721cec22efeab3",
		LastModified: time.Date(2018, 4, 22, 1, 34, 0, 0, time.UTC),
		Name:         "/testContainer/testObject/slo/1524360755.633149/289669120/104857600/00000000",
	},
	{
		Bytes:        104857600,
		ContentType:  "application/swiftclient-segment",
		Hash:         "7fd240a4032a676efd518ffa8601cde1",
		LastModified: time.Date(2018, 4, 22, 1, 34, 0, 0, time.UTC),
		Name:         "/testContainer/testObject/slo/1524360755.633149/289669120/104857600/00000001",
	},
	{
		Bytes:        79953920,
		ContentType:  "application/swiftclient-segment",
		Hash:         "96414e8a758f1ba7107fd03bc5fc4741",
		LastModified: time.Date(2018, 4, 22, 1, 34, 0, 0, time.UTC),
		Name:         "/testContainer/testObject/slo/1524360755.633149/289669120/104857600/00000002",
	},
}

// HandleDownloadManifestSuccessfully creates an HTTP handler at
// `/testContainer/testObject` on the test handler mux that responds with a
// Download response.
func HandleDownloadManifestSuccessfully(t *testing.T) {
	th.Mux.HandleFunc("/testContainer/testObject", func(w http.ResponseWriter, r *http.Request) {
		th.TestMethod(t, r, "GET")
		th.TestHeader(t, r, "X-Auth-Token", fake.TokenID)
		th.TestHeader(t, r, "Accept", "application/json")
		w.Header().Set("Date", "Wed, 10 Nov 2009 23:00:00 GMT")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, multipartManifest)
	})
}
