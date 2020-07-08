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
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	metal3v1alpha1 "github.com/metal3-io/baremetal-operator/api/v1alpha1"
	"github.com/metal3-io/baremetal-operator/provisioner"
)

var log = logf.Log.WithName("baremetalhost")
var maxConcurrentReconciles int = 3

func init() {
	if mcrEnv, ok := os.LookupEnv("BMO_CONCURRENCY"); ok {
		mcr, err := strconv.Atoi(mcrEnv)
		if err != nil {
			log.Error(err, fmt.Sprintf("BMO_CONCURRENCY value: %s is invalid", mcrEnv))
			os.Exit(1)
		}
		if mcr > 0 {
			log.Info(fmt.Sprintf("BMO_CONCURRENCY of %d is set via an environment variable", mcr))
			maxConcurrentReconciles = mcr
		} else {
			log.Info(fmt.Sprintf("Invalid BMO_CONCURRENCY value. Controller Concurrency will be set to a default value of %d", maxConcurrentReconciles))
		}
	} else {
		log.Info(fmt.Sprintf("Controller Concurrency will be set to a default value of %d", maxConcurrentReconciles))
	}
}

// BareMetalHostReconciler reconciles a BareMetalHost object
type BareMetalHostReconciler struct {
	client.Client
	Log                logr.Logger
	Scheme             *runtime.Scheme
	ProvisionerFactory provisioner.Factory
}

// +kubebuilder:rbac:groups=metal3.io,resources=baremetalhosts,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=metal3.io,resources=baremetalhosts/status,verbs=get;update;patch

func (r *BareMetalHostReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	_ = context.Background()
	_ = r.Log.WithValues("baremetalhost", req.NamespacedName)

	// your logic here

	return ctrl.Result{}, nil
}

func (r *BareMetalHostReconciler) SetupWithManager(mgr ctrl.Manager) error {
	opts := controller.Options{
		MaxConcurrentReconciles: maxConcurrentReconciles,
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&metal3v1alpha1.BareMetalHost{}).
		WithOptions(opts).
		Complete(r)
}
