package bmc

import (
	"testing"

	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

func init() {
	logf.SetLogger(logf.ZapLogger(true))
}

func TestValidCredentials(t *testing.T) {
	creds := Credentials{
		Username: "username",
		Password: "password",
	}
	valid, why := creds.AreValid()
	if !valid {
		t.Fatalf("got unexpected validation error: %q", why)
	}
}

func TestMissingUser(t *testing.T) {
	creds := Credentials{
		Password: "password",
	}
	valid, why := creds.AreValid()
	if valid {
		t.Fatal("got unexpected valid result")
	}
	if why != MissingUsernameMsg {
		t.Fatalf("got unexpected reason for invalid creds: %q", why)
	}
}

func TestMissingPassword(t *testing.T) {
	creds := Credentials{
		Username: "username",
	}
	valid, why := creds.AreValid()
	if valid {
		t.Fatal("got unexpected valid result")
	}
	if why != MissingPasswordMsg {
		t.Fatalf("got unexpected reason for invalid creds: %q", why)
	}
}
