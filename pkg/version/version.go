// Package version includes the version information.
package version //nolint:revive

var (
	// Raw is the string representation of the version. This will be replaced
	// with the calculated version at build time.
	// set in the Makefile.
	Raw = "was not built with version info"

	// String is the human-friendly representation of the version.
	String = "metal3-io/baremetal-operator " + Raw

	// Commit is the commit hash from which the software was built.
	// Set via LDFLAGS in Makefile.
	Commit = "unknown"

	// BuildTime is the string representation of build time.
	// Set via LDFLAGS in Makefile.
	BuildTime = "unknown"
)
