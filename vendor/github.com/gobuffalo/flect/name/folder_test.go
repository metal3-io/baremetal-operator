package name

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_Folder(t *testing.T) {
	table := []tt{
		{"", ""},
		{"foo_bar", "foo_bar"},
		{"admin/widget", "admin/widget"},
		{"admin/widgets", "admin/widgets"},
		{"widget", "widget"},
		{"widgets", "widgets"},
		{"User", "user"},
		{"U$er", "uer"},
		{"AdminUser", "admin/user"},
	}

	for _, tt := range table {
		t.Run(tt.act, func(st *testing.T) {
			r := require.New(st)
			r.Equal(tt.exp, Folder(tt.act))
			r.Equal(tt.exp, Folder(tt.exp))
			r.Equal(tt.exp+".a.b", Folder(tt.act, ".a", ".b"))
		})
	}
}
