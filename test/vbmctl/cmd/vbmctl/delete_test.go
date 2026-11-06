//go:build vbmctl
// +build vbmctl

package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestReportDeletion(t *testing.T) {
	tests := []struct {
		name        string
		names       []string
		err         error
		expected    string
		notExpected string
	}{
		{
			name:        "error prints only the warning",
			names:       []string{"net1", "net2"},
			err:         errors.New("boom"),
			expected:    "Warning: failed to delete networks: boom\n",
			notExpected: "Deleted",
		},
		{
			name:     "no error prints the list",
			names:    []string{"net1", "net2"},
			expected: "Deleted networks:\n  - net1\n  - net2\n",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			reportDeletion(&buf, "networks", tc.names, tc.err)

			if buf.String() != tc.expected {
				t.Errorf("expected output %q, got %q", tc.expected, buf.String())
			}
			if tc.notExpected != "" && strings.Contains(buf.String(), tc.notExpected) {
				t.Errorf("output %q must not contain %q", buf.String(), tc.notExpected)
			}
		})
	}
}
