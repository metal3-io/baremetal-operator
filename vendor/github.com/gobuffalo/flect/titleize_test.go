package flect

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_Titleize(t *testing.T) {
	table := []tt{
		{"", ""},
		{"bob dylan", "Bob Dylan"},
		{"Nice to see you!", "Nice To See You!"},
		{"*hello*", "*hello*"},
		{"i've read a book! have you?", "I've Read A Book! Have You?"},
		{"This is `code` ok", "This Is `code` OK"},
		{"foo_bar", "Foo Bar"},
		{"admin/widget", "Admin Widget"},
		{"widget", "Widget"},
	}

	for _, tt := range table {
		t.Run(tt.act, func(st *testing.T) {
			r := require.New(st)
			r.Equal(tt.exp, Titleize(tt.act))
			r.Equal(tt.exp, Titleize(tt.exp))
		})
	}
}
