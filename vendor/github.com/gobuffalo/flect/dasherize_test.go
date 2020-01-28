package flect

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_Dasherize(t *testing.T) {
	table := []tt{
		{"", ""},
		{"admin/WidgetID", "admin-widget-id"},
		{"Donald E. Knuth", "donald-e-knuth"},
		{"Random text with *(bad)* characters", "random-text-with-bad-characters"},
		{"Trailing bad characters!@#", "trailing-bad-characters"},
		{"!@#Leading bad characters", "leading-bad-characters"},
		{"Squeeze   separators", "squeeze-separators"},
		{"Test with + sign", "test-with-sign"},
		{"Test with malformed utf8 \251", "test-with-malformed-utf8"},
	}

	for _, tt := range table {
		t.Run(tt.act, func(st *testing.T) {
			r := require.New(st)
			r.Equal(tt.exp, Dasherize(tt.act))
			r.Equal(tt.exp, Dasherize(tt.exp))
		})
	}
}
