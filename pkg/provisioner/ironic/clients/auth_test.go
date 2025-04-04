package clients

import (
	"os"
	"path"
	"path/filepath"
	"testing"
)

func TestConfigFromEndpointURL(t *testing.T) {
	testCases := []struct {
		Scenario         string
		URL              string
		ExpectedEndpoint string
		ExpectedAuth     AuthConfig
		ExpectErr        bool
	}{
		{
			Scenario:  "garbage",
			URL:       "@foo::bar",
			ExpectErr: true,
		},
		{
			Scenario:         "no auth",
			URL:              "https://example.test/v1",
			ExpectedEndpoint: "https://example.test/v1",
			ExpectedAuth: AuthConfig{
				Type: NoAuth,
			},
		},
		{
			Scenario:         "username-only",
			URL:              "https://user@example.test/v1",
			ExpectedEndpoint: "https://example.test/v1",
			ExpectedAuth: AuthConfig{
				Type:     HTTPBasicAuth,
				Username: "user",
			},
			ExpectErr: true,
		},
		{
			Scenario:         "empty password",
			URL:              "https://user:@example.test/v1",
			ExpectedEndpoint: "https://example.test/v1",
			ExpectedAuth: AuthConfig{
				Type:     HTTPBasicAuth,
				Username: "user",
				Password: "",
			},
		},
		{
			Scenario:         "basic auth",
			URL:              "https://user:pass@example.test/v1",
			ExpectedEndpoint: "https://example.test/v1",
			ExpectedAuth: AuthConfig{
				Type:     HTTPBasicAuth,
				Username: "user",
				Password: "pass",
			},
		},
		{
			Scenario:         "IPv6 no auth",
			URL:              "https://[::1]/v1",
			ExpectedEndpoint: "https://[::1]/v1",
			ExpectedAuth: AuthConfig{
				Type: NoAuth,
			},
		},
		{
			Scenario:         "IPv6 basic auth",
			URL:              "https://user:pass@[::1]/v1",
			ExpectedEndpoint: "https://[::1]/v1",
			ExpectedAuth: AuthConfig{
				Type:     HTTPBasicAuth,
				Username: "user",
				Password: "pass",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Scenario, func(t *testing.T) {
			ep, auth, err := ConfigFromEndpointURL(tc.URL)
			if ep != tc.ExpectedEndpoint {
				t.Errorf("Expected endpoint '%s', got '%s'",
					tc.ExpectedEndpoint, ep)
			}
			if auth.Type != tc.ExpectedAuth.Type {
				t.Errorf("Unexpected auth type %s", auth.Type)
			}
			if auth.Username != tc.ExpectedAuth.Username {
				t.Errorf("Unexpected username '%s'", auth.Username)
			}
			if auth.Password != tc.ExpectedAuth.Password {
				t.Errorf("Unexpected password '%s'", auth.Password)
			}
			if (err != nil) != tc.ExpectErr {
				t.Errorf("Unexpected error %s", err)
			}
		})
	}
}

func TestLoadAuth(t *testing.T) {
	// Helper function to set up the environment
	setup := func(authRoot string, createFiles bool) (cleanup func(), err error) {
		originalAuthRoot := os.Getenv("METAL3_AUTH_ROOT_DIR")
		cleanup = func() {
			t.Setenv("METAL3_AUTH_ROOT_DIR", originalAuthRoot)
			_ = os.RemoveAll(authRoot)
		}

		t.Setenv("METAL3_AUTH_ROOT_DIR", authRoot)

		if createFiles {
			authPath := path.Join(authRoot, "ironic")
			err = os.MkdirAll(authPath, 0755)
			if err != nil {
				return cleanup, err
			}

			err = os.WriteFile(path.Join(authPath, "username"), []byte("testuser"), 0600)
			if err != nil {
				return cleanup, err
			}

			err = os.WriteFile(path.Join(authPath, "password"), []byte("testpassword"), 0600)
			if err != nil {
				return cleanup, err
			}
		}

		return cleanup, nil
	}

	t.Run("NoAuthDirectory", func(t *testing.T) {
		authRoot := filepath.Join(os.TempDir(), "auth_test_no_dir")
		cleanup, err := setup(authRoot, false)
		if err != nil {
			t.Fatalf("Failed to set up test: %v", err)
		}
		defer cleanup()

		auth, err := LoadAuth()
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if auth.Type != NoAuth {
			t.Fatalf("Expected NoAuth, got %v", auth.Type)
		}
	})

	t.Run("ValidAuth", func(t *testing.T) {
		authRoot := filepath.Join(os.TempDir(), "auth_test_valid")
		cleanup, err := setup(authRoot, true)
		if err != nil {
			t.Fatalf("Failed to set up test: %v", err)
		}
		defer cleanup()

		auth, err := LoadAuth()
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if auth.Type != HTTPBasicAuth {
			t.Fatalf("Expected HTTPBasicAuth, got %v", auth.Type)
		}
		if auth.Username != "testuser" {
			t.Fatalf("Expected username 'testuser', got %v", auth.Username)
		}
		if auth.Password != "testpassword" {
			t.Fatalf("Expected password 'testpassword', got %v", auth.Password)
		}
	})

	t.Run("EmptyUsername", func(t *testing.T) {
		authRoot := filepath.Join(os.TempDir(), "auth_test_empty_username")
		cleanup, err := setup(authRoot, true)
		if err != nil {
			t.Fatalf("Failed to set up test: %v", err)
		}
		defer cleanup()

		// Overwrite username file with empty content
		err = os.WriteFile(path.Join(authRoot, "ironic", "username"), []byte(""), 0600)
		if err != nil {
			t.Fatalf("Failed to overwrite username file: %v", err)
		}

		_, err = LoadAuth()
		if err == nil || err.Error() != "empty HTTP Basic Auth username" {
			t.Fatalf("Expected 'empty HTTP Basic Auth username' error, got %v", err)
		}
	})

	t.Run("EmptyPassword", func(t *testing.T) {
		authRoot := filepath.Join(os.TempDir(), "auth_test_empty_password")
		cleanup, err := setup(authRoot, true)
		if err != nil {
			t.Fatalf("Failed to set up test: %v", err)
		}
		defer cleanup()

		// Overwrite password file with empty content
		err = os.WriteFile(path.Join(authRoot, "ironic", "password"), []byte(""), 0600)
		if err != nil {
			t.Fatalf("Failed to overwrite password file: %v", err)
		}

		_, err = LoadAuth()
		if err == nil || err.Error() != "empty HTTP Basic Auth password" {
			t.Fatalf("Expected 'empty HTTP Basic Auth password' error, got %v", err)
		}
	})
}
