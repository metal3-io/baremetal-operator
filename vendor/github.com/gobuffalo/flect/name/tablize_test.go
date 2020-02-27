package name

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_Tableize(t *testing.T) {
	table := []tt{
		{"", ""},
		{"bob dylan", "bob_dylans"},
		{"Nice to see you!", "nice_to_see_yous"},
		{"*hello*", "hellos"},
		{"i've read a book! have you?", "ive_read_a_book_have_yous"},
		{"This is `code` ok", "this_is_code_oks"},
		{"foo_bar", "foo_bars"},
		{"admin/widget", "admin_widgets"},
		{"widget", "widgets"},
		{"widgets", "widgets"},
		{"status", "statuses"},
		{"Statuses", "statuses"},
		{"statuses", "statuses"},
		{"People", "people"},
		{"people", "people"},
		{"BigPerson", "big_people"},
	}

	for _, tt := range table {
		t.Run(tt.act, func(st *testing.T) {
			r := require.New(st)
			r.Equal(tt.exp, Tableize(tt.act))
			r.Equal(tt.exp, Tableize(tt.exp))
		})
	}
}
