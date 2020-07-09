/*
Copyright 2020 Metal3 Authors.

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
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	metal3iov1alpha1 "github.com/metal3-io/baremetal-operator/api/v1alpha1"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner"
)

// BareMetalHostReconciler reconciles a BareMetalHost object
type BareMetalHostReconciler struct {
	client.Client
	Log                logr.Logger
	Scheme             *runtime.Scheme
	ProvisionerFactory provisioner.Factory
}

// +kubebuilder:rbac:groups=metal3.io,resources=baremetalhosts,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=metal3.io,resources=baremetalhosts/status,verbs=get;update;patch

// Reconcile manages updates to BareMetalHost resources
func (r *BareMetalHostReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	_ = context.Background()
	_ = r.Log.WithValues("baremetalhost", req.NamespacedName)

	// your logic here

	return ctrl.Result{}, nil
}

// SetupWithManager configures the reconciler to run as part of a
// controller manager.
func (r *BareMetalHostReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&metal3iov1alpha1.BareMetalHost{}).
		Complete(r)
}
