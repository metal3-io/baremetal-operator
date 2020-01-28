package name

import (
	"go/build"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_Package(t *testing.T) {
	table := []tt{
		{"Foo", "foo"},
		{"Foo/Foo", "foo/foo"},
		{"Foo_Foo", "foofoo"},
		{"create_table", "createtable"},
		{"admin/widget", "admin/widget"},
		{"admin\\widget", "admin/widget"},
	}

	c := build.Default

	for _, src := range c.SrcDirs() {
		adds := []tt{
			{filepath.Join(src, "admin/widget"), "admin/widget"},
			{filepath.Join(src, "admin\\widget"), "admin/widget"},
			{filepath.Join(filepath.Dir(src), "admin/widget"), "admin/widget"},
			{filepath.Join(filepath.Dir(src), "admin\\widget"), "admin/widget"},
		}
		table = append(table, adds...)
	}
	for _, tt := range table {
		t.Run(tt.act, func(st *testing.T) {
			r := require.New(st)
			r.Equal(tt.exp, Package(tt.act))
			r.Equal(tt.exp, Package(tt.exp))
		})
	}
}
