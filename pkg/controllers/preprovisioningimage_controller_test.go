package controllers

import (
	"testing"

	"github.com/go-logr/logr"
	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	"github.com/metal3-io/baremetal-operator/pkg/imageprovider"
	"github.com/stretchr/testify/assert"
	ctrl "sigs.k8s.io/controller-runtime"
)

type mockImageProvider struct {
	supportedFormats map[metal3api.ImageFormat]bool
}

func (m *mockImageProvider) SupportsFormat(format metal3api.ImageFormat) bool {
	return m.supportedFormats[format]
}

func (m *mockImageProvider) SupportsArchitecture(arch string) bool {
	return true
}

func (m *mockImageProvider) BuildImage(_ imageprovider.ImageData, _ imageprovider.NetworkData, _ logr.Logger) (imageprovider.GeneratedImage, error) {
	return imageprovider.GeneratedImage{}, nil
}

func (m *mockImageProvider) DiscardImage(_ imageprovider.ImageData) error {
	return nil
}

func TestCanStart(t *testing.T) {
	testCases := []struct {
		name           string
		supportedFmts  map[metal3api.ImageFormat]bool
		expectedResult bool
	}{
		{
			name: "supports iso only",
			supportedFmts: map[metal3api.ImageFormat]bool{
				metal3api.ImageFormatISO: true,
			},
			expectedResult: true,
		},
		{
			name: "supports initrd only",
			supportedFmts: map[metal3api.ImageFormat]bool{
				metal3api.ImageFormatInitRD: true,
			},
			expectedResult: true,
		},
		{
			name: "supports both formats",
			supportedFmts: map[metal3api.ImageFormat]bool{
				metal3api.ImageFormatISO:    true,
				metal3api.ImageFormatInitRD: true,
			},
			expectedResult: true,
		},
		{
			name:           "cannot start with no formats",
			supportedFmts:  map[metal3api.ImageFormat]bool{},
			expectedResult: false,
		},
		{
			name: "cannot start with unknown format",
			supportedFmts: map[metal3api.ImageFormat]bool{
				"unknown": true,
			},
			expectedResult: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			r := &PreprovisioningImageReconciler{
				ImageProvider: &mockImageProvider{supportedFormats: tc.supportedFmts},
				Log:           ctrl.Log.WithName("test"),
			}
			assert.Equal(t, tc.expectedResult, r.CanStart())
		})
	}
}
func TestConfigChanged(t *testing.T) {
	testCases := []struct {
		name               string
		img                *metal3api.PreprovisioningImage
		format             metal3api.ImageFormat
		networkDataStatus  metal3api.SecretStatus
		expectedHasChanged bool
	}{
		{
			name: "no changes",
			img: &metal3api.PreprovisioningImage{
				Status: metal3api.PreprovisioningImageStatus{
					Format:       metal3api.ImageFormatISO,
					Architecture: "x86_64",
					NetworkData: metal3api.SecretStatus{
						Name:    "test-net",
						Version: "1",
					},
				},
				Spec: metal3api.PreprovisioningImageSpec{
					Architecture: "x86_64",
				},
			},
			format: metal3api.ImageFormatISO,
			networkDataStatus: metal3api.SecretStatus{
				Name:    "test-net",
				Version: "1",
			},
			expectedHasChanged: false,
		},
		{
			name: "format changed",
			img: &metal3api.PreprovisioningImage{
				Status: metal3api.PreprovisioningImageStatus{
					Format:       metal3api.ImageFormatISO,
					Architecture: "x86_64",
					NetworkData: metal3api.SecretStatus{
						Name:    "test-net",
						Version: "1",
					},
				},
				Spec: metal3api.PreprovisioningImageSpec{
					Architecture: "x86_64",
				},
			},
			format: metal3api.ImageFormatInitRD,
			networkDataStatus: metal3api.SecretStatus{
				Name:    "test-net",
				Version: "1",
			},
			expectedHasChanged: true,
		},
		{
			name: "architecture changed",
			img: &metal3api.PreprovisioningImage{
				Status: metal3api.PreprovisioningImageStatus{
					Format:       metal3api.ImageFormatISO,
					Architecture: "x86_64",
					NetworkData: metal3api.SecretStatus{
						Name:    "test-net",
						Version: "1",
					},
				},
				Spec: metal3api.PreprovisioningImageSpec{
					Architecture: "arm64",
				},
			},
			format: metal3api.ImageFormatISO,
			networkDataStatus: metal3api.SecretStatus{
				Name:    "test-net",
				Version: "1",
			},
			expectedHasChanged: true,
		},
		{
			name: "network data changed",
			img: &metal3api.PreprovisioningImage{
				Status: metal3api.PreprovisioningImageStatus{
					Format:       metal3api.ImageFormatISO,
					Architecture: "x86_64",
					NetworkData: metal3api.SecretStatus{
						Name:    "test-net",
						Version: "1",
					},
				},
				Spec: metal3api.PreprovisioningImageSpec{
					Architecture: "x86_64",
				},
			},
			format: metal3api.ImageFormatISO,
			networkDataStatus: metal3api.SecretStatus{
				Name:    "test-net",
				Version: "2",
			},
			expectedHasChanged: true,
		},
		{
			name: "empty status",
			img: &metal3api.PreprovisioningImage{
				Status: metal3api.PreprovisioningImageStatus{},
				Spec: metal3api.PreprovisioningImageSpec{
					Architecture: "x86_64",
				},
			},
			format: metal3api.ImageFormatISO,
			networkDataStatus: metal3api.SecretStatus{
				Name:    "test-net",
				Version: "1",
			},
			expectedHasChanged: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := configChanged(tc.img, tc.format, tc.networkDataStatus)
			assert.Equal(t, tc.expectedHasChanged, result)
		})
	}
}
