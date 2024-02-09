package controllers

import (
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	"github.com/metal3-io/baremetal-operator/pkg/secretutils"
)

// hostConfigData is an implementation of host configuration data interface.
// Object is able to retrieve data from secrets referenced in a host spec.
type hostConfigData struct {
	host          *metal3api.BareMetalHost
	log           logr.Logger
	secretManager secretutils.SecretManager
}

// Generic method for data extraction from a Secret. Function uses dataKey
// parameter to detirmine which data to return in case secret contins multiple
// keys.
func (hcd *hostConfigData) getSecretData(name, namespace, dataKey string) (string, error) {
	key := types.NamespacedName{
		Name:      name,
		Namespace: namespace,
	}

	secret, err := hcd.secretManager.ObtainSecret(key)
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
func (hcd *hostConfigData) UserData() (string, error) {
	if hcd.host.Spec.UserData == nil {
		hcd.log.Info("UserData is not set return empty string")
		return "", nil
	}
	namespace := hcd.host.Spec.UserData.Namespace
	if namespace == "" {
		namespace = hcd.host.Namespace
	}
	return hcd.getSecretData(
		hcd.host.Spec.UserData.Name,
		namespace,
		"userData",
	)
}

// NetworkData get network configuration.
func (hcd *hostConfigData) NetworkData() (string, error) {
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
	networkDataRaw, err := hcd.getSecretData(
		networkData.Name,
		namespace,
		"networkData",
	)
	if err != nil {
		_, isNoDataErr := err.(NoDataInSecretError)
		if isNoDataErr {
			hcd.log.Info("NetworkData key is not set, returning empty data")
			return "", nil
		}
	}
	return networkDataRaw, err
}

// PreprovisioningNetworkData get preprovisioning network configuration.
func (hcd *hostConfigData) PreprovisioningNetworkData() (string, error) {
	if hcd.host.Spec.PreprovisioningNetworkDataName == "" {
		return "", nil
	}
	networkDataRaw, err := hcd.getSecretData(
		hcd.host.Spec.PreprovisioningNetworkDataName,
		hcd.host.Namespace,
		"networkData",
	)
	if err != nil {
		_, isNoDataErr := err.(NoDataInSecretError)
		if isNoDataErr {
			hcd.log.Info("PreprovisioningNetworkData networkData key is not set, returning empty data")
			return "", nil
		}
	}
	return networkDataRaw, err
}

// MetaData get host metatdata.
func (hcd *hostConfigData) MetaData() (string, error) {
	if hcd.host.Spec.MetaData == nil {
		hcd.log.Info("MetaData is not set returning empty(nil) data")
		return "", nil
	}
	namespace := hcd.host.Spec.MetaData.Namespace
	if namespace == "" {
		namespace = hcd.host.Namespace
	}
	return hcd.getSecretData(
		hcd.host.Spec.MetaData.Name,
		namespace,
		"metaData",
	)
}
