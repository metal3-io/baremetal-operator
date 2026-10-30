/*
Copyright 2026 The Metal3 Authors.

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

	"github.com/go-logr/logr"
	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	. "github.com/metal3-io/baremetal-operator/pkg/logging"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// HostDeployPolicyReconciler keeps HostDeployPolicy.status.observedGeneration
// in sync with metadata.generation. Writing status only happens after the
// shared informer cache has observed the current spec, which gives any other
// controller in the same manager — notably the HostClaim controller — a
// reliable signal that it will read the same spec from the cache. The
// finalizer plays the same role for deletion.
type HostDeployPolicyReconciler struct {
	client.Client
	Log logr.Logger
}

//+kubebuilder:rbac:groups=metal3.io,resources=hostdeploypolicies,verbs=get;list;watch;update;patch
//+kubebuilder:rbac:groups=metal3.io,resources=hostdeploypolicies/status,verbs=get;update;patch

func (r *HostDeployPolicyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	hdp := &metal3api.HostDeployPolicy{}
	if err := r.Get(ctx, req.NamespacedName, hdp); err != nil {
		if k8serrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}
	log := r.Log.WithValues(LogFieldNamespace, hdp.Namespace, LogFieldHostDeployPolicy, hdp.Name, LogFieldGeneration, hdp.Generation)
	if !hdp.DeletionTimestamp.IsZero() {
		// The finalizer does nothing. Its removal simply acknowledges the fact that
		// the deletion is taken into account.
		if controllerutil.RemoveFinalizer(hdp, metal3api.HostDeployPolicyFinalizer) {
			log.V(VerbosityLevelDebug).Info("removing finalizer")
			if err := r.Update(ctx, hdp); err != nil {
				log.Error(err, "failed to remove finalizer")
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	if controllerutil.AddFinalizer(hdp, metal3api.HostDeployPolicyFinalizer) {
		log.V(VerbosityLevelDebug).Info("adding finalizer")
		if err := r.Update(ctx, hdp); err != nil {
			log.Error(err, "failed to add finalizer")
			return ctrl.Result{}, err
		}
	}

	if hdp.Status.ObservedGeneration == hdp.Generation {
		return ctrl.Result{}, nil
	}
	patch := client.MergeFrom(hdp.DeepCopy())
	hdp.Status.ObservedGeneration = hdp.Generation
	r.Log.V(VerbosityLevelDebug).Info("updating observedGeneration",
		LogFieldNamespace, hdp.Namespace, LogFieldHostDeployPolicy, hdp.Name, LogFieldGeneration, hdp.Generation)
	if err := r.Status().Patch(ctx, hdp, patch); err != nil {
		r.Log.Error(err, "failed to update observedGeneration",
			LogFieldNamespace, hdp.Namespace, LogFieldHostDeployPolicy, hdp.Name)
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *HostDeployPolicyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&metal3api.HostDeployPolicy{}).
		WithEventFilter(predicate.GenerationChangedPredicate{}).
		Complete(r)
}
