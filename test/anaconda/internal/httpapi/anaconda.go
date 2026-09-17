// SPDX-License-Identifier: Apache-2.0

// Anaconda specific request parsing for the kickstart route.

package httpapi

import (
	"net/http"
	"slices"
	"strings"

	"metal3.local/anaconda/internal/core"
)

// AnacondaMACHeaderPrefix is the wire prefix anaconda uses for the one header it
// emits per interface, enabled by the inst.ks.sendmac boot argument.
const AnacondaMACHeaderPrefix = "X-RHN-Provisioning-MAC-"

// AnacondaMACs returns the normalized MACs anaconda reported, sorted and deduplicated.
// Each header value is "<ifname> <mac>", so only the last field is the address.
func AnacondaMACs(r *http.Request) []string {
	seen := map[string]bool{}

	for name, values := range r.Header {
		// Go canonicalizes the header name on the wire to X-Rhn-Provisioning-Mac-N,
		// so this compare has to ignore case or it silently matches nothing.
		if !strings.HasPrefix(strings.ToLower(name), strings.ToLower(AnacondaMACHeaderPrefix)) {
			continue
		}

		for _, v := range values {
			fields := strings.Fields(v)
			if len(fields) == 0 {
				continue
			}

			if mac := core.NormalizeMAC(fields[len(fields)-1]); mac != "" {
				seen[mac] = true
			}
		}
	}

	out := make([]string, 0, len(seen))
	for mac := range seen {
		out = append(out, mac)
	}

	// Map iteration order is random, so sort for a stable result.
	slices.Sort(out)

	return out
}

// DefaultFallbackKickstart is served when no BareMetalHost matches the caller. It
// carries no storage commands and powers the machine off from %pre, so nothing is written.
const DefaultFallbackKickstart = `# No BareMetalHost matches this machine.
# Deliberately empty of storage commands, nothing is installed.
%pre --interpreter=/bin/bash
echo "anaconda provisioner: no BareMetalHost matches this machine, refusing to install" > /dev/console
/sbin/poweroff -f
%end
`
