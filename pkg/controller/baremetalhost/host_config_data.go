package baremetalhost

import (
	"context"
	"fmt"

	"github.com/pkg/errors"

	metal3v1alpha1 "github.com/metal3-io/baremetal-operator/pkg/apis/metal3/v1alpha1"

	"github.com/go-logr/logr"

	corev1 "k8s.io/api/core/v1"

	"k8s.io/apimachinery/pkg/types"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

// hostConfigData is an implementation of host configuration data interface.
// Object is able to retrive data from secrets referenced in a host spec
type hostConfigData struct {
	host   *metal3v1alpha1.BareMetalHost
	log    logr.Logger
	client client.Client
}

// Generic method for data extraction from a Secret. Function uses dataKey
// parameter to detirmine which data to return in case secret contins multiple
// keys
func (hcd *hostConfigData) getSecretData(name, namespace, dataKey string) (string, error) {
	secret := &corev1.Secret{}
	key := types.NamespacedName{
		Name:      name,
		Namespace: namespace,
	}
	if err := hcd.client.Get(context.TODO(), key, secret); err != nil {
		errMsg := fmt.Sprintf("failed to fetch user data from secret %s defined in namespace %s", name, namespace)
		return "", errors.Wrap(err, errMsg)
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

// UserData get Operating System configuration data
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

// NetworkData get network configuration
func (hcd *hostConfigData) NetworkData() (string, error) {
	if hcd.host.Spec.NetworkData == nil {
		hcd.log.Info("NetworkData is not set returning epmty(nil) data")
		return "", nil
	}
	namespace := hcd.host.Spec.NetworkData.Namespace
	if namespace == "" {
		namespace = hcd.host.Namespace
	}
	return hcd.getSecretData(
		hcd.host.Spec.NetworkData.Name,
		namespace,
		"networkData",
	)
}

// MetaData get host metatdata
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
