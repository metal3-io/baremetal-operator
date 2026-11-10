package profile

import (
	"fmt"

	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
)

// Hardware profiles are deprecated and do not exist in the v1beta1 API. This
// package is a thin wrapper around the profile table defined in the v1alpha1
// package, which owns the data so that the conversion code can resolve the
// hints a profile implies without an import cycle.

const (
	// DefaultProfileName is the default hardware profile to use when
	// no other profile matches.
	DefaultProfileName = metal3api.DefaultProfileName

	// EmptyProfileName is the hardware profile without configuration.
	EmptyProfileName = metal3api.EmptyProfileName
)

// Profile holds the settings for a class of hardware.
type Profile struct {
	// Name holds the profile name
	Name string

	// RootDeviceHints holds the suggestions for placing the storage
	// for the root filesystem.
	RootDeviceHints metal3api.RootDeviceHints
}

// GetProfile returns the named profile.
func GetProfile(name string) (Profile, error) {
	hints, ok := metal3api.HardwareProfileRootDeviceHints(name)
	if !ok {
		return Profile{}, fmt.Errorf("no hardware profile named %q", name)
	}
	return Profile{
		Name:            name,
		RootDeviceHints: hints,
	}, nil
}
