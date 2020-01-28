package name

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_Name_Resource(t *testing.T) {
	r := require.New(t)
	table := []struct {
		V string
		E string
	}{
		{V: "Person", E: "People"},
		{V: "foo_bar", E: "FooBars"},
		{V: "admin/widget", E: "AdminWidgets"},
		{V: "widget", E: "Widgets"},
		{V: "widgets", E: "Widgets"},
		{V: "greatPerson", E: "GreatPeople"},
		{V: "great/person", E: "GreatPeople"},
		{V: "status", E: "Statuses"},
		{V: "Status", E: "Statuses"},
		{V: "Statuses", E: "Statuses"},
		{V: "statuses", E: "Statuses"},
	}
	for _, tt := range table {
		r.Equal(tt.E, New(tt.V).Resource().String())
	}
}
