package clients

import (
	"reflect"
	"testing"

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
			gotClient, err := IronicClient(tt.args.ironicEndpoint, tt.args.auth, tt.args.tls)
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
	noAuthClient, _ := IronicClient(noAuthClientArgs.ironicEndpoint, noAuthClientArgs.auth, noAuthClientArgs.tls)

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
	noAuthTlsClient, _ := IronicClient(noAuthTlsClientArgs.ironicEndpoint, noAuthTlsClientArgs.auth, noAuthTlsClientArgs.tls)

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
	basicAuthClient, _ := IronicClient(basicAuthClientArgs.ironicEndpoint, basicAuthClientArgs.auth, basicAuthClientArgs.tls)

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
	basicAuthTlsClient, _ := IronicClient(basicAuthTlsClientArgs.ironicEndpoint, basicAuthTlsClientArgs.auth, basicAuthTlsClientArgs.tls)

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
			gotClient, err := IronicClient(tt.args.ironicEndpoint, tt.args.auth, tt.args.tls)
			require.NoError(t, err)
			assert.NotNil(t, gotClient)
			assert.Equalf(t, gotClient.Endpoint, tt.wantClient.Endpoint, "IronicClient() gotClient = %v, want %v", gotClient.Endpoint, tt.wantClient.Endpoint)
			assert.Truef(t, reflect.DeepEqual(gotClient.MoreHeaders, tt.wantClient.MoreHeaders), "IronicClient() gotClient = %v, want %v", gotClient.MoreHeaders, tt.wantClient.MoreHeaders)
		})
	}
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
		emptyTlsConfig,
	)

	tests := []struct {
		name string
		args args
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := updateHTTPClient(tt.args.client, tt.args.tlsConf)
			require.NoError(t, err)
		})
	}
}
