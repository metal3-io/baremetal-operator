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
	valid, err := creds.AreValid()
	if !valid {
		t.Fatalf("got unexpected validation error: %q", err)
	}
}

func TestMissingUser(t *testing.T) {
	creds := Credentials{
		Password: "password",
	}
	valid, err := creds.AreValid()
	if valid || err == nil {
		t.Fatal("got unexpected valid result")
	}
}

func TestMissingPassword(t *testing.T) {
	creds := Credentials{
		Username: "username",
	}
	valid, err := creds.AreValid()
	if valid || err == nil {
		t.Fatal("got unexpected valid result")
	}
}
