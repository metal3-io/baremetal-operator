package name

import (
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_Ident_FilePathJoin(t *testing.T) {
	table := map[string]string{
		"foo/bar/baz":   "foo/bar/baz/boo",
		"foo\\bar\\baz": "foo/bar/baz/boo",
	}

	if runtime.GOOS == "windows" {
		table = ident_FilePathJoin_Windows_Table()
	}

	for in, out := range table {
		t.Run(in, func(st *testing.T) {
			r := require.New(st)
			r.Equal(out, FilePathJoin(in, "boo"))
		})
	}
}

func ident_FilePathJoin_Windows_Table() map[string]string {
	return map[string]string{
		"foo/bar/baz":   "foo\\bar\\baz\\boo",
		"foo\\bar\\baz": "foo\\bar\\baz\\boo",
	}
}
