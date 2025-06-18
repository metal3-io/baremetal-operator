package controllers

import (
	"context"
	"testing"

	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	"github.com/metal3-io/baremetal-operator/pkg/hardwareutils/bmc"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner/fixture"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	hostName      string = "myHostName"
	hostNamespace string = "myHostNamespace"
	schemaName    string = "schema-4bcc035f" // Hash generated from schema, change this if the schema is changed
)

var (
	iTrue      = true
	iFalse     = false
	minLength  = 0
	maxLength  = 20
	lowerBound = 0
	upperBound = 20
)

// Test support for HostFirmwareSettings in the HostFirmwareSettingsReconciler.
func getTestHFSReconciler(host *metal3api.HostFirmwareSettings) *HostFirmwareSettingsReconciler {
	c := fakeclient.NewClientBuilder().WithRuntimeObjects(host).WithStatusSubresource(host).
		Build()

	reconciler := &HostFirmwareSettingsReconciler{
		Client: c,
		Log:    ctrl.Log.WithName("test_reconciler").WithName("HostFirmwareSettings"),
	}

	return reconciler
}

func getMockProvisioner(host *metal3api.BareMetalHost, settings metal3api.SettingsMap, schema map[string]metal3api.SettingSchema) provisioner.Provisioner {
	state := fixture.Fixture{
		HostFirmwareSettings: fixture.HostFirmwareSettingsMock{
			Settings: settings,
			Schema:   schema,
		},
	}
	p, _ := state.NewProvisioner(context.TODO(), provisioner.BuildHostData(*host, bmc.Credentials{}),
		func(reason, message string) {})
	return p
}

func getSchema() *metal3api.FirmwareSchema {
	schema := &metal3api.FirmwareSchema{
		ObjectMeta: metav1.ObjectMeta{
			Name:      schemaName,
			Namespace: hostNamespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: "metal3.io/v1alpha1",
					Kind:       "HostFirmwareSettings",
					Name:       "dummyhfs",
				},
			},
		},
	}

	return schema
}

// Mock settings to return from provisioner.
func getCurrentSettings() metal3api.SettingsMap {
	return metal3api.SettingsMap{
		"L2Cache":               "10x512 KB",
		"NetworkBootRetryCount": "20",
		"ProcVirtualization":    "Disabled",
		"SecureBoot":            "Enabled",
		"AssetTag":              "X45672917",
	}
}

// Mock schema to return from provisioner.
func getCurrentSchemaSettings() map[string]metal3api.SettingSchema {
	return map[string]metal3api.SettingSchema{
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

// Create the baremetalhost reconciler and use that to create bmh in same namespace.
func createBaremetalHost() *metal3api.BareMetalHost {
	bmh := &metal3api.BareMetalHost{}
	bmh.ObjectMeta = metav1.ObjectMeta{Name: hostName, Namespace: hostNamespace}
	c := fakeclient.NewFakeClient(bmh)

	reconciler := &BareMetalHostReconciler{
		Client:             c,
		ProvisionerFactory: nil,
		Log:                ctrl.Log.WithName("bmh_reconciler").WithName("BareMetalHost"),
	}

	_ = reconciler.Create(context.TODO(), bmh)

	return bmh
}

func getExpectedSchema() *metal3api.FirmwareSchema {
	firmwareSchema := getSchema()
	firmwareSchema.ObjectMeta.ResourceVersion = "1"
	firmwareSchema.ObjectMeta.OwnerReferences = []metav1.OwnerReference{
		{
			APIVersion: "metal3.io/v1alpha1",
			Kind:       "HostFirmwareSettings",
			Name:       hostName,
		},
	}
	firmwareSchema.Spec.Schema = getCurrentSchemaSettings()

	return firmwareSchema
}

func getExpectedSchemaTwoOwners() *metal3api.FirmwareSchema {
	firmwareSchema := getSchema()
	firmwareSchema.ObjectMeta.ResourceVersion = "2"
	firmwareSchema.ObjectMeta.OwnerReferences = []metav1.OwnerReference{
		{
			APIVersion: "metal3.io/v1alpha1",
			Kind:       "HostFirmwareSettings",
			Name:       "dummyhfs",
		},
		{
			APIVersion: "metal3.io/v1alpha1",
			Kind:       "HostFirmwareSettings",
			Name:       hostName,
		},
	}
	firmwareSchema.Spec.Schema = getCurrentSchemaSettings()

	return firmwareSchema
}

// Create an HFS with input spec settings.
func getHFS(spec metal3api.HostFirmwareSettingsSpec) *metal3api.HostFirmwareSettings {
	hfs := &metal3api.HostFirmwareSettings{}

	hfs.Status = metal3api.HostFirmwareSettingsStatus{
		Settings: metal3api.SettingsMap{
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

	hfs.Spec = spec

	return hfs
}

// Test the hostfirmwaresettings reconciler functions.
func TestStoreHostFirmwareSettings(t *testing.T) {
	testCases := []struct {
		Scenario string
		// the resource that the reconciler is managing
		CurrentHFSResource *metal3api.HostFirmwareSettings
		// whether to create a schema resource before calling reconciler
		CreateSchemaResource bool
		// the expected created or updated resource
		ExpectedSettings *metal3api.HostFirmwareSettings
		// whether the spec values pass the validity test
		SpecIsValid bool
	}{
		{
			Scenario: "initial hfs resource with no schema",
			CurrentHFSResource: &metal3api.HostFirmwareSettings{
				TypeMeta: metav1.TypeMeta{
					Kind:       "HostFirmwareSettings",
					APIVersion: "metal3.io/v1alpha1"},
				ObjectMeta: metav1.ObjectMeta{
					Name:            hostName,
					Namespace:       hostNamespace,
					ResourceVersion: "1"},
				Spec: metal3api.HostFirmwareSettingsSpec{
					Settings: metal3api.DesiredSettingsMap{},
				},
				Status: metal3api.HostFirmwareSettingsStatus{},
			},
			CreateSchemaResource: false,
			ExpectedSettings: &metal3api.HostFirmwareSettings{
				Spec: metal3api.HostFirmwareSettingsSpec{
					Settings: metal3api.DesiredSettingsMap{},
				},
				Status: metal3api.HostFirmwareSettingsStatus{
					FirmwareSchema: &metal3api.SchemaReference{
						Name:      schemaName,
						Namespace: hostNamespace,
					},
					Settings: metal3api.SettingsMap{
						"AssetTag":              "X45672917",
						"L2Cache":               "10x512 KB",
						"NetworkBootRetryCount": "20",
						"ProcVirtualization":    "Disabled",
						"SecureBoot":            "Enabled",
					},
					Conditions: []metav1.Condition{
						{Type: "Valid", Status: "True", Reason: "Success"},
						{Type: "ChangeDetected", Status: "False", Reason: "Success"},
					},
				},
			},
			SpecIsValid: true,
		},
		{
			Scenario: "initial hfs resource with existing schema",
			CurrentHFSResource: &metal3api.HostFirmwareSettings{
				TypeMeta: metav1.TypeMeta{
					Kind:       "HostFirmwareSettings",
					APIVersion: "metal3.io/v1alpha1"},
				ObjectMeta: metav1.ObjectMeta{
					Name:            hostName,
					Namespace:       hostNamespace,
					ResourceVersion: "1"},
				Spec: metal3api.HostFirmwareSettingsSpec{
					Settings: metal3api.DesiredSettingsMap{},
				},
				Status: metal3api.HostFirmwareSettingsStatus{},
			},
			CreateSchemaResource: true,
			ExpectedSettings: &metal3api.HostFirmwareSettings{
				Spec: metal3api.HostFirmwareSettingsSpec{
					Settings: metal3api.DesiredSettingsMap{},
				},
				Status: metal3api.HostFirmwareSettingsStatus{
					FirmwareSchema: &metal3api.SchemaReference{
						Name:      schemaName,
						Namespace: hostNamespace,
					},
					Settings: metal3api.SettingsMap{
						"AssetTag":              "X45672917",
						"L2Cache":               "10x512 KB",
						"NetworkBootRetryCount": "20",
						"ProcVirtualization":    "Disabled",
						"SecureBoot":            "Enabled",
					},
					Conditions: []metav1.Condition{
						{Type: "Valid", Status: "True", Reason: "Success"},
						{Type: "ChangeDetected", Status: "False", Reason: "Success"},
					},
				},
			},
			SpecIsValid: true,
		},
		{
			Scenario: "updated settings",
			CurrentHFSResource: &metal3api.HostFirmwareSettings{
				TypeMeta: metav1.TypeMeta{
					Kind:       "HostFirmwareSettings",
					APIVersion: "metal3.io/v1alpha1"},
				ObjectMeta: metav1.ObjectMeta{
					Name:            hostName,
					Namespace:       hostNamespace,
					ResourceVersion: "1"},
				Spec: metal3api.HostFirmwareSettingsSpec{
					Settings: metal3api.DesiredSettingsMap{
						"NetworkBootRetryCount": intstr.FromString("10"),
						"ProcVirtualization":    intstr.FromString("Enabled"),
						"AssetTag":              intstr.FromString("Z98765432"),
					},
				},
				Status: metal3api.HostFirmwareSettingsStatus{
					FirmwareSchema: &metal3api.SchemaReference{
						Name:      schemaName,
						Namespace: hostNamespace,
					},
					Settings: metal3api.SettingsMap{
						"AssetTag":              "Z98765432",
						"L2Cache":               "10x256 KB",
						"NetworkBootRetryCount": "10",
						"ProcVirtualization":    "Enabled",
					},
				},
			},
			CreateSchemaResource: true,
			ExpectedSettings: &metal3api.HostFirmwareSettings{
				Spec: metal3api.HostFirmwareSettingsSpec{
					Settings: metal3api.DesiredSettingsMap{
						"NetworkBootRetryCount": intstr.FromString("10"),
						"ProcVirtualization":    intstr.FromString("Enabled"),
						"AssetTag":              intstr.FromString("Z98765432"),
					},
				},
				Status: metal3api.HostFirmwareSettingsStatus{
					FirmwareSchema: &metal3api.SchemaReference{
						Name:      schemaName,
						Namespace: hostNamespace,
					},
					Settings: metal3api.SettingsMap{
						"AssetTag":              "X45672917",
						"L2Cache":               "10x512 KB",
						"NetworkBootRetryCount": "20",
						"ProcVirtualization":    "Disabled",
						"SecureBoot":            "Enabled",
					},
					Conditions: []metav1.Condition{
						{Type: "ChangeDetected", Status: "True", Reason: "Success"},
						{Type: "Valid", Status: "True", Reason: "Success"},
					},
				},
			},
			SpecIsValid: true,
		},
		{
			Scenario: "spec updated with invalid setting",
			CurrentHFSResource: &metal3api.HostFirmwareSettings{
				TypeMeta: metav1.TypeMeta{
					Kind:       "HostFirmwareSettings",
					APIVersion: "metal3.io/v1alpha1"},
				ObjectMeta: metav1.ObjectMeta{
					Name:            hostName,
					Namespace:       hostNamespace,
					ResourceVersion: "1"},
				Spec: metal3api.HostFirmwareSettingsSpec{
					Settings: metal3api.DesiredSettingsMap{
						"NetworkBootRetryCount": intstr.FromInt(1000),
						"ProcVirtualization":    intstr.FromString("Enabled"),
					},
				},
				Status: metal3api.HostFirmwareSettingsStatus{
					FirmwareSchema: &metal3api.SchemaReference{
						Name:      schemaName,
						Namespace: hostNamespace,
					},
					Settings: metal3api.SettingsMap{
						"L2Cache":               "10x256 KB",
						"NetworkBootRetryCount": "10",
						"ProcVirtualization":    "Enabled",
					},
				},
			},
			CreateSchemaResource: true,
			ExpectedSettings: &metal3api.HostFirmwareSettings{
				Spec: metal3api.HostFirmwareSettingsSpec{
					Settings: metal3api.DesiredSettingsMap{
						"NetworkBootRetryCount": intstr.FromInt(1000),
						"ProcVirtualization":    intstr.FromString("Enabled"),
					},
				},
				Status: metal3api.HostFirmwareSettingsStatus{
					FirmwareSchema: &metal3api.SchemaReference{
						Name:      schemaName,
						Namespace: hostNamespace,
					},
					Settings: metal3api.SettingsMap{
						"AssetTag":              "X45672917",
						"L2Cache":               "10x512 KB",
						"NetworkBootRetryCount": "20",
						"ProcVirtualization":    "Disabled",
						"SecureBoot":            "Enabled",
					},
					Conditions: []metav1.Condition{
						{Type: "ChangeDetected", Status: "True", Reason: "Success"},
						{Type: "Valid", Status: "False", Reason: "ConfigurationError", Message: "Invalid BIOS setting"},
					},
				},
			},
			SpecIsValid: false,
		},
		{
			Scenario: "spec has same settings as current",
			CurrentHFSResource: &metal3api.HostFirmwareSettings{
				TypeMeta: metav1.TypeMeta{
					Kind:       "HostFirmwareSettings",
					APIVersion: "metal3.io/v1alpha1"},
				ObjectMeta: metav1.ObjectMeta{
					Name:            hostName,
					Namespace:       hostNamespace,
					ResourceVersion: "1"},
				Spec: metal3api.HostFirmwareSettingsSpec{
					Settings: metal3api.DesiredSettingsMap{
						"NetworkBootRetryCount": intstr.FromInt(20),
						"ProcVirtualization":    intstr.FromString("Disabled"),
					},
				},
				Status: metal3api.HostFirmwareSettingsStatus{
					FirmwareSchema: &metal3api.SchemaReference{
						Name:      schemaName,
						Namespace: hostNamespace,
					},
					Settings: metal3api.SettingsMap{
						"L2Cache":               "10x256 KB",
						"NetworkBootRetryCount": "10",
						"ProcVirtualization":    "Enabled",
					},
				},
			},
			CreateSchemaResource: true,
			ExpectedSettings: &metal3api.HostFirmwareSettings{
				Spec: metal3api.HostFirmwareSettingsSpec{
					Settings: metal3api.DesiredSettingsMap{
						"NetworkBootRetryCount": intstr.FromInt(20),
						"ProcVirtualization":    intstr.FromString("Disabled"),
					},
				},
				Status: metal3api.HostFirmwareSettingsStatus{
					FirmwareSchema: &metal3api.SchemaReference{
						Name:      schemaName,
						Namespace: hostNamespace,
					},
					Settings: metal3api.SettingsMap{
						"AssetTag":              "X45672917",
						"L2Cache":               "10x512 KB",
						"NetworkBootRetryCount": "20",
						"ProcVirtualization":    "Disabled",
						"SecureBoot":            "Enabled",
					},
					Conditions: []metav1.Condition{
						{Type: "Valid", Status: "True", Reason: "Success"},
						{Type: "ChangeDetected", Status: "False", Reason: "Success"},
					},
				},
			},
			SpecIsValid: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Scenario, func(t *testing.T) {
			ctx := t.Context()

			tc.ExpectedSettings.TypeMeta = metav1.TypeMeta{
				Kind:       "HostFirmwareSettings",
				APIVersion: "metal3.io/v1alpha1"}
			tc.ExpectedSettings.ObjectMeta = metav1.ObjectMeta{
				Name:            hostName,
				Namespace:       hostNamespace,
				ResourceVersion: "2"}

			hfs := tc.CurrentHFSResource
			r := getTestHFSReconciler(hfs)
			// Create bmh resource needed by hfs reconciler
			bmh := createBaremetalHost()
			prov := getMockProvisioner(bmh, getCurrentSettings(), getCurrentSchemaSettings())

			info := &rInfo{
				log: logf.Log.WithName("controllers").WithName("HostFirmwareSettings"),
				hfs: tc.CurrentHFSResource,
				bmh: bmh,
			}

			if tc.CreateSchemaResource {
				// Create an existing schema with different hfs owner
				firmwareSchema := getSchema()
				firmwareSchema.Spec.Schema = getCurrentSchemaSettings()

				err := r.Client.Create(ctx, firmwareSchema)
				require.NoError(t, err)
			}

			currentSettings, schema, err := prov.GetFirmwareSettings(true)
			require.NoError(t, err)

			err = r.updateHostFirmwareSettings(currentSettings, schema, info)
			require.NoError(t, err)

			// Check that resources get created or updated
			key := client.ObjectKey{
				Namespace: hfs.ObjectMeta.Namespace, Name: hfs.ObjectMeta.Name}
			actualSettings := &metal3api.HostFirmwareSettings{}
			err = r.Client.Get(ctx, key, actualSettings)
			require.NoError(t, err)

			// Use the same time for expected and actual
			currentTime := metav1.Now()
			tc.ExpectedSettings.Status.LastUpdated = &currentTime
			actualSettings.Status.LastUpdated = &currentTime
			for i := range tc.ExpectedSettings.Status.Conditions {
				tc.ExpectedSettings.Status.Conditions[i].LastTransitionTime = currentTime
				actualSettings.Status.Conditions[i].LastTransitionTime = currentTime
			}
			assert.Equal(t, tc.ExpectedSettings, actualSettings)

			key = client.ObjectKey{
				Namespace: hfs.ObjectMeta.Namespace, Name: schemaName}
			actualSchema := &metal3api.FirmwareSchema{}
			err = r.Client.Get(ctx, key, actualSchema)
			require.NoError(t, err)
			var expectedSchema *metal3api.FirmwareSchema
			if tc.CreateSchemaResource {
				expectedSchema = getExpectedSchemaTwoOwners()
			} else {
				expectedSchema = getExpectedSchema()
			}
			assert.Equal(t, expectedSchema, actualSchema)
		})
	}
}

// Test the function to validate hostFirmwareSettings.
func TestValidateHostFirmwareSettings(t *testing.T) {
	testCases := []struct {
		Scenario      string
		SpecSettings  metal3api.HostFirmwareSettingsSpec
		ExpectedError string
	}{
		{
			Scenario: "valid spec changes with schema",
			SpecSettings: metal3api.HostFirmwareSettingsSpec{
				Settings: metal3api.DesiredSettingsMap{
					"CustomPostMessage":     intstr.FromString("All tests passed"),
					"ProcVirtualization":    intstr.FromString("Disabled"),
					"NetworkBootRetryCount": intstr.FromInt(20),
				},
			},
			ExpectedError: "",
		},
		{
			Scenario: "invalid string",
			SpecSettings: metal3api.HostFirmwareSettingsSpec{
				Settings: metal3api.DesiredSettingsMap{
					"CustomPostMessage":     intstr.FromString("A really long POST message"),
					"ProcVirtualization":    intstr.FromString("Disabled"),
					"NetworkBootRetryCount": intstr.FromInt(20),
				},
			},
			ExpectedError: "Setting CustomPostMessage is invalid, string A really long POST message length is above maximum length 20",
		},
		{
			Scenario: "invalid int",
			SpecSettings: metal3api.HostFirmwareSettingsSpec{
				Settings: metal3api.DesiredSettingsMap{
					"CustomPostMessage":     intstr.FromString("All tests passed"),
					"ProcVirtualization":    intstr.FromString("Disabled"),
					"NetworkBootRetryCount": intstr.FromInt(2000),
				},
			},
			ExpectedError: "Setting NetworkBootRetryCount is invalid, integer 2000 is above maximum value 20",
		},
		{
			Scenario: "invalid enum",
			SpecSettings: metal3api.HostFirmwareSettingsSpec{
				Settings: metal3api.DesiredSettingsMap{
					"CustomPostMessage":     intstr.FromString("All tests passed"),
					"ProcVirtualization":    intstr.FromString("Not enabled"),
					"NetworkBootRetryCount": intstr.FromString("20"),
				},
			},
			ExpectedError: "Setting ProcVirtualization is invalid, unknown enumeration value - Not enabled",
		},
		{
			Scenario: "invalid name",
			SpecSettings: metal3api.HostFirmwareSettingsSpec{
				Settings: metal3api.DesiredSettingsMap{
					"SomeNewSetting": intstr.FromString("foo"),
				},
			},
			ExpectedError: "setting SomeNewSetting is not in the Status field",
		},
		{
			Scenario: "invalid password in spec",
			SpecSettings: metal3api.HostFirmwareSettingsSpec{
				Settings: metal3api.DesiredSettingsMap{
					"CustomPostMessage":     intstr.FromString("All tests passed"),
					"ProcVirtualization":    intstr.FromString("Disabled"),
					"NetworkBootRetryCount": intstr.FromString("20"),
					"SysPassword":           intstr.FromString("Pa%$word"),
				},
			},
			ExpectedError: "cannot set Password field",
		},
		{
			Scenario: "string instead of int",
			SpecSettings: metal3api.HostFirmwareSettingsSpec{
				Settings: metal3api.DesiredSettingsMap{
					"CustomPostMessage":     intstr.FromString("All tests passed"),
					"ProcVirtualization":    intstr.FromString("Disabled"),
					"NetworkBootRetryCount": intstr.FromString("foo"),
				},
			},
			ExpectedError: "Setting NetworkBootRetryCount is invalid, String foo entered while integer expected",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Scenario, func(t *testing.T) {
			hfs := getHFS(tc.SpecSettings)
			r := getTestHFSReconciler(hfs)
			info := &rInfo{
				log: logf.Log.WithName("controllers").WithName("HostFirmwareSettings"),
				hfs: hfs,
			}

			errors := r.validateHostFirmwareSettings(info, &info.hfs.Status, getExpectedSchema())
			if len(errors) == 0 {
				assert.Empty(t, tc.ExpectedError)
			} else {
				for _, error := range errors {
					assert.Equal(t, tc.ExpectedError, error.Error())
				}
			}
		})
	}
}
