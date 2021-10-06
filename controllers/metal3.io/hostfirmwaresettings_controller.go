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
	"crypto/sha256"
	"fmt"
	"sort"
	"strings"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/source"

	metal3v1alpha1 "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner"
)

// HostFirmwareSettingsReconciler reconciles a HostFirmwareSettings object
type HostFirmwareSettingsReconciler struct {
	client.Client
	Log                logr.Logger
	ProvisionerFactory provisioner.Factory
}

type rInfo struct {
	log    logr.Logger
	hfs    *metal3v1alpha1.HostFirmwareSettings
	bmh    *metal3v1alpha1.BareMetalHost
	events []corev1.Event
}

type conditionReason string

const (
	reasonSuccess            conditionReason = "Success"
	reasonConfigurationError conditionReason = "ConfigurationError"
)

func (info *rInfo) publishEvent(reason, message string) {

	t := metav1.Now()
	hfsEvent := corev1.Event{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: reason + "-",
			Namespace:    info.hfs.ObjectMeta.Namespace,
		},
		InvolvedObject: corev1.ObjectReference{
			Kind:       "HostFirmwareSettings",
			Namespace:  info.hfs.Namespace,
			Name:       info.hfs.Name,
			UID:        info.hfs.UID,
			APIVersion: metal3v1alpha1.GroupVersion.String(),
		},
		Reason:  reason,
		Message: message,
		Source: corev1.EventSource{
			Component: "metal3-hostfirmwaresettings-controller",
		},
		FirstTimestamp:      t,
		LastTimestamp:       t,
		Count:               1,
		Type:                corev1.EventTypeNormal,
		ReportingController: "metal3.io/hostfirmwaresettings-controller",
	}

	info.events = append(info.events, hfsEvent)
}

//+kubebuilder:rbac:groups=metal3.io,resources=hostfirmwaresettings,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=metal3.io,resources=hostfirmwaresettings/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=metal3.io,resources=firmwareschemas,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=metal3.io,resources=firmwareschemas/status,verbs=get;update;patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.6.4/pkg/reconcile
func (r *HostFirmwareSettingsReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, err error) {

	reqLogger := r.Log.WithValues("hostfirmwaresettings", req.NamespacedName)
	reqLogger.Info("start")

	// Get the corresponding baremetalhost in this namespace, if one doesn't exist don't continue processing
	bmh := &metal3v1alpha1.BareMetalHost{}
	if err = r.Get(context.TODO(), req.NamespacedName, bmh); err != nil {
		reqLogger.Info("could not get baremetalhost, not running reconciler")
		// only run again if created
		return ctrl.Result{}, nil
	}

	// Fetch the HostFirmwareSettings
	hfs := &metal3v1alpha1.HostFirmwareSettings{}
	info := &rInfo{log: reqLogger, hfs: hfs, bmh: bmh}
	if err = r.Get(ctx, req.NamespacedName, hfs); err != nil {
		if k8serrors.IsNotFound(err) {
			// A resource doesn't exist, create one
			if err = r.newHostFirmwareSettings(info, req.NamespacedName); err != nil {

				return ctrl.Result{}, errors.Wrap(err, "could not create firmware settings")
			}
		} else {
			// Error reading the object - requeue the request.
			return ctrl.Result{}, errors.Wrap(err, "could not load host firmware settings")
		}
	}

	// Create a provisioner that can access Ironic API
	prov, err := r.ProvisionerFactory.NewProvisioner(provisioner.BuildHostDataNoBMC(*bmh), info.publishEvent)
	if err != nil {
		return ctrl.Result{}, errors.Wrap(err, "failed to create provisioner")
	}

	ready, err := prov.IsReady()
	if err != nil || !ready {
		reqLogger.Info("provisioner is not ready", "RequeueAfter:", provisionerNotReadyRetryDelay)
		return ctrl.Result{Requeue: true, RequeueAfter: provisionerNotReadyRetryDelay}, nil
	}

	// Get the data from Ironic and update HFS, the call to provisioner may fail if settings can't
	// be retrieved for the bios_interface.
	if err = r.updateHostFirmwareSettings(prov, info); err != nil {
		return ctrl.Result{}, errors.Wrap(err, "Could not update hostFirmwareSettings")
	}

	for _, e := range info.events {
		r.publishEvent(req, e)
	}

	// only run again on changes
	return ctrl.Result{}, nil
}

// Get the firmware settings from the provisioner and update hostFirmwareSettings
func (r *HostFirmwareSettingsReconciler) updateHostFirmwareSettings(prov provisioner.Provisioner, info *rInfo) (err error) {

	info.log.Info("retrieving firmware settings and saving to resource", "node", info.bmh.Status.Provisioning.ID)

	// Get the current settings and schema
	currentSettings, schema, err := prov.GetFirmwareSettings(true)
	if err != nil {
		return errors.Wrap(err, "could not get firmware settings from provisioner")
	}

	// get or create a firmwareSchema to hold schema
	firmwareSchema, err := r.getOrCreateFirmwareSchema(info, schema)
	if err != nil {
		return errors.Wrap(err, "could not get/create firmware schema")
	}

	// Set hostFirmwareSetting to use this schema
	info.hfs.Status.FirmwareSchema = &metal3v1alpha1.SchemaReference{
		Namespace: firmwareSchema.ObjectMeta.Namespace,
		Name:      firmwareSchema.ObjectMeta.Name}

	if err = r.updateStatus(info, currentSettings, firmwareSchema); err != nil {
		return errors.Wrap(err, "could not update hostFirmwareSettings")
	}

	return nil
}

// Update the HostFirmwareSettings resource using the settings and schema from provisioner
func (r *HostFirmwareSettingsReconciler) updateStatus(info *rInfo, settings metal3v1alpha1.SettingsMap, schema *metal3v1alpha1.FirmwareSchema) (err error) {

	if info.hfs.Status.Settings == nil {
		info.hfs.Status.Settings = make(metal3v1alpha1.SettingsMap)
	}

	// Update Status
	for k, v := range settings {
		// Some vendors include encrypted password fields, don't add these
		if strings.Contains(k, "Password") {
			continue
		}
		info.hfs.Status.Settings[k] = v
	}

	// Check if any Spec settings are different than Status
	specMismatch := false
	for k, v := range info.hfs.Spec.Settings {
		if statusVal, ok := info.hfs.Status.Settings[k]; ok {
			if v != intstr.FromString(statusVal) {
				specMismatch = true
				break
			}
		}
	}

	// Set up the conditions which will be used by baremetalhost controller when determining whether to add settings during cleaning
	reason := reasonSuccess
	generation := info.hfs.GetGeneration()

	if specMismatch {
		setCondition(generation, &info.hfs.Status, metal3v1alpha1.UpdateRequested, metav1.ConditionTrue, reason, "")

	} else {
		setCondition(generation, &info.hfs.Status, metal3v1alpha1.UpdateRequested, metav1.ConditionFalse, reason, "")
	}

	// Run validation on the Spec to detect invalid values entered by user, including Spec settings not in Status
	// Eventually this will be handled by a webhook
	errors := r.validateHostFirmwareSettings(info, schema)
	if len(errors) == 0 {
		setCondition(generation, &info.hfs.Status, metal3v1alpha1.SettingsValid, metav1.ConditionTrue, reason, "")
	} else {
		for _, error := range errors {
			info.publishEvent("ValidationFailed", fmt.Sprintf("Invalid BIOS setting: %v", error))
		}

		reason = reasonConfigurationError
		setCondition(generation, &info.hfs.Status, metal3v1alpha1.SettingsValid, metav1.ConditionFalse, reason, "Invalid BIOS setting")
	}

	return r.Status().Update(context.TODO(), info.hfs)
}

// Get a firmware schema that matches the host vendor or create one if it doesn't exist
func (r *HostFirmwareSettingsReconciler) getOrCreateFirmwareSchema(info *rInfo, schema map[string]metal3v1alpha1.SettingSchema) (fSchema *metal3v1alpha1.FirmwareSchema, err error) {

	info.log.Info("getting firmwareSchema")

	schemaName := GetSchemaName(schema)
	firmwareSchema := &metal3v1alpha1.FirmwareSchema{}

	// If a schema exists that matches, use that, otherwise create a new one
	if err = r.Get(context.TODO(), client.ObjectKey{Namespace: info.hfs.ObjectMeta.Namespace, Name: schemaName},
		firmwareSchema); err == nil {

		info.log.Info("found existing firmwareSchema resource")

		return firmwareSchema, nil
	}
	if !k8serrors.IsNotFound(err) {
		// Error reading the object
		return nil, err

	}

	firmwareSchema = &metal3v1alpha1.FirmwareSchema{
		ObjectMeta: metav1.ObjectMeta{
			Name:      schemaName,
			Namespace: info.hfs.ObjectMeta.Namespace,
		},
	}

	// If available, store hardware details in schema for additional info
	if info.bmh.Status.HardwareDetails != nil {
		firmwareSchema.Spec.HardwareVendor = info.bmh.Status.HardwareDetails.SystemVendor.Manufacturer
		firmwareSchema.Spec.HardwareModel = info.bmh.Status.HardwareDetails.SystemVendor.ProductName
	}

	// Copy in the schema from provisioner
	firmwareSchema.Spec.Schema = make(map[string]metal3v1alpha1.SettingSchema)
	for k, v := range schema {
		firmwareSchema.Spec.Schema[k] = v
	}

	if err = r.Create(context.TODO(), firmwareSchema); err != nil {
		return nil, err
	}
	// Set hfs as owner after the create
	if controllerutil.SetOwnerReference(info.hfs, firmwareSchema, r.Scheme()); err != nil {
		return nil, errors.Wrap(err, "could not set owner of firmwareSchema")
	}
	if err = r.Update(context.TODO(), firmwareSchema); err != nil {
		return nil, err
	}

	info.log.Info("created new firmwareSchema resource")

	return firmwareSchema, nil
}

// Create a hostFirmwareSettings
func (r *HostFirmwareSettingsReconciler) newHostFirmwareSettings(info *rInfo, namespacedName types.NamespacedName) (err error) {

	info.hfs.ObjectMeta = metav1.ObjectMeta{
		Name: namespacedName.Name, Namespace: namespacedName.Namespace}
	info.hfs.Status.Settings = make(metal3v1alpha1.SettingsMap)
	info.hfs.Spec.Settings = make(metal3v1alpha1.DesiredSettingsMap)

	if err = r.Create(context.TODO(), info.hfs); err != nil {
		return errors.Wrap(err, "failure creating hostFirmwareSettings resource")
	}

	// Set bmh as owner, this makes sure the resource is deleted when bmh is deleted
	if controllerutil.SetControllerReference(info.bmh, info.hfs, r.Scheme()); err != nil {
		return errors.Wrap(err, "could not set bmh as controller")
	}
	if err = r.Update(context.TODO(), info.hfs); err != nil {
		return errors.Wrap(err, "could not update hostfirmwaresettings")
	}

	info.log.Info("created new hostFirmwareSettings resource")

	return nil
}

// EventHandler for updates to both the baremetalhost and hostfirmwarettings
func (r *HostFirmwareSettingsReconciler) updateEventHandler(e event.UpdateEvent) bool {

	r.Log.Info("hostfirmwaresettings in event handler")

	// If this is a baremetalhost, only return true on certain state transitions
	var oldState metal3v1alpha1.ProvisioningState = metal3v1alpha1.StateNone
	newState := oldState

	oldHost, oldHostExists := e.ObjectOld.(*metal3v1alpha1.BareMetalHost)
	if oldHostExists {
		oldState = oldHost.Status.Provisioning.State
	}
	newHost, newHostExists := e.ObjectNew.(*metal3v1alpha1.BareMetalHost)
	if newHostExists {
		newState = newHost.Status.Provisioning.State
	}

	if newHostExists && !oldHostExists {
		// The baremetalhost has been created
		return true
	}
	if oldHostExists || newHostExists {
		// Data needs to be retrieved from Ironic when node is moved to Ready  e.g. after cleaning and when
		// provisioned, its not necessary on every state change as data from Ironic will be the same.
		if (newState == metal3v1alpha1.StateReady || newState == metal3v1alpha1.StateProvisioned) && oldState != newState {
			r.Log.Info("baremetalhost event state update", "oldstate", oldState, "newstate", newState)
			return true
		}

		return false
	}

	_, oldHFS := e.ObjectOld.(*metal3v1alpha1.HostFirmwareSettings)
	_, newHFS := e.ObjectNew.(*metal3v1alpha1.HostFirmwareSettings)
	if !(oldHFS || newHFS) {
		// Don't process if not a hostFirmwareSetting
		return false
	}

	// This is a hostfirmwaresettings resource, if the update increased the resource generation then process it
	// changes to LastUpdated will not increase the resource generation
	if e.ObjectNew.GetGeneration() != e.ObjectOld.GetGeneration() {
		r.Log.Info("hostfirmwaresettings resource generation changed, updating")
		return true
	}

	return false
}

// SetupWithManager sets up the controller with the Manager.
func (r *HostFirmwareSettingsReconciler) SetupWithManager(mgr ctrl.Manager) error {

	return ctrl.NewControllerManagedBy(mgr).
		For(&metal3v1alpha1.HostFirmwareSettings{}).
		WithEventFilter(
			predicate.Funcs{
				UpdateFunc: r.updateEventHandler,
			}).
		Watches(&source.Kind{Type: &metal3v1alpha1.BareMetalHost{}}, &handler.EnqueueRequestForObject{}, builder.Predicates{}).
		Complete(r)
}

// Validate the HostFirmwareSetting Spec against the schema
func (r *HostFirmwareSettingsReconciler) validateHostFirmwareSettings(info *rInfo, schema *metal3v1alpha1.FirmwareSchema) []error {

	var errors []error

	for name, val := range info.hfs.Spec.Settings {
		// Prohibit any Spec settings with "Password"
		if strings.Contains(name, "Password") {
			errors = append(errors, fmt.Errorf("Cannot set Password field"))
			break
		}

		// The setting must be in the Status
		if _, ok := info.hfs.Status.Settings[name]; !ok {
			errors = append(errors, fmt.Errorf("Setting %s is not in the Status field", name))
			break
		}

		// check validity of updated value
		if schema != nil {
			if err := schema.ValidateSetting(name, val, schema.Spec.Schema); err != nil {
				errors = append(errors, err)
			}
		}
	}

	if len(errors) > 0 {
		return errors
	}

	return nil
}

func (r *HostFirmwareSettingsReconciler) publishEvent(request ctrl.Request, event corev1.Event) {
	reqLogger := r.Log.WithValues("hostfirmwaresettings", request.NamespacedName)
	reqLogger.Info("publishing event", "reason", event.Reason, "message", event.Message)
	err := r.Create(context.TODO(), &event)
	if err != nil {
		reqLogger.Info("failed to record event, ignoring",
			"reason", event.Reason, "message", event.Message, "error", err)
	}
	return
}

func setCondition(generation int64, status *metal3v1alpha1.HostFirmwareSettingsStatus,
	cond metal3v1alpha1.SettingsConditionType, newStatus metav1.ConditionStatus,
	reason conditionReason, message string) {
	newCondition := metav1.Condition{
		Type:               string(cond),
		Status:             newStatus,
		ObservedGeneration: generation,
		Reason:             string(reason),
		Message:            message,
	}
	meta.SetStatusCondition(&status.Conditions, newCondition)
}

// Generate a name based on the schema keys which should be the same for similar hardware
func GetSchemaName(schema map[string]metal3v1alpha1.SettingSchema) string {

	// Schemas from the same vendor and model should be identical for both keys and values.
	// Hash the keys of the map to get a identifier unique to this schema.
	keys := make([]string, 0, len(schema))
	for k := range schema {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	h := sha256.New()
	h.Write([]byte(fmt.Sprintf("%v", keys)))
	hash := fmt.Sprintf("%x", h.Sum(nil))[:8]

	return "schema-" + hash
}
