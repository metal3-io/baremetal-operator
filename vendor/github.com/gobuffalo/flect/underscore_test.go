package flect

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_Underscore(t *testing.T) {
	baseAcronyms["TLC"] = true

	table := []tt{
		{"", ""},
		{"bob dylan", "bob_dylan"},
		{"Nice to see you!", "nice_to_see_you"},
		{"*hello*", "hello"},
		{"i've read a book! have you?", "ive_read_a_book_have_you"},
		{"This is `code` ok", "this_is_code_ok"},
		{"TLCForm", "tlc_form"},
	}

	for _, tt := range table {
		t.Run(tt.act, func(st *testing.T) {
			r := require.New(st)
			r.Equal(tt.exp, Underscore(tt.act))
			r.Equal(tt.exp, Underscore(tt.exp))
		})
	}
}
