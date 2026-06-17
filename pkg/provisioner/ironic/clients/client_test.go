package clients

import (
	"reflect"
	"testing"
	"time"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIronicClientInvalidAuthType(t *testing.T) {
	type args struct {
		ironicEndpoint string
		auth           AuthConfig
		tls            TLSConfig
	}

	badClientArgs := args{
		ironicEndpoint: "test",
		auth: AuthConfig{
			Type:     "unsupportedTYpe",
			Username: "username",
			Password: "password",
		},
		tls: TLSConfig{
			TrustedCAFile:         "",
			ClientCertificateFile: "",
			ClientPrivateKeyFile:  "",
			InsecureSkipVerify:    true,
			SkipClientSANVerify:   true,
		},
	}

	tests := []struct {
		name       string
		args       args
		wantClient *gophercloud.ServiceClient
		wantErr    bool
	}{
		{
			name:       "non supported auth type return error",
			args:       badClientArgs,
			wantClient: nil,
			wantErr:    true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotClient, err := IronicClient(tt.args.ironicEndpoint, tt.args.auth, tt.args.tls, 0)
			require.Error(t, err)
			assert.Nil(t, gotClient)
		})
	}
}

func TestIronicClientValidAuthType(t *testing.T) {
	type args struct {
		ironicEndpoint string
		auth           AuthConfig
		tls            TLSConfig
	}

	noAuthClientArgs := args{
		ironicEndpoint: "http://localhost",
		auth: AuthConfig{
			Type:     NoAuth,
			Username: "",
			Password: "",
		},
		tls: TLSConfig{
			TrustedCAFile:         "",
			ClientCertificateFile: "",
			ClientPrivateKeyFile:  "",
			InsecureSkipVerify:    true,
			SkipClientSANVerify:   true,
		},
	}
	noAuthClient, _ := IronicClient(noAuthClientArgs.ironicEndpoint, noAuthClientArgs.auth, noAuthClientArgs.tls, 0)

	noAuthTlsClientArgs := args{
		ironicEndpoint: "https://localhost",
		auth: AuthConfig{
			Type:     NoAuth,
			Username: "",
			Password: "",
		},
		tls: TLSConfig{
			TrustedCAFile:         "/path/to/ca/file.crt",
			ClientCertificateFile: "/path/to/cert/file.crt",
			ClientPrivateKeyFile:  "/path/to/cert/file.key",
			InsecureSkipVerify:    true,
			SkipClientSANVerify:   true,
		},
	}
	noAuthTlsClient, _ := IronicClient(noAuthTlsClientArgs.ironicEndpoint, noAuthTlsClientArgs.auth, noAuthTlsClientArgs.tls, 0)

	basicAuthClientArgs := args{
		ironicEndpoint: "http://localhost",
		auth: AuthConfig{
			Type:     HTTPBasicAuth,
			Username: "username",
			Password: "password",
		},
		tls: TLSConfig{
			TrustedCAFile:         "",
			ClientCertificateFile: "",
			ClientPrivateKeyFile:  "",
			InsecureSkipVerify:    true,
			SkipClientSANVerify:   true,
		},
	}
	basicAuthClient, _ := IronicClient(basicAuthClientArgs.ironicEndpoint, basicAuthClientArgs.auth, basicAuthClientArgs.tls, 0)

	basicAuthTlsClientArgs := args{
		ironicEndpoint: "https://localhost",
		auth: AuthConfig{
			Type:     HTTPBasicAuth,
			Username: "username",
			Password: "password",
		},
		tls: TLSConfig{
			TrustedCAFile:         "/path/to/ca/file.crt",
			ClientCertificateFile: "/path/to/cert/file.crt",
			ClientPrivateKeyFile:  "/path/to/cert/file.key",
			InsecureSkipVerify:    true,
			SkipClientSANVerify:   true,
		},
	}
	basicAuthTlsClient, _ := IronicClient(basicAuthTlsClientArgs.ironicEndpoint, basicAuthTlsClientArgs.auth, basicAuthTlsClientArgs.tls, 0)

	tests := []struct {
		name       string
		args       args
		wantClient *gophercloud.ServiceClient
	}{
		{
			name:       "noauth auth type without tls return no error",
			args:       noAuthClientArgs,
			wantClient: noAuthClient,
		},
		{
			name:       "noauth auth type with tls return no error",
			args:       noAuthTlsClientArgs,
			wantClient: noAuthTlsClient,
		},
		{
			name:       "basicauth auth type without tls return no error",
			args:       basicAuthClientArgs,
			wantClient: basicAuthClient,
		},
		{
			name:       "basicauth auth type with tls return no error",
			args:       basicAuthTlsClientArgs,
			wantClient: basicAuthTlsClient,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotClient, err := IronicClient(tt.args.ironicEndpoint, tt.args.auth, tt.args.tls, 0)
			require.NoError(t, err)
			assert.NotNil(t, gotClient)
			assert.Equalf(t, gotClient.Endpoint, tt.wantClient.Endpoint, "IronicClient() gotClient = %v, want %v", gotClient.Endpoint, tt.wantClient.Endpoint)
			assert.Truef(t, reflect.DeepEqual(gotClient.MoreHeaders, tt.wantClient.MoreHeaders), "IronicClient() gotClient = %v, want %v", gotClient.MoreHeaders, tt.wantClient.MoreHeaders)
		})
	}
}

func TestIronicClientCustomTimeout(t *testing.T) {
	tlsConf := TLSConfig{
		TrustedCAFile:         "",
		ClientCertificateFile: "",
		ClientPrivateKeyFile:  "",
		InsecureSkipVerify:    true,
		SkipClientSANVerify:   true,
	}

	customTimeout := 120 * time.Second
	gotClient, err := IronicClient("http://localhost", AuthConfig{Type: NoAuth}, tlsConf, customTimeout)
	require.NoError(t, err)
	assert.NotNil(t, gotClient)
	assert.Equal(t, customTimeout, gotClient.HTTPClient.Timeout)
}

func Test_updateHTTPClient(t *testing.T) {
	type args struct {
		client  *gophercloud.ServiceClient
		tlsConf TLSConfig
	}

	emptyTlsConfig := TLSConfig{
		TrustedCAFile:         "",
		ClientCertificateFile: "",
		ClientPrivateKeyFile:  "",
		InsecureSkipVerify:    true,
		SkipClientSANVerify:   true,
	}

	nonEmptyTlsConfig := TLSConfig{
		TrustedCAFile:         "/path/to/ca/file.crt",
		ClientCertificateFile: "/path/to/cert/file.crt",
		ClientPrivateKeyFile:  "/path/to/cert/file.key",
		InsecureSkipVerify:    true,
		SkipClientSANVerify:   true,
	}

	updatedTlsConfig := TLSConfig{
		TrustedCAFile:         "/path/to/ca/file.crt",
		ClientCertificateFile: "/path/to/cert/file.crt",
		ClientPrivateKeyFile:  "/path/to/cert/file.key",
		InsecureSkipVerify:    false,
		SkipClientSANVerify:   true,
	}

	emptyClient, _ := IronicClient("https://localhost", AuthConfig{
		Type:     NoAuth,
		Username: "",
		Password: "",
	},
		emptyTlsConfig, 0,
	)

	tests := []struct {
		name    string
		args    args
		timeout time.Duration
	}{
		{
			name: "tls config with empty values does not fail",
			args: args{
				client:  emptyClient,
				tlsConf: emptyTlsConfig,
			},
		},
		{
			name: "tls config with path to files update do not fail",
			args: args{
				client:  emptyClient,
				tlsConf: nonEmptyTlsConfig,
			},
		},
		{
			name: "tls config with InsecureSkipVerify update do not fail",
			args: args{
				client:  emptyClient,
				tlsConf: updatedTlsConfig,
			},
		},
		{
			name: "custom timeout",
			args: args{
				client:  emptyClient,
				tlsConf: emptyTlsConfig,
			},
			timeout: 120 * time.Second,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := updateHTTPClient(tt.args.client, tt.args.tlsConf, tt.timeout)
			require.NoError(t, err)
			expectedTimeout := tt.timeout
			if expectedTimeout == 0 {
				expectedTimeout = DefaultTimeout
			}
			assert.Equal(t, expectedTimeout, tt.args.client.HTTPClient.Timeout)
		})
	}
}
