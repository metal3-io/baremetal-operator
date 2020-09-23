package controllers

import (
	"encoding/json"

	metal3iov1alpha1 "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	"github.com/metal3-io/baremetal-operator/pkg/utils"
	"github.com/pkg/errors"
)

func hostHasStatus(host *metal3iov1alpha1.BareMetalHost) bool {
	return !host.Status.LastUpdated.IsZero()
}

func hostHasFinalizer(host *metal3iov1alpha1.BareMetalHost) bool {
	return utils.StringInList(host.Finalizers, metal3iov1alpha1.BareMetalHostFinalizer)
}

// extract host from Status annotation
func getHostStatusFromAnnotation(host *metal3iov1alpha1.BareMetalHost) (*metal3iov1alpha1.BareMetalHostStatus, error) {
	annotations := host.GetAnnotations()
	content := []byte(annotations[metal3iov1alpha1.StatusAnnotation])
	if annotations[metal3iov1alpha1.StatusAnnotation] == "" {
		return nil, nil
	}
	objStatus, err := unmarshalStatusAnnotation(content)
	if err != nil {
		return nil, err
	}
	return objStatus, nil
}

func unmarshalStatusAnnotation(content []byte) (*metal3iov1alpha1.BareMetalHostStatus, error) {
	objStatus := &metal3iov1alpha1.BareMetalHostStatus{}
	if err := json.Unmarshal(content, objStatus); err != nil {
		return nil, errors.Wrap(err, "Failed to fetch Status from annotation")
	}
	return objStatus, nil
}
