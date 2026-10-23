//go:build vbmctl
// +build vbmctl

package containers

import (
	"testing"

	"github.com/moby/moby/api/types/network"
)

func TestExactNetworkIDs(t *testing.T) {
	nets := []network.Summary{
		{Network: network.Network{Name: "pr-sub10", ID: "id10"}},
		{Network: network.Network{Name: "pr-sub1", ID: "id1"}},
		{Network: network.Network{Name: "xpr-sub1", ID: "idx"}},
	}

	if got := exactNetworkIDs(nets, "pr-sub1"); len(got) != 1 || got[0] != "id1" {
		t.Errorf("expected [id1], got %v", got)
	}
	if got := exactNetworkIDs(nets[:1], "pr-sub1"); len(got) != 0 {
		t.Errorf("expected no match for substring-only networks, got %v", got)
	}
}
