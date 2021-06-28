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
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

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
	log logr.Logger
	hfs *metal3v1alpha1.HostFirmwareSettings
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

	// Fetch the HostFirmwareSettings
	hfs := &metal3v1alpha1.HostFirmwareSettings{}
	if err = r.Get(ctx, req.NamespacedName, hfs); err != nil {
		if k8serrors.IsNotFound(err) {
			// Request object not found, could have been deleted after
			// reconcile request. Return and don't requeue
			reqLogger.Info("could not find host firmware settings")
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return ctrl.Result{}, errors.Wrap(err, "could not load host firmware settings")
	}

	// Get settings from provisioner if update requested or settings are empty
	if hfs.Status.ProvStatus.Update == true || hfs.Status.Settings == nil {

		// Use the provisioner ID stored in annotation to access Ironic API
		if _, ok := hfs.Annotations[metal3v1alpha1.ProvisionerIdAnnotation]; ok {
			provID := hfs.Annotations[metal3v1alpha1.ProvisionerIdAnnotation]

			info := &rInfo{log: reqLogger, hfs: hfs}

			prov, err := r.ProvisionerFactory.NewProvisioner(provisioner.BuildEmptyHostData(provID), func(reason, message string) {})
			if err != nil {
				return ctrl.Result{}, errors.Wrap(err, "failed to create provisioner")
			}

			if err = r.updateHostFirmwareSettings(prov, info); err != nil {
				return ctrl.Result{}, errors.Wrap(err, "Could not update hostFirmwareSettings")
			}
		} else {
			// This is unexpected but retrying won't help
			reqLogger.Info("could not get provisioner ID from annotation")
		}
	}

	// only run again on changes
	return ctrl.Result{}, nil
}

// Get the firmware settings from the provisioner and update hostFirmwareSettings
func (r *HostFirmwareSettingsReconciler) updateHostFirmwareSettings(prov provisioner.Provisioner, info *rInfo) (err error) {

	info.log.Info("retrieving firmware settings and saving to resource")

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

	if err = r.updateResource(info, currentSettings, firmwareSchema); err != nil {
		return errors.Wrap(err, "could not update hostFirmwareSettings")
	}

	return nil
}

// Update the HostFirmwareSettings resource using the settings and schema from provisioner
func (r *HostFirmwareSettingsReconciler) updateResource(info *rInfo, settings metal3v1alpha1.SettingsMap, schema *metal3v1alpha1.FirmwareSchema) (err error) {

	if info.hfs.Status.Settings == nil {
		info.hfs.Status.Settings = make(metal3v1alpha1.SettingsMap)
	}

	// Update Spec and Status
	specDirty := false
	for k, v := range settings {
		// Some vendors include encrypted password fields, don't add these
		if strings.Contains(k, "Password") {
			continue
		}
		info.hfs.Status.Settings[k] = v

		// Don't include setting in Spec if it cannot be changed (ReadOnly is set)
		// or if it unique to this particular host, in order to prevent the unique
		// settings from being copied to other hosts
		if schema != nil {
			if schemaSetting, ok := schema.Spec.Schema[k]; ok {
				if (schemaSetting.ReadOnly != nil && *schemaSetting.ReadOnly == true) ||
					(schemaSetting.Unique != nil && *schemaSetting.Unique == true) {
					continue
				}
			}
		}
		// Copy to Spec only if doesn't exist as setting may have been set by user
		if _, ok := info.hfs.Spec.Settings[k]; !ok {
			info.hfs.Spec.Settings[k] = intstr.FromString(v)
			specDirty = true
		}
	}

	if specDirty {
		if err = r.Update(context.TODO(), info.hfs); err != nil {
			return errors.Wrap(err, "could not update host firmware settings")
		}
	}

	info.hfs.Status.ProvStatus.Update = false
	t := metav1.Now()
	info.hfs.Status.ProvStatus.LastUpdated = &t
	info.log.Info("clearing hostFirmwareSettings update flag after updating resource")

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
	if _, ok := info.hfs.Annotations[metal3v1alpha1.HardwareVendorAnnotation]; ok {
		firmwareSchema.Spec.HardwareVendor = info.hfs.Annotations[metal3v1alpha1.HardwareVendorAnnotation]
	}
	if _, ok := info.hfs.Annotations[metal3v1alpha1.HardwareModelAnnotation]; ok {
		firmwareSchema.Spec.HardwareModel = info.hfs.Annotations[metal3v1alpha1.HardwareModelAnnotation]
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

// SetupWithManager sets up the controller with the Manager.
func (r *HostFirmwareSettingsReconciler) SetupWithManager(mgr ctrl.Manager) error {

	return ctrl.NewControllerManagedBy(mgr).
		For(&metal3v1alpha1.HostFirmwareSettings{}).
		Complete(r)
}

// Validate the HostFirmwareSetting Spec
func ValidateHostFirmwareSettings(hfs *metal3v1alpha1.HostFirmwareSettings, schema *metal3v1alpha1.FirmwareSchema) error {

	if hfs == nil {
		return fmt.Errorf("Missing parameter to ValidateHostFirmwareSettings")
	}

	for name, val := range hfs.Spec.Settings {
		// Prohibit any Spec settings with "Password"
		if strings.Contains(name, "Password") {
			return fmt.Errorf("Cannot set Password field")
		}

		// The setting must be in the Status
		if _, ok := hfs.Status.Settings[name]; !ok {
			return fmt.Errorf("Setting %s is not in the Status field", name)
		}

		// check validity of updated value
		if schema != nil {
			if err := schema.CheckSettingIsValid(name, val, schema.Spec.Schema); err != nil {
				return err
			}
		}
	}

	return nil
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
