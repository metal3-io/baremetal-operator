package main

import (
	"os"
	"path"
	"path/filepath"
	"testing"

	"github.com/metal3-io/baremetal-operator/pkg/provisioner/ironic/clients"
)

func TestResolveAuthConfig(t *testing.T) {
	t.Run("URL credentials take precedence", func(t *testing.T) {
		urlAuth := clients.AuthConfig{
			Type:     clients.HTTPBasicAuth,
			Username: "urluser",
			Password: "urlpass",
		}

		auth, err := resolveAuthConfig(urlAuth)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if auth != urlAuth {
			t.Fatalf("expected URL auth to be returned unchanged, got %+v", auth)
		}
	})

	t.Run("falls back to LoadAuth when URL has no credentials", func(t *testing.T) {
		authRoot := filepath.Join(t.TempDir(), "auth")
		t.Setenv("METAL3_AUTH_ROOT_DIR", authRoot)

		authPath := path.Join(authRoot, "ironic")
		if err := os.MkdirAll(authPath, 0750); err != nil {
			t.Fatalf("failed to set up auth dir: %v", err)
		}
		if err := os.WriteFile(path.Join(authPath, "username"), []byte("fileuser"), 0600); err != nil {
			t.Fatalf("failed to write username file: %v", err)
		}
		if err := os.WriteFile(path.Join(authPath, "password"), []byte("filepass"), 0600); err != nil {
			t.Fatalf("failed to write password file: %v", err)
		}

		auth, err := resolveAuthConfig(clients.AuthConfig{Type: clients.NoAuth})
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if auth.Type != clients.HTTPBasicAuth || auth.Username != "fileuser" || auth.Password != "filepass" {
			t.Fatalf("expected credentials loaded from auth files, got %+v", auth)
		}
	})

	t.Run("no credentials anywhere yields NoAuth", func(t *testing.T) {
		t.Setenv("METAL3_AUTH_ROOT_DIR", filepath.Join(t.TempDir(), "missing"))

		auth, err := resolveAuthConfig(clients.AuthConfig{Type: clients.NoAuth})
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if auth.Type != clients.NoAuth {
			t.Fatalf("expected NoAuth, got %+v", auth)
		}
	})
}
