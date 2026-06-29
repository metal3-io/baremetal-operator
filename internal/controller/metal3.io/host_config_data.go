package controllers

import (
	"context"
	"errors"
	"fmt"

	"github.com/go-logr/logr"
	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	"github.com/metal3-io/baremetal-operator/pkg/secretutils"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
)

// hostInDeletionFlow reports whether the host is being removed. During this
// window a missing preprovisioning network Secret should not block progress.
func hostInDeletionFlow(host *metal3api.BareMetalHost) bool {
	if !host.DeletionTimestamp.IsZero() {
		return true
	}
	switch host.Status.Provisioning.State {
	case metal3api.StateDeleting, metal3api.StatePoweringOffBeforeDelete:
		return true
	default:
		return false
	}
}

// hostConfigData is an implementation of host configuration data interface.
// Object is able to retrieve data from secrets referenced in a host spec.
type hostConfigData struct {
	host          *metal3api.BareMetalHost
	log           logr.Logger
	secretManager secretutils.SecretManager
}

// Generic method for data extraction from a Secret. Function uses dataKey
// parameter to determine which data to return in case secret contains multiple
// keys. The addFinalizer parameter indicates whether or not a finalizer will be
// added to the secret.
func (hcd *hostConfigData) getSecretDataWithFinalizer(ctx context.Context, name, namespace, dataKey string, addFinalizer bool) (string, error) {
	if namespace != hcd.host.Namespace {
		return "", fmt.Errorf("%s secret must be in same namespace as host %s/%s", dataKey, hcd.host.Namespace, hcd.host.Name)
	}

	key := types.NamespacedName{
		Name:      name,
		Namespace: namespace,
	}

	secret, err := hcd.secretManager.ObtainSecretWithFinalizer(ctx, key, addFinalizer)
	if err != nil {
		return "", err
	}

	data, ok := secret.Data[dataKey]
	if ok {
		return string(data), nil
	}
	// There is no data under dataKey (userData or networkData).
	// Tring to falback to 'value' key
	if data, ok = secret.Data["value"]; !ok {
		hostConfigDataError.WithLabelValues(dataKey).Inc()
		return "", NoDataInSecretError{secret: name, key: dataKey}
	}

	return string(data), nil
}

// UserData get Operating System configuration data.
func (hcd *hostConfigData) UserData(ctx context.Context) (string, error) {
	if hcd.host.Spec.UserData == nil {
		hcd.log.Info("UserData is not set return empty string")
		return "", nil
	}
	namespace := hcd.host.Spec.UserData.Namespace
	if namespace == "" {
		namespace = hcd.host.Namespace
	}
	return hcd.getSecretDataWithFinalizer(
		ctx,
		hcd.host.Spec.UserData.Name,
		namespace,
		"userData",
		false,
	)
}

// NetworkData get network configuration.
func (hcd *hostConfigData) NetworkData(ctx context.Context) (string, error) {
	networkData := hcd.host.Spec.NetworkData
	if networkData == nil && hcd.host.Spec.PreprovisioningNetworkDataName != "" {
		networkData = &corev1.SecretReference{
			Name: hcd.host.Spec.PreprovisioningNetworkDataName,
		}
	}
	if networkData == nil {
		hcd.log.Info("NetworkData is not set, returning empty data")
		return "", nil
	}
	namespace := networkData.Namespace
	if namespace == "" {
		namespace = hcd.host.Namespace
	}
	networkDataRaw, err := hcd.getSecretDataWithFinalizer(
		ctx,
		networkData.Name,
		namespace,
		"networkData",
		false,
	)
	if err != nil {
		var noDataErr NoDataInSecretError
		if errors.As(err, &noDataErr) {
			hcd.log.Info("NetworkData key is not set, returning empty data")
			return "", nil
		}
	}
	return networkDataRaw, err
}

// PreprovisioningNetworkData get preprovisioning network configuration.
func (hcd *hostConfigData) PreprovisioningNetworkData(ctx context.Context) (string, error) {
	if hcd.host.Spec.PreprovisioningNetworkDataName == "" {
		return "", nil
	}
	addFinalizer := !hostInDeletionFlow(hcd.host)
	networkDataRaw, err := hcd.getSecretDataWithFinalizer(
		ctx,
		hcd.host.Spec.PreprovisioningNetworkDataName,
		hcd.host.Namespace,
		"networkData",
		addFinalizer,
	)
	if err != nil {
		var noDataErr NoDataInSecretError
		if errors.As(err, &noDataErr) {
			hcd.log.Info("PreprovisioningNetworkData networkData key is not set, returning empty data")
			return "", nil
		}
		if k8serrors.IsNotFound(err) && hostInDeletionFlow(hcd.host) {
			hcd.log.Info("PreprovisioningNetworkData secret not found during host deletion, returning empty data")
			return "", nil
		}
	}
	return networkDataRaw, err
}

// MetaData get host metatdata.
func (hcd *hostConfigData) MetaData(ctx context.Context) (string, error) {
	if hcd.host.Spec.MetaData == nil {
		hcd.log.Info("MetaData is not set returning empty(nil) data")
		return "", nil
	}
	namespace := hcd.host.Spec.MetaData.Namespace
	if namespace == "" {
		namespace = hcd.host.Namespace
	}
	return hcd.getSecretDataWithFinalizer(
		ctx,
		hcd.host.Spec.MetaData.Name,
		namespace,
		"metaData",
		false,
	)
}
