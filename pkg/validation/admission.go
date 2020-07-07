package validation

import (
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/validation/field"

	metal3v1alpha1 "github.com/metal3-io/baremetal-operator/pkg/apis/metal3/v1alpha1"
)

// ValidateHostForAdmission takes a list of existing hosts, and a host we are
// trying to add to the cluster.  It returns an error if the host fails
// uniqueness validation, and cannot be added to the cluster.  The optional
// third argument `old` denotes the previous version of `host` in the case of an
// update.
//
// This is designed to be run by an admission webhook, `ValidateCreate`, and
// `ValidateUpdate`.
func ValidateHostForAdmission(existingHosts []metal3v1alpha1.BareMetalHost, host metal3v1alpha1.BareMetalHost, old *metal3v1alpha1.BareMetalHost) error {
	if host.Spec.BMC.Address == "" && host.Spec.BootMACAddress == "" && host.Spec.BMC.CredentialsName == "" {
		return nil
	}

	if len(existingHosts) == 0 {
		return nil
	}

	var allErrs field.ErrorList

	for _, existingHost := range existingHosts {
		if old != nil && old.Namespace == existingHost.Namespace && old.Name == existingHost.Name {
			continue
		}

		// The BMC address is optional so only validate it if it's set
		if host.Spec.BMC.Address != "" && host.Spec.BMC.Address == existingHost.Spec.BMC.Address {
			allErrs = append(allErrs, field.Invalid(
				field.NewPath("spec", "bmc", "address"),
				host.Spec.BMC.Address,
				"is not unique",
			))
		}

		// The BootMACAddress is optional so only validate it if it's set
		if host.Spec.BootMACAddress != "" && host.Spec.BootMACAddress == existingHost.Spec.BootMACAddress {
			allErrs = append(allErrs, field.Invalid(
				field.NewPath("spec", "bootMACAddress"),
				host.Spec.BootMACAddress,
				"is not unique",
			))
		}

		// The BMC credentials is optional so only validate it if it's set
		if host.Spec.BMC.CredentialsName != "" && host.Spec.BMC.CredentialsName == existingHost.Spec.BMC.CredentialsName {
			allErrs = append(allErrs, field.Invalid(
				field.NewPath("spec", "bmc", "credentialsName"),
				host.Spec.BMC.CredentialsName,
				"is not unique",
			))
		}
	}

	if len(allErrs) == 0 {
		return nil
	}

	return apierrors.NewInvalid(
		metal3v1alpha1.SchemeGroupVersion.WithKind("BareMetalHost").GroupKind(),
		host.Name,
		allErrs)
}
