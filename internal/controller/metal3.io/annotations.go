package controllers

import (
	"encoding/json"
	"fmt"

	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
)

// hasDetachedAnnotation checks for existence of baremetalhost.metal3.io/detached.
func hasDetachedAnnotation(host *metal3api.BareMetalHost) bool {
	annotations := host.GetAnnotations()
	if annotations != nil {
		if _, ok := annotations[metal3api.DetachedAnnotation]; ok {
			return true
		}
	}
	return false
}

// getDetachedAnnotation gets JSON argument of the annotation or nil.
func getDetachedAnnotation(host *metal3api.BareMetalHost) (*metal3api.DetachedAnnotationArguments, error) {
	value, present := host.GetAnnotations()[metal3api.DetachedAnnotation]
	if !present {
		return nil, nil //nolint:nilnil
	}

	result := &metal3api.DetachedAnnotationArguments{}
	if value != "" {
		if err := json.Unmarshal([]byte(value), result); err != nil {
			return nil, fmt.Errorf("unable to parse the content of the detached annotation: %w", err)
		}
	}

	return result, nil
}

func delayDeleteForDetachedHost(host *metal3api.BareMetalHost) bool {
	args, err := getDetachedAnnotation(host)

	// if the host is detached, but missing the annotation, also delay delete
	// to allow for the host to be re-attached
	if args == nil && err == nil && host.OperationalStatus() == metal3api.OperationalStatusDetached {
		return true
	}

	if args != nil {
		return args.DeleteAction == metal3api.DetachedDeleteActionDelay
	}

	// default behavior if these are missing or not json is to not delay
	return false
}
