package v1alpha1

import (
	"testing"

	"github.com/stretchr/testify/assert"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestValidateSetting(t *testing.T) {
	lower_bound := 1
	upper_bound := 20
	min_length := 1
	max_length := 16
	read_only := true

	fwSchema := &FirmwareSchema{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "myfwschema",
			Namespace: "myns",
		},
		Spec: FirmwareSchemaSpec{
			Schema: make(map[string]SettingSchema),
		},
	}

	fwSchema.Spec.Schema["AssetTag"] = SettingSchema{AttributeType: "String",
		MinLength: &min_length, MaxLength: &max_length}
	fwSchema.Spec.Schema["ProcVirtualization"] = SettingSchema{AttributeType: "Enumeration",
		AllowableValues: []string{"Enabled", "Disabled"}}
	fwSchema.Spec.Schema["NetworkBootRetryCount"] = SettingSchema{AttributeType: "Integer",
		LowerBound: &lower_bound, UpperBound: &upper_bound}
	fwSchema.Spec.Schema["SerialNumber"] = SettingSchema{AttributeType: "String",
		MinLength: &min_length, MaxLength: &max_length, ReadOnly: &read_only}
	fwSchema.Spec.Schema["QuietBoot"] = SettingSchema{AttributeType: "Boolean"}
	fwSchema.Spec.Schema["SriovEnable"] = SettingSchema{} // fields intentionally excluded

	for _, tc := range []struct {
		Scenario string
		Name     string
		Value    intstr.IntOrString
		Expected string
	}{
		{
			Scenario: "StringTypePass",
			Name:     "AssetTag",
			Value:    intstr.FromString("NewServer"),
			Expected: "",
		},
		{
			Scenario: "StringTypeFailUpper",
			Name:     "AssetTag",
			Value:    intstr.FromString("NewServerPutInServiceIn2021"),
			Expected: "Setting AssetTag is invalid, string NewServerPutInServiceIn2021 length is above maximum length 16",
		},
		{
			Scenario: "StringTypeFailLower",
			Name:     "AssetTag",
			Value:    intstr.FromString(""),
			Expected: "Setting AssetTag is invalid, string  length is below minimum length 1",
		},
		{
			Scenario: "EnumerationTypePass",
			Name:     "ProcVirtualization",
			Value:    intstr.FromString("Disabled"),
			Expected: "",
		},
		{
			Scenario: "EnumerationTypeFail",
			Name:     "ProcVirtualization",
			Value:    intstr.FromString("Foo"),
			Expected: "Setting ProcVirtualization is invalid, unknown enumeration value - Foo",
		},
		{
			Scenario: "IntegerTypePassAsString",
			Name:     "NetworkBootRetryCount",
			Value:    intstr.FromString("10"),
			Expected: "",
		},
		{
			Scenario: "IntegerTypePassAsInt",
			Name:     "NetworkBootRetryCount",
			Value:    intstr.FromInt(10),
			Expected: "",
		},
		{
			Scenario: "IntegerTypeFailUpper",
			Name:     "NetworkBootRetryCount",
			Value:    intstr.FromString("42"),
			Expected: "Setting NetworkBootRetryCount is invalid, integer 42 is above maximum value 20",
		},
		{
			Scenario: "IntegerTypeFailLower",
			Name:     "NetworkBootRetryCount",
			Value:    intstr.FromInt(0),
			Expected: "Setting NetworkBootRetryCount is invalid, integer 0 is below minimum value 1",
		},
		{
			Scenario: "BooleanTypePass",
			Name:     "QuietBoot",
			Value:    intstr.FromString("true"),
			Expected: "",
		},
		{
			Scenario: "BooleanTypeFail",
			Name:     "QuietBoot",
			Value:    intstr.FromString("Enabled"),
			Expected: "Setting QuietBoot is invalid, Enabled is not a boolean",
		},
		{
			Scenario: "ReadOnlyTypeFail",
			Name:     "SerialNumber",
			Value:    intstr.FromString("42"),
			Expected: "Setting SerialNumber is invalid, it is ReadOnly",
		},
		{
			Scenario: "MissingEnumerationField",
			Name:     "SriovEnable",
			Value:    intstr.FromString("Disabled"),
			Expected: "",
		},
		{
			Scenario: "UnknownSettingFail",
			Name:     "IceCream",
			Value:    intstr.FromString("Vanilla"),
			Expected: "Setting IceCream is invalid, it is not in the associated schema",
		},
	} {
		t.Run(tc.Scenario, func(t *testing.T) {
			err := fwSchema.ValidateSetting(tc.Name, tc.Value, fwSchema.Spec.Schema)
			if err == nil {
				assert.Equal(t, tc.Expected, "")
			} else {
				assert.Equal(t, tc.Expected, err.Error())
			}
		})
	}
}
