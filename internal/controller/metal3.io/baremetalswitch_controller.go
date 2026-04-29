/*
Copyright 2025 The Metal3 Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	"github.com/metal3-io/baremetal-operator/pkg/secretutils"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	meta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
)

const credentialErrorRequeueDelay = 10 * time.Second

// BareMetalSwitchReconciler reconciles a BareMetalSwitch object.
type BareMetalSwitchReconciler struct {
	client.Client
	Log       logr.Logger
	APIReader client.Reader

	// SwitchConfigsSecretName is the name of the secret that holds the
	// generated INI-format switch configuration (from IRONIC_SWITCH_CONFIGS_SECRET).
	SwitchConfigsSecretName string

	// SwitchCredentialSecretName is the name of the secret that holds SSH
	// private key files for publickey-authenticated switches
	// (from IRONIC_SWITCH_CREDENTIALS_SECRET).
	SwitchCredentialSecretName string

	// SwitchCredentialPath is the mount path where the credentials secret
	// is mounted in the ironic-networking pod (from IRONIC_SWITCH_CREDENTIALS_PATH).
	// Used to construct key_file= paths in the switch config INI.
	SwitchCredentialPath string
}

//+kubebuilder:rbac:groups=metal3.io,resources=baremetalswitches,verbs=get;list;watch
//+kubebuilder:rbac:groups=metal3.io,resources=baremetalswitches/status,verbs=get;update;patch
//+kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *BareMetalSwitchReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := r.Log.WithValues("baremetalswitch", req.NamespacedName)
	logger.Info("starting reconcile")

	sm := secretutils.NewSecretManager(logger, r.Client, r.APIReader)

	// Get the BareMetalSwitch resource
	bmSwitch := &metal3api.BareMetalSwitch{}
	if err := r.Get(ctx, req.NamespacedName, bmSwitch); err != nil {
		if k8serrors.IsNotFound(err) {
			// Resource deleted - regenerate config without this switch
			logger.Info("BareMetalSwitch deleted, updating switch config")
			if _, updateErr := updateSwitchConfigSecret(ctx, r.Client, sm, req.Namespace, r.SwitchConfigsSecretName, r.SwitchCredentialSecretName, r.SwitchCredentialPath, logger); updateErr != nil {
				return ctrl.Result{}, fmt.Errorf("failed to update switch config after deletion: %w", updateErr)
			}
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("could not load BareMetalSwitch: %w", err)
	}

	// Regenerate switch config from all BareMetalSwitch resources in the namespace
	result, err := updateSwitchConfigSecret(ctx, r.Client, sm, bmSwitch.Namespace, r.SwitchConfigsSecretName, r.SwitchCredentialSecretName, r.SwitchCredentialPath, logger)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update switch config: %w", err)
	}

	// Set the Reconciled condition based on whether this switch had a credential error
	if credErr, hasErr := result.credentialErrors[bmSwitch.Name]; hasErr {
		if meta.SetStatusCondition(&bmSwitch.Status.Conditions, metav1.Condition{
			Type:               string(metal3api.SwitchConditionReconciled),
			Status:             metav1.ConditionFalse,
			ObservedGeneration: bmSwitch.Generation,
			Reason:             "CredentialError",
			Message:            credErr.Error(),
		}) {
			if statusErr := r.Status().Update(ctx, bmSwitch); statusErr != nil {
				return ctrl.Result{}, fmt.Errorf("failed to update BareMetalSwitch status: %w", statusErr)
			}
		}
		if errors.As(credErr, new(*credentialSecretNotFoundError)) {
			// The secret doesn't exist yet so the Owns() watch cannot detect
			// its creation (no owner reference). Requeue periodically.
			logger.Info("BareMetalSwitch credential secret missing, requeueing", "error", credErr)
			return ctrl.Result{RequeueAfter: credentialErrorRequeueDelay}, nil
		}
		// The secret exists but is misconfigured. The Owns() watch will
		// trigger a reconcile when the user fixes it, so no requeue needed.
		logger.Info("BareMetalSwitch has credential error", "error", credErr)
		return ctrl.Result{}, nil
	}

	if meta.SetStatusCondition(&bmSwitch.Status.Conditions, metav1.Condition{
		Type:               string(metal3api.SwitchConditionReconciled),
		Status:             metav1.ConditionTrue,
		ObservedGeneration: bmSwitch.Generation,
		Reason:             "Reconciled",
		Message:            "Switch configuration has been successfully reconciled into the config secret",
	}) {
		if err := r.Status().Update(ctx, bmSwitch); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to update BareMetalSwitch status: %w", err)
		}
	}

	logger.Info("BareMetalSwitch reconciled")
	return ctrl.Result{}, nil
}

// SetupWithManager registers the reconciler to be run by the manager.
func (r *BareMetalSwitchReconciler) SetupWithManager(mgr ctrl.Manager, maxConcurrentReconcile int) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&metal3api.BareMetalSwitch{}).
		Owns(&corev1.Secret{}, builder.MatchEveryOwner).
		WithOptions(controller.Options{MaxConcurrentReconciles: maxConcurrentReconcile}).
		Complete(r)
}
