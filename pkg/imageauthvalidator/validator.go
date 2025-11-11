package imageauthvalidator

import (
	"context"
	"strings"

	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	"github.com/metal3-io/baremetal-operator/pkg/secretutils"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// Events.
	EventAuthSecretIrrelevant  = "ImageAuthIrrelevant"
	EventAuthFormatUnsupported = "ImageAuthFormatUnsupported"
)

type Result struct {
	Secret *corev1.Secret
	Valid  bool
	// Credentials contains the base64-encoded credentials in the format
	// expected by Ironic (base64-encoded "username:password").
	// This is only populated if Valid is true for OCI images.
	Credentials string
}

type Validator interface {
	Validate(ctx context.Context, bmh *metal3api.BareMetalHost) (*Result, error)
}

type validator struct {
	c        client.Client
	recorder record.EventRecorder
}

func New(c client.Client, recorder record.EventRecorder) Validator {
	return &validator{c: c, recorder: recorder}
}

func (v *validator) Validate(ctx context.Context, bmh *metal3api.BareMetalHost) (*Result, error) {
	res := &Result{Valid: false}

	img := bmh.Spec.Image
	if img == nil || img.URL == "" {
		return res, nil
	}

	ociRelevant := isOCI(img.URL)

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

	var sec corev1.Secret
	key := types.NamespacedName{Namespace: bmh.Namespace, Name: secretName}
	if err := v.c.Get(ctx, key, &sec); err != nil {
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
		credentials, err := secretutils.ExtractRegistryCredentials(&sec, img.URL)
		if err != nil {
			if v.recorder != nil {
				v.recorder.Eventf(bmh, corev1.EventTypeWarning, "ImageAuthParseError",
					"Failed to extract credentials from secret %q: %v", secretName, err)
			}
			return res, nil
		}
		res.Credentials = credentials
	}

	res.Secret = &sec
	res.Valid = true
	return res, nil
}

func isOCI(url string) bool {
	return strings.HasPrefix(strings.ToLower(url), "oci://")
}

func isAllowedDockerConfigType(t corev1.SecretType) bool {
	return t == corev1.SecretTypeDockerConfigJson || t == corev1.SecretTypeDockercfg
}
