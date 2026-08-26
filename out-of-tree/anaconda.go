/*

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Anaconda specific request parsing for the kickstart route.

package starlark

import (
	"net"
	"net/http"
	"slices"
	"strings"
)

// AnacondaMACHeaderPrefix is the wire prefix anaconda uses for the one header it
// emits per interface, enabled by the inst.ks.sendmac boot argument.
const AnacondaMACHeaderPrefix = "X-RHN-Provisioning-MAC-"

// DefaultFallbackKickstart is served when no BareMetalHost matches the caller. It
// carries no storage commands and powers the machine off from %pre, so nothing is written.
const DefaultFallbackKickstart = `# No BareMetalHost matches this machine.
# Deliberately empty of storage commands, nothing is installed.
%pre --interpreter=/bin/bash
echo "starlark provisioner: no BareMetalHost matches this machine, refusing to install" > /dev/console
/sbin/poweroff -f
%end
`

// NormalizeMAC returns the canonical lowercase form of a MAC address, or empty when
// it does not parse, so colon, hyphen and dotted notations all compare equal.
func NormalizeMAC(s string) string {
	hw, err := net.ParseMAC(strings.TrimSpace(s))
	if err != nil {
		return ""
	}

	return strings.ToLower(hw.String())
}

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

			if mac := NormalizeMAC(fields[len(fields)-1]); mac != "" {
				seen[mac] = true
			}
		}
	}

	out := make([]string, 0, len(seen))
	for mac := range seen {
		out = append(out, mac)
	}

	// Header iteration order is random, so sort for a stable result.
	slices.Sort(out)

	return out
}
