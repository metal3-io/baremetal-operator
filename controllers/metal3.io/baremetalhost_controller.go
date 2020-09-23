/*


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
	"os"
	"strconv"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"

	metal3iov1alpha1 "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
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

// Reconcile is called when a host changes.
func (r *BareMetalHostReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	_ = context.Background()
	_ = r.Log.WithValues("baremetalhost", req.NamespacedName)

	// your logic here

	return ctrl.Result{}, nil
}

// SetupWithManager configures the reconciler to run as part of a
// controller manager.
func (r *BareMetalHostReconciler) SetupWithManager(mgr ctrl.Manager) error {

	maxConcurrentReconciles := 3
	if mcrEnv, ok := os.LookupEnv("BMO_CONCURRENCY"); ok {
		mcr, err := strconv.Atoi(mcrEnv)
		if err != nil {
			return errors.Wrap(err, fmt.Sprintf("BMO_CONCURRENCY value: %s is invalid", mcrEnv))
		}
		if mcr > 0 {
			ctrl.Log.Info(fmt.Sprintf("BMO_CONCURRENCY of %d is set via an environment variable", mcr))
			maxConcurrentReconciles = mcr
		} else {
			ctrl.Log.Info(fmt.Sprintf("Invalid BMO_CONCURRENCY value. Operator Concurrency will be set to a default value of %d", maxConcurrentReconciles))
		}
	} else {
		ctrl.Log.Info(fmt.Sprintf("Operator Concurrency will be set to a default value of %d", maxConcurrentReconciles))
	}

	opts := controller.Options{
		MaxConcurrentReconciles: maxConcurrentReconciles,
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&metal3iov1alpha1.BareMetalHost{}).
		WithOptions(opts).
		Owns(&corev1.Secret{}).
		Complete(r)
}
