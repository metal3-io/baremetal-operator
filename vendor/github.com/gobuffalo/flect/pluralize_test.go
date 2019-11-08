package flect

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_Pluralize(t *testing.T) {
	for _, tt := range singlePluralAssertions {
		t.Run(tt.act, func(st *testing.T) {
			r := require.New(st)
			r.Equal(tt.exp, Pluralize(tt.act))
			r.Equal(tt.exp, Pluralize(tt.exp))
		})
	}
}
