package controllers

import (
	"context"
	"fmt"

	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	"github.com/metal3-io/baremetal-operator/pkg/secretutils"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
)

const (
	// Events.
	EventAuthFormatUnsupported = "ImageAuthFormatUnsupported"
	EventAuthParseError        = "ImageAuthParseError"
)

// ImageAuthValidator validates image authentication secrets.
type ImageAuthValidator struct {
	recorder record.EventRecorder
}

// NewImageAuthValidator creates a new ImageAuthValidator.
func NewImageAuthValidator(recorder record.EventRecorder) *ImageAuthValidator {
	return &ImageAuthValidator{recorder: recorder}
}

// Validate validates the image authentication secret for the given BMH and
// returns the base64-encoded credentials in the format expected by Ironic.
func (v *ImageAuthValidator) Validate(ctx context.Context, bmh *metal3api.BareMetalHost, secretMgr secretutils.SecretManager) (string, error) {
	img := bmh.Spec.Image
	if img == nil || !img.IsOCI() || img.OCIAuthSecretName == nil || *img.OCIAuthSecretName == "" {
		return "", nil
	}
	secretName := *img.OCIAuthSecretName

	key := types.NamespacedName{Namespace: bmh.Namespace, Name: secretName}
	sec, err := secretMgr.ObtainSecret(ctx, key)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return "", fmt.Errorf("auth secret %q not found in namespace %q", secretName, bmh.Namespace)
		}
		return "", err
	}

	if sec.Type != corev1.SecretTypeDockerConfigJson && sec.Type != corev1.SecretTypeDockercfg {
		if v.recorder != nil {
			v.recorder.Eventf(bmh, corev1.EventTypeWarning, EventAuthFormatUnsupported,
				"Secret %q has unsupported type %q", secretName, sec.Type)
		}
		return "", fmt.Errorf("secret %q has unsupported type %q (expected %s or %s)",
			secretName, sec.Type, corev1.SecretTypeDockerConfigJson, corev1.SecretTypeDockercfg)
	}

	credentials, err := secretutils.ExtractRegistryCredentials(sec, img.URL)
	if err != nil {
		if v.recorder != nil {
			v.recorder.Eventf(bmh, corev1.EventTypeWarning, EventAuthParseError,
				"Failed to extract credentials from secret %q: %v", secretName, err)
		}
		return "", fmt.Errorf("failed to extract credentials from secret %q: %w", secretName, err)
	}

	return credentials, nil
}
