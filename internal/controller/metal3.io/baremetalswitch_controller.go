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
	"fmt"

	"github.com/go-logr/logr"
	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// BareMetalSwitchReconciler reconciles a BareMetalSwitch object.
type BareMetalSwitchReconciler struct {
	client.Client
	Log logr.Logger

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
//+kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *BareMetalSwitchReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := r.Log.WithValues("baremetalswitch", req.NamespacedName)
	logger.Info("starting reconcile")

	// Get the BareMetalSwitch resource
	bmSwitch := &metal3api.BareMetalSwitch{}
	if err := r.Get(ctx, req.NamespacedName, bmSwitch); err != nil {
		if k8serrors.IsNotFound(err) {
			// Resource deleted - regenerate config without this switch
			logger.Info("BareMetalSwitch deleted, updating switch config")
			if err := updateSwitchConfigSecret(ctx, r.Client, req.Namespace, r.SwitchConfigsSecretName, r.SwitchCredentialSecretName, r.SwitchCredentialPath, logger); err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to update switch config after deletion: %w", err)
			}
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("could not load BareMetalSwitch: %w", err)
	}

	// Regenerate switch config from all BareMetalSwitch resources in the namespace
	if err := updateSwitchConfigSecret(ctx, r.Client, bmSwitch.Namespace, r.SwitchConfigsSecretName, r.SwitchCredentialSecretName, r.SwitchCredentialPath, logger); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update switch config: %w", err)
	}

	logger.Info("BareMetalSwitch reconciled")
	return ctrl.Result{}, nil
}

// findSwitchesForSecret returns reconcile requests for BareMetalSwitch resources
// that reference the given secret via spec.credentials.secretName. This ensures
// that credential secret changes (e.g. password rotation) trigger config regeneration.
func (r *BareMetalSwitchReconciler) findSwitchesForSecret(ctx context.Context, secret client.Object) []reconcile.Request {
	switchList := &metal3api.BareMetalSwitchList{}
	if err := r.List(ctx, switchList, client.InNamespace(secret.GetNamespace())); err != nil {
		r.Log.Error(err, "failed to list BareMetalSwitch resources for secret watch")
		return nil
	}

	var requests []reconcile.Request
	for i := range switchList.Items {
		if switchList.Items[i].Spec.Credentials.SecretName == secret.GetName() {
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      switchList.Items[i].Name,
					Namespace: switchList.Items[i].Namespace,
				},
			})
		}
	}
	return requests
}

// SetupWithManager registers the reconciler to be run by the manager.
func (r *BareMetalSwitchReconciler) SetupWithManager(mgr ctrl.Manager, maxConcurrentReconcile int) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&metal3api.BareMetalSwitch{}).
		Watches(&corev1.Secret{}, handler.EnqueueRequestsFromMapFunc(r.findSwitchesForSecret)).
		WithOptions(controller.Options{MaxConcurrentReconciles: maxConcurrentReconcile}).
		Complete(r)
}
