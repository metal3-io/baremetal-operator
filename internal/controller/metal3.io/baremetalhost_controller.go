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
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"runtime"
	"strings"
	"time"

	"github.com/go-logr/logr"
	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	"github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1/profile"
	"github.com/metal3-io/baremetal-operator/pkg/hardwareutils/bmc"
	"github.com/metal3-io/baremetal-operator/pkg/imageauthvalidator"
	"github.com/metal3-io/baremetal-operator/pkg/imageprovider"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner"
	"github.com/metal3-io/baremetal-operator/pkg/secretutils"
	"github.com/metal3-io/baremetal-operator/pkg/utils"
	"github.com/prometheus/client_golang/prometheus"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

const (
	hostErrorRetryDelay           = time.Second * 10
	unmanagedRetryDelay           = time.Minute * 10
	preprovImageRetryDelay        = time.Minute * 5
	provisionerNotReadyRetryDelay = time.Second * 30
	subResourceNotReadyRetryDelay = time.Second * 60
	clarifySoftPoweroffFailure    = "Continuing with hard poweroff after soft poweroff fails. More details: "
	hardwareDataFinalizer         = metal3api.BareMetalHostFinalizer + "/hardwareData"
	NotReady                      = "Not ready"
)

// BareMetalHostReconciler reconciles a BareMetalHost object.
type BareMetalHostReconciler struct {
	client.Client
	Log                logr.Logger
	ProvisionerFactory provisioner.Factory
	APIReader          client.Reader
	Recorder           record.EventRecorder
}

// Instead of passing a zillion arguments to the action of a phase,
// hold them in a context.
type reconcileInfo struct {
	ctx               context.Context
	log               logr.Logger
	host              *metal3api.BareMetalHost
	request           ctrl.Request
	bmcCredsSecret    *corev1.Secret
	events            []corev1.Event
	postSaveCallbacks []func()
}

// match the provisioner.EventPublisher interface.
func (info *reconcileInfo) publishEvent(reason, message string) {
	info.events = append(info.events, info.host.NewEvent(reason, message))
}

// +kubebuilder:rbac:groups=metal3.io,resources=baremetalhosts,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=metal3.io,resources=baremetalhosts/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=metal3.io,resources=baremetalhosts/finalizers,verbs=update
// +kubebuilder:rbac:groups=metal3.io,resources=preprovisioningimages,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=metal3.io,resources=hardwaredata,verbs=get;list;watch;create;delete;patch;update
// +kubebuilder:rbac:groups=metal3.io,resources=hardware/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;update;delete
// +kubebuilder:rbac:groups="",resources=events,verbs=get;list;watch;create;update;patch

// Allow for managing hostfirmwaresettings, firmwareschema, bmceventsubscriptions and hostfirmwarecomponents
// +kubebuilder:rbac:groups=metal3.io,resources=hostfirmwaresettings,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups=metal3.io,resources=firmwareschemas,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups=metal3.io,resources=bmceventsubscriptions,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups=metal3.io,resources=hostfirmwarecomponents,verbs=get;list;watch;create;update;patch

// Allow for updating dataimage
// +kubebuilder:rbac:groups=metal3.io,resources=dataimages,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups=metal3.io,resources=dataimages/status,verbs=get;update;patch

// Allow for updating hostupdatepolicies
// +kubebuilder:rbac:groups=metal3.io,resources=hostupdatepolicies,verbs=get;list;watch;update

// Allow reading Ironic resources
// +kubebuilder:rbac:groups=ironic.metal3.io,resources=ironics,verbs=get;list;watch

// Reconcile handles changes to BareMetalHost resources.
func (r *BareMetalHostReconciler) Reconcile(ctx context.Context, request ctrl.Request) (result ctrl.Result, err error) {
	reconcileCounters.With(hostMetricLabels(request)).Inc()
	defer func() {
		if err != nil {
			reconcileErrorCounter.Inc()
		}
	}()

	reqLogger := r.Log.WithValues("baremetalhost", request.NamespacedName)
	reqLogger.Info("start")

	// Fetch the BareMetalHost
	host := &metal3api.BareMetalHost{}
	err = r.Get(ctx, request.NamespacedName, host)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			// Request object not found, could have been deleted after
			// reconcile request.  Owned objects are automatically
			// garbage collected. For additional cleanup logic use
			// finalizers.  Return and don't requeue
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return ctrl.Result{}, fmt.Errorf("could not load host data: %w", err)
	}

	// If the reconciliation is paused, requeue
	annotations := host.GetAnnotations()
	if annotations != nil {
		if _, ok := annotations[metal3api.PausedAnnotation]; ok {
			reqLogger.Info("host is paused, no work to do")
			return ctrl.Result{Requeue: false}, nil
		}
	}

	hostData, err := r.reconcileHostData(ctx, host, request)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("could not reconcile host data: %w", err)
	} else if hostData.Requeue {
		return ctrl.Result{Requeue: true}, nil
	}

	// Consume hardwaredetails from annotation if present
	hwdUpdated, err := r.updateHardwareDetails(ctx, request, host)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("could not update hardware details: %w", err)
	} else if hwdUpdated {
		return ctrl.Result{Requeue: true}, nil
	}

	// NOTE(dhellmann): Handle a few steps outside of the phase
	// structure because they require extra data lookup (like the
	// credential checks) or have to be done "first" (like delete
	// handling) to avoid looping.

	// Add a finalizer to newly created objects.
	if host.DeletionTimestamp.IsZero() && !hostHasFinalizer(host) {
		reqLogger.Info(
			"adding finalizer",
			"existingFinalizers", host.Finalizers,
			"newValue", metal3api.BareMetalHostFinalizer,
		)
		host.Finalizers = append(host.Finalizers,
			metal3api.BareMetalHostFinalizer)
		err = r.Update(ctx, host)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to add finalizer: %w", err)
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// Retrieve the BMC details from the host spec and validate host
	// BMC details and build the credentials for talking to the
	// management controller.
	var bmcCreds *bmc.Credentials
	var bmcCredsSecret *corev1.Secret
	haveCreds := false
	switch host.Status.Provisioning.State {
	case metal3api.StateNone, metal3api.StateUnmanaged:
		bmcCreds = &bmc.Credentials{}
	default:
		bmcCreds, bmcCredsSecret, err = r.buildAndValidateBMCCredentials(ctx, request, host)
		if err != nil || bmcCreds == nil {
			if !host.DeletionTimestamp.IsZero() {
				// If we are in the process of deletion, try with empty credentials
				bmcCreds = &bmc.Credentials{}
				bmcCredsSecret = &corev1.Secret{}
			} else {
				return r.credentialsErrorResult(ctx, err, request, host)
			}
		} else {
			haveCreds = true
		}
	}

	initialState := host.Status.Provisioning.State
	info := &reconcileInfo{
		ctx:            ctx,
		log:            reqLogger.WithValues("provisioningState", initialState),
		host:           host,
		request:        request,
		bmcCredsSecret: bmcCredsSecret,
	}

	prov, err := r.ProvisionerFactory.NewProvisioner(ctx, provisioner.BuildHostData(*host, *bmcCreds), info.publishEvent)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to create provisioner: %w", err)
	}

	ready, err := prov.TryInit()
	if err != nil || !ready {
		var msg string
		if err == nil {
			msg = NotReady
		} else {
			msg = err.Error()
		}
		provisionerNotReady.Inc()
		reqLogger.Info("provisioner is not ready", "Error", msg, "RequeueAfter", provisionerNotReadyRetryDelay)
		return ctrl.Result{Requeue: true, RequeueAfter: provisionerNotReadyRetryDelay}, nil
	}

	stateMachine := newHostStateMachine(host, r, prov, haveCreds)
	actResult := stateMachine.ReconcileState(info)
	result, err = actResult.Result()

	if err != nil {
		err = fmt.Errorf("action %q failed: %w", initialState, err)
		return result, err
	}

	// Only save status when we're told to, otherwise we
	// introduce an infinite loop reconciling the same object over and
	// over when there is an unrecoverable error (tracked through the
	// error state of the host).
	if actResult.Dirty() {
		// Save Host
		info.log.Info("saving host status",
			"operational status", host.OperationalStatus(),
			"provisioning state", host.Status.Provisioning.State)
		err = r.saveHostStatus(ctx, host)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to save host status after %q: %w", initialState, err)
		}

		for _, cb := range info.postSaveCallbacks {
			cb()
		}
	}

	for _, e := range info.events {
		r.publishEvent(ctx, request, e)
	}

	logResult(info, result)

	return result, nil
}

// Consume inspect.metal3.io/hardwaredetails when either
// inspect.metal3.io=disabled or there are no existing HardwareDetails.
func (r *BareMetalHostReconciler) updateHardwareDetails(ctx context.Context, request ctrl.Request, host *metal3api.BareMetalHost) (bool, error) {
	updated := false
	if host.Status.HardwareDetails == nil || host.InspectionDisabled() {
		objHardwareDetails, err := r.getHardwareDetailsFromAnnotation(host)
		if err != nil {
			return updated, fmt.Errorf("error parsing HardwareDetails from annotation: %w", err)
		}
		if objHardwareDetails != nil {
			host.Status.HardwareDetails = objHardwareDetails
			err = r.saveHostStatus(ctx, host)
			if err != nil {
				return updated, fmt.Errorf("could not update hardwaredetails from annotation: %w", err)
			}
			r.publishEvent(ctx, request, host.NewEvent("UpdateHardwareDetails", "Set HardwareDetails from annotation"))
			updated = true
		}
	}
	// We either just processed the annotation, or the status is already set
	// so we remove it
	annotations := host.GetAnnotations()
	if _, present := annotations[metal3api.HardwareDetailsAnnotation]; present {
		delete(host.Annotations, metal3api.HardwareDetailsAnnotation)
		err := r.Update(ctx, host)
		if err != nil {
			return updated, fmt.Errorf("could not update removing hardwaredetails annotation: %w", err)
		}
		// In the case where the value was not just consumed, generate an event
		if !updated {
			r.publishEvent(ctx, request, host.NewEvent("RemoveAnnotation", "HardwareDetails annotation ignored, status already set and inspection is not disabled"))
		}
	}
	return updated, nil
}

func logResult(info *reconcileInfo, result ctrl.Result) {
	if result.Requeue || result.RequeueAfter != 0 ||
		!utils.StringInList(info.host.Finalizers,
			metal3api.BareMetalHostFinalizer) {
		info.log.Info("done",
			"requeue", result.Requeue,
			"after", result.RequeueAfter)
	} else {
		info.log.Info("stopping on host error",
			"message", info.host.Status.ErrorMessage)
	}
}

func recordActionFailure(info *reconcileInfo, errorType metal3api.ErrorType, errorMessage string) actionFailed {
	setErrorMessage(info.host, errorType, errorMessage)

	eventType := map[metal3api.ErrorType]string{
		metal3api.DetachError:                  "DetachError",
		metal3api.ProvisionedRegistrationError: "ProvisionedRegistrationError",
		metal3api.RegistrationError:            "RegistrationError",
		metal3api.InspectionError:              "InspectionError",
		metal3api.ProvisioningError:            "ProvisioningError",
		metal3api.PowerManagementError:         "PowerManagementError",
		metal3api.PreparationError:             "PreparationError",
		metal3api.ServicingError:               "ServicingError",
	}[errorType]

	counter := actionFailureCounters.WithLabelValues(eventType)
	info.postSaveCallbacks = append(info.postSaveCallbacks, counter.Inc)

	info.publishEvent(eventType, errorMessage)

	return actionFailed{dirty: true, ErrorType: errorType, errorCount: info.host.Status.ErrorCount}
}

func recordActionDelayed(info *reconcileInfo, state metal3api.ProvisioningState) actionResult {
	var counter prometheus.Counter

	if state == metal3api.StateDeprovisioning {
		counter = delayedDeprovisioningHostCounters.With(hostMetricLabels(info.request))
	} else {
		counter = delayedProvisioningHostCounters.With(hostMetricLabels(info.request))
	}

	info.postSaveCallbacks = append(info.postSaveCallbacks, counter.Inc)

	info.host.SetOperationalStatus(metal3api.OperationalStatusDelayed)
	return actionDelayed{}
}

func (r *BareMetalHostReconciler) credentialsErrorResult(ctx context.Context, err error, request ctrl.Request, host *metal3api.BareMetalHost) (ctrl.Result, error) {
	switch err.(type) {
	// In the event a credential secret is defined, but we cannot find it
	// we requeue the host as we will not know if they create the secret
	// at some point in the future.
	case *ResolveBMCSecretRefError:
		credentialsMissing.Inc()
		saveErr := r.setErrorCondition(ctx, request, host, metal3api.RegistrationError, err.Error())
		if saveErr != nil {
			return ctrl.Result{Requeue: true}, saveErr
		}
		r.publishEvent(ctx, request, host.NewEvent("BMCCredentialError", err.Error()))

		return ctrl.Result{Requeue: true, RequeueAfter: hostErrorRetryDelay}, nil
	// If a managed Host is missing a BMC address or secret, or
	// we have found the secret but it is missing the required fields,
	// or the BMC address is defined but malformed, we set the
	// host into an error state but we do not Requeue it
	// as fixing the secret or the host BMC info will trigger
	// the host to be reconciled again
	case *EmptyBMCAddressError, *EmptyBMCSecretError,
		*bmc.CredentialsValidationError, *bmc.UnknownBMCTypeError:
		credentialsInvalid.Inc()
		saveErr := r.setErrorCondition(ctx, request, host, metal3api.RegistrationError, err.Error())
		if saveErr != nil {
			return ctrl.Result{Requeue: true}, saveErr
		}
		// Only publish the event if we do not have an error
		// after saving so that we only publish one time.
		r.publishEvent(ctx, request, host.NewEvent("BMCCredentialError", err.Error()))
		return ctrl.Result{}, nil
	default:
		unhandledCredentialsError.Inc()
		return ctrl.Result{}, fmt.Errorf("an unhandled failure occurred with the BMC secret: %w", err)
	}
}

// hasRebootAnnotation checks for existence of reboot annotations and returns true if at least one exist.
func hasRebootAnnotation(info *reconcileInfo, expectForce bool) (hasReboot bool, rebootMode metal3api.RebootMode) {
	rebootMode = metal3api.RebootModeSoft

	for annotation, value := range info.host.GetAnnotations() {
		if isRebootAnnotation(annotation) {
			newReboot := getRebootAnnotationArguments(value, info)
			if expectForce && !newReboot.Force {
				continue
			}

			hasReboot = true
			// If any annotation has asked for a hard reboot, that
			// mode takes precedence.
			if newReboot.Mode == metal3api.RebootModeHard {
				rebootMode = newReboot.Mode
			}

			// Don't use a break here as we may have multiple clients setting
			// reboot annotations and we always want hard requests honoured
		}
	}
	return
}

func getRebootAnnotationArguments(annotation string, info *reconcileInfo) (result metal3api.RebootAnnotationArguments) {
	result.Mode = metal3api.RebootModeSoft
	if annotation == "" {
		info.log.Info("No reboot annotation value specified, assuming soft-reboot.")
		return
	}

	err := json.Unmarshal([]byte(annotation), &result)
	if err != nil {
		info.publishEvent("InvalidAnnotationValue", fmt.Sprintf("could not parse reboot annotation (%s) - invalid json, assuming soft-reboot", annotation))
		info.log.Info(fmt.Sprintf("Could not parse reboot annotation (%q) - invalid json, assuming soft-reboot", annotation))
		return
	}
	return
}

// isRebootAnnotation returns true if the provided annotation is a reboot annotation (either suffixed or not).
func isRebootAnnotation(annotation string) bool {
	return strings.HasPrefix(annotation, metal3api.RebootAnnotationPrefix+"/") || annotation == metal3api.RebootAnnotationPrefix
}

// clearRebootAnnotations deletes all reboot annotations exist on the provided host.
func clearRebootAnnotations(host *metal3api.BareMetalHost) (dirty bool) {
	for annotation := range host.Annotations {
		if isRebootAnnotation(annotation) {
			delete(host.Annotations, annotation)
			dirty = true
		}
	}

	return
}

// inspectionRefreshRequested checks for existence of inspect.metal3.io
// annotation and returns true if it exist.
func inspectionRefreshRequested(host *metal3api.BareMetalHost) bool {
	annotations := host.GetAnnotations()
	if annotations != nil {
		if expect, ok := annotations[metal3api.InspectAnnotationPrefix]; ok && expect != metal3api.InspectAnnotationValueDisabled {
			return true
		}
	}
	return false
}

// clearErrorWithStatus removes any existing error message and sets operational status.
func clearErrorWithStatus(host *metal3api.BareMetalHost, status metal3api.OperationalStatus) (dirty bool) {
	dirty = host.SetOperationalStatus(status)
	var emptyErrType metal3api.ErrorType
	if host.Status.ErrorType != emptyErrType {
		host.Status.ErrorType = emptyErrType
		dirty = true
	}
	if host.Status.ErrorMessage != "" {
		host.Status.ErrorMessage = ""
		dirty = true
	}
	return dirty
}

// clearError removes any existing error message.
func clearError(host *metal3api.BareMetalHost) (dirty bool) {
	return clearErrorWithStatus(host, metal3api.OperationalStatusOK)
}

// setErrorMessage updates the ErrorMessage in the host Status struct
// and increases the ErrorCount.
func setErrorMessage(host *metal3api.BareMetalHost, errType metal3api.ErrorType, message string) {
	host.Status.OperationalStatus = metal3api.OperationalStatusError
	host.Status.ErrorType = errType
	host.Status.ErrorMessage = message
	host.Status.ErrorCount++
}

func (r *BareMetalHostReconciler) actionPowerOffBeforeDeleting(prov provisioner.Provisioner, info *reconcileInfo) actionResult {
	if info.host.Spec.DisablePowerOff {
		info.log.Info("Skipping host powered off as Power Off has been disabled")
		return actionComplete{}
	}

	info.log.Info("host ready to be powered off")
	provResult, err := prov.PowerOff(
		metal3api.RebootModeHard,
		info.host.Status.ErrorType == metal3api.PowerManagementError)

	if err != nil {
		return actionError{fmt.Errorf("failed to power off before deleting node: %w", err)}
	}

	if provResult.ErrorMessage != "" {
		return recordActionFailure(info, metal3api.PowerManagementError, provResult.ErrorMessage)
	}

	if provResult.Dirty {
		result := actionContinue{provResult.RequeueAfter}
		if clearError(info.host) {
			return actionUpdate{result}
		}
		return result
	}

	return actionComplete{}
}

// Manage deletion of the host.
func (r *BareMetalHostReconciler) actionDeleting(prov provisioner.Provisioner, info *reconcileInfo) actionResult {
	info.log.Info(
		"marked to be deleted",
		"timestamp", info.host.DeletionTimestamp,
	)

	// no-op if finalizer has been removed.
	if !utils.StringInList(info.host.Finalizers, metal3api.BareMetalHostFinalizer) {
		info.log.Info("ready to be deleted")
		return deleteComplete{}
	}

	provResult, err := prov.Delete()
	if err != nil {
		return actionError{fmt.Errorf("failed to delete: %w", err)}
	}
	if provResult.Dirty {
		return actionContinue{provResult.RequeueAfter}
	}

	// Remove finalizer to allow deletion
	secretManager := secretutils.NewSecretManager(info.ctx, info.log, r.Client, r.APIReader)

	err = secretManager.ReleaseSecret(info.bmcCredsSecret)
	if err != nil {
		return actionError{err}
	}

	info.host.Finalizers = utils.FilterStringFromList(
		info.host.Finalizers, metal3api.BareMetalHostFinalizer)
	info.log.Info("cleanup is complete, removed finalizer",
		"remaining", info.host.Finalizers)
	if err := r.Update(info.ctx, info.host); err != nil {
		return actionError{fmt.Errorf("failed to remove finalizer: %w", err)}
	}

	return deleteComplete{}
}

func (r *BareMetalHostReconciler) actionUnmanaged(_ provisioner.Provisioner, info *reconcileInfo) actionResult {
	if info.host.HasBMCDetails() {
		return actionComplete{}
	}
	return actionContinue{unmanagedRetryDelay}
}

// getCurrentImage() returns the current image that has been or is being
// provisioned.
func getCurrentImage(host *metal3api.BareMetalHost) *metal3api.Image {
	// If an image is currently provisioned, return it
	if host.Status.Provisioning.Image.URL != "" {
		return host.Status.Provisioning.Image.DeepCopy()
	}

	// If we are in the process of provisioning an image, return that image
	switch host.Status.Provisioning.State {
	case metal3api.StateProvisioning, metal3api.StateExternallyProvisioned:
		if host.Spec.Image != nil && host.Spec.Image.URL != "" {
			return host.Spec.Image.DeepCopy()
		}
	default:
	}
	return nil
}

func hasCustomDeploy(host *metal3api.BareMetalHost) bool {
	if host.Status.Provisioning.CustomDeploy != nil && host.Status.Provisioning.CustomDeploy.Method != "" {
		return true
	}

	switch host.Status.Provisioning.State {
	case metal3api.StateProvisioning, metal3api.StateExternallyProvisioned:
		return host.Spec.CustomDeploy != nil && host.Spec.CustomDeploy.Method != ""
	default:
		return false
	}
}

// detachHost() detaches the host from the Provisioner.
func (r *BareMetalHostReconciler) detachHost(prov provisioner.Provisioner, info *reconcileInfo) actionResult {
	provResult, err := prov.Detach()
	if err != nil {
		return actionError{fmt.Errorf("failed to detach: %w", err)}
	}
	if provResult.ErrorMessage != "" {
		return recordActionFailure(info, metal3api.DetachError, provResult.ErrorMessage)
	}
	if provResult.Dirty {
		if info.host.Status.ErrorType == metal3api.DetachError && clearError(info.host) {
			return actionUpdate{actionContinue{provResult.RequeueAfter}}
		}
		return actionContinue{provResult.RequeueAfter}
	}
	slowPoll := actionContinue{unmanagedRetryDelay}
	if info.host.Status.ErrorType == metal3api.DetachError {
		clearError(info.host)
		info.host.Status.ErrorCount = 0
	}
	if info.host.SetOperationalStatus(metal3api.OperationalStatusDetached) {
		info.log.Info("host is detached, removed from provisioner")
		return actionUpdate{slowPoll}
	}
	return slowPoll
}

type imageBuildError struct {
	Message string
}

func (ibe imageBuildError) Error() string {
	return ibe.Message
}

func (r *BareMetalHostReconciler) preprovImageAvailable(info *reconcileInfo, image *metal3api.PreprovisioningImage) (bool, error) {
	if image.Status.Architecture != image.Spec.Architecture {
		info.log.Info("pre-provisioning image architecture mismatch",
			"wanted", image.Spec.Architecture,
			"current", image.Status.Architecture)
		return false, nil
	}

	validFormat := false
	for _, f := range image.Spec.AcceptFormats {
		if image.Status.Format == f {
			validFormat = true
			break
		}
	}
	if !validFormat {
		info.log.Info("pre-provisioning image format not accepted",
			"format", image.Status.Format)
		return false, nil
	}

	if image.Spec.NetworkDataName != "" {
		secretKey := client.ObjectKey{
			Name:      image.Spec.NetworkDataName,
			Namespace: image.ObjectMeta.Namespace,
		}
		secretManager := r.secretManager(info.ctx, info.log)
		networkData, err := secretManager.AcquireSecret(secretKey, info.host, false)
		if err != nil {
			return false, err
		}
		if image.Status.NetworkData.Version != networkData.GetResourceVersion() {
			info.log.Info("network data in pre-provisioning image is out of date",
				"latestVersion", networkData.GetResourceVersion(),
				"currentVersion", image.Status.NetworkData.Version)
			return false, nil
		}
	}
	if image.Status.NetworkData.Name != image.Spec.NetworkDataName {
		info.log.Info("network data location in pre-provisioning image is out of date")
		return false, nil
	}

	if errCond := meta.FindStatusCondition(image.Status.Conditions, string(metal3api.ConditionImageError)); errCond != nil && errCond.Status == metav1.ConditionTrue {
		info.log.Info("error building PreprovisioningImage",
			"message", errCond.Message)
		return false, imageBuildError{errCond.Message}
	}
	if readyCond := meta.FindStatusCondition(image.Status.Conditions, string(metal3api.ConditionImageReady)); readyCond != nil && readyCond.Status == metav1.ConditionTrue && readyCond.ObservedGeneration == image.Generation {
		return true, nil
	}

	info.log.Info("pending PreprovisioningImage not ready")
	return false, nil
}

// getControllerArchitecture returns the CPU architecture of the currently
// running Go program in a format that mimics the output of "uname -p".
func getControllerArchitecture() string {
	switch runtime.GOARCH {
	case "amd64":
		return "x86_64"
	case "arm64":
		return "aarch64"
	default:
		return runtime.GOARCH
	}
}

func getHostArchitecture(host *metal3api.BareMetalHost) string {
	if host.Spec.Architecture != "" {
		return host.Spec.Architecture
	}
	// FIXME(dtantsur): this relies on the essentially deprecated HardwareDetails field.
	if host.Status.HardwareDetails != nil &&
		host.Status.HardwareDetails.CPU.Arch != "" {
		return host.Status.HardwareDetails.CPU.Arch
	}

	return getControllerArchitecture()
}

func (r *BareMetalHostReconciler) getPreprovImage(info *reconcileInfo, formats []metal3api.ImageFormat) (*provisioner.PreprovisioningImage, error) {
	if formats == nil {
		// No image build requested
		return nil, nil //nolint:nilnil
	}

	if len(formats) == 0 {
		return nil, imageBuildError{"no acceptable formats for preprovisioning image"}
	}

	expectedSpec := metal3api.PreprovisioningImageSpec{
		NetworkDataName: info.host.Spec.PreprovisioningNetworkDataName,
		Architecture:    getHostArchitecture(info.host),
		AcceptFormats:   formats,
	}

	preprovImage := metal3api.PreprovisioningImage{}
	key := client.ObjectKey{
		Name:      info.host.Name,
		Namespace: info.host.Namespace,
	}
	err := r.Get(info.ctx, key, &preprovImage)
	if k8serrors.IsNotFound(err) {
		info.log.Info("creating new PreprovisioningImage")
		preprovImage = metal3api.PreprovisioningImage{
			ObjectMeta: metav1.ObjectMeta{
				Name:      key.Name,
				Namespace: key.Namespace,
				Labels:    info.host.Labels,
			},
			Spec: expectedSpec,
		}
		err = controllerutil.SetControllerReference(info.host, &preprovImage, r.Scheme())
		if err != nil {
			return nil, fmt.Errorf("failed to set controller reference for PreprovisioningImage due to %w", err)
		}

		err = r.Create(info.ctx, &preprovImage)
		return nil, err
	}
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve pre-provisioning image data: %w", err)
	}

	needsUpdate := false
	if preprovImage.Labels == nil && len(info.host.Labels) > 0 {
		preprovImage.Labels = make(map[string]string, len(info.host.Labels))
	}
	for k, v := range info.host.Labels {
		if cur, ok := preprovImage.Labels[k]; !ok || cur != v {
			preprovImage.Labels[k] = v
			needsUpdate = true
		}
	}
	if !apiequality.Semantic.DeepEqual(preprovImage.Spec, expectedSpec) {
		info.log.Info("updating PreprovisioningImage spec")
		preprovImage.Spec = expectedSpec
		needsUpdate = true
	}
	if needsUpdate {
		info.log.Info("updating PreprovisioningImage")
		err = r.Update(info.ctx, &preprovImage)
		return nil, err
	}

	if available, err := r.preprovImageAvailable(info, &preprovImage); err != nil || !available {
		return nil, err
	}

	image := provisioner.PreprovisioningImage{
		GeneratedImage: imageprovider.GeneratedImage{
			ImageURL:          preprovImage.Status.ImageUrl,
			KernelURL:         preprovImage.Status.KernelUrl,
			ExtraKernelParams: preprovImage.Status.ExtraKernelParams,
		},
		Format: preprovImage.Status.Format,
	}
	info.log.Info("using PreprovisioningImage", "Image", image)
	return &image, nil
}

// Test the credentials by connecting to the management controller.
func (r *BareMetalHostReconciler) registerHost(prov provisioner.Provisioner, info *reconcileInfo) actionResult {
	info.log.V(1).Info("registering and validating access to management controller",
		"credentials", info.host.Status.TriedCredentials)
	dirty := false

	credsChanged := !info.host.Status.TriedCredentials.Match(*info.bmcCredsSecret)
	if credsChanged {
		info.log.Info("new credentials", "newVersion", info.bmcCredsSecret.ResourceVersion)
		info.host.UpdateTriedCredentials(*info.bmcCredsSecret)
		info.postSaveCallbacks = append(info.postSaveCallbacks, updatedCredentials.Inc)
		dirty = true
	}

	preprovImgFormats, err := prov.PreprovisioningImageFormats()
	if err != nil {
		return actionError{err}
	}
	switch info.host.Status.Provisioning.State {
	case metal3api.StateRegistering, metal3api.StateDeleting, metal3api.StatePoweringOffBeforeDelete:
		// No need to create PreprovisioningImage if host is not yet registered
		preprovImgFormats = nil
	case metal3api.StateProvisioned, metal3api.StateExternallyProvisioned:
		// Provisioned hosts only need the image for servicing
		if info.host.Status.OperationalStatus != metal3api.OperationalStatusServicing {
			preprovImgFormats = nil
		}
	case metal3api.StateDeprovisioning:
		// PreprovisioningImage is not required for deprovisioning when cleaning is disabled
		if info.host.Spec.AutomatedCleaningMode == metal3api.CleaningModeDisabled {
			preprovImgFormats = nil
		}
	default:
	}

	preprovImg, err := r.getPreprovImage(info, preprovImgFormats)
	if err != nil {
		if errors.As(err, &imageBuildError{}) {
			return recordActionFailure(info, metal3api.RegistrationError, err.Error())
		}
		return actionError{err}
	}

	hostConf := &hostConfigData{
		host:          info.host,
		log:           info.log.WithName("host_config_data"),
		secretManager: r.secretManager(info.ctx, info.log),
	}
	preprovisioningNetworkData, err := hostConf.PreprovisioningNetworkData()
	if err != nil {
		return recordActionFailure(info, metal3api.RegistrationError, "failed to read preprovisioningNetworkData")
	}

	provResult, provID, err := prov.Register(
		provisioner.ManagementAccessData{
			BootMode:                   info.host.Status.Provisioning.BootMode,
			AutomatedCleaningMode:      info.host.Spec.AutomatedCleaningMode,
			State:                      info.host.Status.Provisioning.State,
			OperationalStatus:          info.host.Status.OperationalStatus,
			CurrentImage:               getCurrentImage(info.host),
			PreprovisioningImage:       preprovImg,
			PreprovisioningNetworkData: preprovisioningNetworkData,
			HasCustomDeploy:            hasCustomDeploy(info.host),
			DisablePowerOff:            info.host.Spec.DisablePowerOff,
			CPUArchitecture:            getHostArchitecture(info.host),
		},
		credsChanged,
		info.host.Status.ErrorType == metal3api.RegistrationError)

	if errors.Is(err, provisioner.ErrNeedsPreprovisioningImage) &&
		preprovImgFormats != nil {
		if preprovImg == nil {
			waitingForPreprovImage.Inc()
			return actionContinue{preprovImageRetryDelay}
		}
		return recordActionFailure(info, metal3api.RegistrationError,
			"Preprovisioning Image is not acceptable to provisioner")
	}
	if err != nil {
		noManagementAccess.Inc()
		return actionError{fmt.Errorf("failed to validate BMC access: %w", err)}
	}

	if provResult.ErrorMessage != "" {
		return recordActionFailure(info, metal3api.RegistrationError, provResult.ErrorMessage)
	}

	if provID != "" && info.host.Status.Provisioning.ID != provID {
		info.log.Info("setting provisioning id", "ID", provID)
		info.host.Status.Provisioning.ID = provID
		if info.host.Status.Provisioning.State == metal3api.StatePreparing {
			clearHostProvisioningSettings(info.host)
		}
		dirty = true
	}

	if provResult.Dirty {
		info.log.Info("host not ready", "wait", provResult.RequeueAfter)
		result := actionContinue{provResult.RequeueAfter}
		if clearError(info.host) {
			dirty = true
		}
		if dirty {
			return actionUpdate{result}
		}
		return result
	}

	dirty, err = r.matchProfile(info)
	if err != nil {
		return recordActionFailure(info, metal3api.RegistrationError, err.Error())
	}
	if dirty {
		return actionUpdate{}
	}

	// Check if the host can support firmware components before creating the resource
	_, errGetFirmwareComponents := prov.GetFirmwareComponents()
	supportsFirmwareComponents := !errors.Is(errGetFirmwareComponents, provisioner.ErrFirmwareUpdateUnsupported)

	// Create the hostFirmwareSettings resource with same host name/namespace if it doesn't exist
	// Create the hostFirmwareComponents resource with same host name/namespace if it doesn't exist
	if info.host.Name != "" {
		if !info.host.DeletionTimestamp.IsZero() {
			info.log.Info("will not attempt to create new hostFirmwareSettings and hostFirmwareComponents in " + info.host.Namespace)
		} else {
			if err = r.createHostFirmwareSettings(info); err != nil {
				info.log.Info("failed creating hostfirmwaresettings")
				return actionError{fmt.Errorf("failed to validate BMC access: %w", err)}
			}
			if supportsFirmwareComponents {
				if err = r.createHostFirmwareComponents(info); err != nil {
					info.log.Info("failed creating hostfirmwarecomponents")
					return actionError{fmt.Errorf("failed creating hostFirmwareComponents: %w", err)}
				}
			}
			if _, err = r.acquireHostUpdatePolicy(info); err != nil {
				info.log.Info("failed setting owner reference on hostupdatepolicy")
				return actionError{fmt.Errorf("failed setting owner reference on hostUpdatePolicy: %w", err)}
			}
		}
	}
	// Reaching this point means the credentials are valid and worked,
	// so clear any previous error and record the success in the
	// status block.
	registeredNewCreds := !info.host.Status.GoodCredentials.Match(*info.bmcCredsSecret)
	if registeredNewCreds {
		info.log.Info("updating credentials success status fields")
		info.host.UpdateGoodCredentials(*info.bmcCredsSecret)
		info.publishEvent("BMCAccessValidated", "Verified access to BMC")
		dirty = true
	} else {
		info.log.V(1).Info("verified access to the BMC")
	}

	if info.host.Status.ErrorType == metal3api.RegistrationError || registeredNewCreds {
		info.log.Info("clearing previous error message")
		dirty = clearError(info.host)
	}

	if dirty {
		return actionComplete{}
	}
	return nil
}

func updateRootDeviceHints(host *metal3api.BareMetalHost, info *reconcileInfo) (dirty bool, err error) {
	// Ensure the root device hints we're going to use are stored.
	//
	// If the user has provided explicit root device hints, they take
	// precedence. Otherwise use the values from the hardware profile.
	hintSource := host.Spec.RootDeviceHints
	if hintSource == nil {
		hwProf, err := profile.GetProfile(host.HardwareProfile())
		if err != nil {
			return false, fmt.Errorf("failed to update root device hints: %w", err)
		}
		hintSource = &hwProf.RootDeviceHints
	}
	if !reflect.DeepEqual(hintSource, host.Status.Provisioning.RootDeviceHints) {
		info.log.Info("RootDeviceHints have changed", "old", host.Status.Provisioning.RootDeviceHints, "new", hintSource)
		host.Status.Provisioning.RootDeviceHints = hintSource.DeepCopy()
		dirty = true
	}
	return
}

// Ensure we have the information about the hardware on the host.
func (r *BareMetalHostReconciler) actionInspecting(prov provisioner.Provisioner, info *reconcileInfo) actionResult {
	info.log.Info("inspecting hardware")

	if info.host.InspectionDisabled() {
		info.log.Info("inspection disabled by user")
		info.publishEvent("InspectionSkipped", "disabled by user")
		return actionComplete{}
	}

	info.log.Info("inspecting hardware")

	refresh := inspectionRefreshRequested(info.host)
	forceReboot, _ := hasRebootAnnotation(info, true)

	provResult, started, details, err := prov.InspectHardware(
		provisioner.InspectData{
			BootMode: info.host.Status.Provisioning.BootMode,
		},
		info.host.Status.ErrorType == metal3api.InspectionError,
		refresh,
		forceReboot)
	if err != nil {
		return actionError{fmt.Errorf("hardware inspection failed: %w", err)}
	}

	if provResult.ErrorMessage != "" {
		return recordActionFailure(info, metal3api.InspectionError, provResult.ErrorMessage)
	}

	if started {
		dirty := false

		// Delete inspect annotation if exists
		if inspectionRefreshRequested(info.host) {
			delete(info.host.Annotations, metal3api.InspectAnnotationPrefix)
			dirty = true
		}

		// Inspection is either freshly started or was aborted. Either way, remove the reboot annotation.
		if clearRebootAnnotations(info.host) {
			dirty = true
		}

		if dirty {
			if err = r.Update(info.ctx, info.host); err != nil {
				return actionError{fmt.Errorf("failed to update the host after inspection start: %w", err)}
			}
			return actionContinue{}
		}
	}

	if provResult.Dirty || details == nil {
		result := actionContinue{provResult.RequeueAfter}
		if clearError(info.host) {
			return actionUpdate{result}
		}
		return result
	}

	clearError(info.host)
	info.host.Status.HardwareDetails = details

	// Create HardwareData with the same name and namesapce as BareMetalHost
	hardwareData := &metal3api.HardwareData{}
	hardwareDataKey := client.ObjectKey{
		Name:      info.host.Name,
		Namespace: info.host.Namespace,
	}
	hd := &metal3api.HardwareData{
		TypeMeta: metav1.TypeMeta{
			Kind:       "HardwareData",
			APIVersion: metal3api.GroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      info.host.Name,
			Namespace: info.host.Namespace,
			// Register the finalizer immediately
			Finalizers: []string{
				hardwareDataFinalizer,
			},
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(info.host, metal3api.GroupVersion.WithKind("BareMetalHost")),
			},
		},
		Spec: metal3api.HardwareDataSpec{
			HardwareDetails: details,
		},
	}

	err = r.Client.Get(info.ctx, hardwareDataKey, hardwareData)
	if err == nil || !k8serrors.IsNotFound(err) {
		// hardwareData found and we reached here due to request for another inspection.
		// Delete it before re-creating.
		if controllerutil.ContainsFinalizer(hardwareData, hardwareDataFinalizer) {
			controllerutil.RemoveFinalizer(hardwareData, hardwareDataFinalizer)
		}
		if err = r.Update(info.ctx, hardwareData); err != nil {
			return actionError{fmt.Errorf("failed to remove hardwareData finalizer: %w", err)}
		}
		if err = r.Client.Delete(info.ctx, hd); err != nil {
			return actionError{fmt.Errorf("failed to delete hardwareData: %w", err)}
		}
	}

	if !info.host.DeletionTimestamp.IsZero() {
		info.log.Info(fmt.Sprintf("will not attempt to create hardwareData in %q", hd.Namespace))
		return actionComplete{}
	}

	// either hardwareData was deleted above, or not found. We need to re-create it
	if err := r.Client.Create(info.ctx, hd); err != nil {
		return actionError{fmt.Errorf("failed to create hardwareData: %w", err)}
	}
	info.log.Info(fmt.Sprintf("Created hardwareData %q in %q namespace\n", hd.Name, hd.Namespace))

	return actionComplete{}
}

func getHardwareProfileName(host *metal3api.BareMetalHost) string {
	if host.Status.HardwareProfile != "" {
		// Profile name already set
		return host.Status.HardwareProfile
	}
	if host.Spec.HardwareProfile != "" {
		// Profile name supplied by user
		return host.Spec.HardwareProfile
	}

	// FIXME(dhellmann): Insert more robust logic to match
	// hardware profiles here.
	if strings.HasPrefix(host.Spec.BMC.Address, "libvirt") {
		return "libvirt"
	}
	return profile.DefaultProfileName
}

func (r *BareMetalHostReconciler) matchProfile(info *reconcileInfo) (dirty bool, err error) {
	hardwareProfile := getHardwareProfileName(info.host)
	info.log.V(1).Info("using hardware profile", "profile", hardwareProfile)

	_, err = profile.GetProfile(hardwareProfile)
	if err != nil {
		info.log.Info("invalid hardware profile", "profile", hardwareProfile)
		return
	}

	if info.host.SetHardwareProfile(hardwareProfile) {
		dirty = true
		info.log.Info("updating hardware profile", "profile", hardwareProfile)
		info.publishEvent("ProfileSet", "Hardware profile set: "+hardwareProfile)
	}

	hintsDirty, err := updateRootDeviceHints(info.host, info)
	if err != nil {
		return
	}

	dirty = dirty || hintsDirty
	return
}

func getUpdatesDifference(specUpdates []metal3api.FirmwareUpdate, statusUpdates []metal3api.FirmwareUpdate) []metal3api.FirmwareUpdate {
	diff := []metal3api.FirmwareUpdate{}
	// Mapping already updated components
	updated := make(map[string]string, len(statusUpdates))
	for _, s := range statusUpdates {
		updated[s.Component] = s.URL
	}

	for _, firmware := range specUpdates {
		if _, ok := updated[firmware.Component]; !ok || firmware.URL != updated[firmware.Component] {
			diff = append(diff, firmware)
		}
	}
	return diff
}

func (r *BareMetalHostReconciler) actionPreparing(prov provisioner.Provisioner, info *reconcileInfo) actionResult {
	info.log.Info("preparing")

	bmhDirty, newStatus, err := getHostProvisioningSettings(info.host, info)
	if err != nil {
		return actionError{err}
	}

	prepareData := provisioner.PrepareData{
		TargetRAIDConfig: newStatus.Provisioning.RAID.DeepCopy(),
		ActualRAIDConfig: info.host.Status.Provisioning.RAID.DeepCopy(),
		RootDeviceHints:  newStatus.Provisioning.RootDeviceHints.DeepCopy(),
		FirmwareConfig:   newStatus.Provisioning.Firmware.DeepCopy(),
	}
	// When manual cleaning fails, we think that the existing RAID configuration
	// is invalid and needs to be reconfigured.
	if info.host.Status.ErrorType == metal3api.PreparationError {
		prepareData.ActualRAIDConfig = nil
		bmhDirty = true
	}

	// The hfsDirty flag is used to push the new settings to Ironic as part of the clean steps.
	// The HFS Status field will be updated in the HostFirmwareSettingsReconciler when it reads the settings from Ironic.
	// After manual cleaning is complete the HFS Spec should match the Status.
	hfsDirty, hfs, err := r.getHostFirmwareSettings(info)

	if err != nil {
		// wait until hostFirmwareSettings are ready
		return actionContinue{subResourceNotReadyRetryDelay}
	}
	if hfsDirty {
		prepareData.ActualFirmwareSettings = hfs.Status.Settings.DeepCopy()
		prepareData.TargetFirmwareSettings = hfs.Spec.Settings.DeepCopy()
	}

	// The hfcDirty flag is used to push the new versions of components to Ironic as part of the clean steps.
	// The HFC Status field will be updated in the HostFirmwareComponentsReconciler when it reads the settings from Ironic.
	// After manual cleaning is complete the HFC Spec should match the Status.
	hfcDirty, hfc, err := r.getHostFirmwareComponents(info)

	if err != nil {
		// wait until hostFirmwareComponents are ready
		return actionContinue{subResourceNotReadyRetryDelay}
	}
	if hfcDirty {
		// Handle only Firmware Component that it is in hfc.Spec.Updates but not in hfc.Status.Updates.
		if hfc.Status.Updates != nil {
			prepareData.TargetFirmwareComponents = getUpdatesDifference(hfc.Spec.Updates, hfc.Status.Updates)
		} else {
			prepareData.TargetFirmwareComponents = hfc.Spec.Updates
		}
	}

	provResult, started, err := prov.Prepare(prepareData, bmhDirty || hfsDirty || hfcDirty,
		info.host.Status.ErrorType == metal3api.PreparationError)

	if err != nil {
		return actionError{fmt.Errorf("error preparing host: %w", err)}
	}

	if provResult.ErrorMessage != "" {
		if bmhDirty {
			info.log.Info("handling cleaning error in controller")
			clearHostProvisioningSettings(info.host)
		}
		if hfcDirty && hfc.Status.Updates != nil {
			info.log.Info("handling cleaning error during firmware update")
			hfc.Status.Updates = nil
			if err := r.Status().Update(info.ctx, hfc); err != nil {
				return actionError{fmt.Errorf("failed to update hostfirmwarecomponents status: %w", err)}
			}
		}
		return recordActionFailure(info, metal3api.PreparationError, provResult.ErrorMessage)
	}

	if hfcDirty && started {
		hfcStillDirty, err := r.saveHostFirmwareComponents(prov, info, hfc)
		if err != nil {
			return actionError{fmt.Errorf("could not save the host firmware components: %w", err)}
		}

		if hfcStillDirty {
			info.log.Info("going to update the host firmware components")
			if err := r.Status().Update(info.ctx, hfc); err != nil {
				return actionError{fmt.Errorf("failed to update hostfirmwarecomponents status: %w", err)}
			}
		}
	}

	if bmhDirty && started {
		info.log.Info("saving host provisioning settings")
		_, err := saveHostProvisioningSettings(info.host, info)
		if err != nil {
			return actionError{fmt.Errorf("could not save the host provisioning settings: %w", err)}
		}
	}

	if started && clearError(info.host) {
		bmhDirty = true
	}
	if provResult.Dirty {
		result := actionContinue{provResult.RequeueAfter}
		if bmhDirty {
			return actionUpdate{result}
		}
		return result
	}

	return actionComplete{}
}

// Start/continue provisioning if we need to.
func (r *BareMetalHostReconciler) actionProvisioning(prov provisioner.Provisioner, info *reconcileInfo) actionResult {
	hostConf := &hostConfigData{
		host:          info.host,
		log:           info.log.WithName("host_config_data"),
		secretManager: r.secretManager(info.ctx, info.log),
	}
	info.log.Info("provisioning")

	hwProf, err := profile.GetProfile(info.host.HardwareProfile())
	if err != nil {
		return actionError{fmt.Errorf(" could not start provisioning with bad hardware profile %s: %w",
			info.host.HardwareProfile(), err)}
	}

	forceReboot, _ := hasRebootAnnotation(info, true)

	var image metal3api.Image
	if info.host.Spec.Image != nil {
		image = *info.host.Spec.Image.DeepCopy()
	}

	// Extract OCI auth secret credentials if needed
	authSecret, err := r.getImageAuthSecret(info.ctx, info.request, info.host, &image)
	if err != nil {
		return recordActionFailure(info, metal3api.ProvisioningError,
			"failed to get image auth secret: "+err.Error())
	}

	provResult, err := prov.Provision(provisioner.ProvisionData{
		Image:           image,
		CustomDeploy:    info.host.Spec.CustomDeploy.DeepCopy(),
		HostConfig:      hostConf,
		BootMode:        info.host.Status.Provisioning.BootMode,
		HardwareProfile: hwProf,
		RootDeviceHints: info.host.Status.Provisioning.RootDeviceHints.DeepCopy(),
		ImagePullSecret: authSecret,
	}, forceReboot)
	if err != nil {
		return actionError{fmt.Errorf("failed to provision: %w", err)}
	}

	if provResult.ErrorMessage != "" {
		info.log.Info("handling provisioning error in controller")
		return recordActionFailure(info, metal3api.ProvisioningError, provResult.ErrorMessage)
	}

	if clearRebootAnnotations(info.host) {
		if err := r.Update(info.ctx, info.host); err != nil {
			return actionError{fmt.Errorf("failed to remove reboot annotations from host: %w", err)}
		}
		return actionContinue{}
	}

	if provResult.Dirty {
		// Go back into the queue and wait for the Provision() method
		// to return false, indicating that it has no more work to
		// do.
		result := actionContinue{provResult.RequeueAfter}
		if clearError(info.host) {
			return actionUpdate{result}
		}
		return result
	}

	// If the provisioner had no work, ensure the image settings match.
	if info.host.Spec.Image != nil && info.host.Status.Provisioning.Image != *(info.host.Spec.Image) {
		info.log.Info("updating deployed image in status")
		info.host.Status.Provisioning.Image = *(info.host.Spec.Image)
	}

	if info.host.Spec.CustomDeploy != nil && (info.host.Status.Provisioning.CustomDeploy == nil || !reflect.DeepEqual(*info.host.Spec.CustomDeploy, *info.host.Status.Provisioning.CustomDeploy)) {
		info.log.Info("updating custom deploy in status")
		info.host.Status.Provisioning.CustomDeploy = info.host.Spec.CustomDeploy.DeepCopy()
	}

	// After provisioning we always requeue to ensure we enter the
	// "provisioned" state and start monitoring power status.
	return actionComplete{}
}

// clearHostProvisioningSettings removes the values related to
// provisioning that do not trigger re-provisioning from the status
// fields of a host.
func clearHostProvisioningSettings(host *metal3api.BareMetalHost) {
	host.Status.Provisioning.RootDeviceHints = nil
	// Keep `HardwareRAIDVolumes` to avoid configuring the same hardware RAID repeatedly
	if host.Status.Provisioning.RAID != nil {
		host.Status.Provisioning.RAID.SoftwareRAIDVolumes = nil
	}
	host.Status.Provisioning.Firmware = nil
}

func (r *BareMetalHostReconciler) actionDeprovisioning(prov provisioner.Provisioner, info *reconcileInfo) actionResult {
	if info.host.Status.Provisioning.Image.URL != "" || info.host.Status.Provisioning.CustomDeploy != nil {
		// Adopt the host in case it has been re-registered during the
		// deprovisioning process before it completed
		provResult, err := prov.Adopt(
			provisioner.AdoptData{State: info.host.Status.Provisioning.State},
			info.host.Status.ErrorType == metal3api.ProvisionedRegistrationError)
		if err != nil {
			return actionError{err}
		}
		if provResult.ErrorMessage != "" {
			return recordActionFailure(info, metal3api.ProvisionedRegistrationError, provResult.ErrorMessage)
		}
		if provResult.Dirty {
			result := actionContinue{provResult.RequeueAfter}
			if clearError(info.host) {
				return actionUpdate{result}
			}
			return result
		}
	}

	info.log.Info("deprovisioning")

	provResult, err := prov.Deprovision(info.host.Status.ErrorType == metal3api.ProvisioningError)
	if err != nil {
		return actionError{fmt.Errorf("failed to deprovision: %w", err)}
	}

	if provResult.ErrorMessage != "" {
		return recordActionFailure(info, metal3api.ProvisioningError, provResult.ErrorMessage)
	}

	if provResult.Dirty {
		result := actionContinue{provResult.RequeueAfter}
		if clearError(info.host) {
			return actionUpdate{result}
		}
		return result
	}

	if clearRebootAnnotations(info.host) {
		if err = r.Update(info.ctx, info.host); err != nil {
			return actionError{fmt.Errorf("failed to remove reboot annotations from host: %w", err)}
		}
		return actionContinue{}
	}

	// After the provisioner is done, clear the provisioning settings
	// so we transition to the next state.
	info.host.Status.Provisioning.Image = metal3api.Image{}
	info.host.Status.Provisioning.CustomDeploy = nil
	clearHostProvisioningSettings(info.host)

	return actionComplete{}
}

func (r *BareMetalHostReconciler) doServiceIfNeeded(prov provisioner.Provisioner, info *reconcileInfo, hup *metal3api.HostUpdatePolicy) (result actionResult) {
	servicingData := provisioner.ServicingData{}

	// (NOTE)janders: since Servicing is an opt-in feature that requires HostUpdatePolicy to be created and set to onReboot
	// set below booleans to false by default and change to true based on policy settings

	var fwDirty bool
	var hfsDirty bool
	var hfcDirty bool
	var hfc *metal3api.HostFirmwareComponents
	var liveFirmwareSettingsAllowed, liveFirmwareUpdatesAllowed bool

	if hup != nil {
		liveFirmwareSettingsAllowed = (hup.Spec.FirmwareSettings == metal3api.HostUpdatePolicyOnReboot)
		liveFirmwareUpdatesAllowed = (hup.Spec.FirmwareUpdates == metal3api.HostUpdatePolicyOnReboot)
	}

	if liveFirmwareSettingsAllowed {
		// handling pre-HFS FirmwareSettings here
		if !reflect.DeepEqual(info.host.Status.Provisioning.Firmware, info.host.Spec.Firmware) {
			servicingData.FirmwareConfig = info.host.Spec.Firmware
			fwDirty = true
		}
		// handling HFS based FirmwareSettings here
		var hfs *metal3api.HostFirmwareSettings
		var err error
		hfsDirty, hfs, err = r.getHostFirmwareSettings(info)
		if err != nil {
			return actionError{fmt.Errorf("could not determine updated settings: %w", err)}
		}
		if hfsDirty {
			servicingData.ActualFirmwareSettings = hfs.Status.Settings
			servicingData.TargetFirmwareSettings = hfs.Spec.Settings
		}
	}

	if liveFirmwareUpdatesAllowed {
		var err error
		hfcDirty, hfc, err = r.getHostFirmwareComponents(info)
		if err != nil {
			return actionError{fmt.Errorf("could not determine firmware components: %w", err)}
		}
		if hfcDirty {
			// Handle only Firmware Component that it is in hfc.Spec.Updates but not in hfc.Status.Updates.
			if hfc.Status.Updates != nil {
				servicingData.TargetFirmwareComponents = getUpdatesDifference(hfc.Spec.Updates, hfc.Status.Updates)
			} else {
				servicingData.TargetFirmwareComponents = hfc.Spec.Updates
			}
		}
	}

	hasChanges := fwDirty || hfsDirty || hfcDirty

	// Even if settings are clean, we need to check the result of the current servicing.
	if !hasChanges && info.host.Status.OperationalStatus != metal3api.OperationalStatusServicing && info.host.Status.ErrorType != metal3api.ServicingError {
		// If nothing is going on, return control to the power management.
		return nil
	}

	// FIXME(janders/dtantsur): this implementation may lead to a scenario where if we never actually
	// succeed before leaving this state (e.g. by deprovisioning) we lose the signal that the
	// update didn't actually happen. This is deemed an acceptable risk for the moment since it is only
	// going to impact a small subset of Firmware Settings implementations.
	if info.host.Status.OperationalStatus != metal3api.OperationalStatusServicing {
		info.host.Status.OperationalStatus = metal3api.OperationalStatusServicing
		// NOTE(dtantsur): it's very important to yield to the controller and retry before actually calling Ironic:
		// a PreprovisioningImage may be missing until we get to the registration code.
		return actionUpdate{}
	}

	provResult, started, err := prov.Service(servicingData, hasChanges,
		info.host.Status.ErrorType == metal3api.ServicingError)
	if err != nil {
		return actionError{fmt.Errorf("error servicing host: %w", err)}
	}
	if provResult.ErrorMessage != "" {
		info.host.Status.Provisioning.Firmware = nil
		if hfcDirty && hfc.Status.Updates != nil {
			hfc.Status.Updates = nil
			if err = r.Status().Update(info.ctx, hfc); err != nil {
				return actionError{fmt.Errorf("failed to update hostfirmwarecomponents status: %w", err)}
			}
		}
		result = recordActionFailure(info, metal3api.ServicingError, provResult.ErrorMessage)
		return result
	}

	dirty := clearErrorWithStatus(info.host, metal3api.OperationalStatusServicing)

	if started && fwDirty {
		info.host.Status.Provisioning.Firmware = info.host.Spec.Firmware.DeepCopy()
		dirty = true
	}

	if hfcDirty && started {
		hfcDirty, err = r.saveHostFirmwareComponents(prov, info, hfc)
		if err != nil {
			return actionError{fmt.Errorf("could not save the host firmware components: %w", err)}
		}

		if hfcDirty {
			if err := r.Status().Update(info.ctx, hfc); err != nil {
				return actionError{fmt.Errorf("failed to update hostfirmwarecomponents status: %w", err)}
			}
		}
	}

	resultAction := actionContinue{delay: provResult.RequeueAfter}
	if dirty {
		return actionUpdate{resultAction}
	} else if provResult.Dirty {
		return resultAction
	}

	// Servicing is finished at this point, clean up operational status
	if clearErrorWithStatus(info.host, metal3api.OperationalStatusOK) {
		// FIXME(janders/dtantsur): this can be racy. We should consider
		// using a generation number to decide if we start servicing or not.
		return actionUpdate{actionContinue{delay: subResourceNotReadyRetryDelay}}
	}
	return nil
}

// Check the current power status against the desired power status.
func (r *BareMetalHostReconciler) manageHostPower(prov provisioner.Provisioner, info *reconcileInfo) actionResult {
	var provResult provisioner.Result

	// Check the current status and save it before trying to update it.
	hwState, err := prov.UpdateHardwareState()
	if err != nil {
		return actionError{fmt.Errorf("failed to update the host power status: %w", err)}
	}

	if hwState.PoweredOn != nil && *hwState.PoweredOn != info.host.Status.PoweredOn {
		info.log.Info("updating power status", "discovered", *hwState.PoweredOn)
		info.host.Status.PoweredOn = *hwState.PoweredOn
		if info.host.Status.OperationalStatus == metal3api.OperationalStatusError && info.host.Status.ErrorType == metal3api.PowerManagementError {
			clearError(info.host)
		}
		return actionUpdate{}
	}

	desiredPowerOnState := info.host.Spec.Online

	provState := info.host.Status.Provisioning.State
	// Normal reboots only work in provisioned states, changing online is also possible for available hosts.
	isProvisioned := provState == metal3api.StateProvisioned || provState == metal3api.StateExternallyProvisioned
	// FIXME(janders/dtantsur) it would be preferrable to pass in state as an argument
	// however this falls outside the scope of this specific change.

	// If DataImage exists, handle attachment/detachment
	handleDataImage := isProvisioned && desiredPowerOnState && !info.host.Status.PoweredOn
	if handleDataImage {
		info.log.Info("provisioned host power on requested, handle dataImage if it exists")
		dataImageResult := r.handleDataImageActions(prov, info)
		if dataImageResult != nil {
			// attaching/detaching DataImage failed, so we will requeue
			return dataImageResult
		}
	}

	if !info.host.Status.PoweredOn {
		if _, suffixlessAnnotationExists := info.host.Annotations[metal3api.RebootAnnotationPrefix]; suffixlessAnnotationExists {
			delete(info.host.Annotations, metal3api.RebootAnnotationPrefix)

			if err = r.Update(info.ctx, info.host); err != nil {
				return actionError{fmt.Errorf("failed to remove reboot annotation from host: %w", err)}
			}

			return actionContinue{}
		}
	}

	servicingAllowed := isProvisioned && !info.host.Status.PoweredOn && desiredPowerOnState
	if servicingAllowed || info.host.Status.OperationalStatus == metal3api.OperationalStatusServicing || info.host.Status.ErrorType == metal3api.ServicingError {
		var hup *metal3api.HostUpdatePolicy
		hup, err = r.acquireHostUpdatePolicy(info)
		if err != nil {
			info.log.Info("failed setting owner reference on hostupdatepolicy")
			return actionError{fmt.Errorf("failed setting owner reference on hostUpdatePolicy: %w", err)}
		}

		result := r.doServiceIfNeeded(prov, info, hup)
		if result != nil {
			return result
		}
	}

	desiredReboot, desiredRebootMode := hasRebootAnnotation(info, !isProvisioned)

	if desiredReboot {
		desiredPowerOnState = false
	}

	// Power state needs to be monitored regularly, so if we leave
	// this function without an error we always want to requeue after
	// a delay.
	steadyStateResult := actionContinue{time.Second * 60}
	if info.host.Status.PoweredOn == desiredPowerOnState {
		return steadyStateResult
	}

	info.log.Info("power state change needed",
		"expected", desiredPowerOnState,
		"actual", info.host.Status.PoweredOn,
		"reboot mode", desiredRebootMode,
		"reboot process", desiredPowerOnState != info.host.Spec.Online)

	if desiredPowerOnState {
		provResult, err = prov.PowerOn(info.host.Status.ErrorType == metal3api.PowerManagementError)
	} else {
		if info.host.Status.ErrorCount > 0 {
			desiredRebootMode = metal3api.RebootModeHard
		}
		provResult, err = prov.PowerOff(desiredRebootMode, info.host.Status.ErrorType == metal3api.PowerManagementError)
	}
	if err != nil {
		return actionError{fmt.Errorf("failed setting owner reference on hostUpdatePolicy: %w", err)}
	}

	// If DisablePowerOff was enabled then prov.PowerOff above will have rebooted instead of powering off, in this case
	// the operation is complete (no need to power on) and any reboot annotation can be removed
	if info.host.Spec.DisablePowerOff {
		if _, suffixlessAnnotationExists := info.host.Annotations[metal3api.RebootAnnotationPrefix]; suffixlessAnnotationExists {
			delete(info.host.Annotations, metal3api.RebootAnnotationPrefix)
			if err = r.Update(info.ctx, info.host); err != nil {
				return actionError{fmt.Errorf("failed to remove reboot annotation from host: %w", err)}
			}
			return actionContinue{}
		}
	}

	if provResult.ErrorMessage != "" {
		if !desiredPowerOnState && desiredRebootMode == metal3api.RebootModeSoft &&
			info.host.Status.ErrorType != metal3api.PowerManagementError {
			provResult.ErrorMessage = clarifySoftPoweroffFailure + provResult.ErrorMessage
		}
		return recordActionFailure(info, metal3api.PowerManagementError, provResult.ErrorMessage)
	}

	if provResult.Dirty {
		info.postSaveCallbacks = append(info.postSaveCallbacks, func() {
			metricLabels := hostMetricLabels(info.request)
			if desiredPowerOnState {
				metricLabels[labelPowerOnOff] = "on"
			} else {
				metricLabels[labelPowerOnOff] = "off"
			}
			powerChangeAttempts.With(metricLabels).Inc()
		})
		result := actionContinue{provResult.RequeueAfter}
		if clearError(info.host) {
			return actionUpdate{result}
		}
		return result
	}

	// The provisioner did not have to do anything to change the power
	// state and there were no errors, so reflect the new state in the
	// host status field.
	info.host.Status.PoweredOn = info.host.Spec.Online
	info.host.Status.ErrorCount = 0
	return actionUpdate{steadyStateResult}
}

// DataImage handler for attaching/detaching image.
func (r *BareMetalHostReconciler) handleDataImageActions(prov provisioner.Provisioner, info *reconcileInfo) actionResult {
	dataImage := &metal3api.DataImage{}
	if err := r.Get(info.ctx, info.request.NamespacedName, dataImage); err != nil {
		// DataImage does not exist or it may have been deleted
		if k8serrors.IsNotFound(err) {
			info.log.Info("dataImage not found")
			return nil
		}
		// Error reading the object - requeue the request.
		return actionError{fmt.Errorf("could not load dataImage, %w", err)}
	}

	// Update reconciliation timestamp for dataImage
	dataImage.Status.LastReconciled = &metav1.Time{Time: time.Now()}

	// dataImageRetryBackoff calculated based on persistent errors while
	// attaching/detaching dataImage, every requeue when handling
	// dataImage will use this delay
	// TODO(hroyrh) : Should we fail after the error count exceeds a
	// given constant ?
	dataImageRetryBackoff := max(dataImageUpdateDelay, calculateBackoff(dataImage.Status.Error.Count))

	// Check if dataImage is attached to the node or not
	// Given that this is a synchronous call to Ironic, should we add
	// a longer wait ?
	isImageAttached, getVmediaError := prov.GetDataImageStatus()
	if getVmediaError != nil {
		info.log.Error(getVmediaError, "Error fetching Virtual Media details")

		if !errors.Is(getVmediaError, provisioner.ErrNodeIsBusy) {
			dataImage.Status.Error.Message = getVmediaError.Error()
			dataImage.Status.Error.Count++

			if err := r.Status().Update(info.ctx, dataImage); err != nil {
				return actionError{fmt.Errorf("failed to update DataImage status, %w", err)}
			}
		}

		return actionContinue{dataImageRetryBackoff}
	}

	deleteDataImage := !dataImage.DeletionTimestamp.IsZero()

	requestedURL := dataImage.Spec.URL

	// We will assume that the attached url is correct
	// In case of failed attach, this variable will be overridden
	attachedURL := dataImage.Status.AttachedImage.URL

	// If there is no attached image, we will override the attachedURL
	// so that further actions are handled accordingly
	if !isImageAttached && attachedURL != "" {
		dataImage.Status.AttachedImage.URL = ""

		// Update DataImage Status
		if err := r.Status().Update(info.ctx, dataImage); err != nil {
			return actionError{fmt.Errorf("failed to update DataImage status, %w", err)}
		}

		return actionContinue{}
	}

	if deleteDataImage {
		info.log.Info("DataImage requested for deletion")
		if isImageAttached {
			info.log.Info("Detaching DataImage as its deletion has been requested")
			err := r.detachDataImage(prov, info, dataImage)
			if err != nil {
				return actionError{fmt.Errorf("failed to detach, %w", err)}
			}

			// Requeue to give time to the DataImage Reconciler to update the
			// status. In case of failure, we will enter this section and
			// detachDataImage will be called again -> can this cause issues ?
			return actionContinue{dataImageRetryBackoff}
		}

		return nil
	}

	if requestedURL != attachedURL {
		info.log.Info("DataImage change detected")
		if attachedURL != "" {
			info.log.Info("Detaching DataImage")
			err := r.detachDataImage(prov, info, dataImage)
			if err != nil {
				return actionError{fmt.Errorf("failed to detach, %w", err)}
			}

			// Requeue to give time to the DataImage Reconciler to update the
			// status. In case of failure, we will enter this section and
			// detachDataImage will be called again -> can this cause issues ?
			return actionContinue{dataImageRetryBackoff}
		}
		if requestedURL != "" {
			info.log.Info("Attaching DataImage", "URL", requestedURL)
			err := r.attachDataImage(prov, info, dataImage)
			if err != nil {
				return actionError{fmt.Errorf("failed to attach, %w", err)}
			}

			// Requeue to give time to the DataImage Reconciler to update the
			// status. In case of failure, we will enter this section and
			// attachDataImage will be called again -> can this cause issues ?
			return actionContinue{dataImageRetryBackoff}
		}
	}

	// Clear dataImage errors as relevant error cases were already handled
	dataImage.Status.Error.Message = ""
	dataImage.Status.Error.Count = 0

	if err := r.Status().Update(info.ctx, dataImage); err != nil {
		return actionError{fmt.Errorf("failed to update DataImage status, %w", err)}
	}

	info.log.Info("Updated DataImage Status triggered after handling attachment/detachment")

	return nil
}

func ownerReferenceExists(owner metav1.Object, resource metav1.Object) bool {
	ownerReferences := resource.GetOwnerReferences()

	for _, ownRef := range ownerReferences {
		if ownRef.UID == owner.GetUID() {
			// Owner reference exists
			return true
		}
	}

	return false
}

// Attach the DataImage to the BareMetalHost.
func (r *BareMetalHostReconciler) attachDataImage(prov provisioner.Provisioner, info *reconcileInfo, dataImage *metal3api.DataImage) error {
	if err := prov.AttachDataImage(dataImage.Spec.URL); err != nil {
		info.log.Info("Error while attaching DataImage", "URL", dataImage.Spec.URL, "Error", err.Error())

		dataImage.Status.Error.Count++
		dataImage.Status.Error.Message = err.Error()
		// Error updating DataImage Status
		if errOnUpdate := r.Status().Update(info.ctx, dataImage); errOnUpdate != nil {
			return fmt.Errorf("failed to update DataImage status, %w", errOnUpdate)
		}

		return fmt.Errorf("failed to attach dataImage, %w", err)
	}

	// Update the attached URL here assuming this operation will succeed
	// In case of failure, this will be overridden in next reconcile
	// We have to do this, as there is no other way to make sure the
	// attached image url since the virtual media get api always returns
	// the same standard url
	dataImage.Status.AttachedImage.URL = dataImage.Spec.URL

	// Error updating DataImage Status
	if err := r.Status().Update(info.ctx, dataImage); err != nil {
		return fmt.Errorf("failed to update DataImage status, %w", err)
	}

	return nil
}

// Detach the DataImage from the BareMetalHost.
func (r *BareMetalHostReconciler) detachDataImage(prov provisioner.Provisioner, info *reconcileInfo, dataImage *metal3api.DataImage) error {
	if err := prov.DetachDataImage(); err != nil {
		info.log.Info("Error while detaching DataImage", "DataImage", dataImage.Name, "Error", err.Error())

		dataImage.Status.Error.Count++
		dataImage.Status.Error.Message = err.Error()
		// Error updating DataImage Status
		if errOnUpdate := r.Status().Update(info.ctx, dataImage); errOnUpdate != nil {
			return fmt.Errorf("failed to update DataImage status, %w", errOnUpdate)
		}

		return fmt.Errorf("failed to detach dataImage, %w", err)
	}

	return nil
}

// A host reaching this action handler should be provisioned or externally
// provisioned -- a state that it will stay in until the user takes further
// action. We use the Adopt() API to make sure that the provisioner is aware of
// the provisioning details. Then we monitor its power status.
func (r *BareMetalHostReconciler) actionManageSteadyState(prov provisioner.Provisioner, info *reconcileInfo) actionResult {
	provResult, err := prov.Adopt(
		provisioner.AdoptData{State: info.host.Status.Provisioning.State},
		info.host.Status.ErrorType == metal3api.ProvisionedRegistrationError)
	if err != nil {
		return actionError{err}
	}
	if provResult.ErrorMessage != "" {
		return recordActionFailure(info, metal3api.ProvisionedRegistrationError, provResult.ErrorMessage)
	}
	if provResult.Dirty {
		result := actionContinue{provResult.RequeueAfter}
		if clearError(info.host) {
			return actionUpdate{result}
		}
		return result
	}

	return r.manageHostPower(prov, info)
}

// A host reaching this action handler should be available -- a state that
// it will stay in until the user takes further action. We don't
// use Adopt() because we don't want Ironic to treat the host as
// having been provisioned. Then we monitor its power status.
func (r *BareMetalHostReconciler) actionManageAvailable(prov provisioner.Provisioner, info *reconcileInfo) actionResult {
	if info.host.NeedsProvisioning() {
		clearError(info.host)
		return actionComplete{}
	}
	return r.manageHostPower(prov, info)
}

func getHostProvisioningSettings(host *metal3api.BareMetalHost, info *reconcileInfo) (dirty bool, status *metal3api.BareMetalHostStatus, err error) {
	hostCopy := host.DeepCopy()
	dirty, err = saveHostProvisioningSettings(hostCopy, info)
	if err != nil {
		err = fmt.Errorf("could not determine the host provisioning settings: %w", err)
	}
	status = &hostCopy.Status
	return
}

// saveHostProvisioningSettings copies the values related to
// provisioning that do not trigger re-provisioning into the status
// fields of the host.
func saveHostProvisioningSettings(host *metal3api.BareMetalHost, info *reconcileInfo) (dirty bool, err error) {
	// Root device hints may change as a result of RAID
	dirty, err = updateRootDeviceHints(host, info)
	if err != nil {
		return dirty, err
	}

	// Copy RAID settings
	specRAID := host.Spec.RAID
	// If RAID configure is nil or empty, means that we need to keep the current hardware RAID configuration
	// or clear current software RAID configuration
	if specRAID == nil || reflect.DeepEqual(specRAID, &metal3api.RAIDConfig{}) {
		// Short-circuit logic when no RAID is set and no RAID is requested
		if host.Status.Provisioning.RAID != nil {
			// Set the default value of RAID configure:
			// {
			//     HardwareRAIDVolumes: nil or Status.Provisioning.RAID.HardwareRAIDVolumes(not empty),
			//     SoftwareRAIDVolume: [],
			// }
			specRAID = &metal3api.RAIDConfig{}
			if len(host.Status.Provisioning.RAID.HardwareRAIDVolumes) != 0 {
				specRAID.HardwareRAIDVolumes = host.Status.Provisioning.RAID.HardwareRAIDVolumes
			}
			specRAID.SoftwareRAIDVolumes = []metal3api.SoftwareRAIDVolume{}
		}
	}
	if !reflect.DeepEqual(host.Status.Provisioning.RAID, specRAID) {
		info.log.Info("RAID settings have changed", "old", host.Status.Provisioning.RAID, "new", specRAID)
		host.Status.Provisioning.RAID = specRAID
		dirty = true
	}

	// Copy BIOS settings
	if !reflect.DeepEqual(host.Status.Provisioning.Firmware, host.Spec.Firmware) {
		host.Status.Provisioning.Firmware = host.Spec.Firmware
		info.log.Info("Firmware settings have changed")
		dirty = true
	}

	return dirty, nil
}

func (r *BareMetalHostReconciler) saveHostFirmwareComponents(prov provisioner.Provisioner, info *reconcileInfo, hfc *metal3api.HostFirmwareComponents) (dirty bool, err error) {
	dirty = false
	if reflect.DeepEqual(hfc.Status.Updates, hfc.Spec.Updates) {
		info.log.Info("not saving hostFirmwareComponents information since is not necessary")
		return dirty, nil
	}

	info.log.Info("saving hostFirmwareComponents information", "spec updates", hfc.Spec.Updates, "status updates", hfc.Status.Updates)

	hfc.Status.Updates = make([]metal3api.FirmwareUpdate, len(hfc.Spec.Updates))
	hfc.Status.Updates = hfc.Spec.Updates

	// Retrieve new information about the firmware components stored in ironic
	components, err := prov.GetFirmwareComponents()
	if err != nil {
		info.log.Error(err, "failed to get new information for firmware components in ironic")
		return dirty, err
	}
	hfc.Status.Components = components
	dirty = true

	return dirty, nil
}

func (r *BareMetalHostReconciler) createHostFirmwareComponents(info *reconcileInfo) error {
	// Check if HostFirmwareComponents already exists
	hfc := &metal3api.HostFirmwareComponents{}
	if err := r.Get(info.ctx, info.request.NamespacedName, hfc); err != nil {
		if k8serrors.IsNotFound(err) {
			// A resource doesn't exist, create one
			hfc.ObjectMeta = metav1.ObjectMeta{
				Name:      info.host.Name,
				Namespace: info.host.Namespace}

			hfc.Spec = metal3api.HostFirmwareComponentsSpec{
				Updates: []metal3api.FirmwareUpdate{}}

			// Set bmh as owner, this makes sure the resource is deleted when bmh is deleted
			if err = controllerutil.SetOwnerReference(info.host, hfc, r.Scheme()); err != nil {
				return fmt.Errorf("could not set bmh as owner for hostFirmwareComponents: %w", err)
			}
			if err = r.Create(info.ctx, hfc); err != nil {
				return fmt.Errorf("failure creating hostFirmwareComponents resource: %w", err)
			}

			info.log.Info("created new hostFirmwareComponents resource")
			return nil
		}
		// Error reading the object
		return fmt.Errorf("could not load hostFirmwareComponents resource: %w", err)
	}
	// Necessary in case the CRD is created manually.

	if !ownerReferenceExists(info.host, hfc) {
		if err := controllerutil.SetOwnerReference(info.host, hfc, r.Scheme()); err != nil {
			return fmt.Errorf("could not set bmh as owner for hostFirmwareComponents: %w", err)
		}
		if err := r.Update(info.ctx, hfc); err != nil {
			return fmt.Errorf("failure updating hostFirmwareComponents resource: %w", err)
		}

		return nil
	}

	return nil
}

func (r *BareMetalHostReconciler) createHostFirmwareSettings(info *reconcileInfo) error {
	// Check if HostFirmwareSettings already exists
	hfs := &metal3api.HostFirmwareSettings{}
	if err := r.Get(info.ctx, info.request.NamespacedName, hfs); err != nil {
		if k8serrors.IsNotFound(err) {
			// A resource doesn't exist, create one
			hfs.ObjectMeta = metav1.ObjectMeta{
				Name:      info.host.Name,
				Namespace: info.host.Namespace}
			hfs.Status.Settings = make(metal3api.SettingsMap)
			hfs.Spec.Settings = make(metal3api.DesiredSettingsMap)

			// Set bmh as owner, this makes sure the resource is deleted when bmh is deleted
			if err = controllerutil.SetControllerReference(info.host, hfs, r.Scheme()); err != nil {
				return fmt.Errorf("could not set bmh as controller: %w", err)
			}
			if err = r.Create(info.ctx, hfs); err != nil {
				return fmt.Errorf("failure creating hostFirmwareSettings resource: %w", err)
			}

			info.log.Info("created new hostFirmwareSettings resource")
		} else {
			// Error reading the object
			return fmt.Errorf("could not load hostFirmwareSettings resource: %w", err)
		}
	}

	return nil
}

func (r *BareMetalHostReconciler) acquireHostUpdatePolicy(info *reconcileInfo) (policy *metal3api.HostUpdatePolicy, err error) {
	// NOTE(janders) the goal here is to ensure that the controller reads the hup resource and adds OwnerReference to it
	hup := &metal3api.HostUpdatePolicy{}
	if err := r.Get(info.ctx, info.request.NamespacedName, hup); err != nil {
		if k8serrors.IsNotFound(err) {
			// Request object not found, could have been deleted after
			// reconcile request.  Owned objects are automatically
			// garbage collected. For additional cleanup logic use
			// finalizers.  Return and don't requeue

			return nil, nil //nolint:nilnil
		}
		// Error reading the object
		return nil, fmt.Errorf("could not load hostUpdatePolicy resource due to %w", err)
	}
	if !ownerReferenceExists(info.host, hup) {
		if err := controllerutil.SetOwnerReference(info.host, hup, r.Scheme()); err != nil {
			return hup, fmt.Errorf("could not set bmh as owner for hostUpdatePolicy due to %w", err)
		}
		if err := r.Update(info.ctx, hup); err != nil {
			return hup, fmt.Errorf("failure updating hostUpdatePolicy resource due to %w", err)
		}

		return hup, nil
	}

	return hup, nil
}

// Get the stored firmware settings if there are valid changes.
func (r *BareMetalHostReconciler) getHostFirmwareSettings(info *reconcileInfo) (dirty bool, hfs *metal3api.HostFirmwareSettings, err error) {
	hfs = &metal3api.HostFirmwareSettings{}
	if err = r.Get(info.ctx, info.request.NamespacedName, hfs); err != nil {
		if !k8serrors.IsNotFound(err) {
			// Error reading the object
			return false, nil, fmt.Errorf("could not load host firmware settings: %w", err)
		}

		// Could not get settings, log it but don't return error as settings may not have been available at provisioner
		info.log.Info("could not get hostFirmwareSettings", "namespacename", info.request.NamespacedName)
		return false, nil, nil
	}

	// Check if there are settings in the Spec that are different than the Status
	if meta.IsStatusConditionTrue(hfs.Status.Conditions, string(metal3api.FirmwareSettingsChangeDetected)) {
		// Check if the status settings have been populated
		if len(hfs.Status.Settings) == 0 {
			return false, nil, errors.New("host firmware status settings not available")
		}

		if meta.IsStatusConditionTrue(hfs.Status.Conditions, string(metal3api.FirmwareSettingsValid)) {
			info.log.Info("hostFirmwareSettings indicating ChangeDetected", "namespacename", info.request.NamespacedName)
			return true, hfs, nil
		}

		info.log.Info("hostFirmwareSettings not valid", "namespacename", info.request.NamespacedName)
		return false, nil, nil
	}

	info.log.Info("hostFirmwareSettings no updates", "namespacename", info.request.NamespacedName)
	return false, nil, nil
}

// Get the stored firmware settings if there are valid changes.

func (r *BareMetalHostReconciler) getHostFirmwareComponents(info *reconcileInfo) (dirty bool, hfc *metal3api.HostFirmwareComponents, err error) {
	hfc = &metal3api.HostFirmwareComponents{}
	if err = r.Get(info.ctx, info.request.NamespacedName, hfc); err != nil {
		if !k8serrors.IsNotFound(err) {
			// Error reading the object
			return false, nil, fmt.Errorf("could not load host firmware components: %w", err)
		}

		// Could not get settings, log it but don't return error as settings may not have been available at provisioner
		info.log.Info("could not get hostFirmwareComponents", "namespacename", info.request.NamespacedName)
		return false, nil, nil
	}

	// Check if the condition matches the current Generation to know if the data is not out of date.
	if readyCond := meta.FindStatusCondition(hfc.Status.Conditions, string(metal3api.HostFirmwareComponentsChangeDetected)); readyCond != nil && readyCond.Status == metav1.ConditionTrue && readyCond.ObservedGeneration == hfc.Generation {
		if meta.IsStatusConditionTrue(hfc.Status.Conditions, string(metal3api.HostFirmwareComponentsValid)) {
			info.log.Info("hostFirmwareComponents indicating ChangeDetected", "namespacename", info.request.NamespacedName)
			return true, hfc, nil
		}

		info.log.Info("hostFirmwareComponents not valid", "namespacename", info.request.NamespacedName)
		return false, nil, nil
	}

	info.log.Info("hostFirmwareComponents no updates", "namespacename", info.request.NamespacedName)
	return false, nil, nil
}

func (r *BareMetalHostReconciler) saveHostStatus(ctx context.Context, host *metal3api.BareMetalHost) error {
	t := metav1.Now()
	host.Status.LastUpdated = &t

	return r.Status().Update(ctx, host)
}

func unmarshalStatusAnnotation(content []byte) (*metal3api.BareMetalHostStatus, error) {
	objStatus := &metal3api.BareMetalHostStatus{}
	if err := json.Unmarshal(content, objStatus); err != nil {
		return nil, fmt.Errorf("failed to fetch status from annotation: %w", err)
	}
	return objStatus, nil
}

// extract host from Status annotation.
func (r *BareMetalHostReconciler) getHostStatusFromAnnotation(host *metal3api.BareMetalHost) (*metal3api.BareMetalHostStatus, error) {
	annotations := host.GetAnnotations()
	content := []byte(annotations[metal3api.StatusAnnotation])
	if annotations[metal3api.StatusAnnotation] == "" {
		return nil, errors.New("status annotation not found")
	}
	objStatus, err := unmarshalStatusAnnotation(content)
	if err != nil {
		return nil, err
	}
	return objStatus, nil
}

// extract HardwareDetails from annotation if present.
func (r *BareMetalHostReconciler) getHardwareDetailsFromAnnotation(host *metal3api.BareMetalHost) (*metal3api.HardwareDetails, error) {
	annotations := host.GetAnnotations()
	if annotations[metal3api.HardwareDetailsAnnotation] == "" {
		return nil, nil //nolint:nilnil
	}
	objHardwareDetails := &metal3api.HardwareDetails{}
	decoder := json.NewDecoder(strings.NewReader(annotations[metal3api.HardwareDetailsAnnotation]))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(objHardwareDetails); err != nil {
		return nil, err
	}
	return objHardwareDetails, nil
}

func (r *BareMetalHostReconciler) setErrorCondition(ctx context.Context, request ctrl.Request, host *metal3api.BareMetalHost, errType metal3api.ErrorType, message string) (err error) {
	reqLogger := r.Log.WithValues("baremetalhost", request.NamespacedName)

	setErrorMessage(host, errType, message)

	reqLogger.Info(
		"adding error message",
		"message", message,
	)
	err = r.saveHostStatus(ctx, host)
	if err != nil {
		err = fmt.Errorf("failed to update error message: %w", err)
	}

	return
}

func (r *BareMetalHostReconciler) secretManager(ctx context.Context, log logr.Logger) secretutils.SecretManager {
	return secretutils.NewSecretManager(ctx, log, r.Client, r.APIReader)
}

// Retrieve the secret containing the credentials for talking to the BMC.
func (r *BareMetalHostReconciler) getBMCSecretAndSetOwner(ctx context.Context, request ctrl.Request, host *metal3api.BareMetalHost) (*corev1.Secret, error) {
	if host.Spec.BMC.CredentialsName == "" {
		return nil, &EmptyBMCSecretError{message: "The BMC secret reference is empty"}
	}

	reqLogger := r.Log.WithValues("baremetalhost", request.NamespacedName)
	secretManager := r.secretManager(ctx, reqLogger)

	bmcCredsSecret, err := secretManager.AcquireSecret(host.CredentialsKey(), host, host.Status.Provisioning.State != metal3api.StateDeleting)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, &ResolveBMCSecretRefError{message: fmt.Sprintf("The BMC secret %s does not exist", host.CredentialsKey())}
		}
		return nil, err
	}

	return bmcCredsSecret, nil
}

// getImageAuthSecret validates and extracts the OCI registry credentials for the image.
// It returns the base64-encoded credentials in the format expected by Ironic, or an empty
// string if no auth secret is configured.
func (r *BareMetalHostReconciler) getImageAuthSecret(ctx context.Context, _ ctrl.Request, host *metal3api.BareMetalHost, image *metal3api.Image) (string, error) {
	// Only process OCI images
	if image == nil || !strings.HasPrefix(image.URL, "oci://") {
		r.setImageAuthConditions(host, nil, false)
		return "", nil
	}

	// Check for per-host auth secret
	if image.AuthSecretName == nil || *image.AuthSecretName == "" {
		// No auth secret configured - set conditions accordingly
		r.setImageAuthConditions(host, nil, false)
		return "", nil
	}

	// Use the validator to validate and extract credentials
	validator := imageauthvalidator.New(r.Client, r.Recorder)
	result, err := validator.Validate(ctx, host)
	if err != nil {
		return "", fmt.Errorf("failed to validate auth secret: %w", err)
	}

	// Set ImageAuthValid condition based on validation result
	r.setImageAuthConditions(host, result, result.Valid && result.Credentials != "")

	// If validation failed, return the error message
	if !result.Valid {
		return "", fmt.Errorf("auth secret validation failed: %s", result.Message)
	}

	// Return the extracted credentials
	return result.Credentials, nil
}

// setImageAuthConditions sets the ImageAuthValid and ImageAuthInUse conditions on the host.
func (r *BareMetalHostReconciler) setImageAuthConditions(host *metal3api.BareMetalHost, result *imageauthvalidator.Result, credentialsInjected bool) {
	now := metav1.Now()

	// Set ImageAuthValid condition
	if result == nil {
		// No auth secret referenced
		meta.SetStatusCondition(&host.Status.Conditions, metav1.Condition{
			Type:               imageauthvalidator.ConditionImageAuthValid,
			Status:             metav1.ConditionFalse,
			Reason:             imageauthvalidator.ReasonNotRequired,
			Message:            "No auth secret referenced",
			LastTransitionTime: now,
			ObservedGeneration: host.Generation,
		})
	} else if result.Valid {
		meta.SetStatusCondition(&host.Status.Conditions, metav1.Condition{
			Type:               imageauthvalidator.ConditionImageAuthValid,
			Status:             metav1.ConditionTrue,
			Reason:             result.Reason,
			Message:            result.Message,
			LastTransitionTime: now,
			ObservedGeneration: host.Generation,
		})
	} else {
		meta.SetStatusCondition(&host.Status.Conditions, metav1.Condition{
			Type:               imageauthvalidator.ConditionImageAuthValid,
			Status:             metav1.ConditionFalse,
			Reason:             result.Reason,
			Message:            result.Message,
			LastTransitionTime: now,
			ObservedGeneration: host.Generation,
		})
	}

	// Set ImageAuthInUse condition
	if credentialsInjected {
		meta.SetStatusCondition(&host.Status.Conditions, metav1.Condition{
			Type:               imageauthvalidator.ConditionImageAuthInUse,
			Status:             metav1.ConditionTrue,
			Reason:             imageauthvalidator.ReasonCredentialsInjected,
			Message:            "OCI registry credentials injected into provisioning",
			LastTransitionTime: now,
			ObservedGeneration: host.Generation,
		})
	} else {
		reason := imageauthvalidator.ReasonNoOCIImage
		message := "No OCI image or auth secret in use"
		if result != nil && !result.OCIRelevant {
			reason = imageauthvalidator.ReasonNoOCIImage
			message = "Image is not an OCI image"
		}
		meta.SetStatusCondition(&host.Status.Conditions, metav1.Condition{
			Type:               imageauthvalidator.ConditionImageAuthInUse,
			Status:             metav1.ConditionFalse,
			Reason:             reason,
			Message:            message,
			LastTransitionTime: now,
			ObservedGeneration: host.Generation,
		})
	}
}

func credentialsFromSecret(bmcCredsSecret *corev1.Secret) *bmc.Credentials {
	// We trim surrounding whitespace because those characters are
	// unlikely to be part of the username or password and it is
	// common for users to encode the values with a command like
	//
	//     echo "my-password" | base64
	//
	// which introduces a trailing newline.
	return &bmc.Credentials{
		Username: strings.TrimSpace(string(bmcCredsSecret.Data["username"])),
		Password: strings.TrimSpace(string(bmcCredsSecret.Data["password"])),
	}
}

// Make sure the credentials for the management controller look
// right and manufacture bmc.Credentials.  This does not actually try
// to use the credentials.
func (r *BareMetalHostReconciler) buildAndValidateBMCCredentials(ctx context.Context, request ctrl.Request, host *metal3api.BareMetalHost) (bmcCreds *bmc.Credentials, bmcCredsSecret *corev1.Secret, err error) {
	// Retrieve the BMC secret from Kubernetes for this host
	bmcCredsSecret, err = r.getBMCSecretAndSetOwner(ctx, request, host)
	if err != nil {
		return nil, nil, err
	}

	// Check for a "discovered" host vs. one that we have all the info for
	// and find empty Address or CredentialsName fields
	if host.Spec.BMC.Address == "" {
		return nil, nil, &EmptyBMCAddressError{message: "Missing BMC connection detail 'Address'"}
	}

	bmcCreds = credentialsFromSecret(bmcCredsSecret)

	// Verify that the secret contains the expected info.
	err = bmcCreds.Validate()
	if err != nil {
		return nil, bmcCredsSecret, err
	}

	return bmcCreds, bmcCredsSecret, nil
}

func (r *BareMetalHostReconciler) publishEvent(ctx context.Context, request ctrl.Request, event corev1.Event) {
	reqLogger := r.Log.WithValues("baremetalhost", request.NamespacedName)
	reqLogger.Info("publishing event", "reason", event.Reason, "message", event.Message)
	err := r.Create(ctx, &event)
	if err != nil {
		reqLogger.Info("failed to record event, ignoring",
			"reason", event.Reason, "message", event.Message, "error", err)
	}
}

func (r *BareMetalHostReconciler) hostHasStatus(host *metal3api.BareMetalHost) bool {
	return !host.Status.LastUpdated.IsZero()
}

func hostHasFinalizer(host *metal3api.BareMetalHost) bool {
	return utils.StringInList(host.Finalizers, metal3api.BareMetalHostFinalizer)
}

func (r *BareMetalHostReconciler) updateEventHandler(e event.UpdateEvent) bool {
	_, oldOK := e.ObjectOld.(*metal3api.BareMetalHost)
	_, newOK := e.ObjectNew.(*metal3api.BareMetalHost)
	if !(oldOK && newOK) {
		// The thing that changed wasn't a host, so we
		// need to assume that we must update. This
		// happens when, for example, an owned Secret
		// changes.
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
func (r *BareMetalHostReconciler) SetupWithManager(mgr ctrl.Manager, preprovImgEnable bool, maxConcurrentReconcile int) error {
	r.Recorder = mgr.GetEventRecorderFor("baremetalhost-controller")
	controller := ctrl.NewControllerManagedBy(mgr).
		For(&metal3api.BareMetalHost{}).
		WithEventFilter(
			predicate.Funcs{
				UpdateFunc: r.updateEventHandler,
			}).
		WithOptions(controller.Options{MaxConcurrentReconciles: maxConcurrentReconcile}).
		Owns(&corev1.Secret{}, builder.MatchEveryOwner)

	if preprovImgEnable {
		// We use SetControllerReference() to set the owner reference, so no
		// need to pass MatchEveryOwner
		controller.Owns(&metal3api.PreprovisioningImage{})
	}

	return controller.Complete(r)
}

func (r *BareMetalHostReconciler) reconcileHostData(ctx context.Context, host *metal3api.BareMetalHost, request ctrl.Request) (result ctrl.Result, err error) {
	reqLogger := r.Log.WithValues("baremetalhost", request.NamespacedName)

	// Fetch the HardwareData
	hardwareData := &metal3api.HardwareData{}
	hardwareDataKey := client.ObjectKey{
		Name:      host.Name,
		Namespace: host.Namespace,
	}
	err = r.Get(ctx, hardwareDataKey, hardwareData)
	if err != nil && !k8serrors.IsNotFound(err) {
		reqLogger.Error(err, "failed to find hardwareData")
	}

	// Host is being deleted, so we delete the finalizer from the hardwareData to allow its deletion.
	if !host.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(hardwareData, hardwareDataFinalizer) {
			controllerutil.RemoveFinalizer(hardwareData, hardwareDataFinalizer)
			reqLogger.Info("removing finalizer from hardwareData")
			if err := r.Update(ctx, hardwareData); err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to remove hardwareData finalizer: %w", err)
			}
		}
		reqLogger.Info("hardwareData is ready to be deleted")
	}

	// Check if Status is empty and status annotation is present
	// Manually restore data.
	if !r.hostHasStatus(host) {
		objStatus, err := r.getHostStatusFromAnnotation(host)

		if err == nil && objStatus != nil {
			reqLogger.Info("reconstructing Status from hardwareData and annotation")
			// hardwareData takes predence over statusAnnotation data
			if hardwareData.Spec.HardwareDetails != nil && objStatus.HardwareDetails != hardwareData.Spec.HardwareDetails {
				objStatus.HardwareDetails = hardwareData.Spec.HardwareDetails
			}

			host.Status = *objStatus
			if host.Status.LastUpdated.IsZero() {
				// Ensure the LastUpdated timestamp is set to avoid
				// infinite loops if the annotation only contained
				// part of the status information.
				t := metav1.Now()
				host.Status.LastUpdated = &t
			}
			errStatus := r.Status().Update(ctx, host)
			if errStatus != nil {
				return ctrl.Result{}, fmt.Errorf("could not restore status from annotation: %w", errStatus)
			}
			return ctrl.Result{Requeue: true}, nil
		}
		reqLogger.V(1).Info("no status cache found")
	}
	// The status annotation is unneeded, as the status subresource is
	// already present. The annotation data will get outdated, so remove it.
	annotations := host.GetAnnotations()
	if _, present := annotations[metal3api.StatusAnnotation]; present {
		delete(annotations, metal3api.StatusAnnotation)
		errStatus := r.Update(ctx, host)
		if errStatus != nil {
			return ctrl.Result{}, fmt.Errorf("could not delete status annotation: %w", errStatus)
		}
		reqLogger.Info("deleted status annotation")
		return ctrl.Result{Requeue: true}, nil
	}

	if host.Spec.Architecture == "" && hardwareData != nil && hardwareData.Spec.HardwareDetails != nil && hardwareData.Spec.HardwareDetails.CPU.Arch != "" {
		newArch := hardwareData.Spec.HardwareDetails.CPU.Arch
		reqLogger.Info("updating architecture", "Architecture", newArch)
		host.Spec.Architecture = newArch
		if err := r.Client.Update(ctx, host); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to update architecture: %w", err)
		}
		return ctrl.Result{Requeue: true}, nil
	}
	return ctrl.Result{}, nil
}
