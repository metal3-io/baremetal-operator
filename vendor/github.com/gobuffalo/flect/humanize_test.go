package flect

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_Humanize(t *testing.T) {
	table := []tt{
		{"", ""},
		{"IBM", "IBM"},
		{"employee_mobile_number", "Employee mobile number"},
		{"employee_salary", "Employee salary"},
		{"employee_id", "Employee ID"},
		{"employee_ID", "Employee ID"},
		{"first_name", "First name"},
	}

	for _, tt := range table {
		t.Run(tt.act, func(st *testing.T) {
			r := require.New(st)
			r.Equal(tt.exp, Humanize(tt.act))
			r.Equal(tt.exp, Humanize(tt.exp))
		})
	}
}
