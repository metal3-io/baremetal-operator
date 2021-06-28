package controllers

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	metal3v1alpha1 "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	hostName      string = "myHostName"
	hostNamespace string = "myHostNamespace"
	schemaName    string = "schema-17e7ebad" // Hash generated from schema, change this if the schema is changed
)

var (
	iTrue      bool = true
	iFalse     bool = false
	minLength  int  = 0
	maxLength  int  = 20
	lowerBound int  = 0
	upperBound int  = 20
)

// Test support for HostFirmwareSettings in the BareMetalHostReconciler
func getTestHostReconciler(host *metal3v1alpha1.BareMetalHost) *BareMetalHostReconciler {

	c := fakeclient.NewFakeClient(host)
	reconciler := &BareMetalHostReconciler{
		Client:             c,
		ProvisionerFactory: nil,
		Log:                ctrl.Log.WithName("host_state_machine").WithName("BareMetalHost"),
	}

	return reconciler
}

// Test support for HostFirmwareSettings in the HostFirmwareSettingsReconciler
func getTestHFSReconciler(host *metal3v1alpha1.HostFirmwareSettings) *HostFirmwareSettingsReconciler {

	c := fakeclient.NewFakeClient(host)
	reconciler := &HostFirmwareSettingsReconciler{
		Client: c,
		Log:    ctrl.Log.WithName("test_reconciler").WithName("HostFirmwareSettings"),
	}

	return reconciler
}

func getDefaultHostReconcileInfo(host *metal3v1alpha1.BareMetalHost, name string, namespace string) *reconcileInfo {
	r := &reconcileInfo{
		log:     logf.Log.WithName("controllers").WithName("HostFirmwareSettings"),
		host:    host,
		request: ctrl.Request{},
	}
	r.request.NamespacedName = types.NamespacedName{Namespace: namespace, Name: name}

	return r
}

func getMockProvisioner(settings metal3v1alpha1.SettingsMap, schema map[string]metal3v1alpha1.SettingSchema) *hsfMockProvisioner {
	return &hsfMockProvisioner{
		Settings: settings,
		Schema:   schema,
		Error:    nil,
	}
}

type hsfMockProvisioner struct {
	Settings metal3v1alpha1.SettingsMap
	Schema   map[string]metal3v1alpha1.SettingSchema
	Error    error
}

func (m *hsfMockProvisioner) HasCapacity() (result bool, err error) {
	return
}

func (m *hsfMockProvisioner) ValidateManagementAccess(data provisioner.ManagementAccessData, credentialsChanged, force bool) (result provisioner.Result, provID string, err error) {
	return
}

func (m *hsfMockProvisioner) InspectHardware(data provisioner.InspectData, force, refresh bool) (result provisioner.Result, started bool, details *metal3v1alpha1.HardwareDetails, err error) {
	return
}

func (m *hsfMockProvisioner) UpdateHardwareState() (hwState provisioner.HardwareState, err error) {
	return
}

func (m *hsfMockProvisioner) Prepare(data provisioner.PrepareData, unprepared bool) (result provisioner.Result, started bool, err error) {
	return
}

func (m *hsfMockProvisioner) Adopt(data provisioner.AdoptData, force bool) (result provisioner.Result, err error) {
	return
}

func (m *hsfMockProvisioner) Provision(data provisioner.ProvisionData) (result provisioner.Result, err error) {
	return
}

func (m *hsfMockProvisioner) Deprovision(force bool) (result provisioner.Result, err error) {
	return
}

func (m *hsfMockProvisioner) Delete() (result provisioner.Result, err error) {
	return
}

func (m *hsfMockProvisioner) Detach() (result provisioner.Result, err error) {
	return
}

func (m *hsfMockProvisioner) PowerOn(force bool) (result provisioner.Result, err error) {
	return
}

func (m *hsfMockProvisioner) PowerOff(rebootMode metal3v1alpha1.RebootMode, force bool) (result provisioner.Result, err error) {
	return
}

func (m *hsfMockProvisioner) IsReady() (result bool, err error) {
	return
}

func (m *hsfMockProvisioner) GetFirmwareSettings(includeSchema bool) (settings metal3v1alpha1.SettingsMap, schema map[string]metal3v1alpha1.SettingSchema, err error) {

	return m.Settings, m.Schema, m.Error
}

func getSchema() *metal3v1alpha1.FirmwareSchema {

	schema := &metal3v1alpha1.FirmwareSchema{
		TypeMeta: metav1.TypeMeta{
			Kind:       "FirmwareSchema",
			APIVersion: "metal3.io/v1alpha1"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      schemaName,
			Namespace: hostNamespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: "metal3.io/v1alpha1",
					Kind:       "HostFirmwareSettings",
					Name:       hostName,
				},
			},
		},
	}

	return schema
}

func getCurrentSchemaSettings() map[string]metal3v1alpha1.SettingSchema {

	return map[string]metal3v1alpha1.SettingSchema{
		"AssetTag": {
			AttributeType: "String",
			MinLength:     &minLength,
			MaxLength:     &maxLength,
			Unique:        &iTrue,
		},
		"CustomPostMessage": {
			AttributeType: "String",
			MinLength:     &minLength,
			MaxLength:     &maxLength,
			Unique:        &iFalse,
			ReadOnly:      &iFalse,
		},
		"L2Cache": {
			AttributeType: "String",
			MinLength:     &minLength,
			MaxLength:     &maxLength,
			ReadOnly:      &iTrue,
		},
		"NetworkBootRetryCount": {
			AttributeType: "Integer",
			LowerBound:    &lowerBound,
			UpperBound:    &upperBound,
			ReadOnly:      &iFalse,
		},
		"ProcVirtualization": {
			AttributeType:   "Enumeration",
			AllowableValues: []string{"Enabled", "Disabled"},
			ReadOnly:        &iFalse,
		},
		"SecureBoot": {
			AttributeType:   "Enumeration",
			AllowableValues: []string{"Enabled", "Disabled"},
			ReadOnly:        &iTrue,
		},
	}
}

func createSchemaResource(ctx context.Context, r *BareMetalHostReconciler) {
	firmwareSchema := getSchema()
	firmwareSchema.Spec.Schema = getCurrentSchemaSettings()

	r.Client.Create(ctx, firmwareSchema)
	r.Client.Update(ctx, firmwareSchema) // needed to make sure ResourceVersion matches existing schemas
}

func getExpectedSchemaResource() *metal3v1alpha1.FirmwareSchema {
	firmwareSchema := getSchema()
	firmwareSchema.Spec.Schema = getCurrentSchemaSettings()

	return firmwareSchema
}

func createHFSResource(ctx context.Context, r *BareMetalHostReconciler, hfs *metal3v1alpha1.HostFirmwareSettings, createSchema bool) {

	hfs.Status = metal3v1alpha1.HostFirmwareSettingsStatus{
		Settings: metal3v1alpha1.SettingsMap{
			"CustomPostMessage":     "All tests passed",
			"L2Cache":               "10x256 KB",
			"NetworkBootRetryCount": "10",
			"ProcVirtualization":    "Enabled",
			"SecureBoot":            "Enabled",
			"AssetTag":              "X45672917",
		},
	}
	hfs.TypeMeta = metav1.TypeMeta{
		Kind:       "HostFirmwareSettings",
		APIVersion: "metal3.io/v1alpha1"}
	hfs.ObjectMeta = metav1.ObjectMeta{
		Name:      hostName,
		Namespace: hostNamespace}

	if createSchema {
		hfs.Status.FirmwareSchema =
			&metal3v1alpha1.SchemaReference{
				Name:      schemaName,
				Namespace: hostNamespace}
	}

	r.Client.Create(ctx, hfs)
	r.Client.Update(ctx, hfs)
}

// Test the hostfirmwaresettings reconciler functions
func TestStoreHostFirmwareSettings(t *testing.T) {

	testCases := []struct {
		Scenario string
		// the resource that the reconciler is managing
		CurrentHFSResource *metal3v1alpha1.HostFirmwareSettings
		// whether to create a schema resource before calling reconciler
		CreateSchemaResource bool
		// mock data returned from Ironic via the provisioner
		CurrentSettings metal3v1alpha1.SettingsMap
		// the expected created or updated resource
		ExpectedSettings *metal3v1alpha1.HostFirmwareSettings
	}{
		{
			Scenario: "inital hfs resource with no schema",
			CurrentHFSResource: &metal3v1alpha1.HostFirmwareSettings{
				TypeMeta: metav1.TypeMeta{
					Kind:       "HostFirmwareSettings",
					APIVersion: "metal3.io/v1alpha1"},
				ObjectMeta: metav1.ObjectMeta{
					Name:            hostName,
					Namespace:       hostNamespace,
					ResourceVersion: "1"},
				Spec: metal3v1alpha1.HostFirmwareSettingsSpec{
					Settings: metal3v1alpha1.DesiredSettingsMap{},
				},
				Status: metal3v1alpha1.HostFirmwareSettingsStatus{},
			},
			CreateSchemaResource: false,
			CurrentSettings: metal3v1alpha1.SettingsMap{
				"L2Cache":               "10x512 KB",
				"NetworkBootRetryCount": "20",
				"ProcVirtualization":    "Disabled",
				"SecureBoot":            "Enabled",
				"AssetTag":              "X45672917",
			},
			ExpectedSettings: &metal3v1alpha1.HostFirmwareSettings{
				Spec: metal3v1alpha1.HostFirmwareSettingsSpec{
					Settings: metal3v1alpha1.DesiredSettingsMap{
						"NetworkBootRetryCount": intstr.FromString("20"),
						"ProcVirtualization":    intstr.FromString("Disabled"),
					},
				},
				Status: metal3v1alpha1.HostFirmwareSettingsStatus{
					FirmwareSchema: &metal3v1alpha1.SchemaReference{
						Name:      schemaName,
						Namespace: hostNamespace,
					},
					Settings: metal3v1alpha1.SettingsMap{
						"AssetTag":              "X45672917",
						"L2Cache":               "10x512 KB",
						"NetworkBootRetryCount": "20",
						"ProcVirtualization":    "Disabled",
						"SecureBoot":            "Enabled",
					},
				},
			},
		},
		{
			Scenario: "inital hfs resource with existing schema",
			CurrentHFSResource: &metal3v1alpha1.HostFirmwareSettings{
				TypeMeta: metav1.TypeMeta{
					Kind:       "HostFirmwareSettings",
					APIVersion: "metal3.io/v1alpha1"},
				ObjectMeta: metav1.ObjectMeta{
					Name:            hostName,
					Namespace:       hostNamespace,
					ResourceVersion: "1"},
				Spec: metal3v1alpha1.HostFirmwareSettingsSpec{
					Settings: metal3v1alpha1.DesiredSettingsMap{},
				},
				Status: metal3v1alpha1.HostFirmwareSettingsStatus{},
			},
			CreateSchemaResource: true,
			CurrentSettings: metal3v1alpha1.SettingsMap{
				"L2Cache":               "10x512 KB",
				"NetworkBootRetryCount": "20",
				"ProcVirtualization":    "Disabled",
				"SecureBoot":            "Enabled",
				"AssetTag":              "X45672917",
			},
			ExpectedSettings: &metal3v1alpha1.HostFirmwareSettings{
				Spec: metal3v1alpha1.HostFirmwareSettingsSpec{
					Settings: metal3v1alpha1.DesiredSettingsMap{
						"NetworkBootRetryCount": intstr.FromString("20"),
						"ProcVirtualization":    intstr.FromString("Disabled"),
					},
				},
				Status: metal3v1alpha1.HostFirmwareSettingsStatus{
					FirmwareSchema: &metal3v1alpha1.SchemaReference{
						Name:      schemaName,
						Namespace: hostNamespace,
					},
					Settings: metal3v1alpha1.SettingsMap{
						"AssetTag":              "X45672917",
						"L2Cache":               "10x512 KB",
						"NetworkBootRetryCount": "20",
						"ProcVirtualization":    "Disabled",
						"SecureBoot":            "Enabled",
					},
				},
			},
		},
		{
			Scenario: "updated settings",
			CurrentHFSResource: &metal3v1alpha1.HostFirmwareSettings{
				TypeMeta: metav1.TypeMeta{
					Kind:       "HostFirmwareSettings",
					APIVersion: "metal3.io/v1alpha1"},
				ObjectMeta: metav1.ObjectMeta{
					Name:            hostName,
					Namespace:       hostNamespace,
					ResourceVersion: "2"},
				Spec: metal3v1alpha1.HostFirmwareSettingsSpec{
					Settings: metal3v1alpha1.DesiredSettingsMap{
						"NetworkBootRetryCount": intstr.FromString("10"),
						"ProcVirtualization":    intstr.FromString("Enabled"),
					},
				},
				Status: metal3v1alpha1.HostFirmwareSettingsStatus{
					FirmwareSchema: &metal3v1alpha1.SchemaReference{
						Name:      schemaName,
						Namespace: hostNamespace,
					},
					Settings: metal3v1alpha1.SettingsMap{
						"L2Cache":               "10x256 KB",
						"NetworkBootRetryCount": "10",
						"ProcVirtualization":    "Enabled",
					},
				},
			},
			CreateSchemaResource: true,
			CurrentSettings: metal3v1alpha1.SettingsMap{
				"L2Cache":               "10x512 KB",
				"NetworkBootRetryCount": "20",
				"ProcVirtualization":    "Disabled",
				"SecureBoot":            "Enabled",
				"AssetTag":              "X45672917",
			},
			ExpectedSettings: &metal3v1alpha1.HostFirmwareSettings{
				Spec: metal3v1alpha1.HostFirmwareSettingsSpec{
					Settings: metal3v1alpha1.DesiredSettingsMap{
						"NetworkBootRetryCount": intstr.FromString("10"),
						"ProcVirtualization":    intstr.FromString("Enabled"),
					},
				},
				Status: metal3v1alpha1.HostFirmwareSettingsStatus{
					FirmwareSchema: &metal3v1alpha1.SchemaReference{
						Name:      schemaName,
						Namespace: hostNamespace,
					},
					Settings: metal3v1alpha1.SettingsMap{
						"AssetTag":              "X45672917",
						"L2Cache":               "10x512 KB",
						"NetworkBootRetryCount": "20",
						"ProcVirtualization":    "Disabled",
						"SecureBoot":            "Enabled",
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Scenario, func(t *testing.T) {

			ctx := context.TODO()
			prov := getMockProvisioner(tc.CurrentSettings, getCurrentSchemaSettings())

			tc.ExpectedSettings.TypeMeta = metav1.TypeMeta{
				Kind:       "HostFirmwareSettings",
				APIVersion: "metal3.io/v1alpha1"}
			tc.ExpectedSettings.ObjectMeta = metav1.ObjectMeta{
				Name:            hostName,
				Namespace:       hostNamespace,
				ResourceVersion: "3"}

			hfs := tc.CurrentHFSResource
			r := getTestHFSReconciler(hfs)
			info := &rInfo{
				log: logf.Log.WithName("controllers").WithName("HostFirmwareSettings"),
				hfs: tc.CurrentHFSResource,
			}

			if tc.CreateSchemaResource {
				firmwareSchema := getSchema()
				firmwareSchema.Spec.Schema = getCurrentSchemaSettings()

				r.Client.Create(ctx, firmwareSchema)
				r.Client.Update(ctx, firmwareSchema) // in order to set resource version
			}

			err := r.updateHostFirmwareSettings(prov, info)
			assert.Equal(t, nil, err)

			// Check that resources get created or updated
			key := client.ObjectKey{
				Namespace: hfs.ObjectMeta.Namespace, Name: hfs.ObjectMeta.Name}
			actualSettings := &metal3v1alpha1.HostFirmwareSettings{}
			err = r.Client.Get(ctx, key, actualSettings)
			assert.Equal(t, nil, err)

			// Use the same time for expected and actual
			currentTime := metav1.Now()
			actualSettings.Status.ProvStatus.LastUpdated = &currentTime
			tc.ExpectedSettings.Status.ProvStatus.LastUpdated = &currentTime

			assert.Equal(t, tc.ExpectedSettings, actualSettings)

			key = client.ObjectKey{
				Namespace: hfs.ObjectMeta.Namespace, Name: schemaName}
			actualSchema := &metal3v1alpha1.FirmwareSchema{}
			err = r.Client.Get(ctx, key, actualSchema)
			assert.Equal(t, nil, err)
			expectedSchema := getExpectedSchemaResource()
			expectedSchema.ObjectMeta.ResourceVersion = "2"
			assert.Equal(t, expectedSchema, actualSchema)
		})
	}
}

// Test the function to get hostFirmwareSettings for cleaning in the baremtalhost_controller
func TestGetValidHostFirmwareSettings(t *testing.T) {

	testCases := []struct {
		Scenario string
		// the existing resources
		CurrentHFSResource   *metal3v1alpha1.HostFirmwareSettings
		CreateSchemaResource bool
		// the expected updated resource
		ExpectedStatusSettings metal3v1alpha1.SettingsMap
		ExpectedSpecSettings   metal3v1alpha1.DesiredSettingsMap
		ExpectedError          string
	}{
		{
			Scenario: "valid spec changes no schema",
			CurrentHFSResource: &metal3v1alpha1.HostFirmwareSettings{
				Spec: metal3v1alpha1.HostFirmwareSettingsSpec{
					Settings: metal3v1alpha1.DesiredSettingsMap{
						"L2Cache":               intstr.FromString("10x512 KB"),
						"ProcVirtualization":    intstr.FromString("Disabled"),
						"NetworkBootRetryCount": intstr.FromString("20"),
					},
				},
			},
			CreateSchemaResource: false,
			ExpectedStatusSettings: metal3v1alpha1.SettingsMap{
				"CustomPostMessage":     "All tests passed",
				"L2Cache":               "10x256 KB",
				"NetworkBootRetryCount": "10",
				"ProcVirtualization":    "Enabled",
				"SecureBoot":            "Enabled",
				"AssetTag":              "X45672917",
			},
			ExpectedSpecSettings: metal3v1alpha1.DesiredSettingsMap{
				"L2Cache":               intstr.FromString("10x512 KB"),
				"ProcVirtualization":    intstr.FromString("Disabled"),
				"NetworkBootRetryCount": intstr.FromString("20"),
			},
			ExpectedError: "",
		},
		{
			Scenario: "valid spec changes with schema",
			CurrentHFSResource: &metal3v1alpha1.HostFirmwareSettings{
				Spec: metal3v1alpha1.HostFirmwareSettingsSpec{
					Settings: metal3v1alpha1.DesiredSettingsMap{
						"CustomPostMessage":     intstr.FromString("All tests passed"),
						"ProcVirtualization":    intstr.FromString("Disabled"),
						"NetworkBootRetryCount": intstr.FromString("20"),
					},
				},
			},
			CreateSchemaResource: true,
			ExpectedStatusSettings: metal3v1alpha1.SettingsMap{
				"CustomPostMessage":     "All tests passed",
				"L2Cache":               "10x256 KB",
				"NetworkBootRetryCount": "10",
				"ProcVirtualization":    "Enabled",
				"SecureBoot":            "Enabled",
				"AssetTag":              "X45672917",
			},
			ExpectedSpecSettings: metal3v1alpha1.DesiredSettingsMap{
				"CustomPostMessage":     intstr.FromString("All tests passed"),
				"ProcVirtualization":    intstr.FromString("Disabled"),
				"NetworkBootRetryCount": intstr.FromString("20"),
			},
			ExpectedError: "",
		},
		{
			Scenario: "invalid string",
			CurrentHFSResource: &metal3v1alpha1.HostFirmwareSettings{
				Spec: metal3v1alpha1.HostFirmwareSettingsSpec{
					Settings: metal3v1alpha1.DesiredSettingsMap{
						"CustomPostMessage":     intstr.FromString("A really long POST message"),
						"ProcVirtualization":    intstr.FromString("Disabled"),
						"NetworkBootRetryCount": intstr.FromString("20"),
					},
				},
			},
			CreateSchemaResource:   true,
			ExpectedStatusSettings: nil,
			ExpectedSpecSettings:   nil,
			ExpectedError:          "Setting CustomPostMessage is invalid, string A really long POST message length is above range 20",
		},
		{
			Scenario: "invalid int",
			CurrentHFSResource: &metal3v1alpha1.HostFirmwareSettings{
				Spec: metal3v1alpha1.HostFirmwareSettingsSpec{
					Settings: metal3v1alpha1.DesiredSettingsMap{
						"CustomPostMessage":     intstr.FromString("All tests passed"),
						"ProcVirtualization":    intstr.FromString("Disabled"),
						"NetworkBootRetryCount": intstr.FromString("2000"),
					},
				},
			},
			CreateSchemaResource:   true,
			ExpectedStatusSettings: nil,
			ExpectedSpecSettings:   nil,
			ExpectedError:          "Setting NetworkBootRetryCount is invalid, integer 2000 is above range 20",
		},
		{
			Scenario: "invalid enum",
			CurrentHFSResource: &metal3v1alpha1.HostFirmwareSettings{
				Spec: metal3v1alpha1.HostFirmwareSettingsSpec{
					Settings: metal3v1alpha1.DesiredSettingsMap{
						"CustomPostMessage":     intstr.FromString("All tests passed"),
						"ProcVirtualization":    intstr.FromString("Not enabled"),
						"NetworkBootRetryCount": intstr.FromString("20"),
					},
				},
			},
			CreateSchemaResource:   true,
			ExpectedStatusSettings: nil,
			ExpectedSpecSettings:   nil,
			ExpectedError:          "Setting ProcVirtualization is invalid, unknown enumeration value - Not enabled",
		},
		{
			Scenario: "invalid name",
			CurrentHFSResource: &metal3v1alpha1.HostFirmwareSettings{
				Spec: metal3v1alpha1.HostFirmwareSettingsSpec{
					Settings: metal3v1alpha1.DesiredSettingsMap{
						"SomeNewSetting": intstr.FromString("foo"),
					},
				},
			},
			CreateSchemaResource:   true,
			ExpectedStatusSettings: nil,
			ExpectedSpecSettings:   nil,
			ExpectedError:          "Setting SomeNewSetting is not in the Status field",
		},
		{
			Scenario: "invalid password in spec",
			CurrentHFSResource: &metal3v1alpha1.HostFirmwareSettings{
				Spec: metal3v1alpha1.HostFirmwareSettingsSpec{
					Settings: metal3v1alpha1.DesiredSettingsMap{
						"CustomPostMessage":     intstr.FromString("All tests passed"),
						"ProcVirtualization":    intstr.FromString("Disabled"),
						"NetworkBootRetryCount": intstr.FromString("20"),
						"SysPassword":           intstr.FromString("Pa%$word"),
					},
				},
			},
			CreateSchemaResource:   true,
			ExpectedStatusSettings: nil,
			ExpectedSpecSettings:   nil,
			ExpectedError:          "Cannot set Password field",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Scenario, func(t *testing.T) {

			ctx := context.TODO()
			host := &metal3v1alpha1.BareMetalHost{}
			host.ObjectMeta.Name = hostName
			host.ObjectMeta.Namespace = hostNamespace

			r := getTestHostReconciler(host)
			info := getDefaultHostReconcileInfo(host, hostName, hostNamespace)

			// Create the resources using fakeclient
			if tc.CurrentHFSResource != nil {

				createHFSResource(ctx, r, tc.CurrentHFSResource, tc.CreateSchemaResource)
			}
			if tc.CreateSchemaResource {
				createSchemaResource(ctx, r)
			}

			hfs, err := r.getValidHostFirmwareSettings(info)
			if err == nil {
				assert.Equal(t, tc.ExpectedError, "")
			} else {
				assert.Equal(t, tc.ExpectedError, err.Error())
			}
			if tc.ExpectedStatusSettings != nil {
				assert.Equal(t, tc.ExpectedStatusSettings, hfs.Status.Settings)
				assert.Equal(t, tc.ExpectedSpecSettings, hfs.Spec.Settings)
			}
		})
	}
}
