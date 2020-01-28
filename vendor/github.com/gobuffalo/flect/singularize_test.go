package flect

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_Singularize(t *testing.T) {
	for _, tt := range pluralSingularAssertions {
		t.Run(tt.exp, func(st *testing.T) {
			r := require.New(st)
			r.Equal(tt.exp, Singularize(tt.act))
			r.Equal(tt.exp, Singularize(tt.exp))
		})
	}
}
