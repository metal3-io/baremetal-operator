package secretutils

import (
	"context"
	"fmt"
	"slices"

	"github.com/go-logr/logr"
	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	SecretsFinalizer = metal3api.BareMetalHostFinalizer + "/secret"
)

// SecretManager is a type for fetching Secrets whether or not they are in the
// client cache, labelling so that they will be included in the client cache,
// and optionally setting an owner reference.
type SecretManager struct {
	log       logr.Logger
	client    client.Client
	apiReader client.Reader
}

// NewSecretManager returns a new SecretManager.
func NewSecretManager(log logr.Logger, cacheClient client.Client, apiReader client.Reader) SecretManager {
	return SecretManager{
		log:       log.WithName("secret_manager"),
		client:    cacheClient,
		apiReader: apiReader,
	}
}

// findSecret retrieves a Secret from the cache if it is available, and from the
// k8s API if not.
func (sm *SecretManager) findSecret(ctx context.Context, key types.NamespacedName) (secret *corev1.Secret, err error) {
	secret = &corev1.Secret{}

	// Look for secret in the filtered cache
	err = sm.client.Get(ctx, key, secret)
	if err == nil {
		return secret, nil
	}
	if !k8serrors.IsNotFound(err) {
		return nil, err
	}

	// Secret not in cache; check API directly for unlabelled Secret
	err = sm.apiReader.Get(ctx, key, secret)
	if err != nil {
		return nil, err
	}

	return secret, nil
}

// claimSecret ensures that the Secret has a label that will ensure it is
// present in the cache (and that we can watch for changes), and optionally
// that it has a particular owner reference.
func (sm *SecretManager) claimSecret(ctx context.Context, secret *corev1.Secret, owner client.Object, addFinalizer bool) error {
	log := sm.log.WithValues("secret", secret.Name, "secretNamespace", secret.Namespace)
	needsUpdate := false
	if !metav1.HasLabel(secret.ObjectMeta, LabelEnvironmentName) {
		log.Info("setting secret environment label")
		metav1.SetMetaDataLabel(&secret.ObjectMeta, LabelEnvironmentName, LabelEnvironmentValue)
		needsUpdate = true
	}
	if owner != nil {
		ownerLog := log.WithValues(
			"ownerKind", owner.GetObjectKind().GroupVersionKind().Kind,
			"owner", owner.GetNamespace()+"/"+owner.GetName(),
			"ownerUID", owner.GetUID())
		alreadyOwned := false
		ownerUID := owner.GetUID()
		for _, ref := range secret.GetOwnerReferences() {
			// We used to add controller references to BMC
			// secrets. This was wrong, update.
			if ref.UID == ownerUID && ref.Controller == nil {
				alreadyOwned = true
				break
			} else if ref.Controller != nil && *ref.Controller {
				ownerLog.Info("updating secret to no longer have an owner of type controller")
			}
		}
		if !alreadyOwned {
			ownerLog.Info("setting secret owner reference")
			if err := controllerutil.SetOwnerReference(owner, secret, sm.client.Scheme()); err != nil {
				return fmt.Errorf("failed to set secret owner reference: %w", err)
			}
			needsUpdate = true
		}
	}

	if addFinalizer && !slices.Contains(secret.Finalizers, SecretsFinalizer) {
		log.Info("setting secret finalizer")
		secret.Finalizers = append(secret.Finalizers, SecretsFinalizer)
		needsUpdate = true
	}

	if needsUpdate {
		if err := sm.client.Update(ctx, secret); err != nil {
			return fmt.Errorf("failed to update secret %s in namespace %s: %w", secret.ObjectMeta.Name, secret.ObjectMeta.Namespace, err)
		}
	}

	return nil
}

// obtainSecretForOwner retrieves a Secret and ensures that it has a label that
// will ensure it is present in the cache (and that we can watch for changes),
// and optionally that it has a particular owner reference. The owner reference
// may optionally be a controller reference.
func (sm *SecretManager) obtainSecretForOwner(ctx context.Context, key types.NamespacedName, owner client.Object, addFinalizer bool) (*corev1.Secret, error) {
	secret, err := sm.findSecret(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch secret %s in namespace %s: %w", key.Name, key.Namespace, err)
	}
	err = sm.claimSecret(ctx, secret, owner, addFinalizer)

	return secret, err
}

// AcquireSecret retrieves a Secret and ensures that it has a label that will
// ensure it is present in the cache (and that we can watch for changes), and
// that it has a particular owner reference. The owner reference may optionally
// be a controller reference.
func (sm *SecretManager) AcquireSecret(ctx context.Context, key types.NamespacedName, owner client.Object, addFinalizer bool) (*corev1.Secret, error) {
	if owner == nil {
		panic("AcquireSecret called with no owner")
	}

	return sm.obtainSecretForOwner(ctx, key, owner, addFinalizer)
}

// ObtainSecret retrieves a Secret and ensures that it has a label that will
// ensure it is present in the cache (and that we can watch for changes).
func (sm *SecretManager) ObtainSecret(ctx context.Context, key types.NamespacedName) (*corev1.Secret, error) {
	return sm.obtainSecretForOwner(ctx, key, nil, false)
}

// ReleaseSecret removes secrets manager finalizer from specified secret when needed.
func (sm *SecretManager) ReleaseSecret(ctx context.Context, secret *corev1.Secret) error {
	if !slices.Contains(secret.Finalizers, SecretsFinalizer) {
		return nil
	}

	// Remove finalizer from secret to allow deletion
	controllerutil.RemoveFinalizer(secret, SecretsFinalizer)

	if err := sm.client.Update(ctx, secret); err != nil {
		return fmt.Errorf("failed to remove finalizer from secret %s in namespace %s: %w",
			secret.ObjectMeta.Name, secret.ObjectMeta.Namespace, err)
	}

	sm.log.Info("removed secret finalizer",
		"remaining", secret.Finalizers)

	return nil
}
