package fuzz

import (
	"testing"

	"github.com/metal3-io/baremetal-operator/pkg/hardwareutils/bmc"
)

// FuzzCredentialsValidate tests BMC credentials validation with various
// username/password combinations to ensure robust validation.
func FuzzCredentialsValidate(f *testing.F) {
	// Valid credentials: both fields non-empty, Validate should accept.
	f.Add("admin", "password123")
	f.Add("root", "p@ssw0rd!")
	f.Add("user123", "very-long-password-with-special-chars!@#$%")
	f.Add("ADMIN", "ADMIN")
	f.Add("admin", "admin")
	f.Add("root", "calvin")
	f.Add("Administrator", "password")
	f.Add("ipmiadmin", "IpmiPass2023!")
	f.Add("redfish", "R3dfish-Secret")
	f.Add("operator", "Ch@ngeMe123")
	f.Add("admin", "Sup3r$ecretP@ssw0rdWithLength1234")
	f.Add("bmc-user", "P@ss w0rd With Spaces")
	f.Add("  ", "  ")
	f.Add("user\twith\ttabs", "pass\nwith\nnewlines")

	// Invalid credentials: at least one empty field, Validate should reject.
	f.Add("", "password")
	f.Add("user", "")
	f.Add("", "")

	f.Fuzz(func(t *testing.T, username, password string) {
		creds := bmc.Credentials{
			Username: username,
			Password: password,
		}

		err := creds.Validate()

		// Verify validation logic invariants
		if username == "" || password == "" {
			// Should return error when either field is empty
			if err == nil {
				t.Errorf("expected error for credentials (username=%q, password=%q), got nil",
					username, password)
			}
		} else {
			// Should succeed when both fields are non-empty
			if err != nil {
				t.Errorf("unexpected error for valid credentials (username=%q, password=%q): %v",
					username, password, err)
			}
		}
	})
}
