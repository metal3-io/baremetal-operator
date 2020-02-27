package name

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_ParamID(t *testing.T) {
	table := []tt{
		{"foo_bar", "foo_bar_id"},
		{"admin/widget", "admin_widget_id"},
		{"admin/widgets", "admin_widget_id"},
		{"widget", "widget_id"},
		{"User", "user_id"},
		{"user", "user_id"},
		{"UserID", "user_id"},
	}

	for _, tt := range table {
		t.Run(tt.act, func(st *testing.T) {
			r := require.New(st)
			r.Equal(tt.exp, ParamID(tt.act))
			r.Equal(tt.exp, ParamID(tt.exp))
		})
	}
}
