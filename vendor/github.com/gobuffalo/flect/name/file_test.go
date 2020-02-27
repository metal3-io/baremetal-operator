package name

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_File(t *testing.T) {
	table := []tt{
		{"", ""},
		{"foo_bar", "foo_bar"},
		{"admin/widget", "admin/widget"},
		{"admin/widgets", "admin/widgets"},
		{"widget", "widget"},
		{"widgets", "widgets"},
		{"User", "user"},
		{"U$er", "u_er"},
	}

	for _, tt := range table {
		t.Run(tt.act, func(st *testing.T) {
			r := require.New(st)
			r.Equal(tt.exp, File(tt.act))
			r.Equal(tt.exp, File(tt.exp))
			r.Equal(tt.exp+".a.b", File(tt.act, ".a", ".b"))
		})
	}
}
