package name

import (
	"testing"

	"github.com/stretchr/testify/require"
)

type tt struct {
	act string
	exp string
}

func Test_Name(t *testing.T) {
	table := []tt{
		{"", ""},
		{"bob dylan", "BobDylan"},
		{"widgetID", "WidgetID"},
		{"widget_ID", "WidgetID"},
		{"Widget_ID", "WidgetID"},
		{"Widget_Id", "WidgetID"},
		{"Widget_id", "WidgetID"},
		{"Nice to see you!", "NiceToSeeYou"},
		{"*hello*", "Hello"},
		{"i've read a book! have you?", "IveReadABookHaveYou"},
		{"This is `code` ok", "ThisIsCodeOK"},
		{"foo_bar", "FooBar"},
		{"admin/widget", "AdminWidget"},
		{"admin/widgets", "AdminWidget"},
		{"widget", "Widget"},
		{"widgets", "Widget"},
		{"status", "Status"},
		{"Statuses", "Status"},
		{"statuses", "Status"},
		{"People", "Person"},
		{"people", "Person"},
	}

	for _, tt := range table {
		t.Run(tt.act, func(st *testing.T) {
			r := require.New(st)
			r.Equal(tt.exp, Proper(tt.act))
			r.Equal(tt.exp, Proper(tt.exp))
		})
	}
}

func Test_Group(t *testing.T) {
	table := []tt{
		{"", ""},
		{"Person", "People"},
		{"foo_bar", "FooBars"},
		{"admin/widget", "AdminWidgets"},
		{"widget", "Widgets"},
		{"widgets", "Widgets"},
		{"greatPerson", "GreatPeople"},
		{"great/person", "GreatPeople"},
		{"status", "Statuses"},
		{"Status", "Statuses"},
		{"Statuses", "Statuses"},
		{"statuses", "Statuses"},
	}

	for _, tt := range table {
		t.Run(tt.act, func(st *testing.T) {
			r := require.New(st)
			r.Equal(tt.exp, Group(tt.act))
			r.Equal(tt.exp, Group(tt.exp))
		})
	}
}

func Test_MarshalText(t *testing.T) {
	r := require.New(t)

	n := New("mark")
	b, err := n.MarshalText()
	r.NoError(err)
	r.Equal("mark", string(b))

	r.NoError((&n).UnmarshalText([]byte("bates")))
	r.Equal("bates", n.String())
}
