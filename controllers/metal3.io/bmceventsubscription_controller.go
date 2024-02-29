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
	"reflect"
	"time"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner"
	"github.com/metal3-io/baremetal-operator/pkg/utils"
)

const (
	subscriptionRetryDelay = time.Minute * 10
)

type BMCEventSubscriptionReconciler struct {
	client.Client
	Log                logr.Logger
	ProvisionerFactory provisioner.Factory
	APIReader          client.Reader
}

//+kubebuilder:rbac:groups=metal3.io,resources=bmceventsubscriptions,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=metal3.io,resources=bmceventsubscriptions/status,verbs=get;update;patch

func (r *BMCEventSubscriptionReconciler) Reconcile(ctx context.Context, request ctrl.Request) (result ctrl.Result, err error) {
	reqLogger := r.Log.WithValues("bmceventsubscription", request.NamespacedName)
	reqLogger.Info("start")

	defer func() {
		reqLogger.Info("done")
	}()

	// Fetch the BMCEventSubscription
	subscription := &metal3api.BMCEventSubscription{}
	err = r.Get(ctx, request.NamespacedName, subscription)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			// Request object not found, could have been deleted after
			// reconcile request.  Owned objects are automatically
			// garbage collected. For additional cleanup logic use
			// finalizers.  Return and don't requeue
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return ctrl.Result{}, errors.Wrap(err, "could not load subscription")
	}

	host := &metal3api.BareMetalHost{}
	namespacedHostName := types.NamespacedName{
		Name:      subscription.Spec.HostName,
		Namespace: request.Namespace,
	}
	err = r.Get(ctx, namespacedHostName, host)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			reqLogger.Error(err, "baremetalhost not found", "host", subscription.Spec.HostName)

			message := fmt.Sprintf("baremetal host %q", subscription.Status.Error)
			return r.handleError(ctx, subscription, err, message, true)
		}
		// Error reading the object - requeue the request.
		return ctrl.Result{}, errors.Wrap(err, "could not load host data")
	}

	err = r.addFinalizer(ctx, subscription)

	if err != nil {
		return ctrl.Result{}, errors.Wrap(err, "failed add finalizer")
	}

	prov, ready, err := r.getProvisioner(ctx, request, host)

	if err != nil {
		return ctrl.Result{}, errors.Wrap(err, "failed to create provisioner")
	}

	if !ready {
		reqLogger.Info("provisioner is not ready", "RequeueAfter:", provisionerNotReadyRetryDelay)
		return ctrl.Result{RequeueAfter: provisionerNotReadyRetryDelay}, nil
	}

	if subscription.DeletionTimestamp.IsZero() {
		// Not being deleted
		if err := r.createSubscription(ctx, prov, subscription); err != nil {
			return r.handleError(ctx, subscription, err, "failed to create a subscription", false)
		}
	} else {
		// Being deleted
		if err := r.deleteSubscription(ctx, prov, subscription); err != nil {
			return r.handleError(ctx, subscription, err, "failed to delete a subscription", false)
		}

		return ctrl.Result{}, nil
	}

	return
}

func (r *BMCEventSubscriptionReconciler) handleError(ctx context.Context, subscription *metal3api.BMCEventSubscription, e error, message string, requeue bool) (ctrl.Result, error) {
	subscription.Status.Error = message
	err := r.Status().Update(ctx, subscription)
	if err != nil {
		return ctrl.Result{}, errors.Wrap(err, "failed to update subscription status")
	}

	if requeue {
		return ctrl.Result{RequeueAfter: subscriptionRetryDelay}, nil
	}

	return ctrl.Result{}, errors.Wrap(e, message)
}

func (r *BMCEventSubscriptionReconciler) addFinalizer(ctx context.Context, subscription *metal3api.BMCEventSubscription) error {
	reqLogger := r.Log.WithName("bmceventsubscription")

	// Add a finalizer to newly created objects.
	if subscription.DeletionTimestamp.IsZero() && !subscriptionHasFinalizer(subscription) {
		reqLogger.Info(
			"adding finalizer",
			"existingFinalizers", subscription.Finalizers,
			"newValue", metal3api.BMCEventSubscriptionFinalizer,
		)
		subscription.Finalizers = append(subscription.Finalizers,
			metal3api.BMCEventSubscriptionFinalizer)
		err := r.Update(ctx, subscription)
		if err != nil {
			return errors.Wrap(err, "failed to add finalizer")
		}
		return nil
	}

	return nil
}

func (r *BMCEventSubscriptionReconciler) createSubscription(ctx context.Context, prov provisioner.Provisioner, subscription *metal3api.BMCEventSubscription) error {
	reqLogger := r.Log.WithName("bmceventsubscription")

	if subscription.Status.SubscriptionID != "" {
		reqLogger.Info("subscription already exists")
		return nil
	}

	headers, err := r.getHTTPHeaders(ctx, *subscription)

	if err != nil {
		reqLogger.Error(err, "failed to get http headers")
		subscription.Status.Error = "failed to retrieve HTTP headers secret"
		updateErr := r.Status().Update(ctx, subscription)
		if updateErr != nil {
			return errors.Wrap(updateErr, "failed to update subscription status")
		}
		return err
	}

	if _, err := prov.AddBMCEventSubscriptionForNode(subscription, headers); err != nil {
		return errors.Wrap(err, "failed to create subscription")
	}

	return r.Status().Update(ctx, subscription)
}

func (r *BMCEventSubscriptionReconciler) deleteSubscription(ctx context.Context, prov provisioner.Provisioner, subscription *metal3api.BMCEventSubscription) error {
	reqLogger := r.Log.WithName("bmceventsubscription")
	reqLogger.Info("deleting subscription")

	if subscriptionHasFinalizer(subscription) {
		if _, err := prov.RemoveBMCEventSubscriptionForNode(*subscription); err != nil {
			return errors.Wrap(err, "failed to remove a subscription")
		}

		// Remove finalizer to allow deletion
		subscription.Finalizers = utils.FilterStringFromList(
			subscription.Finalizers, metal3api.BMCEventSubscriptionFinalizer)
		reqLogger.Info("cleanup is complete, removed finalizer",
			"remaining", subscription.Finalizers)
		if err := r.Update(ctx, subscription); err != nil {
			return err
		}
	}

	return nil
}

func (r *BMCEventSubscriptionReconciler) getProvisioner(ctx context.Context, request ctrl.Request, host *metal3api.BareMetalHost) (prov provisioner.Provisioner, ready bool, err error) {
	reqLogger := r.Log.WithValues("bmceventsubscription", request.NamespacedName)

	prov, err = r.ProvisionerFactory.NewProvisioner(ctx, provisioner.BuildHostDataNoBMC(*host), nil)
	if err != nil {
		return prov, ready, errors.Wrap(err, "failed to create provisioner")
	}

	ready, err = prov.TryInit()
	if err != nil {
		return prov, ready, errors.Wrap(err, "failed to check services availability")
	}

	if !ready {
		reqLogger.Info("provisioner is not ready", "RequeueAfter:", provisionerNotReadyRetryDelay)
		return prov, ready, nil
	}

	return prov, ready, nil
}

func (r *BMCEventSubscriptionReconciler) getHTTPHeaders(ctx context.Context, subscription metal3api.BMCEventSubscription) ([]map[string]string, error) {
	headers := []map[string]string{}

	if subscription.Spec.HTTPHeadersRef == nil {
		return headers, nil
	}

	secret := &corev1.Secret{}
	secretKey := types.NamespacedName{
		Name:      subscription.Spec.HTTPHeadersRef.Name,
		Namespace: subscription.Spec.HTTPHeadersRef.Namespace,
	}

	err := r.Get(ctx, secretKey, secret)

	if err != nil {
		return headers, err
	}

	for headerName, headerValueBytes := range secret.Data {
		header := map[string]string{}
		header[headerName] = string(headerValueBytes)
		headers = append(headers, header)
	}

	return headers, err
}

func (r *BMCEventSubscriptionReconciler) updateEventHandler(e event.UpdateEvent) bool {
	_, oldOK := e.ObjectOld.(*metal3api.BMCEventSubscription)
	_, newOK := e.ObjectNew.(*metal3api.BMCEventSubscription)
	if !(oldOK && newOK) {
		return true
	}

	// If the update increased the resource Generation then let's process it
	if e.ObjectNew.GetGeneration() != e.ObjectOld.GetGeneration() {
		return true
	}

	// Discard updates that did not increase the resource Generation (such as on Status.LastUpdated), except for the finalizers or annotations
	if reflect.DeepEqual(e.ObjectNew.GetFinalizers(), e.ObjectOld.GetFinalizers()) && reflect.DeepEqual(e.ObjectNew.GetAnnotations(), e.ObjectOld.GetAnnotations()) {
		return false
	}

	return true
}

// SetupWithManager registers the reconciler to be run by the manager.
func (r *BMCEventSubscriptionReconciler) SetupWithManager(mgr ctrl.Manager, maxConcurrentReconcile int) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&metal3api.BMCEventSubscription{}).
		WithOptions(controller.Options{MaxConcurrentReconciles: maxConcurrentReconcile}).
		WithEventFilter(predicate.Funcs{
			UpdateFunc: r.updateEventHandler,
		}).
		Watches(&metal3api.BareMetalHost{}, &handler.EnqueueRequestForObject{}, builder.Predicates{}).
		Complete(r)
}

func subscriptionHasFinalizer(subscription *metal3api.BMCEventSubscription) bool {
	return utils.StringInList(subscription.Finalizers, metal3api.BMCEventSubscriptionFinalizer)
}
