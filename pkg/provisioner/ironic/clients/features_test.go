package clients

import (
	"fmt"
	"testing"
)

func TestAvailableFeatures_ChooseMicroversion(t *testing.T) {
	microVersion := "1.86"
	type fields struct {
		MaxVersion int
	}
	tests := []struct {
		name    string
		feature fields
		want    string
	}{
		{
			name: fmt.Sprintf("MaxVersion < %d return microversion %s", 86, baseline),
			feature: fields{
				MaxVersion: 50,
			},
			want: baseline,
		},
		{
			name: fmt.Sprintf("MaxVersion = %d return %s", 86, microVersion),
			feature: fields{
				MaxVersion: 86,
			},
			want: microVersion,
		},
		{
			name: fmt.Sprintf("MaxVersion > %d return %s", 86, microVersion),
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

func TestAvailableFeatures_HasFirmwareUpdates(t *testing.T) {
	maxVersion := 86
	type fields struct {
		MaxVersion int
	}
	tests := []struct {
		name    string
		feature fields
		want    bool
	}{
		{
			name: fmt.Sprintf("Firmware < %d", maxVersion),
			feature: fields{
				MaxVersion: 50,
			},
			want: false,
		},
		{
			name: fmt.Sprintf("Firmware = %d", maxVersion),
			feature: fields{
				MaxVersion: 86,
			},
			want: true,
		},
		{
			name: fmt.Sprintf("Firmware > %d", maxVersion),
			feature: fields{
				MaxVersion: 100,
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			af := AvailableFeatures{
				MaxVersion: tt.feature.MaxVersion,
			}
			if got := af.HasFirmwareUpdates(); got != tt.want {
				t.Errorf("HasFirmwareUpdates() = %v, want %v", got, tt.want)
			}
		})
	}
}
