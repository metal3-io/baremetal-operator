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

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/go-logr/logr"
	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner"
)

// HostFirmwareComponentsReconciler reconciles a HostFirmwareComponents object.
type HostFirmwareComponentsReconciler struct {
	client.Client
	Log                logr.Logger
	ProvisionerFactory provisioner.Factory
}

// rhfcInfo is used to simplify the pass or arguments.
type rhfcInfo struct {
	ctx    context.Context
	log    logr.Logger
	hfc    *metal3api.HostFirmwareComponents
	bmh    *metal3api.BareMetalHost
	events []corev1.Event
}

type conditionReasonHFC string

const (
	reasonInvalidComponent conditionReasonHFC = "InvalidComponent"
	reasonValidComponent   conditionReasonHFC = "OK"
)

func (info *rhfcInfo) publishEvent(reason, message string) {
	t := metav1.Now()
	hfcEvent := corev1.Event{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: reason + "-",
			Namespace:    info.hfc.ObjectMeta.Namespace,
		},
		InvolvedObject: corev1.ObjectReference{
			Kind:       info.hfc.Kind,
			Namespace:  info.hfc.Namespace,
			Name:       info.hfc.Name,
			UID:        info.hfc.UID,
			APIVersion: metal3api.GroupVersion.String(),
		},
		Reason:  reason,
		Message: message,
		Source: corev1.EventSource{
			Component: "metal3-hostfirmwarecomponents-controller",
		},
		FirstTimestamp:      t,
		LastTimestamp:       t,
		Count:               1,
		Type:                corev1.EventTypeNormal,
		ReportingController: "metal3.io/hostfirmwarecomponents-controller",
	}

	info.events = append(info.events, hfcEvent)
}

//+kubebuilder:rbac:groups=metal3.io,resources=hostfirmwarecomponents,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=metal3.io,resources=hostfirmwarecomponents/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=metal3.io,resources=hostfirmwarecomponents/finalizers,verbs=update

// Reconcile handles changes to HostFirmwareComponents resources.
func (r *HostFirmwareComponentsReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, err error) {
	reqLogger := r.Log.WithValues("hostfirmwarecomponents", req.NamespacedName)
	reqLogger.Info("start")

	// Get the corresponding baremetalhost in this namespace, if one doesn't exist don't continue processing
	bmh := &metal3api.BareMetalHost{}
	err = r.Get(ctx, req.NamespacedName, bmh)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		reqLogger.Error(err, "could not get baremetal host, not running hostfirmwarecomponents reconciler")
		return ctrl.Result{Requeue: true, RequeueAfter: resourceNotAvailableRetryDelay}, err
	}

	if hasDetachedAnnotation(bmh) {
		reqLogger.Info("the host is detached, not running hostfirmwarecomponents reconciler")
		return ctrl.Result{Requeue: true, RequeueAfter: unmanagedRetryDelay}, nil
	}
	// If the reconciliation is paused, requeue
	annotations := bmh.GetAnnotations()

	if _, ok := annotations[metal3api.PausedAnnotation]; ok {
		reqLogger.Info("host is paused, no work to do")
		return ctrl.Result{Requeue: true, RequeueAfter: subResourceNotReadyRetryDelay}, nil
	}

	// Fetch the HostFirmwareComponents
	hfc := &metal3api.HostFirmwareComponents{}
	info := &rhfcInfo{ctx: ctx, log: reqLogger, hfc: hfc, bmh: bmh}
	if err = r.Get(ctx, req.NamespacedName, hfc); err != nil {
		// The HFC resource may have been deleted
		if k8serrors.IsNotFound(err) {
			reqLogger.Info("HostFirmwareComponents not found")
			return ctrl.Result{Requeue: false}, err
		}
		// Error reading the object - requeue the request.
		return ctrl.Result{}, fmt.Errorf("could not load hostFirmwareComponents: %w", err)
	}

	// Create a provisioner to access Ironic API
	prov, err := r.ProvisionerFactory.NewProvisioner(ctx, provisioner.BuildHostDataNoBMC(*bmh), info.publishEvent)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to create provisioner: %w", err)
	}

	ready, err := prov.TryInit()
	if err != nil || !ready {
		var msg string
		if err == nil {
			msg = "not ready"
		} else {
			msg = err.Error()
		}
		reqLogger.Info("provisioner is not ready", "Error", msg, "RequeueAfter", provisionerRetryDelay)
		return ctrl.Result{Requeue: true, RequeueAfter: provisionerRetryDelay}, nil
	}

	newStatus, err := r.updateHostFirmware(info)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("could not update hostfirmwarecomponents: %w", err)
	}

	// Check ironic for the components information if possible
	components, err := prov.GetFirmwareComponents()
	info.log.Info("retrieving firmware components and saving to resource", "Node", bmh.Status.Provisioning.ID)

	if err != nil {
		reqLogger.Error(err, "provisioner returns error", "RequeueAfter", provisionerRetryDelay)
		setUpdatesCondition(info.hfc.GetGeneration(), &newStatus, info, metal3api.HostFirmwareComponentsValid, metav1.ConditionFalse, reasonInvalidComponent, err.Error())
		return ctrl.Result{Requeue: true, RequeueAfter: provisionerRetryDelay}, err
	}

	if err = r.updateHostFirmwareComponents(newStatus, components, info); err != nil {
		return ctrl.Result{Requeue: false}, err
	}

	for _, e := range info.events {
		r.publishEvent(info.ctx, req, e)
	}

	if meta.IsStatusConditionTrue(info.hfc.Status.Conditions, string(metal3api.HostFirmwareComponentsChangeDetected)) {
		return ctrl.Result{Requeue: true, RequeueAfter: reconcilerRequeueDelayChangeDetected}, nil
	}
	return ctrl.Result{Requeue: true, RequeueAfter: reconcilerRequeueDelay}, nil
}

// Update the HostFirmwareComponents resource using the components from provisioner.
func (r *HostFirmwareComponentsReconciler) updateHostFirmware(info *rhfcInfo) (newStatus metal3api.HostFirmwareComponentsStatus, err error) {
	dirty := false

	// change the Updates in Status
	newStatus.Updates = info.hfc.Spec.Updates

	// Check if the updates in the Spec are different than Status
	updatesMismatch := !reflect.DeepEqual(info.hfc.Status.Updates, info.hfc.Spec.Updates)

	reason := reasonValidComponent
	generation := info.hfc.GetGeneration()

	if updatesMismatch {
		if setUpdatesCondition(generation, &newStatus, info, metal3api.HostFirmwareComponentsChangeDetected, metav1.ConditionTrue, reason, "") {
			dirty = true
		}

		err := r.validateHostFirmwareComponents(info)
		if err != nil {
			info.publishEvent("ValidationFailed", fmt.Sprintf("Invalid Firmware Components: %s", err))
			reason = reasonInvalidComponent
			if setUpdatesCondition(generation, &newStatus, info, metal3api.HostFirmwareComponentsValid, metav1.ConditionFalse, reason, fmt.Sprintf("Invalid Firmware Components: %s", err)) {
				dirty = true
			}
		} else if setUpdatesCondition(generation, &newStatus, info, metal3api.HostFirmwareComponentsValid, metav1.ConditionTrue, reason, "") {
			dirty = true
		}
	} else {
		if setUpdatesCondition(generation, &newStatus, info, metal3api.HostFirmwareComponentsValid, metav1.ConditionTrue, reason, "") {
			dirty = true
		}
		if setUpdatesCondition(generation, &newStatus, info, metal3api.HostFirmwareComponentsChangeDetected, metav1.ConditionFalse, reason, "") {
			dirty = true
		}
	}

	// Update Status if has changed
	if dirty {
		info.log.Info("Status for HostFirmwareComponents changed")
		info.hfc.Status = *newStatus.DeepCopy()

		t := metav1.Now()
		info.hfc.Status.LastUpdated = &t
		return newStatus, r.Status().Update(info.ctx, info.hfc)
	}
	return newStatus, nil
}

// Update the HostFirmwareComponents resource using the components from provisioner.
func (r *HostFirmwareComponentsReconciler) updateHostFirmwareComponents(newStatus metal3api.HostFirmwareComponentsStatus, components []metal3api.FirmwareComponentStatus, info *rhfcInfo) (err error) {
	dirty := false
	// change the Components in Status
	newStatus.Components = components
	// Check if the components information we retrieved is different from the one in Status
	componentsInfoMismatch := !reflect.DeepEqual(components, info.hfc.Status.Components)
	reason := reasonValidComponent
	generation := info.hfc.GetGeneration()
	// Log the components we have
	info.log.Info("firmware components for node", "components", components, "bmh", info.bmh.Name)
	if componentsInfoMismatch {
		setUpdatesCondition(generation, &newStatus, info, metal3api.HostFirmwareComponentsChangeDetected, metav1.ConditionTrue, reason, "")
		dirty = true
	}
	if dirty {
		info.log.Info("Components Status for HostFirmwareComponents changed")
		info.hfc.Status = *newStatus.DeepCopy()

		t := metav1.Now()
		info.hfc.Status.LastUpdated = &t
		return r.Status().Update(info.ctx, info.hfc)
	}
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *HostFirmwareComponentsReconciler) SetupWithManager(mgr ctrl.Manager, maxConcurrentReconcile int) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&metal3api.HostFirmwareComponents{}).
		WithOptions(controller.Options{MaxConcurrentReconciles: maxConcurrentReconcile}).
		WithEventFilter(
			predicate.Funcs{
				UpdateFunc: r.updateEventHandler,
			}).
		Complete(r)
}

func (r *HostFirmwareComponentsReconciler) updateEventHandler(e event.UpdateEvent) bool {
	r.Log.Info("hostfirmwarecomponents in event handler")

	return e.ObjectNew.GetGeneration() != e.ObjectOld.GetGeneration()
}

func (r *HostFirmwareComponentsReconciler) validateHostFirmwareComponents(info *rhfcInfo) []error {
	var errors []error
	allowedNames := map[string]struct{}{"bmc": {}, "bios": {}}
	for _, update := range info.hfc.Spec.Updates {
		componentName := update.Component
		if _, ok := allowedNames[componentName]; !ok {
			errors = append(errors, fmt.Errorf("component %s is invalid, only 'bmc' or 'bios' are allowed as update names", componentName))
		}
		if len(errors) == 0 {
			componentInStatus := false
			for _, componentStatus := range info.hfc.Status.Components {
				if componentName == componentStatus.Component {
					componentInStatus = true
					break
				}
			}
			if !componentInStatus {
				errors = append(errors, fmt.Errorf("component %s is invalid because is not present in status", componentName))
			}
		}
	}

	return errors
}

func (r *HostFirmwareComponentsReconciler) publishEvent(ctx context.Context, request ctrl.Request, event corev1.Event) {
	reqLogger := r.Log.WithValues("hostfirmwarecomponents", request.NamespacedName)
	reqLogger.Info("publishing event", "reason", event.Reason, "message", event.Message)
	err := r.Create(ctx, &event)
	if err != nil {
		reqLogger.Info("failed to record event, ignoring",
			"reason", event.Reason, "message", event.Message, "error", err)
	}
}

func setUpdatesCondition(generation int64, status *metal3api.HostFirmwareComponentsStatus, info *rhfcInfo,
	cond metal3api.UpdatesConditionType, newStatus metav1.ConditionStatus,
	reason conditionReasonHFC, message string) bool {
	newCondition := metav1.Condition{
		Type:               string(cond),
		Status:             newStatus,
		ObservedGeneration: generation,
		Reason:             string(reason),
		Message:            message,
	}
	currCond := meta.FindStatusCondition(info.hfc.Status.Conditions, string(cond))
	meta.SetStatusCondition(&status.Conditions, newCondition)

	if currCond == nil || currCond.Status != newStatus {
		return true
	}
	return false
}
