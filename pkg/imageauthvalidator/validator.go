package imageauthvalidator

import (
	"context"
	"fmt"
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
	// Conditions.
	ConditionImageAuthValid = "ImageAuthValid"
	ConditionImageAuthInUse = "ImageAuthInUse"

	// Reasons.
	ReasonUnknown              = "Unknown"
	ReasonNotRequired          = "NotRequired"
	ReasonValid                = "Valid"
	ReasonSecretNotFound       = "SecretNotFound"
	ReasonWrongType            = "WrongType"
	ReasonParseError           = "ParseError"
	ReasonRegistryEntryMissing = "RegistryEntryMissing"
	ReasonCredentialsInjected  = "CredentialsInjected"
	ReasonNoOCIImage           = "NoOCIImage"

	// Events.
	EventAuthSecretIrrelevant  = "ImageAuthIrrelevant"
	EventAuthFormatUnsupported = "ImageAuthFormatUnsupported"
)

type Result struct {
	Secret      *corev1.Secret
	Valid       bool
	Reason      string
	Message     string
	OCIRelevant bool
	// Credentials contains the base64-encoded credentials in the format
	// expected by Ironic (base64-encoded "username:password").
	// This is only populated if Valid is true and OCIRelevant is true.
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
	res := &Result{Valid: false, Reason: ReasonUnknown}

	img := bmh.Spec.Image
	if img == nil || img.URL == "" {
		res.Message = "image URL not set"
		return res, nil
	}

	res.OCIRelevant = isOCI(img.URL)

	// No per-host secret referenced → not required
	if img.AuthSecretName == nil || *img.AuthSecretName == "" {
		res.Reason = ReasonNotRequired
		res.Message = "no per-host auth secret referenced"
		return res, nil
	}
	secretName := *img.AuthSecretName

	// Warn if secret is set for non-OCI image (future-proof, do not fail)
	if !res.OCIRelevant && v.recorder != nil {
		v.recorder.Eventf(bmh, corev1.EventTypeWarning, EventAuthSecretIrrelevant,
			"authSecretName=%q is set but image URL is not oci:// (%s)", secretName, img.URL)
	}

	var sec corev1.Secret
	key := types.NamespacedName{Namespace: bmh.Namespace, Name: secretName}
	if err := v.c.Get(ctx, key, &sec); err != nil {
		if k8serrors.IsNotFound(err) {
			res.Reason = ReasonSecretNotFound
			res.Message = fmt.Sprintf("secret %q not found in namespace %q", secretName, bmh.Namespace)
			return res, nil
		}
		return res, err
	}

	if !isAllowedDockerConfigType(sec.Type) {
		res.Reason = ReasonWrongType
		res.Message = fmt.Sprintf("secret %q has unsupported type %q; expected %q or %q",
			secretName, sec.Type, corev1.SecretTypeDockerConfigJson, corev1.SecretTypeDockercfg)
		if v.recorder != nil {
			v.recorder.Eventf(bmh, corev1.EventTypeWarning, EventAuthFormatUnsupported,
				"Secret %q has unsupported type %q", secretName, sec.Type)
		}
		return res, nil
	}

	// For OCI images, extract the credentials from the Docker config
	if res.OCIRelevant {
		credentials, err := secretutils.ExtractRegistryCredentials(&sec, img.URL)
		if err != nil {
			res.Reason = ReasonParseError
			res.Message = fmt.Sprintf("failed to extract credentials from secret %q: %v", secretName, err)
			if v.recorder != nil {
				v.recorder.Eventf(bmh, corev1.EventTypeWarning, ReasonParseError,
					"Failed to extract credentials from secret %q: %v", secretName, err)
			}
			// Check if the error is about registry not found
			if strings.Contains(err.Error(), "not found in auth config") {
				res.Reason = ReasonRegistryEntryMissing
				res.Message = fmt.Sprintf("secret %q does not contain credentials for registry in %q", secretName, img.URL)
			}
			return res, nil
		}
		res.Credentials = credentials
	}

	res.Secret = &sec
	res.Valid = true
	res.Reason = ReasonValid
	res.Message = "auth secret present and of a supported type"
	return res, nil
}

func isOCI(url string) bool {
	return strings.HasPrefix(strings.ToLower(url), "oci://")
}

func isAllowedDockerConfigType(t corev1.SecretType) bool {
	return t == corev1.SecretTypeDockerConfigJson || t == corev1.SecretTypeDockercfg
}
