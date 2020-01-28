package name

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_Char(t *testing.T) {
	table := []tt{
		{"", "x"},
		{"foo_bar", "f"},
		{"admin/widget", "a"},
		{"123d4545", "d"},
		{"!@#$%^&*", "x"},
	}

	for _, tt := range table {
		t.Run(tt.act, func(st *testing.T) {
			r := require.New(st)
			r.Equal(tt.exp, Char(tt.act))
			r.Equal(tt.exp, Char(tt.exp))
		})
	}
}
