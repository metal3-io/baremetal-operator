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
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	metal3v1alpha1 "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	"github.com/metal3-io/baremetal-operator/pkg/bmc"
	"github.com/metal3-io/baremetal-operator/pkg/hardware"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner"
	"github.com/metal3-io/baremetal-operator/pkg/utils"
)

const (
	hostErrorRetryDelay           = time.Second * 10
	unmanagedRetryDelay           = time.Minute * 10
	provisionerNotReadyRetryDelay = time.Second * 30
	rebootAnnotationPrefix        = "reboot.metal3.io"
)

func init() {
}

// BareMetalHostReconciler reconciles a BareMetalHost object
type BareMetalHostReconciler struct {
	client.Client
	Log                logr.Logger
	Scheme             *runtime.Scheme
	ProvisionerFactory provisioner.Factory
}

// Instead of passing a zillion arguments to the action of a phase,
// hold them in a context
type reconcileInfo struct {
	log               logr.Logger
	host              *metal3v1alpha1.BareMetalHost
	request           ctrl.Request
	bmcCredsSecret    *corev1.Secret
	events            []corev1.Event
	errorMessage      string
	postSaveCallbacks []func()
}

// match the provisioner.EventPublisher interface
func (info *reconcileInfo) publishEvent(reason, message string) {
	info.events = append(info.events, info.host.NewEvent(reason, message))
}

// +kubebuilder:rbac:groups=metal3.io,resources=baremetalhosts,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=metal3.io,resources=baremetalhosts/status,verbs=get;update;patch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;update
// +kubebuilder:rbac:groups="",resources=events,verbs=get;list;watch;create;update;patch

// Reconcile handles changes to BareMetalHost resources
func (r *BareMetalHostReconciler) Reconcile(request ctrl.Request) (result ctrl.Result, err error) {

	reconcileCounters.With(hostMetricLabels(request)).Inc()
	defer func() {
		if err != nil {
			reconcileErrorCounter.Inc()
		}
	}()

	reqLogger := r.Log.WithValues("baremetalhost", request.NamespacedName)

	// Fetch the BareMetalHost
	host := &metal3v1alpha1.BareMetalHost{}
	err = r.Get(context.TODO(), request.NamespacedName, host)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			// Request object not found, could have been deleted after
			// reconcile request.  Owned objects are automatically
			// garbage collected. For additional cleanup logic use
			// finalizers.  Return and don't requeue
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return ctrl.Result{}, errors.Wrap(err, "could not load host data")
	}

	// If the reconciliation is paused, requeue
	annotations := host.GetAnnotations()
	if annotations != nil {
		if _, ok := annotations[metal3v1alpha1.PausedAnnotation]; ok {
			reqLogger.Info("host is paused, no work to do")
			return ctrl.Result{Requeue: false}, nil
		}
	}

	// Check if Status is empty and status annotation is present
	// Manually restore data.
	if !r.hostHasStatus(host) {
		reqLogger.Info("Fetching Status from Annotation")
		objStatus, err := r.getHostStatusFromAnnotation(host)
		if err == nil && objStatus != nil {
			host.Status = *objStatus
			if host.Status.LastUpdated.IsZero() {
				// Ensure the LastUpdated timestamp in set to avoid
				// infinite loops if the annotation only contained
				// part of the status information.
				t := metav1.Now()
				host.Status.LastUpdated = &t
			}
			errStatus := r.Status().Update(context.TODO(), host)
			if errStatus != nil {
				return ctrl.Result{}, errors.Wrap(errStatus, "Could not restore status from annotation")
			}
			return ctrl.Result{Requeue: true}, nil
		}
		reqLogger.Info("No status cache found")
	} else {
		// The status annotation is unneeded, as the status subresource is
		// already present. The annotation data will get outdated, so remove it.
		if _, present := annotations[metal3v1alpha1.StatusAnnotation]; present {
			delete(annotations, metal3v1alpha1.StatusAnnotation)
			errStatus := r.Update(context.TODO(), host)
			if errStatus != nil {
				return ctrl.Result{}, errors.Wrap(errStatus, "Could not delete status annotation")
			}
			return ctrl.Result{Requeue: true}, nil
		}
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
			"newValue", metal3v1alpha1.BareMetalHostFinalizer,
		)
		host.Finalizers = append(host.Finalizers,
			metal3v1alpha1.BareMetalHostFinalizer)
		err := r.Update(context.TODO(), host)
		if err != nil {
			return ctrl.Result{}, errors.Wrap(err, "failed to add finalizer")
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
	case metal3v1alpha1.StateNone, metal3v1alpha1.StateUnmanaged:
		bmcCreds = &bmc.Credentials{}
	default:
		bmcCreds, bmcCredsSecret, err = r.buildAndValidateBMCCredentials(request, host)
		if err != nil || bmcCreds == nil {
			if !host.DeletionTimestamp.IsZero() {
				// If we are in the process of deletion, try with empty credentials
				bmcCreds = &bmc.Credentials{}
				bmcCredsSecret = &corev1.Secret{}
			} else {
				return r.credentialsErrorResult(err, request, host)
			}
		} else {
			haveCreds = true
		}
	}

	initialState := host.Status.Provisioning.State
	info := &reconcileInfo{
		log:            reqLogger.WithValues("provisioningState", initialState),
		host:           host,
		request:        request,
		bmcCredsSecret: bmcCredsSecret,
	}
	prov, err := r.ProvisionerFactory(*host, *bmcCreds, info.publishEvent)
	if err != nil {
		return ctrl.Result{}, errors.Wrap(err, "failed to create provisioner")
	}

	ready, err := prov.IsReady()

	if err != nil {
		return ctrl.Result{}, errors.Wrap(err, "failed to check services availability")
	}
	if !ready {
		reqLogger.Info("provisioner is not ready", "RequeueAfter:", provisionerNotReadyRetryDelay)
		return ctrl.Result{Requeue: true, RequeueAfter: provisionerNotReadyRetryDelay}, nil
	}

	stateMachine := newHostStateMachine(host, r, prov, haveCreds)
	actResult := stateMachine.ReconcileState(info)
	result, err = actResult.Result()

	if err != nil {
		err = errors.Wrap(err, fmt.Sprintf("action %q failed", initialState))
		return
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
		err = r.saveHostStatus(host)
		if err != nil {
			return ctrl.Result{}, errors.Wrap(err,
				fmt.Sprintf("failed to save host status after %q", initialState))
		}

		for _, cb := range info.postSaveCallbacks {
			cb()
		}
	}

	for _, e := range info.events {
		r.publishEvent(request, e)
	}

	logResult(info, result)

	return
}

func logResult(info *reconcileInfo, result ctrl.Result) {
	if result.Requeue || result.RequeueAfter != 0 ||
		!utils.StringInList(info.host.Finalizers,
			metal3v1alpha1.BareMetalHostFinalizer) {
		info.log.Info("done",
			"requeue", result.Requeue,
			"after", result.RequeueAfter)
	} else {
		info.log.Info("stopping on host error",
			"message", info.host.Status.ErrorMessage)
	}
}

func recordActionFailure(info *reconcileInfo, errorType metal3v1alpha1.ErrorType, errorMessage string) actionFailed {

	setErrorMessage(info.host, errorType, errorMessage)

	eventType := map[metal3v1alpha1.ErrorType]string{
		metal3v1alpha1.RegistrationError:    "RegistrationError",
		metal3v1alpha1.InspectionError:      "InspectionError",
		metal3v1alpha1.ProvisioningError:    "ProvisioningError",
		metal3v1alpha1.PowerManagementError: "PowerManagementError",
	}[errorType]

	counter := actionFailureCounters.WithLabelValues(eventType)
	info.postSaveCallbacks = append(info.postSaveCallbacks, counter.Inc)

	info.publishEvent(eventType, errorMessage)

	return actionFailed{dirty: true, ErrorType: errorType, errorCount: info.host.Status.ErrorCount}
}

func (r *BareMetalHostReconciler) credentialsErrorResult(err error, request ctrl.Request, host *metal3v1alpha1.BareMetalHost) (ctrl.Result, error) {
	switch err.(type) {
	// In the event a credential secret is defined, but we cannot find it
	// we requeue the host as we will not know if they create the secret
	// at some point in the future.
	case *ResolveBMCSecretRefError:
		credentialsMissing.Inc()
		saveErr := r.setErrorCondition(request, host, metal3v1alpha1.RegistrationError, err.Error())
		if saveErr != nil {
			return ctrl.Result{Requeue: true}, saveErr
		}
		r.publishEvent(request, host.NewEvent("BMCCredentialError", err.Error()))

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
		saveErr := r.setErrorCondition(request, host, metal3v1alpha1.RegistrationError, err.Error())
		if saveErr != nil {
			return ctrl.Result{Requeue: true}, saveErr
		}
		// Only publish the event if we do not have an error
		// after saving so that we only publish one time.
		r.publishEvent(request, host.NewEvent("BMCCredentialError", err.Error()))
		return ctrl.Result{}, nil
	default:
		unhandledCredentialsError.Inc()
		return ctrl.Result{}, errors.Wrap(err, "An unhandled failure occurred with the BMC secret")
	}
}

// hasRebootAnnotation checks for existence of reboot annotations and returns true if at least one exist
func hasRebootAnnotation(host *metal3v1alpha1.BareMetalHost) bool {
	for annotation := range host.Annotations {
		if isRebootAnnotation(annotation) {
			return true
		}
	}
	return false
}

// isRebootAnnotation returns true if the provided annotation is a reboot annotation (either suffixed or not)
func isRebootAnnotation(annotation string) bool {
	return strings.HasPrefix(annotation, rebootAnnotationPrefix+"/") || annotation == rebootAnnotationPrefix
}

// clearRebootAnnotations deletes all reboot annotations exist on the provided host
func clearRebootAnnotations(host *metal3v1alpha1.BareMetalHost) (dirty bool) {
	for annotation := range host.Annotations {
		if isRebootAnnotation(annotation) {
			delete(host.Annotations, annotation)
			dirty = true
		}
	}

	return
}

// clearError removes any existing error message.
func clearError(host *metal3v1alpha1.BareMetalHost) (dirty bool) {
	dirty = host.SetOperationalStatus(metal3v1alpha1.OperationalStatusOK)
	var emptyErrType metal3v1alpha1.ErrorType = ""
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

// setErrorMessage updates the ErrorMessage in the host Status struct
// and increases the ErrorCount
func setErrorMessage(host *metal3v1alpha1.BareMetalHost, errType metal3v1alpha1.ErrorType, message string) {
	host.Status.OperationalStatus = metal3v1alpha1.OperationalStatusError
	host.Status.ErrorType = errType
	host.Status.ErrorMessage = message
	host.Status.ErrorCount++
}

// Manage deletion of the host
func (r *BareMetalHostReconciler) actionDeleting(prov provisioner.Provisioner, info *reconcileInfo) actionResult {
	info.log.Info(
		"marked to be deleted",
		"timestamp", info.host.DeletionTimestamp,
	)

	// no-op if finalizer has been removed.
	if !utils.StringInList(info.host.Finalizers, metal3v1alpha1.BareMetalHostFinalizer) {
		info.log.Info("ready to be deleted")
		return deleteComplete{}
	}

	provResult, err := prov.Delete()
	if err != nil {
		return actionError{errors.Wrap(err, "failed to delete")}
	}
	if provResult.Dirty {
		return actionContinue{provResult.RequeueAfter}
	}

	// Remove finalizer to allow deletion
	info.host.Finalizers = utils.FilterStringFromList(
		info.host.Finalizers, metal3v1alpha1.BareMetalHostFinalizer)
	info.log.Info("cleanup is complete, removed finalizer",
		"remaining", info.host.Finalizers)
	if err := r.Update(context.Background(), info.host); err != nil {
		return actionError{errors.Wrap(err, "failed to remove finalizer")}
	}

	return deleteComplete{}
}

func (r *BareMetalHostReconciler) actionUnmanaged(prov provisioner.Provisioner, info *reconcileInfo) actionResult {
	if info.host.HasBMCDetails() {
		return actionComplete{}
	}
	return actionContinue{unmanagedRetryDelay}
}

// Test the credentials by connecting to the management controller.
func (r *BareMetalHostReconciler) registerHost(prov provisioner.Provisioner, info *reconcileInfo) actionResult {
	info.log.Info("registering and validating access to management controller",
		"credentials", info.host.Status.TriedCredentials)
	dirty := false

	credsChanged := !info.host.Status.TriedCredentials.Match(*info.bmcCredsSecret)
	if credsChanged {
		info.log.Info("new credentials")
		info.host.UpdateTriedCredentials(*info.bmcCredsSecret)
		info.postSaveCallbacks = append(info.postSaveCallbacks, updatedCredentials.Inc)
		dirty = true
	}

	provResult, provID, err := prov.ValidateManagementAccess(credsChanged, info.host.Status.ErrorType == metal3v1alpha1.RegistrationError)
	if err != nil {
		noManagementAccess.Inc()
		return actionError{errors.Wrap(err, "failed to validate BMC access")}
	}

	if provResult.ErrorMessage != "" {
		return recordActionFailure(info, metal3v1alpha1.RegistrationError, provResult.ErrorMessage)
	}

	if provID != "" && info.host.Status.Provisioning.ID != provID {
		info.log.Info("setting provisioning id", "ID", provID)
		info.host.Status.Provisioning.ID = provID
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

	// Reaching this point means the credentials are valid and worked,
	// so clear any previous error and record the success in the
	// status block.
	if !info.host.Status.GoodCredentials.Match(*info.bmcCredsSecret) {
		info.log.Info("updating credentials success status fields")
		info.host.UpdateGoodCredentials(*info.bmcCredsSecret)
		info.publishEvent("BMCAccessValidated", "Verified access to BMC")
		dirty = true
	} else {
		info.log.Info("verified access to the BMC")
	}
	if clearError(info.host) {
		info.log.Info("clearing previous error message")
		dirty = true
	}

	if dirty {
		return actionComplete{}
	}
	return nil
}

// Ensure we have the information about the hardware on the host.
func (r *BareMetalHostReconciler) actionInspecting(prov provisioner.Provisioner, info *reconcileInfo) actionResult {
	info.log.Info("inspecting hardware")

	provResult, details, err := prov.InspectHardware(info.host.Status.ErrorType == metal3v1alpha1.InspectionError)
	if err != nil {
		return actionError{errors.Wrap(err, "hardware inspection failed")}
	}

	if provResult.ErrorMessage != "" {
		return recordActionFailure(info, metal3v1alpha1.InspectionError, provResult.ErrorMessage)
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
	return actionComplete{}
}

func (r *BareMetalHostReconciler) actionMatchProfile(prov provisioner.Provisioner, info *reconcileInfo) actionResult {

	var hardwareProfile string

	info.log.Info("determining hardware profile")

	// Start by looking for an override value from the user
	if info.host.Spec.HardwareProfile != "" {
		info.log.Info("using spec value for profile name",
			"name", info.host.Spec.HardwareProfile)
		hardwareProfile = info.host.Spec.HardwareProfile
		_, err := hardware.GetProfile(hardwareProfile)
		if err != nil {
			info.log.Info("invalid hardware profile", "profile", hardwareProfile)
			return actionError{err}
		}
	}

	// Now do a bit of matching.
	//
	// FIXME(dhellmann): Insert more robust logic to match
	// hardware profiles here.
	if hardwareProfile == "" {
		if strings.HasPrefix(info.host.Spec.BMC.Address, "libvirt") {
			hardwareProfile = "libvirt"
			info.log.Info("determining from BMC address", "name", hardwareProfile)
		}
	}

	// Now default to a value just in case there is no match
	if hardwareProfile == "" {
		hardwareProfile = hardware.DefaultProfileName
		info.log.Info("using the default", "name", hardwareProfile)
	}

	if info.host.SetHardwareProfile(hardwareProfile) {
		info.log.Info("updating hardware profile", "profile", hardwareProfile)
		info.publishEvent("ProfileSet", fmt.Sprintf("Hardware profile set: %s", hardwareProfile))
	}

	clearError(info.host)
	return actionComplete{}
}

// Start/continue provisioning if we need to.
func (r *BareMetalHostReconciler) actionProvisioning(prov provisioner.Provisioner, info *reconcileInfo) actionResult {
	hostConf := &hostConfigData{
		host:   info.host,
		log:    info.log.WithName("host_config_data"),
		client: r,
	}
	info.log.Info("provisioning")

	if clearRebootAnnotations(info.host) {
		if err := r.Update(context.TODO(), info.host); err != nil {
			return actionError{errors.Wrap(err, "failed to remove reboot annotations from host")}
		}
		return actionContinue{}
	}

	provResult, err := prov.Provision(hostConf)
	if err != nil {
		return actionError{errors.Wrap(err, "failed to provision")}
	}

	if provResult.ErrorMessage != "" {
		info.log.Info("handling provisioning error in controller")
		return recordActionFailure(info, metal3v1alpha1.ProvisioningError, provResult.ErrorMessage)
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
	if info.host.Status.Provisioning.Image != *(info.host.Spec.Image) {
		info.log.Info("updating deployed image in status")
		info.host.Status.Provisioning.Image = *(info.host.Spec.Image)
	}

	// After provisioning we always requeue to ensure we enter the
	// "provisioned" state and start monitoring power status.
	return actionComplete{}
}

// clearHostProvisioningSettings removes the values related to
// provisioning that do not trigger re-provisioning from the status
// fields of a host.
func clearHostProvisioningSettings(host *metal3v1alpha1.BareMetalHost) {
	host.Status.Provisioning.RootDeviceHints = nil
}

func (r *BareMetalHostReconciler) actionDeprovisioning(prov provisioner.Provisioner, info *reconcileInfo) actionResult {
	if info.host.Status.Provisioning.Image.URL != "" {
		// Adopt the host in case it has been re-registered during the
		// deprovisioning process before it completed
		provResult, err := prov.Adopt(info.host.Status.ErrorType == metal3v1alpha1.RegistrationError)
		if err != nil {
			return actionError{err}
		}
		if provResult.ErrorMessage != "" {
			return recordActionFailure(info, metal3v1alpha1.RegistrationError, provResult.ErrorMessage)
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

	provResult, err := prov.Deprovision(info.host.Status.ErrorType == metal3v1alpha1.ProvisioningError)
	if err != nil {
		return actionError{errors.Wrap(err, "failed to deprovision")}
	}

	if provResult.ErrorMessage != "" {
		return recordActionFailure(info, metal3v1alpha1.ProvisioningError, provResult.ErrorMessage)
	}

	if provResult.Dirty {
		result := actionContinue{provResult.RequeueAfter}
		if clearError(info.host) {
			return actionUpdate{result}
		}
		return result
	}

	if clearRebootAnnotations(info.host) {
		if err = r.Update(context.TODO(), info.host); err != nil {
			return actionError{errors.Wrap(err, "failed to remove reboot annotations from host")}
		}
		return actionContinue{}
	}

	// After the provisioner is done, clear the provisioning settings
	// so we transition to the next state.
	info.host.Status.Provisioning.Image = metal3v1alpha1.Image{}
	clearHostProvisioningSettings(info.host)

	return actionComplete{}
}

// Check the current power status against the desired power status.
func (r *BareMetalHostReconciler) manageHostPower(prov provisioner.Provisioner, info *reconcileInfo) actionResult {
	var provResult provisioner.Result

	// Check the current status and save it before trying to update it.
	hwState, err := prov.UpdateHardwareState()
	if err != nil {
		return actionError{errors.Wrap(err, "failed to update the host power status")}
	}

	if hwState.PoweredOn != nil && *hwState.PoweredOn != info.host.Status.PoweredOn {
		info.log.Info("updating power status", "discovered", *hwState.PoweredOn)
		info.host.Status.PoweredOn = *hwState.PoweredOn
		clearError(info.host)
		return actionUpdate{}
	}

	desiredPowerOnState := info.host.Spec.Online

	if !info.host.Status.PoweredOn {
		if _, suffixlessAnnotationExists := info.host.Annotations[rebootAnnotationPrefix]; suffixlessAnnotationExists {
			delete(info.host.Annotations, rebootAnnotationPrefix)

			if err = r.Update(context.TODO(), info.host); err != nil {
				return actionError{errors.Wrap(err, "failed to remove reboot annotation from host")}
			}

			return actionContinue{}
		}
	}

	provState := info.host.Status.Provisioning.State
	isProvisioned := provState == metal3v1alpha1.StateProvisioned || provState == metal3v1alpha1.StateExternallyProvisioned
	if hasRebootAnnotation(info.host) && isProvisioned {
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
		"reboot process", desiredPowerOnState != info.host.Spec.Online)

	if desiredPowerOnState {
		provResult, err = prov.PowerOn()
	} else {
		provResult, err = prov.PowerOff()
	}
	if err != nil {
		return actionError{errors.Wrap(err, "failed to manage power state of host")}
	}

	if provResult.ErrorMessage != "" {
		return recordActionFailure(info, metal3v1alpha1.PowerManagementError, provResult.ErrorMessage)
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

// A host reaching this action handler should be provisioned or externally
// provisioned -- a state that it will stay in until the user takes further
// action. We use the Adopt() API to make sure that the provisioner is aware of
// the provisioning details. Then we monitor its power status.
func (r *BareMetalHostReconciler) actionManageSteadyState(prov provisioner.Provisioner, info *reconcileInfo) actionResult {
	provResult, err := prov.Adopt(info.host.Status.ErrorType == metal3v1alpha1.RegistrationError)
	if err != nil {
		return actionError{err}
	}
	if provResult.ErrorMessage != "" {
		return recordActionFailure(info, metal3v1alpha1.RegistrationError, provResult.ErrorMessage)
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

// A host reaching this action handler should be ready -- a state that
// it will stay in until the user takes further action. We don't
// use Adopt() because we don't want Ironic to treat the host as
// having been provisioned. Then we monitor its power status.
func (r *BareMetalHostReconciler) actionManageReady(prov provisioner.Provisioner, info *reconcileInfo) actionResult {
	if info.host.NeedsProvisioning() {
		// Ensure the provisioning settings we're going to use are stored.
		dirty, err := saveHostProvisioningSettings(info.host)
		if err != nil {
			return actionError{errors.Wrap(err, "Could not save the host provisioning settings")}
		}
		if dirty {
			info.log.Info("updating host provisioning settings")
		}
		clearError(info.host)
		return actionComplete{}
	}
	return r.manageHostPower(prov, info)
}

// saveHostProvisioningSettings copies the values related to
// provisioning that do not trigger re-provisioning into the status
// fields of the host.
func saveHostProvisioningSettings(host *metal3v1alpha1.BareMetalHost) (dirty bool, err error) {

	// Ensure the root device hints we're going to use are stored.
	//
	// If the user has provided explicit root device hints, they take
	// precedence. Otherwise use the values from the hardware profile.
	hintSource := host.Spec.RootDeviceHints
	if hintSource == nil {
		hwProf, err := hardware.GetProfile(host.HardwareProfile())
		if err != nil {
			return false, errors.Wrap(err, "Could not update root device hints")
		}
		hintSource = &hwProf.RootDeviceHints
	}
	if (hintSource != nil && host.Status.Provisioning.RootDeviceHints == nil) || *hintSource != *(host.Status.Provisioning.RootDeviceHints) {
		host.Status.Provisioning.RootDeviceHints = hintSource
		dirty = true
	}

	return
}

func (r *BareMetalHostReconciler) saveHostStatus(host *metal3v1alpha1.BareMetalHost) error {
	t := metav1.Now()
	host.Status.LastUpdated = &t

	return r.Status().Update(context.TODO(), host)
}

func unmarshalStatusAnnotation(content []byte) (*metal3v1alpha1.BareMetalHostStatus, error) {
	objStatus := &metal3v1alpha1.BareMetalHostStatus{}
	if err := json.Unmarshal(content, objStatus); err != nil {
		return nil, errors.Wrap(err, "Failed to fetch Status from annotation")
	}
	return objStatus, nil
}

// extract host from Status annotation
func (r *BareMetalHostReconciler) getHostStatusFromAnnotation(host *metal3v1alpha1.BareMetalHost) (*metal3v1alpha1.BareMetalHostStatus, error) {
	annotations := host.GetAnnotations()
	content := []byte(annotations[metal3v1alpha1.StatusAnnotation])
	if annotations[metal3v1alpha1.StatusAnnotation] == "" {
		return nil, nil
	}
	objStatus, err := unmarshalStatusAnnotation(content)
	if err != nil {
		return nil, err
	}
	return objStatus, nil
}

func (r *BareMetalHostReconciler) setErrorCondition(request ctrl.Request, host *metal3v1alpha1.BareMetalHost, errType metal3v1alpha1.ErrorType, message string) (err error) {
	reqLogger := r.Log.WithValues("baremetalhost", request.NamespacedName)

	setErrorMessage(host, errType, message)

	reqLogger.Info(
		"adding error message",
		"message", message,
	)
	err = r.saveHostStatus(host)
	if err != nil {
		err = errors.Wrap(err, "failed to update error message")
	}

	return
}

// Retrieve the secret containing the credentials for talking to the BMC.
func (r *BareMetalHostReconciler) getBMCSecretAndSetOwner(request ctrl.Request, host *metal3v1alpha1.BareMetalHost) (bmcCredsSecret *corev1.Secret, err error) {

	if host.Spec.BMC.CredentialsName == "" {
		return nil, &EmptyBMCSecretError{message: "The BMC secret reference is empty"}
	}
	secretKey := host.CredentialsKey()
	bmcCredsSecret = &corev1.Secret{}
	err = r.Get(context.TODO(), secretKey, bmcCredsSecret)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, &ResolveBMCSecretRefError{message: fmt.Sprintf("The BMC secret %s does not exist", secretKey)}
		}
		return nil, err
	}

	// Make sure the secret has the correct owner as soon as we can.
	// This can return an SaveBMCSecretOwnerError
	// which isn't handled causing us to immediately try again
	// which seems fine as we expect this to be a transient failure
	err = r.setBMCCredentialsSecretOwner(request, host, bmcCredsSecret)
	if err != nil {
		return bmcCredsSecret, err
	}

	return bmcCredsSecret, nil
}

// Make sure the credentials for the management controller look
// right and manufacture bmc.Credentials.  This does not actually try
// to use the credentials.
func (r *BareMetalHostReconciler) buildAndValidateBMCCredentials(request ctrl.Request, host *metal3v1alpha1.BareMetalHost) (bmcCreds *bmc.Credentials, bmcCredsSecret *corev1.Secret, err error) {

	// Retrieve the BMC secret from Kubernetes for this host
	bmcCredsSecret, err = r.getBMCSecretAndSetOwner(request, host)
	if err != nil {
		return nil, nil, err
	}

	// Check for a "discovered" host vs. one that we have all the info for
	// and find empty Address or CredentialsName fields
	if host.Spec.BMC.Address == "" {
		return nil, nil, &EmptyBMCAddressError{message: "Missing BMC connection detail 'Address'"}
	}

	// pass the bmc address to bmc.NewAccessDetails which will do
	// more in-depth checking on the url to ensure it is
	// a valid bmc address, returning a bmc.UnknownBMCTypeError
	// if it is not conformant
	_, err = bmc.NewAccessDetails(host.Spec.BMC.Address, host.Spec.BMC.DisableCertificateVerification)
	if err != nil {
		return nil, nil, err
	}

	bmcCreds = &bmc.Credentials{
		Username: string(bmcCredsSecret.Data["username"]),
		Password: string(bmcCredsSecret.Data["password"]),
	}

	// Verify that the secret contains the expected info.
	err = bmcCreds.Validate()
	if err != nil {
		return nil, bmcCredsSecret, err
	}

	return bmcCreds, bmcCredsSecret, nil
}

func (r *BareMetalHostReconciler) setBMCCredentialsSecretOwner(request ctrl.Request, host *metal3v1alpha1.BareMetalHost, secret *corev1.Secret) (err error) {
	reqLogger := r.Log.WithValues("baremetalhost", request.NamespacedName)
	if metav1.IsControlledBy(secret, host) {
		return nil
	}
	reqLogger.Info("updating owner of secret")
	err = controllerutil.SetControllerReference(host, secret, r.Scheme)
	if err != nil {
		return &SaveBMCSecretOwnerError{message: fmt.Sprintf("cannot set owner: %q", err.Error())}
	}
	err = r.Update(context.TODO(), secret)
	if err != nil {
		return &SaveBMCSecretOwnerError{message: fmt.Sprintf("cannot save owner: %q", err.Error())}
	}
	return nil
}

func (r *BareMetalHostReconciler) publishEvent(request ctrl.Request, event corev1.Event) {
	reqLogger := r.Log.WithValues("baremetalhost", request.NamespacedName)
	reqLogger.Info("publishing event", "reason", event.Reason, "message", event.Message)
	err := r.Create(context.TODO(), &event)
	if err != nil {
		reqLogger.Info("failed to record event, ignoring",
			"reason", event.Reason, "message", event.Message, "error", err)
	}
	return
}

func (r *BareMetalHostReconciler) hostHasStatus(host *metal3v1alpha1.BareMetalHost) bool {
	return !host.Status.LastUpdated.IsZero()
}

func hostHasFinalizer(host *metal3v1alpha1.BareMetalHost) bool {
	return utils.StringInList(host.Finalizers, metal3v1alpha1.BareMetalHostFinalizer)
}

func (r *BareMetalHostReconciler) updateEventHandler(e event.UpdateEvent) bool {
	_, oldOK := e.ObjectOld.(*metal3v1alpha1.BareMetalHost)
	_, newOK := e.ObjectNew.(*metal3v1alpha1.BareMetalHost)
	if !(oldOK && newOK) {
		// The thing that changed wasn't a host, so we
		// need to assume that we must update. This
		// happens when, for example, an owned Secret
		// changes.
		return true
	}

	//If the update increased the resource Generation then let's process it
	if e.MetaNew.GetGeneration() != e.MetaOld.GetGeneration() {
		return true
	}

	//Discard updates that did not increase the resource Generation (such as on Status.LastUpdated), except for the finalizers or annotations
	if reflect.DeepEqual(e.MetaNew.GetFinalizers(), e.MetaOld.GetFinalizers()) && reflect.DeepEqual(e.MetaNew.GetAnnotations(), e.MetaOld.GetAnnotations()) {
		return false
	}

	return true
}

// SetupWithManager reigsters the reconciler to be run by the manager
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
		For(&metal3v1alpha1.BareMetalHost{}).
		WithEventFilter(
			predicate.Funcs{
				UpdateFunc: r.updateEventHandler,
			}).
		WithOptions(opts).
		Owns(&corev1.Secret{}).
		Complete(r)
}
