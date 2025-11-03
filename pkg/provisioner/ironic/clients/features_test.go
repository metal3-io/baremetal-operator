package clients

import (
	"fmt"
	"testing"
)

func TestAvailableFeatures_ChooseMicroversion(t *testing.T) {
	microVersion := "1.95"
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
			name: fmt.Sprintf("MaxVersion = %d return %s", 89, microVersion),
			feature: fields{
				MaxVersion: 95,
			},
			want: microVersion,
		},
		{
			name: fmt.Sprintf("MaxVersion > %d return %s", 89, microVersion),
			feature: fields{
				MaxVersion: 100,
			},
			want: microVersion,
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
