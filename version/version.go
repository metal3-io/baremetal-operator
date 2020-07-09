// Package version includes the version information.
package version

var (
	// Raw is the string representation of the version. This will be replaced
	// with the calculated version at build time.
	// set in the Makefile.
	Raw = "was not built with version info"

	// Commit is the commit hash from which the software was built.
	// Set via LDFLAGS in Makefile.
	Commit = ""
)
