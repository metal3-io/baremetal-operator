package bmc

import (
	"context"
	"errors"
	"fmt"

	metalkubev1alpha1 "github.com/metalkube/baremetal-operator/pkg/apis/metalkube/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Error is the error type usually returned by functions in the bmc
// package. It describes the host, whether or not this is a fatal error,
// and any host error messages that should be set as well as the actual
// error.
type Error struct {
	Host             *metalkubev1alpha1.BareMetalHost // the host this error is for
	SecretPath       string                           // the secret name
	DelayRequeue     bool                             // whether or not we should reqeue after this error
	SetHostError     bool                             // whether or not this error should result in a host.setErrorMessage()
	HostErrorMessage string                           // optional host error message to use for host.setErrorMessage()
	Err              error                            // Error is the error that occurred during the operation
}

func (e *Error) getErr() error {
	return e.Err
}

func (e *Error) Error() string {

	if e == nil {
		return "<nil>"
	}

	s := ""

	if e.Host.Name != "" {
		s += fmt.Sprintf("[host=%q]", e.Host.GetName())
	}

	if e.DelayRequeue == true {
		s += " " + "[DelayRequeue=true]"
	}

	if e.SetHostError == true {
		s += " " + "[SetHostError=true]"
	}

	s += " " + fmt.Sprintf("[secret=%q]", e.SecretPath)

	s += ": " + e.Err.Error()

	return s
}

// Various errors contained in bmcError
var (

	// errMissingCredentials is returned as a validation failure
	// reason when there are no credentials at all.
	ErrMissingCredentials = errors.New("Missing BMC connection details: Credentials")

	// errMissingAddress is returned as a validation failure
	// reason when there is no address for the BMC.
	ErrMissingAddress = errors.New("Missing BMC connection details: Address")

	// errMissingUsername is returned as a validation failure reason
	// when the credentials do not include a "username" field.
	ErrMissingUsername = errors.New("Missing BMC connection details: 'username' in credentials")

	// errMissingPassword is returned as a validation failure reason
	// when the credentials do not include a "password" field.
	ErrMissingPassword = errors.New("Missing BMC connection details: 'password' in credentials")

	// errMissingBMCSecret is returned when the BMC k8s secret
	// returns a NotFound
	ErrMissingBMCSecret = errors.New("Missing BMC k8s secret")

	// errFailedToRetrieveBMCSecret is returned when the BMC k8s secret
	// returns a NotFound
	ErrFailedToRetrieveBMCSecret = errors.New("Temporary failure in retrieving BMC k8s secret")
)

// Credentials holds the information for authenticating with the BMC.
type Credentials struct {
	Host            *metalkubev1alpha1.BareMetalHost //	the host these credentials are for
	Secret          *corev1.Secret                   // the actual secret for these credentials
	SecretName      string                           // the secret name
	SecretNameSpace string                           // the secret namespace
	SecretPath      string                           // namespace + "/" + name
	Username        string                           // the username for the BMC connection
	Password        string                           // the password for the BMC connection
}

// NewCredentials is the proper way to create and return a Credentials object
// containing the BMC credentials for a given host
func NewCredentials(k8sClient client.Client, host *metalkubev1alpha1.BareMetalHost) (bmcCreds *Credentials, err error) {

	// Handle missing BMC address
	if host.Spec.BMC.Address == "" {

		return nil, &Error{
			Err:              ErrMissingAddress,
			DelayRequeue:     true,
			Host:             host,
			SecretPath:       host.Spec.BMC.CredentialsName,
			SetHostError:     true,
			HostErrorMessage: ErrMissingAddress.Error() + " for host " + host.Name}
	}

	// Handle empty BMC credentials field
	if host.Spec.BMC.CredentialsName == "" {

		return nil, &Error{
			Err:              ErrMissingCredentials,
			DelayRequeue:     true,
			Host:             host,
			SecretPath:       host.Spec.BMC.CredentialsName,
			SetHostError:     true,
			HostErrorMessage: ErrMissingCredentials.Error() + " for host " + host.Name}

	}

	// Retrieve the BMC Secret
	secretKey := host.CredentialsKey()

	// Manufacturer an empty k8s secret
	bmcCredsSecret := &corev1.Secret{}

	err = k8sClient.Get(context.TODO(), secretKey, bmcCredsSecret)
	if err != nil {

		// We need to handle the case where the secret is not found
		// and distinguish that from a transient error

		if k8serrors.IsNotFound(err) {

			return nil, &Error{
				Err:              ErrMissingBMCSecret,
				DelayRequeue:     true,
				Host:             host,
				SecretPath:       host.Spec.BMC.CredentialsName,
				SetHostError:     true,
				HostErrorMessage: ErrMissingBMCSecret.Error() + " with secret " + host.Spec.BMC.CredentialsName}
		}

		return nil, &Error{
			Err:          ErrFailedToRetrieveBMCSecret,
			DelayRequeue: false,
			Host:         host,
			SecretPath:   bmcCredsSecret.Namespace + "/" + bmcCredsSecret.Name}

	}

	// extract the username and password from the secret
	username := string(bmcCredsSecret.Data["username"])
	password := string(bmcCredsSecret.Data["password"])

	// final check for empty fields
	if username == "" {
		return nil, &Error{
			Err:              ErrMissingUsername,
			DelayRequeue:     false,
			Host:             host,
			SecretPath:       bmcCredsSecret.Namespace + "/" + bmcCredsSecret.Name,
			SetHostError:     true,
			HostErrorMessage: ErrMissingUsername.Error() + " with secret " + host.Spec.BMC.CredentialsName}
	}

	if password == "" {
		return nil, &Error{
			Err:              ErrMissingPassword,
			DelayRequeue:     false,
			Host:             host,
			SecretPath:       bmcCredsSecret.Namespace + "/" + bmcCredsSecret.Name,
			SetHostError:     true,
			HostErrorMessage: ErrMissingPassword.Error() + " with secret " + host.Spec.BMC.CredentialsName}
	}

	bmcCreds = &Credentials{
		Host:       host,
		SecretPath: bmcCredsSecret.Namespace + "/" + bmcCredsSecret.Name,
		Username:   username,
		Password:   password,
		Secret:     bmcCredsSecret}

	return bmcCreds, nil
}
