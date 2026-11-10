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

package v1alpha1

// Hardware profiles are deprecated. They are a legacy mechanism for supplying
// default root device hints and they do not exist in v1beta1, which behaves as
// if the "empty" profile was always used.
//
// The profile table lives in this package, next to the API types, so that the
// v1alpha1 -> v1beta1 conversion can resolve the hints a profile implies. The
// profile package cannot be used for that: it imports this package, so
// importing it back here would create an import cycle. The profile package is
// a thin wrapper around the table below, which stays the single source of
// truth.

const (
	// DefaultProfileName is the default hardware profile to use when
	// no other profile matches.
	DefaultProfileName string = "unknown"

	// EmptyProfileName is the hardware profile without configuration.
	EmptyProfileName string = "empty"
)

// hardwareProfileRootDeviceHints holds the root device hints implied by each
// known hardware profile.
var hardwareProfileRootDeviceHints = map[string]RootDeviceHints{
	DefaultProfileName: {
		DeviceName: "/dev/sda",
	},
	"libvirt": {
		DeviceName: "/dev/vda",
	},
	"dell": {
		HCTL: "0:0:0:0",
	},
	"dell-raid": {
		HCTL: "0:2:0:0",
	},
	"openstack": {
		DeviceName: "/dev/vdb",
	},
	EmptyProfileName: {},
}

// HardwareProfileRootDeviceHints returns the root device hints implied by the
// named hardware profile. The second return value is false when the profile is
// not known.
func HardwareProfileRootDeviceHints(name string) (RootDeviceHints, bool) {
	hints, ok := hardwareProfileRootDeviceHints[name]
	return hints, ok
}
