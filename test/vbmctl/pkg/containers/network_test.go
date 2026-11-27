//go:build vbmctl
// +build vbmctl

package containers

import (
	"bytes"
	"errors"
	"log"
	"strconv"
	"testing"

	"github.com/moby/moby/api/types/network"
)

// networkList returns networks with the given names. The ID of each network is
// "id-" followed by its position in the list.
func networkList(names ...string) []network.Summary {
	list := make([]network.Summary, 0, len(names))
	for i, name := range names {
		list = append(list, network.Summary{Network: network.Network{Name: name, ID: "id-" + strconv.Itoa(i)}})
	}
	return list
}

func TestSelectNetworkByName(t *testing.T) {
	tests := []struct {
		name        string
		networks    []network.Summary
		lookup      string
		wantID      string
		wantErr     error
		wantWarning bool
	}{
		{
			name:     "picks the exact match among substring matches",
			networks: networkList("pr3677-sub10", "pr3677-sub1", "my-pr3677-sub1"),
			lookup:   "pr3677-sub1",
			wantID:   "id-1",
		},
		{
			name:     "does not warn about a longer name that contains the name",
			networks: networkList("pr3677-busy", "pr3677-busy2"),
			lookup:   "pr3677-busy",
			wantID:   "id-0",
		},
		{
			name:     "returns not found if only a longer name contains the name",
			networks: networkList("pr3677-sub10"),
			lookup:   "pr3677-sub1",
			wantErr:  ErrNetworkNotFound,
		},
		{
			name:     "returns not found if the name only matches as a pattern",
			networks: networkList("netx1"),
			lookup:   "net.1",
			wantErr:  ErrNetworkNotFound,
		},
		{
			name:    "returns not found for an empty list",
			lookup:  "pr3677-sub1",
			wantErr: ErrNetworkNotFound,
		},
		{
			name:        "returns the first network and warns if the exact name is duplicated",
			networks:    networkList("pr3677-sub10", "pr3677-sub1", "pr3677-sub1"),
			lookup:      "pr3677-sub1",
			wantID:      "id-1",
			wantWarning: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var logs bytes.Buffer
			orig := log.Writer()
			log.SetOutput(&logs)
			t.Cleanup(func() { log.SetOutput(orig) })

			id, err := selectNetworkByName(tt.networks, tt.lookup)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("expected error wrapping %v, got: %v", tt.wantErr, err)
			}
			if id != tt.wantID {
				t.Errorf("expected network ID %q, got %q", tt.wantID, id)
			}
			if warned := logs.Len() > 0; warned != tt.wantWarning {
				t.Errorf("expected warning logged: %v, got log output %q", tt.wantWarning, logs.String())
			}
		})
	}
}
