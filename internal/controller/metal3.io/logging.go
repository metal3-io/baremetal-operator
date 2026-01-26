/*
Copyright 2025 The Metal3 Authors.

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

// Log verbosity levels for structured logging.
// Use VerbosityLevelDebug for key decision points and important state changes.
// Use VerbosityLevelTrace for detailed step-by-step flow tracing (function entry/exit).
//
// TODO: Review all existing info.log.Info() calls to determine if they should be:
// - V(VerbosityLevelDebug) for key decision points (state changes, important branches)
// - V(VerbosityLevelTrace) for function entry/exit and detailed flow
// - Left as Info() for critical operational messages visible by default
//
// Guidelines for log levels:
// - Info() (level 0): Critical operational messages, errors, state transitions
// - V(1-3): Reserved for controller-runtime internal use
// - V(VerbosityLevelDebug=4): Decision points, important branches, config changes
// - V(VerbosityLevelTrace=5): Function entry/exit, detailed flow tracing
const (
	VerbosityLevelDebug = 4
	VerbosityLevelTrace = 5
)

// LogField* constants define standardized keys for structured logging.
// Using consistent field names across all controllers improves log searchability
// and makes it easier to build dashboards and alerts.
const (
	// LogFieldController is the name of the controller.
	LogFieldController = "controller"

	// LogFieldHost is the name of the BareMetalHost resource.
	LogFieldHost = "host"

	// LogFieldHostNamespace is the namespace of the BareMetalHost resource.
	LogFieldHostNamespace = "namespace"

	// LogFieldBMC is the BMC address.
	LogFieldBMC = "bmc"

	// LogFieldBMCAddress is the BMC address (alias for BMC).
	LogFieldBMCAddress = "bmcAddress"

	// LogFieldProvisioner is the provisioner being used.
	LogFieldProvisioner = "provisioner"

	// LogFieldProvisioningID is the Ironic node UUID.
	LogFieldProvisioningID = "provisioningID"

	// LogFieldProvisioningState is the current provisioning state of the host.
	LogFieldProvisioningState = "provisioningState"

	// LogFieldTargetProvisioningState is the target state for provisioning.
	LogFieldTargetProvisioningState = "targetProvisioningState"

	// LogFieldPowerState is the current power state.
	LogFieldPowerState = "powerState"

	// LogFieldPoweredOn indicates if the host is powered on.
	LogFieldPoweredOn = "poweredOn"

	// LogFieldImage is the image being provisioned.
	LogFieldImage = "image"

	// LogFieldImageURL is the URL of the image being provisioned.
	LogFieldImageURL = "imageURL"

	// LogFieldError is the error message.
	LogFieldError = "error"

	// LogFieldErrorMessage is the error message (alias).
	LogFieldErrorMessage = "errorMessage"

	// LogFieldErrorType is the type of error.
	LogFieldErrorType = "errorType"

	// LogFieldReason is the reason for an action or state.
	LogFieldReason = "reason"

	// LogFieldRequeue indicates if the reconciliation should be requeued.
	LogFieldRequeue = "requeue"

	// LogFieldRequeueAfter is the duration after which to requeue.
	LogFieldRequeueAfter = "requeueAfter"

	// LogFieldHardwareProfile is the hardware profile being used.
	LogFieldHardwareProfile = "hardwareProfile"

	// LogFieldRAID is the RAID configuration.
	LogFieldRAID = "raid"

	// LogFieldFirmware is the firmware configuration.
	LogFieldFirmware = "firmware"

	// LogFieldFirmwareSettings are the firmware settings.
	LogFieldFirmwareSettings = "firmwareSettings"

	// LogFieldFirmwareComponents are the firmware components.
	LogFieldFirmwareComponents = "firmwareComponents"

	// LogFieldFirmwareSchema is the firmware schema.
	LogFieldFirmwareSchema = "firmwareSchema"

	// LogFieldSecret is the name of a secret resource.
	LogFieldSecret = "secret"

	// LogFieldSecretNamespace is the namespace of a secret resource.
	LogFieldSecretNamespace = "secretNamespace"

	// LogFieldNode is the Ironic node ID.
	LogFieldNode = "node"

	// LogFieldNodeID is the Ironic node ID (alias).
	LogFieldNodeID = "nodeID"

	// LogFieldMACAddress is a MAC address.
	LogFieldMACAddress = "macAddress"

	// LogFieldDataImage is the data image configuration.
	LogFieldDataImage = "dataImage"

	// LogFieldAction is the action being performed.
	LogFieldAction = "action"

	// LogFieldResult is the result of an action.
	LogFieldResult = "result"

	// LogFieldDirty indicates if the resource needs updating.
	LogFieldDirty = "dirty"

	// LogFieldGeneration is the resource generation.
	LogFieldGeneration = "generation"

	// LogFieldObservedGeneration is the observed generation.
	LogFieldObservedGeneration = "observedGeneration"

	// LogFieldConsumerRef is the consumer reference.
	LogFieldConsumerRef = "consumerRef"

	// LogFieldPreprovisioningImage is the preprovisioning image.
	LogFieldPreprovisioningImage = "preprovisioningImage"

	// LogFieldCredentials indicates credential-related operations.
	LogFieldCredentials = "credentials"

	// LogFieldDuration is a time duration.
	LogFieldDuration = "duration"

	// LogFieldRetryAfter is the retry interval.
	LogFieldRetryAfter = "retryAfter"

	// LogFieldAnnotation is an annotation key.
	LogFieldAnnotation = "annotation"

	// LogFieldAnnotationValue is an annotation value.
	LogFieldAnnotationValue = "annotationValue"

	// LogFieldLabel is a label key.
	LogFieldLabel = "label"

	// LogFieldLabelValue is a label value.
	LogFieldLabelValue = "labelValue"

	// LogFieldInspection relates to hardware inspection.
	LogFieldInspection = "inspection"

	// LogFieldInspectionData is the inspection data.
	LogFieldInspectionData = "inspectionData"

	// LogFieldCleaning relates to disk cleaning.
	LogFieldCleaning = "cleaning"

	// LogFieldServicing relates to host servicing.
	LogFieldServicing = "servicing"

	// LogFieldOperationalStatus is the operational status.
	LogFieldOperationalStatus = "operationalStatus"

	// LogFieldErrorCount is the count of errors.
	LogFieldErrorCount = "errorCount"

	// LogFieldReconcileID is the unique ID for a reconciliation.
	LogFieldReconcileID = "reconcileID"

	// LogFieldStatusCondition is a status condition type.
	LogFieldStatusCondition = "condition"

	// LogFieldStatusConditionStatus is the status of a condition.
	LogFieldStatusConditionStatus = "conditionStatus"
)
