package controllers

import (
	"context"

	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	"github.com/metal3-io/baremetal-operator/pkg/secretutils"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
)

const (
	// Events.
	EventAuthSecretIrrelevant  = "ImageAuthIrrelevant"
	EventAuthFormatUnsupported = "ImageAuthFormatUnsupported"
	EventAuthParseError        = "ImageAuthParseError"
)

// ImageAuthResult holds the result of image auth secret validation.
type ImageAuthResult struct {
	Secret *corev1.Secret
	Valid  bool
	// Credentials contains the base64-encoded credentials in the format
	// expected by Ironic (base64-encoded "username:password").
	// This is only populated if Valid is true for OCI images.
	Credentials string
}

// ImageAuthValidator validates image authentication secrets.
type ImageAuthValidator struct {
	recorder record.EventRecorder
}

// NewImageAuthValidator creates a new ImageAuthValidator.
func NewImageAuthValidator(recorder record.EventRecorder) *ImageAuthValidator {
	return &ImageAuthValidator{recorder: recorder}
}

// Validate validates the image authentication secret for the given BMH.
func (v *ImageAuthValidator) Validate(_ context.Context, bmh *metal3api.BareMetalHost, secretMgr secretutils.SecretManager) (*ImageAuthResult, error) {
	res := &ImageAuthResult{Valid: false}

	img := bmh.Spec.Image
	if img == nil || img.URL == "" {
		return res, nil
	}

	ociRelevant := img.IsOCI()

	// No per-host secret referenced → not required, valid for public images
	if img.AuthSecretName == nil || *img.AuthSecretName == "" {
		return res, nil
	}
	secretName := *img.AuthSecretName

	// Warn if secret is set for non-OCI image (future-proof, do not fail)
	if !ociRelevant && v.recorder != nil {
		v.recorder.Eventf(bmh, corev1.EventTypeWarning, EventAuthSecretIrrelevant,
			"authSecretName=%q is set but image URL is not oci:// (%s)", secretName, img.URL)
	}

	// Use SecretManager to obtain and label the secret (following BMC credentials pattern)
	key := types.NamespacedName{Namespace: bmh.Namespace, Name: secretName}
	sec, err := secretMgr.ObtainSecret(key)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return res, nil
		}
		return res, err
	}

	if !isAllowedDockerConfigType(sec.Type) {
		if v.recorder != nil {
			v.recorder.Eventf(bmh, corev1.EventTypeWarning, EventAuthFormatUnsupported,
				"Secret %q has unsupported type %q", secretName, sec.Type)
		}
		return res, nil
	}

	// For OCI images, extract the credentials from the Docker config
	if ociRelevant {
		credentials, err := secretutils.ExtractRegistryCredentials(sec, img.URL)
		if err != nil {
			if v.recorder != nil {
				v.recorder.Eventf(bmh, corev1.EventTypeWarning, EventAuthParseError,
					"Failed to extract credentials from secret %q: %v", secretName, err)
			}
			return res, nil
		}
		res.Credentials = credentials
	}

	res.Secret = sec
	res.Valid = true
	return res, nil
}

func isAllowedDockerConfigType(t corev1.SecretType) bool {
	return t == corev1.SecretTypeDockerConfigJson || t == corev1.SecretTypeDockercfg
}
