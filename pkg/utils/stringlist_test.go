package utils

import (
	"reflect"
	"testing"
)

func TestFilterStringFromList(t *testing.T) {
	cases := []struct {
		list      []string
		str       string
		expectStr []string
	}{
		{
			list:      []string{"a", "b", "c"},
			str:       "a",
			expectStr: []string{"b", "c"},
		},
		{
			list:      []string{"a", "b", "c"},
			str:       "d",
			expectStr: []string{"a", "b", "c"},
		},
	}

	for _, c := range cases {
		newList := FilterStringFromList(c.list, c.str)
		if !reflect.DeepEqual(newList, c.expectStr) {
			t.Errorf("Expected '%s', but got '%s'", c.expectStr, newList)
		}
	}
}
