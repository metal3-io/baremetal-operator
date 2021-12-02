package clients

import (
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
