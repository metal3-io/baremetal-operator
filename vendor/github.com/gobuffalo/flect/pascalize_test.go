package flect

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_Pascalize(t *testing.T) {
	table := []tt{
		{"", ""},
		{"bob dylan", "BobDylan"},
		{"widgetID", "WidgetID"},
		{"widget_ID", "WidgetID"},
		{"Widget_ID", "WidgetID"},
		{"Nice to see you!", "NiceToSeeYou"},
		{"*hello*", "Hello"},
		{"i've read a book! have you?", "IveReadABookHaveYou"},
		{"This is `code` ok", "ThisIsCodeOK"},
		{"id", "ID"},
	}

	for _, tt := range table {
		t.Run(tt.act, func(st *testing.T) {
			r := require.New(st)
			r.Equal(tt.exp, Pascalize(tt.act))
			r.Equal(tt.exp, Pascalize(tt.exp))
		})
	}
}
