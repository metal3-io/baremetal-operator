package flect

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_New(t *testing.T) {
	table := []Ident{
		{"", []string{}},
		{"widget", []string{"widget"}},
		{"widget_id", []string{"widget", "ID"}},
		{"WidgetID", []string{"Widget", "ID"}},
		{"Widget_ID", []string{"Widget", "ID"}},
		{"widget_ID", []string{"widget", "ID"}},
		{"widget/ID", []string{"widget", "ID"}},
		{"widgetID", []string{"widget", "ID"}},
		{"widgetName", []string{"widget", "Name"}},
		{"JWTName", []string{"JWT", "Name"}},
		{"JWTname", []string{"JWTname"}},
		{"jwtname", []string{"jwtname"}},
		{"sql", []string{"SQL"}},
		{"sQl", []string{"SQL"}},
		{"id", []string{"ID"}},
		{"Id", []string{"ID"}},
		{"iD", []string{"ID"}},
		{"html", []string{"HTML"}},
		{"Html", []string{"HTML"}},
		{"HTML", []string{"HTML"}},
		{"with `code` inside", []string{"with", "`code`", "inside"}},
		{"Donald E. Knuth", []string{"Donald", "E.", "Knuth"}},
		{"Random text with *(bad)* characters", []string{"Random", "text", "with", "*(bad)*", "characters"}},
		{"Allow_Under_Scores", []string{"Allow", "Under", "Scores"}},
		{"Trailing bad characters!@#", []string{"Trailing", "bad", "characters!@#"}},
		{"!@#Leading bad characters", []string{"!@#", "Leading", "bad", "characters"}},
		{"Squeeze	 separators", []string{"Squeeze", "separators"}},
		{"Test with + sign", []string{"Test", "with", "sign"}},
		{"Malmö", []string{"Malmö"}},
		{"Garçons", []string{"Garçons"}},
		{"Opsů", []string{"Opsů"}},
		{"Ærøskøbing", []string{"Ærøskøbing"}},
		{"Aßlar", []string{"Aßlar"}},
		{"Japanese: 日本語", []string{"Japanese", "日本語"}},
	}

	for _, tt := range table {
		t.Run(tt.Original, func(st *testing.T) {
			r := require.New(st)
			i := New(tt.Original)
			r.Equal(tt.Original, i.Original)
			r.Equal(tt.Parts, i.Parts)
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
