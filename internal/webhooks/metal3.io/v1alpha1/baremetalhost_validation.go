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

package webhooks

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"slices"
	"strings"

	"github.com/google/uuid"
	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	"github.com/metal3-io/baremetal-operator/pkg/hardwareutils/bmc"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var (
	supportedRebootModes           = []string{"hard", "soft", ""}
	supportedRebootModesString     = "\"hard\", \"soft\" or \"\""
	inspectAnnotationAllowed       = []string{"disabled", ""}
	inspectAnnotationAllowedString = "\"disabled\" or \"\""
)

// validateHost validates BareMetalHost resource for creation.
func (webhook *BareMetalHost) validateHost(host *metal3api.BareMetalHost) []error {
	var errs []error
	var bmcAccess bmc.AccessDetails

	if host.Spec.BMC.Address != "" {
		var err error
		bmcAccess, err = bmc.NewAccessDetails(host.Spec.BMC.Address, host.Spec.BMC.DisableCertificateVerification)
		if err != nil {
			errs = append(errs, err)
		}
	}

	errs = append(errs, webhook.validateCrossNamespaceSecretReferences(host)...)

	if raidErrors := validateRAID(host.Spec.RAID); raidErrors != nil {
		errs = append(errs, raidErrors...)
	}

	errs = append(errs, validateBMCAccess(host.Spec, bmcAccess)...)

	if err := validateBMHName(host.Name); err != nil {
		errs = append(errs, err)
	}

	if err := validateDNSName(host.Spec.BMC.Address); err != nil {
		errs = append(errs, err)
	}

	if err := validateRootDeviceHints(host.Spec.RootDeviceHints); err != nil {
		errs = append(errs, err)
	}

	if host.Spec.Image != nil {
		if err := validateImageURL(host.Spec.Image.URL); err != nil {
			errs = append(errs, err)
		}
	}

	if annotationErrors := validateAnnotations(host); annotationErrors != nil {
		errs = append(errs, annotationErrors...)
	}

	if err := validatePowerStatus(host); err != nil {
		errs = append(errs, err)
	}

	return errs
}

// validateChanges validates BareMetalHost resource on changes
// but also covers the validations of creation.
func (webhook *BareMetalHost) validateChanges(oldObj *metal3api.BareMetalHost, newObj *metal3api.BareMetalHost) []error {
	var errs []error

	if err := webhook.validateHost(newObj); err != nil {
		errs = append(errs, err...)
	}

	if oldObj.Spec.BMC.Address != "" &&
		newObj.Spec.BMC.Address != oldObj.Spec.BMC.Address &&
		newObj.Status.OperationalStatus != metal3api.OperationalStatusDetached &&
		newObj.Status.Provisioning.State != metal3api.StateRegistering {
		errs = append(errs, errors.New("BMC address can not be changed if the BMH is not in the Registering state, or if the BMH is not detached"))
	}

	if oldObj.Spec.BootMACAddress != "" && newObj.Spec.BootMACAddress != oldObj.Spec.BootMACAddress {
		errs = append(errs, errors.New("bootMACAddress can not be changed once it is set"))
	}

	if oldObj.Spec.ExternallyProvisioned != newObj.Spec.ExternallyProvisioned {
		errs = append(errs, errors.New("externallyProvisioned can not be changed"))
	}

	return errs
}

func validateBMCAccess(s metal3api.BareMetalHostSpec, bmcAccess bmc.AccessDetails) []error {
	var errs []error

	if bmcAccess == nil {
		return errs
	}

	if s.RAID != nil && len(s.RAID.HardwareRAIDVolumes) > 0 {
		if bmcAccess.RAIDInterface() == "no-raid" {
			errs = append(errs, fmt.Errorf("BMC driver %s does not support configuring RAID", bmcAccess.Type()))
		}
	}

	if s.Firmware != nil {
		if _, err := bmcAccess.BuildBIOSSettings((*bmc.FirmwareConfig)(s.Firmware)); err != nil {
			errs = append(errs, err)
		}
	}

	if bmcAccess.NeedsMAC() && s.BootMACAddress == "" {
		errs = append(errs, fmt.Errorf("BMC driver %s requires a BootMACAddress value", bmcAccess.Type()))
	}

	if s.BootMACAddress != "" {
		_, err := net.ParseMAC(s.BootMACAddress)
		if err != nil {
			errs = append(errs, err)
		}
	}

	if s.BootMode == metal3api.UEFISecureBoot && !bmcAccess.SupportsSecureBoot() {
		errs = append(errs, fmt.Errorf("BMC driver %s does not support secure boot", bmcAccess.Type()))
	}

	return errs
}

func validateRAID(r *metal3api.RAIDConfig) []error {
	var errs []error

	if r == nil {
		return nil
	}

	// check if both hardware and software RAID are specified
	if len(r.HardwareRAIDVolumes) > 0 && len(r.SoftwareRAIDVolumes) > 0 {
		errs = append(errs, errors.New("hardwareRAIDVolumes and softwareRAIDVolumes can not be set at the same time"))
	}

	for index, volume := range r.HardwareRAIDVolumes {
		// check if physicalDisks are specified without a controller
		if len(volume.PhysicalDisks) != 0 {
			if volume.Controller == "" {
				errs = append(errs, fmt.Errorf("'physicalDisks' specified without 'controller' in hardware RAID volume %d", index))
			}
		}
		// check if numberOfPhysicalDisks is not same as len(physicalDisks)
		if volume.NumberOfPhysicalDisks != nil && len(volume.PhysicalDisks) != 0 {
			if *volume.NumberOfPhysicalDisks != len(volume.PhysicalDisks) {
				errs = append(errs, fmt.Errorf("the 'numberOfPhysicalDisks'[%d] and number of 'physicalDisks'[%d] is not same for volume %d", *volume.NumberOfPhysicalDisks, len(volume.PhysicalDisks), index))
			}
		}
	}

	return errs
}

func validateBMHName(bmhname string) error {
	invalidname, _ := regexp.MatchString(`[^A-Za-z0-9\.\-\_]`, bmhname)
	if invalidname {
		return errors.New("BareMetalHost resource name cannot contain characters other than [A-Za-z0-9._-]")
	}

	_, err := uuid.Parse(bmhname)
	if err == nil {
		return errors.New("BareMetalHost resource name cannot be a UUID")
	}

	return nil
}

func validateDNSName(hostaddress string) error {
	if hostaddress == "" {
		return nil
	}

	_, err := bmc.GetParsedURL(hostaddress)
	return err // the error has enough context already
}

func validateAnnotations(host *metal3api.BareMetalHost) []error {
	var errs []error
	var err error

	for annotation, value := range host.Annotations {
		switch {
		case annotation == metal3api.StatusAnnotation:
			err = validateStatusAnnotation(value)
		case strings.HasPrefix(annotation, metal3api.RebootAnnotationPrefix+"/") || annotation == metal3api.RebootAnnotationPrefix:
			err = validateRebootAnnotation(value)
		case annotation == metal3api.InspectAnnotationPrefix:
			err = validateInspectAnnotation(value)
		case annotation == metal3api.HardwareDetailsAnnotation:
			inspect := host.Annotations[metal3api.InspectAnnotationPrefix]
			err = validateHwdDetailsAnnotation(value, inspect)
		default:
			err = nil
		}
		if err != nil {
			errs = append(errs, err)
		}
	}

	return errs
}

func validateStatusAnnotation(statusAnnotation string) error {
	if statusAnnotation != "" {
		objBMHStatus := &metal3api.BareMetalHostStatus{}

		deco := json.NewDecoder(strings.NewReader(statusAnnotation))
		deco.DisallowUnknownFields()
		if err := deco.Decode(objBMHStatus); err != nil {
			return fmt.Errorf("error decoding status annotation: %w", err)
		}

		if err := checkStatusAnnotation(objBMHStatus); err != nil {
			return err
		}
	}

	return nil
}

func validateImageURL(imageURL string) error {
	_, err := url.ParseRequestURI(imageURL)
	if err != nil {
		return fmt.Errorf("image URL %s is invalid: %w", imageURL, err)
	}

	return nil
}

func validateRootDeviceHints(rdh *metal3api.RootDeviceHints) error {
	if rdh == nil || rdh.DeviceName == "" {
		return nil
	}

	subpath := strings.TrimPrefix(rdh.DeviceName, "/dev/")
	if rdh.DeviceName == subpath {
		return fmt.Errorf("device name of root device hint must be a /dev/ path, not \"%s\"", rdh.DeviceName)
	}

	subpath = strings.TrimPrefix(subpath, "disk/by-path/")
	if strings.Contains(subpath, "/") {
		return fmt.Errorf("device name of root device hint must be path in /dev/ or /dev/disk/by-path/, not \"%s\"", rdh.DeviceName)
	}
	return nil
}

// When making changes to this function for operationalstatus and errortype,
// also make the corresponding changes in the OperationalStatus and
// ErrorType fields in the struct definition of BareMetalHostStatus in
// the file baremetalhost_types.go.
func checkStatusAnnotation(bmhStatus *metal3api.BareMetalHostStatus) error {
	if !slices.Contains(metal3api.OperationalStatusAllowed, string(bmhStatus.OperationalStatus)) {
		return fmt.Errorf("invalid operationalStatus '%s' in the %s annotation", string(bmhStatus.OperationalStatus), metal3api.StatusAnnotation)
	}

	if !slices.Contains(metal3api.ErrorTypeAllowed, string(bmhStatus.ErrorType)) {
		return fmt.Errorf("invalid errorType '%s' in the %s annotation", string(bmhStatus.ErrorType), metal3api.StatusAnnotation)
	}

	return nil
}

func validateHwdDetailsAnnotation(hwdDetAnnotation string, inspect string) error {
	if hwdDetAnnotation == "" {
		return nil
	}

	if inspect != "disabled" {
		return errors.New("when hardware details are provided, the inspect.metal3.io annotation must be set to disabled")
	}

	objHwdDet := &metal3api.HardwareDetails{}

	deco := json.NewDecoder(strings.NewReader(hwdDetAnnotation))
	deco.DisallowUnknownFields()
	if err := deco.Decode(objHwdDet); err != nil {
		return fmt.Errorf("error decoding the %s annotation: %w", metal3api.HardwareDetailsAnnotation, err)
	}

	return nil
}

func validateInspectAnnotation(inspectAnnotation string) error {
	if !slices.Contains(inspectAnnotationAllowed, inspectAnnotation) {
		return fmt.Errorf("invalid value for the %s annotation, allowed are %v", metal3api.InspectAnnotationPrefix, inspectAnnotationAllowedString)
	}

	return nil
}

func validateRebootAnnotation(rebootAnnotation string) error {
	if rebootAnnotation == "" {
		return nil
	}

	objStatus := &metal3api.RebootAnnotationArguments{}
	err := json.Unmarshal([]byte(rebootAnnotation), objStatus)
	if err != nil {
		return fmt.Errorf("failed to unmarshal the data from the %s annotation: %w", metal3api.RebootAnnotationPrefix, err)
	}

	if !slices.Contains(supportedRebootModes, string(objStatus.Mode)) {
		return fmt.Errorf("invalid mode in the %s annotation, allowed are %v", metal3api.RebootAnnotationPrefix, supportedRebootModesString)
	}

	return nil
}

// validateCrossNamespaceSecretReferences validates that a SecretReference does not refer to a Secret
// in a different namespace than the host resource.
func validateCrossNamespaceSecretReferences(hostNamespace, hostName, fieldName string, ref *corev1.SecretReference) error {
	if ref != nil &&
		ref.Namespace != "" &&
		ref.Namespace != hostNamespace {
		return k8serrors.NewForbidden(
			schema.GroupResource{
				Group:    "metal3.io",
				Resource: "baremetalhosts",
			},
			hostName,
			fmt.Errorf("%s: cross-namespace Secret references are not allowed", fieldName),
		)
	}
	return nil
}

// validateCrossNamespaceSecretReferences checks all Secret references in the BareMetalHost spec
// to ensure they do not reference Secrets from other namespaces. This includes userData,
// networkData, and metaData Secret references.
func (webhook *BareMetalHost) validateCrossNamespaceSecretReferences(host *metal3api.BareMetalHost) []error {
	secretRefs := map[*corev1.SecretReference]string{
		host.Spec.UserData:    "userData",
		host.Spec.NetworkData: "networkData",
		host.Spec.MetaData:    "metaData",
	}
	errs := []error{}
	for ref, fieldName := range secretRefs {
		if err := validateCrossNamespaceSecretReferences(host.Namespace, host.Name, fieldName, ref); err != nil {
			errs = append(errs, err)
		}
	}
	return errs
}

func validatePowerStatus(host *metal3api.BareMetalHost) error {
	if host.Spec.DisablePowerOff && !host.Spec.Online {
		return errors.New("node can't simultaneously have online set to false and have power off disabled")
	}
	return nil
}
