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
	"errors"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner"
	"github.com/metal3-io/baremetal-operator/pkg/utils"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

const (
	dataImageRetryDelay          = time.Second * 60
	dataImageUpdateDelay         = time.Second * 30
	dataImageUnmanagedRetryDelay = time.Second * 20
)

// DataImageReconciler reconciles a DataImage object.
type DataImageReconciler struct {
	client.Client
	Log                logr.Logger
	ProvisionerFactory provisioner.Factory
}

type rdiInfo struct {
	ctx     context.Context
	log     logr.Logger
	request ctrl.Request
	di      *metal3api.DataImage
	bmh     *metal3api.BareMetalHost
	events  []corev1.Event
}

// match the provisioner.EventPublisher interface.
func (info *rdiInfo) publishEvent(reason, message string) {
	t := metav1.Now()
	dataImageEvent := corev1.Event{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: reason + "-",
			Namespace:    info.di.ObjectMeta.Namespace,
		},
		InvolvedObject: corev1.ObjectReference{
			Kind:       "DataImage",
			Namespace:  info.di.Namespace,
			Name:       info.di.Name,
			UID:        info.di.UID,
			APIVersion: metal3api.GroupVersion.String(),
		},
		Reason:  reason,
		Message: message,
		Source: corev1.EventSource{
			Component: "metal3-dataimage-controller",
		},
		FirstTimestamp:      t,
		LastTimestamp:       t,
		Count:               1,
		Type:                corev1.EventTypeNormal,
		ReportingController: "metal3.io/dataimage-controller",
	}

	info.events = append(info.events, dataImageEvent)
}

//+kubebuilder:rbac:groups=metal3.io,resources=dataimages,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=metal3.io,resources=dataimages/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=metal3.io,resources=dataimages/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *DataImageReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	reqLogger := r.Log.WithValues("dataimage", req.NamespacedName)
	reqLogger.Info("start dataImage reconciliation")

	di := &metal3api.DataImage{}
	if err := r.Get(ctx, req.NamespacedName, di); err != nil {
		// The DataImage resource may have been deleted
		if k8serrors.IsNotFound(err) {
			reqLogger.Info("dataImage not found")
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return ctrl.Result{Requeue: true, RequeueAfter: dataImageRetryDelay}, fmt.Errorf("could not load dataImage, %w", err)
	}

	// If a corresponding BareMetalHost is missing, keep retrying
	bmh := &metal3api.BareMetalHost{}
	if err := r.Get(ctx, req.NamespacedName, bmh); err != nil {
		// There might not be any BareMetalHost for the DataImage
		if k8serrors.IsNotFound(err) {
			reqLogger.Info("bareMetalHost not found for the dataImage, remove finalizer if it exists")
			di.Finalizers = utils.FilterStringFromList(
				di.Finalizers, metal3api.DataImageFinalizer)

			if err := r.Update(ctx, di); err != nil {
				return ctrl.Result{Requeue: true, RequeueAfter: dataImageRetryDelay}, fmt.Errorf("failed to update resource after remove finalizer, %w", err)
			}
			return ctrl.Result{}, nil
		}

		// Error reading the object - requeue the request.
		return ctrl.Result{Requeue: true, RequeueAfter: dataImageRetryDelay}, fmt.Errorf("could not load baremetalhost, %w", err)
	}

	info := &rdiInfo{ctx: ctx, log: reqLogger, request: req, di: di, bmh: bmh}

	// If the reconciliation is paused, requeue
	annotations := bmh.GetAnnotations()
	if _, ok := annotations[metal3api.PausedAnnotation]; ok {
		reqLogger.Info("host associated with dataImage is paused, no work to do")
		return ctrl.Result{Requeue: false}, nil
	}

	// If DataImage exists, add its ownerReference
	if !ownerReferenceExists(bmh, di) {
		if err := controllerutil.SetOwnerReference(bmh, di, r.Scheme()); err != nil {
			return ctrl.Result{Requeue: true, RequeueAfter: dataImageRetryDelay}, fmt.Errorf("could not set bmh as controller, %w", err)
		}
		if err := r.Update(ctx, di); err != nil {
			return ctrl.Result{Requeue: true, RequeueAfter: dataImageRetryDelay}, fmt.Errorf("failure updating dataImage status, %w", err)
		}

		return ctrl.Result{Requeue: true}, nil
	}

	// Add finalizer for newly created DataImage
	if di.DeletionTimestamp.IsZero() && !utils.StringInList(di.Finalizers, metal3api.DataImageFinalizer) {
		reqLogger.Info("adding finalizer")
		di.Finalizers = append(di.Finalizers, metal3api.DataImageFinalizer)

		// Update dataImage after adding finalizer, requeue in case of failure
		err := r.Update(ctx, di)
		if err != nil {
			return ctrl.Result{RequeueAfter: dataImageUpdateDelay}, fmt.Errorf("failed to update resource after add finalizer, %w", err)
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// If the associated BMH is detached, keep requeuing till the annotation is removed
	if hasDetachedAnnotation(bmh) {
		reqLogger.Info("the host is detached, not running reconciler")
		return ctrl.Result{Requeue: true, RequeueAfter: dataImageUnmanagedRetryDelay}, nil
	}

	// Create a provisioner that can access Ironic API
	prov, err := r.ProvisionerFactory.NewProvisioner(ctx, provisioner.BuildHostDataNoBMC(*bmh), info.publishEvent)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to create provisioner, %w", err)
	}

	ready, err := prov.TryInit()
	if err != nil || !ready {
		var msg string
		if err == nil {
			msg = "Not ready"
		} else {
			msg = err.Error()
		}
		reqLogger.Info("provisioner is not ready", "Error", msg, "RequeueAfter", provisionerRetryDelay)
		return ctrl.Result{Requeue: true, RequeueAfter: provisionerRetryDelay}, nil
	}

	// Check if any attach/detach action is pending or failed to attach
	isImageAttached, vmediaGetError := prov.GetDataImageStatus()

	// In case there was an error fetching vmedia details
	// upadate message and counter
	if vmediaGetError != nil {
		reqLogger.Error(vmediaGetError, "failed to fetch Virtual Media details")

		if !errors.Is(vmediaGetError, provisioner.ErrNodeIsBusy) {
			di.Status.Error.Message = vmediaGetError.Error()
			di.Status.Error.Count++

			// Update dataImage status and requeue
			if err := r.updateStatus(info); err != nil {
				return ctrl.Result{Requeue: true, RequeueAfter: dataImageRetryDelay}, fmt.Errorf("failed to update resource status, %w", err)
			}
		}

		return ctrl.Result{Requeue: true, RequeueAfter: dataImageUpdateDelay}, nil
	}
	di.Status.Error.Message = ""
	di.Status.Error.Count = 0

	// Remove finalizer if DataImage has been requested for deletion and
	// there is no attached image, else wait for the detachment.
	if !di.DeletionTimestamp.IsZero() {
		reqLogger.Info("cleaning up deleted dataImage resource")

		if isImageAttached {
			reqLogger.Info("Wait for DataImage to detach before removing finalizer, requeueing")
			return ctrl.Result{Requeue: true, RequeueAfter: dataImageRetryDelay}, nil
		}

		di.Finalizers = utils.FilterStringFromList(
			di.Finalizers, metal3api.DataImageFinalizer)

		if err := r.Update(ctx, di); err != nil {
			return ctrl.Result{Requeue: true, RequeueAfter: dataImageRetryDelay}, fmt.Errorf("failed to update resource after remove finalizer, %w", err)
		}
		return ctrl.Result{}, nil
	}

	// Update the latest status fetched from the Node
	if err := r.updateStatus(info); err != nil {
		return ctrl.Result{Requeue: true, RequeueAfter: dataImageRetryDelay}, fmt.Errorf("failed to update resource statu, %w", err)
	}

	for _, e := range info.events {
		r.publishEvent(ctx, req, e)
	}

	return ctrl.Result{}, nil
}

// Update the DataImage status after fetching current status from provisioner.
func (r *DataImageReconciler) updateStatus(info *rdiInfo) (err error) {
	dataImage := info.di

	if err := r.Status().Update(info.ctx, dataImage); err != nil {
		return fmt.Errorf("failed to update DataImage status, %w", err)
	}
	info.log.Info("Updating DataImage Status", "Updated DataImage is", dataImage)

	return nil
}

// Publish reconciler events.
func (r *DataImageReconciler) publishEvent(ctx context.Context, request ctrl.Request, event corev1.Event) {
	reqLogger := r.Log.WithValues("dataimage", request.NamespacedName)
	reqLogger.Info("publishing event", "reason", event.Reason, "message", event.Message)
	err := r.Create(ctx, &event)
	if err != nil {
		reqLogger.Info("failed to record event, ignoring",
			"reason", event.Reason, "message", event.Message, "error", err)
	}
}

// Update events.
func (r *DataImageReconciler) updateEventHandler(e event.UpdateEvent) bool {
	r.Log.Info("dataimage in event handler")

	// If the update increased the resource Generation then let's process it
	if e.ObjectNew.GetGeneration() != e.ObjectOld.GetGeneration() {
		r.Log.Info("returning true as generation changed from event handler")
		return true
	}

	return false
}

// SetupWithManager sets up the controller with the Manager.
func (r *DataImageReconciler) SetupWithManager(mgr ctrl.Manager, maxConcurrentReconcile int) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&metal3api.DataImage{}).
		WithOptions(controller.Options{MaxConcurrentReconciles: maxConcurrentReconcile}).
		WithEventFilter(
			predicate.Funcs{
				UpdateFunc: r.updateEventHandler,
			}).
		Complete(r)
}
