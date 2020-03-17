package baremetalhost

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"fmt"
	retryablehttp "github.com/hashicorp/go-retryablehttp"
	"io/ioutil"
	"net/http"
	"strings"

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
		return hcd.getFullIgnitionData(string(data))
	}
	// There is no data under dataKey (userData or networkData).
	// Tring to falback to 'value' key
	if data, ok = secret.Data["value"]; !ok {
		hostConfigDataError.WithLabelValues(dataKey).Inc()
		return "", NoDataInSecretError{secret: name, key: dataKey}
	}

	return hcd.getFullIgnitionData(string(data))
}

// UserData get Operating System configuration data
func (hcd *hostConfigData) UserData() (string, error) {
	if hcd.host.Spec.UserData == nil {
		hcd.log.Info("UserData is not set return empty string")
		return "", nil
	}
	return hcd.getSecretData(
		hcd.host.Spec.UserData.Name,
		hcd.host.Spec.UserData.Namespace,
		"userData",
	)

}

// NetworkData get network configuration
func (hcd *hostConfigData) NetworkData() (string, error) {
	if hcd.host.Spec.NetworkData == nil {
		hcd.log.Info("NetworkData is not set returning epmty(nil) data")
		return "", nil
	}
	return hcd.getSecretData(
		hcd.host.Spec.NetworkData.Name,
		hcd.host.Spec.NetworkData.Namespace,
		"networkData",
	)
}

// Fetch Full ignition from Pointer ignition
func (hcd *hostConfigData) getFullIgnitionData(pointerIgnitionB64 string) (string, error) {
	// The pointerIgnition is in base64. Convert it to string
	pointerIgnition, err := base64.StdEncoding.DecodeString(pointerIgnitionB64)
	if err != nil {
		return "", err
	}
	var ignConf map[string]interface{}
	if err := json.Unmarshal(pointerIgnition, &ignConf); err != nil {
		return "", err
	}
	ignitionURL := ignConf["ignition"].(map[string]interface{})["config"].(map[string]interface{})["append"].([]interface{})[0].(map[string]interface{})["source"].(string)
	caCertRaw := ignConf["ignition"].(map[string]interface{})["security"].(map[string]interface{})["tls"].(map[string]interface{})["certificateAuthorities"].([]interface{})[0].(map[string]interface{})["source"].(string)
	caCertB64 := strings.TrimPrefix(caCertRaw, "data:text/plain;charset=utf-8;base64,")
	caCert, err := base64.StdEncoding.DecodeString(caCertB64)
	if err != nil {
		return "", err
	}

	var fullIgnition []byte
	if ignitionURL != "" {
		transport := &http.Transport{}
		caCertPool := x509.NewCertPool()
		caCertPool.AppendCertsFromPEM(caCert)
		transport.TLSClientConfig = &tls.Config{RootCAs: caCertPool}

		client := retryablehttp.NewClient()
		client.HTTPClient.Transport = transport

		// Get the data
		resp, err := client.Get(ignitionURL)
		if err != nil {
			return "", err
		}

		defer resp.Body.Close()
		fullIgnition, err = ioutil.ReadAll(resp.Body)
		if err != nil {
			return "", err
		}
	}
	return string(fullIgnition), err
}
