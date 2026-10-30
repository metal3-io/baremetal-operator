package bmc

import (
	"errors"
	"fmt"
	"net/url"
	"strconv"
)

// ValidateSerialConsoleAddress validates an explicit IPMI SOL address. Passwords
// belong in the BMC Secret, not in URLs that can appear in logs and status.
func ValidateSerialConsoleAddress(address string) error {
	if address == "" {
		return nil
	}
	parsed, err := url.Parse(address)
	if err != nil {
		return fmt.Errorf("invalid serial console address: %w", err)
	}
	if parsed.Scheme != "ipmi" || parsed.Hostname() == "" || parsed.User != nil || parsed.Path != "" || parsed.Fragment != "" {
		return errors.New("serial console address must be an ipmi://host[:port] URL without credentials or path")
	}
	if _, err = NewAccessDetails(address, false); err != nil {
		return err
	}
	if port := parsed.Port(); port != "" {
		n, portErr := strconv.ParseUint(port, 10, 16)
		if portErr != nil || n == 0 {
			return errors.New("invalid serial console IPMI port")
		}
	}
	query, err := url.ParseQuery(parsed.RawQuery)
	if err != nil {
		return fmt.Errorf("invalid serial console query: %w", err)
	}
	for key, values := range query {
		if (key != "privilegelevel" && key != "cipher_suite") || len(values) != 1 {
			return fmt.Errorf("unsupported serial console URL parameter %q", key)
		}
		if key == "cipher_suite" {
			if values[0] == "" {
				return errors.New("serial console cipher_suite must not be empty")
			}
			continue // NewAccessDetails validated the cipher suite above.
		}
		switch values[0] {
		case "ADMINISTRATOR", "OPERATOR", "USER", "CALLBACK":
		default:
			return errors.New("invalid serial console privilege level")
		}
	}
	return nil
}
