package bmc

import (
	"testing"
)

func TestValidateSerialConsoleAddress(t *testing.T) {
	for _, address := range []string{"", "ipmi://bmc.example", "ipmi://192.0.2.1:6233", "ipmi://[2001:db8::1]:623?privilegelevel=OPERATOR", "ipmi://192.0.2.1:623?cipher_suite=3"} {
		t.Run(address, func(t *testing.T) {
			if err := ValidateSerialConsoleAddress(address); err != nil {
				t.Fatal(err)
			}
		})
	}
	for _, address := range []string{"redfish://bmc.example", "ipmi://user:pass@bmc.example", "ipmi://bmc.example:0", "ipmi://bmc.example:65536", "ipmi://bmc.example:abc", "ipmi://bmc.example/path", "ipmi://bmc.example?password=secret", "ipmi://bmc.example?privilegelevel=ROOT", "ipmi://", "ipmi://bmc.example?cipher_suite=0", "ipmi://bmc.example?cipher_suite=18", "ipmi://bmc.example?cipher_suite=", "ipmi://bmc.example?cipher_suite=invalid"} {
		t.Run(address, func(t *testing.T) {
			if err := ValidateSerialConsoleAddress(address); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestSerialConsoleCipherSuite(t *testing.T) {
	access, err := NewAccessDetails("ipmi://192.0.2.1:623?cipher_suite=3", false)
	if err != nil {
		t.Fatal(err)
	}
	if got := access.DriverInfo(Credentials{})["ipmi_cipher_suite"]; got != 3 {
		t.Fatalf("unexpected cipher suite %v", got)
	}
	access, err = NewAccessDetails("ipmi://192.0.2.1:623", false)
	if err != nil {
		t.Fatal(err)
	}
	if _, specified := access.DriverInfo(Credentials{})["ipmi_cipher_suite"]; specified {
		t.Fatal("cipher suite must remain a backend default unless explicitly supplied")
	}
}
