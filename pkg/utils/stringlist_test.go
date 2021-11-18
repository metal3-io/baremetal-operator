package utils

import (
	"testing"
)

func TestStringInList(t *testing.T) {
	cases := []struct {
		list   []string
		str    string
		expect bool
	}{
		{
			list:   []string{"a", "b", "c"},
			str:    "a",
			expect: true,
		},
		{
			list:   []string{"a", "b", "c"},
			str:    "d",
			expect: false,
		},
	}

	for _, c := range cases {
		got := StringInList(c.list, c.str)
		if c.expect != got {
			t.Errorf("Expected '%t', but got '%t'", c.expect, got)
		}

	}
}
