package name

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_URL(t *testing.T) {
	table := []struct {
		in  string
		out string
	}{
		{"User", "users"},
		{"widget", "widgets"},
		{"AdminUser", "admin_users"},
		{"Admin/User", "admin/users"},
		{"Admin/Users", "admin/users"},
		{"/Admin/Users", "/admin/users"},
	}

	for _, tt := range table {
		t.Run(tt.in, func(st *testing.T) {
			r := require.New(st)
			n := New(tt.in)
			r.Equal(tt.out, n.URL().String(), "URL of %v", tt.in)
		})
	}
}
