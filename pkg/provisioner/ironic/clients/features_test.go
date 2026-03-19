package clients

import (
	"fmt"
	"testing"
)

func TestAvailableFeatures_ChooseMicroversion(t *testing.T) {
	type fields struct {
		MaxVersion int
	}
	tests := []struct {
		name    string
		feature fields
		want    string
	}{
		{
			name: fmt.Sprintf("MaxVersion < %d return microversion %s", 89, baselineVersionString),
			feature: fields{
				MaxVersion: 50,
			},
			want: baselineVersionString,
		},
		{
			name: "MaxVersion = 95 return 1.95",
			feature: fields{
				MaxVersion: 95,
			},
			want: "1.95",
		},
		{
			name: "MaxVersion = 100 return 1.95",
			feature: fields{
				MaxVersion: 100,
			},
			want: "1.95",
		},
		{
			name: "MaxVersion = 109 return 1.109",
			feature: fields{
				MaxVersion: 109,
			},
			want: "1.109",
		},
		{
			name: "MaxVersion > 109 return 1.109",
			feature: fields{
				MaxVersion: 115,
			},
			want: "1.109",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			af := AvailableFeatures{
				MaxVersion: tt.feature.MaxVersion,
			}
			if got := af.ChooseMicroversion(); got != tt.want {
				t.Errorf("ChooseMicroversion() = %v, want %v", got, tt.want)
			}
		})
	}
}
