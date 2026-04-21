package controllers

import (
	"testing"

	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func newHostWithAnnotations(annotations map[string]string) *metal3api.BareMetalHost {
	return &metal3api.BareMetalHost{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: annotations,
		},
	}
}

func TestDetachedAnnotation(t *testing.T) {
	tests := []struct {
		name        string
		annotations map[string]string
		hasIt       bool
		expectArgs  bool
		expectForce bool
		expectErr   bool
	}{
		{
			name:        "no annotations",
			annotations: nil,
			hasIt:       false,
		},
		{
			name:        "other annotation",
			annotations: map[string]string{"foo": "bar"},
			hasIt:       false,
		},
		{
			name:        "valid with delay and force",
			annotations: map[string]string{metal3api.DetachedAnnotation: `{"deleteAction":"delay","force":true}`},
			hasIt:       true,
			expectArgs:  true,
			expectForce: true,
		},
		{
			name:        "valid with defaults",
			annotations: map[string]string{metal3api.DetachedAnnotation: `{}`},
			hasIt:       true,
			expectArgs:  true,
		},
		{
			name:        "invalid JSON",
			annotations: map[string]string{metal3api.DetachedAnnotation: "not-json"},
			hasIt:       true,
			expectErr:   true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			host := newHostWithAnnotations(tc.annotations)

			assert.Equal(t, tc.hasIt, hasDetachedAnnotation(host))

			args, err := getDetachedAnnotation(host)
			if tc.expectErr {
				require.Error(t, err)
				assert.Nil(t, args)
			} else {
				require.NoError(t, err)
				if tc.expectArgs {
					assert.NotNil(t, args)
					assert.Equal(t, tc.expectForce, args.Force)
				} else {
					assert.Nil(t, args)
				}
			}
		})
	}
}

func TestDelayDeleteForDetachedHost(t *testing.T) {
	tests := []struct {
		name        string
		annotations map[string]string
		status      metal3api.OperationalStatus
		expectDelay bool
	}{
		{
			name:        "no annotation, not detached",
			status:      metal3api.OperationalStatusOK,
			expectDelay: false,
		},
		{
			name:        "no annotation, detached status",
			status:      metal3api.OperationalStatusDetached,
			expectDelay: true,
		},
		{
			name:        "delay action",
			annotations: map[string]string{metal3api.DetachedAnnotation: `{"deleteAction":"delay"}`},
			expectDelay: true,
		},
		{
			name:        "delete action",
			annotations: map[string]string{metal3api.DetachedAnnotation: `{"deleteAction":"delete"}`},
			expectDelay: false,
		},
		{
			name:        "empty JSON defaults to no delay",
			annotations: map[string]string{metal3api.DetachedAnnotation: `{}`},
			expectDelay: false,
		},
		{
			name:        "invalid JSON defaults to no delay",
			annotations: map[string]string{metal3api.DetachedAnnotation: "bad"},
			expectDelay: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			host := newHostWithAnnotations(tc.annotations)
			host.Status.OperationalStatus = tc.status
			assert.Equal(t, tc.expectDelay, delayDeleteForDetachedHost(host))
		})
	}
}
