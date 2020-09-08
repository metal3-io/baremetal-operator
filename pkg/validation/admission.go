package validation

import (
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/validation/field"

	metal3v1alpha1 "github.com/metal3-io/baremetal-operator/pkg/apis/metal3/v1alpha1"
	"github.com/metal3-io/baremetal-operator/pkg/bmc"
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
	if len(existingHosts) == 0 {
		return nil
	}

	var allErrs field.ErrorList
	bmcAccess, err := bmc.NewAccessDetails(host.Spec.BMC.Address, true)

	if err != nil {
		// If access details aren't set, there is nothing to validate.
		return nil
	}

	for _, existingHost := range existingHosts {
		// Don't compare against itself
		if old != nil && old.Namespace == existingHost.Namespace && old.Name == existingHost.Name {
			continue
		}

		existingBmc, err := bmc.NewAccessDetails(existingHost.Spec.BMC.Address, false)

		// If access details aren't set, there is nothing to validate.
		if err != nil {
			continue
		}

		// If the two hosts don't share drivers, we can assume that they don't conflict
		if existingBmc.Driver() != bmcAccess.Driver() {
			continue
		}

		if existingBmc.UniqueAccessPath() == bmcAccess.UniqueAccessPath() {
			allErrs = append(allErrs, field.Invalid(
				field.NewPath("spec", "bmc"),
				host.Spec.BMC,
				"is not unique"))
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
