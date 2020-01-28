package flect

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_Capitalize(t *testing.T) {
	table := []tt{
		{"", ""},
		{"foo", "Foo"},
		{"bob dylan", "Bob dylan"},
		{"WidgetID", "WidgetID"},
		{"widget_id", "Widget_id"},
		{"widget_ID", "Widget_ID"},
		{"widget ID", "Widget ID"},
	}

	for _, tt := range table {
		t.Run(tt.act, func(st *testing.T) {
			r := require.New(st)
			r.Equal(tt.exp, Capitalize(tt.act))
			r.Equal(tt.exp, Capitalize(tt.exp))
		})
	}
}
