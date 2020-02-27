package name

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_Ident_Key(t *testing.T) {
	table := map[string]string{
		"Foo/bar/baz":   "foo/bar/baz",
		"Foo\\bar\\baz": "foo/bar/baz",
	}

	for in, out := range table {
		t.Run(in, func(st *testing.T) {
			r := require.New(st)
			r.Equal(out, Key(in))
		})
	}
}
