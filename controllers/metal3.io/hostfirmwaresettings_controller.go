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
	"strconv"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	metal3v1alpha1 "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner"
)

const (
	provisionerRetryDelay                = time.Second * 30
	resourceNotAvailableRetryDelay       = time.Second * 30
	reconcilerRequeueDelay               = time.Minute * 5
	reconcilerRequeueDelayChangeDetected = time.Minute * 1
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
		return ctrl.Result{Requeue: true, RequeueAfter: resourceNotAvailableRetryDelay}, nil
	}

	// Fetch the HostFirmwareSettings
	hfs := &metal3v1alpha1.HostFirmwareSettings{}
	info := &rInfo{log: reqLogger, hfs: hfs, bmh: bmh}
	if err = r.Get(ctx, req.NamespacedName, hfs); err != nil {
		// The HFS resource may have been deleted
		if k8serrors.IsNotFound(err) {
			reqLogger.Info("hostFirmwareSettings not found")
			return ctrl.Result{Requeue: true, RequeueAfter: resourceNotAvailableRetryDelay}, nil
		}
		// Error reading the object - requeue the request.
		return ctrl.Result{}, errors.Wrap(err, "could not load hostFirmwareSettings")
	}

	// Create a provisioner that can access Ironic API
	prov, err := r.ProvisionerFactory.NewProvisioner(provisioner.BuildHostDataNoBMC(*bmh), info.publishEvent)
	if err != nil {
		return ctrl.Result{}, errors.Wrap(err, "failed to create provisioner")
	}

	ready, err := prov.IsReady()
	if err != nil || !ready {
		reqLogger.Info("provisioner is not ready", "RequeueAfter:", provisionerRetryDelay)
		return ctrl.Result{Requeue: true, RequeueAfter: provisionerRetryDelay}, nil
	}

	info.log.Info("retrieving firmware settings and saving to resource", "node", bmh.Status.Provisioning.ID)

	// Get the current settings and schema, retry if provisioner returns error
	currentSettings, schema, err := prov.GetFirmwareSettings(true)
	if err != nil {
		reqLogger.Info("provisioner returns error", "RequeueAfter:", provisionerRetryDelay)
		return ctrl.Result{Requeue: true, RequeueAfter: provisionerRetryDelay}, nil
	}

	if err = r.updateHostFirmwareSettings(currentSettings, schema, info); err != nil {
		return ctrl.Result{}, errors.Wrap(err, "Could not update hostFirmwareSettings")
	}

	for _, e := range info.events {
		r.publishEvent(req, e)
	}

	// requeue to run again after delay
	if meta.IsStatusConditionTrue(info.hfs.Status.Conditions, string(metal3v1alpha1.FirmwareSettingsChangeDetected)) {
		// If there is a difference between Spec and Status shorten the query from Ironic so that the Status is updated when cleaning completes
		return ctrl.Result{Requeue: true, RequeueAfter: reconcilerRequeueDelayChangeDetected}, nil
	}
	return ctrl.Result{Requeue: true, RequeueAfter: reconcilerRequeueDelay}, nil
}

// Get the firmware settings from the provisioner and update hostFirmwareSettings
func (r *HostFirmwareSettingsReconciler) updateHostFirmwareSettings(currentSettings metal3v1alpha1.SettingsMap, schema map[string]metal3v1alpha1.SettingSchema, info *rInfo) (err error) {

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

	// Update Status on changes
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
			if v.String() != statusVal {
				specMismatch = true
				break
			}
		} else {
			// Spec setting is not in Status, this will be handled by validateHostFirmwareSettings
			specMismatch = true
			break
		}
	}

	// Set up the conditions which will be used by baremetalhost controller when determining whether to add settings during cleaning
	reason := reasonSuccess
	generation := info.hfs.GetGeneration()

	if specMismatch {
		setCondition(generation, &info.hfs.Status, metal3v1alpha1.FirmwareSettingsChangeDetected, metav1.ConditionTrue, reason, "")

		// Run validation on the Spec to detect invalid values entered by user, including Spec settings not in Status
		// Eventually this will be handled by a webhook
		errors := r.validateHostFirmwareSettings(info, schema)
		if len(errors) == 0 {
			setCondition(generation, &info.hfs.Status, metal3v1alpha1.FirmwareSettingsValid, metav1.ConditionTrue, reason, "")
		} else {
			for _, error := range errors {
				info.publishEvent("ValidationFailed", fmt.Sprintf("Invalid BIOS setting: %v", error))
			}

			reason = reasonConfigurationError
			setCondition(generation, &info.hfs.Status, metal3v1alpha1.FirmwareSettingsValid, metav1.ConditionFalse, reason, "Invalid BIOS setting")
		}
	} else {
		// Reset conditions
		if meta.IsStatusConditionTrue(info.hfs.Status.Conditions, string(metal3v1alpha1.FirmwareSettingsChangeDetected)) {
			setCondition(generation, &info.hfs.Status, metal3v1alpha1.FirmwareSettingsChangeDetected, metav1.ConditionFalse, reason, "")
		}
		if !meta.IsStatusConditionTrue(info.hfs.Status.Conditions, string(metal3v1alpha1.FirmwareSettingsValid)) {
			setCondition(generation, &info.hfs.Status, metal3v1alpha1.FirmwareSettingsValid, metav1.ConditionTrue, reason, "")
		}
	}

	t := metav1.Now()
	info.hfs.Status.LastUpdated = &t
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

		// Add hfs as owner so can be garbage collected on delete, if already an owner it will just be overwritten
		if err = controllerutil.SetOwnerReference(info.hfs, firmwareSchema, r.Scheme()); err != nil {
			return nil, errors.Wrap(err, "could not set owner of existing firmwareSchema")
		}
		if err = r.Update(context.TODO(), firmwareSchema); err != nil {
			return nil, err
		}

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
		// Don't store Password settings in Schema as these aren't stored in HostFirmwareSettings
		if strings.Contains(k, "Password") {
			continue
		}
		if v.AttributeType == "Password" {
			continue
		}

		firmwareSchema.Spec.Schema[k] = v
	}
	// Set hfs as owner
	if err = controllerutil.SetOwnerReference(info.hfs, firmwareSchema, r.Scheme()); err != nil {
		return nil, errors.Wrap(err, "could not set owner of firmwareSchema")
	}

	if err = r.Create(context.TODO(), firmwareSchema); err != nil {
		return nil, err
	}

	info.log.Info("created new firmwareSchema resource")

	return firmwareSchema, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *HostFirmwareSettingsReconciler) SetupWithManager(mgr ctrl.Manager) error {

	return ctrl.NewControllerManagedBy(mgr).
		For(&metal3v1alpha1.HostFirmwareSettings{}).
		Complete(r)
}

// Validate the HostFirmwareSetting Spec against the schema
func (r *HostFirmwareSettingsReconciler) validateHostFirmwareSettings(info *rInfo, schema *metal3v1alpha1.FirmwareSchema) []error {

	var errors []error

	for name, val := range info.hfs.Spec.Settings {
		// Prohibit any Spec settings with "Password"
		if strings.Contains(name, "Password") {
			errors = append(errors, fmt.Errorf("Cannot set Password field"))
			continue
		}

		// The setting must be in the Status
		if _, ok := info.hfs.Status.Settings[name]; !ok {
			errors = append(errors, fmt.Errorf("Setting %s is not in the Status field", name))
			continue
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

// Generate a name based on the schema key and values which should be the same for similar hardware
func GetSchemaName(schema map[string]metal3v1alpha1.SettingSchema) string {

	// Schemas from the same vendor and model should be identical for both keys and values.
	hashkeys := make([]string, 0, len(schema))
	for k, v := range schema {
		hashkeys = append(hashkeys, k)
		hashkeys = append(hashkeys, v.AttributeType)
		for _, av := range v.AllowableValues {
			hashkeys = append(hashkeys, av)
		}
		if v.LowerBound != nil {
			hashkeys = append(hashkeys, strconv.Itoa(*v.LowerBound))
		}
		if v.UpperBound != nil {
			hashkeys = append(hashkeys, strconv.Itoa(*v.UpperBound))
		}
		if v.MinLength != nil {
			hashkeys = append(hashkeys, strconv.Itoa(*v.MinLength))
		}
		if v.MaxLength != nil {
			hashkeys = append(hashkeys, strconv.Itoa(*v.MaxLength))
		}
		if v.ReadOnly != nil {
			hashkeys = append(hashkeys, strconv.FormatBool(*v.ReadOnly))
		}
	}
	sort.Strings(hashkeys)

	h := sha256.New()
	h.Write([]byte(fmt.Sprintf("%v", hashkeys)))
	hash := fmt.Sprintf("%x", h.Sum(nil))[:8]

	return "schema-" + hash
}
