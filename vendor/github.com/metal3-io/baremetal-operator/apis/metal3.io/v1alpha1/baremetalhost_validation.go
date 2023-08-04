package v1alpha1

import (
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"github.com/pkg/errors"
	"golang.org/x/exp/slices"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/metal3-io/baremetal-operator/pkg/hardwareutils/bmc"
)

// log is for logging in this package.
var log = logf.Log.WithName("baremetalhost-validation")

// validateHost validates BareMetalHost resource for creation
func (host *BareMetalHost) validateHost() []error {
	log.Info("validate create", "name", host.Name)
	var errs []error
	var bmcAccess bmc.AccessDetails

	if host.Spec.BMC.Address != "" {
		var err error
		bmcAccess, err = bmc.NewAccessDetails(host.Spec.BMC.Address, host.Spec.BMC.DisableCertificateVerification)
		if err != nil {
			errs = append(errs, err)
		}
	}

	if raid_errors := validateRAID(host.Spec.RAID); raid_errors != nil {
		errs = append(errs, raid_errors...)
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

	return errs
}

// validateChanges validates BareMetalHost resource on changes
// but also covers the validations of creation
func (host *BareMetalHost) validateChanges(old *BareMetalHost) []error {
	log.Info("validate update", "name", host.Name)
	var errs []error

	if err := host.validateHost(); err != nil {
		errs = append(errs, err...)
	}

	if old.Spec.BMC.Address != "" && host.Spec.BMC.Address != old.Spec.BMC.Address {
		errs = append(errs, fmt.Errorf("BMC address can not be changed once it is set"))
	}

	if old.Spec.BootMACAddress != "" && host.Spec.BootMACAddress != old.Spec.BootMACAddress {
		errs = append(errs, fmt.Errorf("bootMACAddress can not be changed once it is set"))
	}

	return errs
}

func validateBMCAccess(s BareMetalHostSpec, bmcAccess bmc.AccessDetails) []error {
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

	if s.BootMode == UEFISecureBoot && !bmcAccess.SupportsSecureBoot() {
		errs = append(errs, fmt.Errorf("BMC driver %s does not support secure boot", bmcAccess.Type()))
	}

	return errs
}

func validateRAID(r *RAIDConfig) []error {
	var errors []error

	if r == nil {
		return nil
	}

	// check if both hardware and software RAID are specified
	if len(r.HardwareRAIDVolumes) > 0 && len(r.SoftwareRAIDVolumes) > 0 {
		errors = append(errors, fmt.Errorf("hardwareRAIDVolumes and softwareRAIDVolumes can not be set at the same time"))
	}

	for index, volume := range r.HardwareRAIDVolumes {
		// check if physicalDisks are specified without a controller
		if len(volume.PhysicalDisks) != 0 {
			if volume.Controller == "" {
				errors = append(errors, fmt.Errorf("'physicalDisks' specified without 'controller' in hardware RAID volume %d", index))
			}
		}
		// check if numberOfPhysicalDisks is not same as len(physicalDisks)
		if volume.NumberOfPhysicalDisks != nil && len(volume.PhysicalDisks) != 0 {
			if *volume.NumberOfPhysicalDisks != len(volume.PhysicalDisks) {
				errors = append(errors, fmt.Errorf("the 'numberOfPhysicalDisks'[%d] and number of 'physicalDisks'[%d] is not same for volume %d", *volume.NumberOfPhysicalDisks, len(volume.PhysicalDisks), index))
			}
		}
	}

	return errors
}

func validateBMHName(bmhname string) error {

	invalidname, _ := regexp.MatchString(`[^A-Za-z0-9\.\-\_]`, bmhname)
	if invalidname {
		return fmt.Errorf("BareMetalHost resource name cannot contain characters other than [A-Za-z0-9._-]")
	}

	_, err := uuid.Parse(bmhname)
	if err == nil {
		return fmt.Errorf("BareMetalHost resource name cannot be a UUID")
	}

	return nil
}

func validateDNSName(hostaddress string) error {

	if hostaddress == "" {
		return nil
	}

	_, err := bmc.GetParsedURL(hostaddress)
	if err != nil {
		return errors.Wrap(err, "BMO validation")
	}

	return nil
}

func validateAnnotations(host *BareMetalHost) []error {
	var errs []error
	var err error

	for annotation, value := range host.Annotations {

		switch {
		case annotation == StatusAnnotation:
			err = validateStatusAnnotation(value)
		case strings.HasPrefix(annotation, RebootAnnotationPrefix+"/") || annotation == RebootAnnotationPrefix:
			err = validateRebootAnnotation(value)
		case annotation == InspectAnnotationPrefix:
			err = validateInspectAnnotation(value)
		case annotation == HardwareDetailsAnnotation:
			inspect := host.Annotations[InspectAnnotationPrefix]
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

		objBMHStatus := &BareMetalHostStatus{}

		deco := json.NewDecoder(strings.NewReader(statusAnnotation))
		deco.DisallowUnknownFields()
		if err := deco.Decode(objBMHStatus); err != nil {
			return fmt.Errorf("error decoding status annotation, error=%w", err)
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
		return fmt.Errorf("Image URL %s is an invalid URL", imageURL)
	}

	return nil
}

func validateRootDeviceHints(rdh *RootDeviceHints) error {
	if rdh == nil || rdh.DeviceName == "" {
		return nil
	}

	subpath := strings.TrimPrefix(rdh.DeviceName, "/dev/")
	if rdh.DeviceName == subpath {
		return fmt.Errorf("Device Name of root device hint must be a /dev/ path, not \"%s\"", rdh.DeviceName)
	}

	subpath = strings.TrimPrefix(subpath, "disk/by-path/")
	if strings.Contains(subpath, "/") {
		return fmt.Errorf("Device Name of root device hint must be path in /dev/ or /dev/disk/by-path/, not \"%s\"", rdh.DeviceName)
	}
	return nil
}

// When making changes to this function for operationalstatus and errortype,
// also make the corresponding changes in the OperationalStatus and
// ErrorType fields in the struct definition of BareMetalHostStatus in
// the file baremetalhost_types.go
func checkStatusAnnotation(bmhStatus *BareMetalHostStatus) error {

	if !slices.Contains(OperationalStatusAllowed, string(bmhStatus.OperationalStatus)) {
		return fmt.Errorf("invalid OperationalStatus='%s' in StatusAnnotation", string(bmhStatus.OperationalStatus))
	}

	if !slices.Contains(ErrorTypeAllowed, string(bmhStatus.ErrorType)) {
		return fmt.Errorf("invalid ErrorType='%s' in StatusAnnotation", string(bmhStatus.ErrorType))
	}

	return nil
}

func validateHwdDetailsAnnotation(hwdDetAnnotation string, inspect string) error {
	if hwdDetAnnotation == "" {
		return nil
	}

	if inspect != "disabled" {
		return fmt.Errorf("inspection has to be disabled for HardwareDetailsAnnotation, check if {'inspect.metal3.io' : 'disabled'}")
	}

	objHwdDet := &HardwareDetails{}

	deco := json.NewDecoder(strings.NewReader(hwdDetAnnotation))
	deco.DisallowUnknownFields()
	if err := deco.Decode(objHwdDet); err != nil {
		return fmt.Errorf("error decoding hardware details annotation, error=%w", err)
	}

	return nil
}

func validateInspectAnnotation(inspectAnnotation string) error {

	inspectAnnotationAllowed := []string{"disabled", ""}

	if !slices.Contains(inspectAnnotationAllowed, inspectAnnotation) {
		return fmt.Errorf("invalid value for Inspect Annotation")
	}

	return nil
}

func validateRebootAnnotation(rebootAnnotation string) error {
	if rebootAnnotation == "" {
		return nil
	}

	objStatus := &RebootAnnotationArguments{}
	err := json.Unmarshal([]byte(rebootAnnotation), objStatus)
	if err != nil {
		return fmt.Errorf("failed to fetch Reboot from annotation, %w", err)
	}

	supportedRebootModes := []string{"hard", "soft", ""}

	if !slices.Contains(supportedRebootModes, string(objStatus.Mode)) {
		return fmt.Errorf("invalid RebootMode in RebootAnnotation")
	}

	return nil
}
