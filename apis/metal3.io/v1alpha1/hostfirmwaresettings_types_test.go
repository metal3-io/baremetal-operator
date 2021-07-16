package v1alpha1

import (
	"testing"

	"github.com/stretchr/testify/assert"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestCheckSettingIsValid(t *testing.T) {

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
		Expected bool
	}{
		{
			Scenario: "StringTypePass",
			Name:     "AssetTag",
			Value:    intstr.FromString("NewServer"),
			Expected: true,
		},
		{
			Scenario: "StringTypeFailUpper",
			Name:     "AssetTag",
			Value:    intstr.FromString("NewServerPutInServiceIn2021"),
			Expected: false,
		},
		{
			Scenario: "StringTypeFailLower",
			Name:     "AssetTag",
			Value:    intstr.FromString(""),
			Expected: false,
		},
		{
			Scenario: "EnumerationTypePass",
			Name:     "ProcVirtualization",
			Value:    intstr.FromString("Disabled"),
			Expected: true,
		},
		{
			Scenario: "EnumerationTypeFail",
			Name:     "ProcVirtualization",
			Value:    intstr.FromString("Foo"),
			Expected: false,
		},
		{
			Scenario: "IntegerTypePassAsString",
			Name:     "NetworkBootRetryCount",
			Value:    intstr.FromString("10"),
			Expected: true,
		},
		{
			Scenario: "IntegerTypePassAsInt",
			Name:     "NetworkBootRetryCount",
			Value:    intstr.FromInt(10),
			Expected: true,
		},
		{
			Scenario: "IntegerTypeFailUpper",
			Name:     "NetworkBootRetryCount",
			Value:    intstr.FromString("42"),
			Expected: false,
		},
		{
			Scenario: "IntegerTypeFailLower",
			Name:     "NetworkBootRetryCount",
			Value:    intstr.FromInt(0),
			Expected: false,
		},
		{
			Scenario: "BooleanTypePass",
			Name:     "QuietBoot",
			Value:    intstr.FromString("true"),
			Expected: true,
		},
		{
			Scenario: "BooleanTypeFail",
			Name:     "QuietBoot",
			Value:    intstr.FromString("Enabled"),
			Expected: false,
		},
		{
			Scenario: "ReadOnlyTypeFail",
			Name:     "SerialNumber",
			Value:    intstr.FromString("42"),
			Expected: false,
		},
		{
			Scenario: "MissingEnumerationField",
			Name:     "SriovEnable",
			Value:    intstr.FromString("Disabled"),
			Expected: true,
		},
		{
			Scenario: "UnknownSettingFail",
			Name:     "IceCream",
			Value:    intstr.FromString("Vanilla"),
			Expected: false,
		},
	} {
		t.Run(tc.Scenario, func(t *testing.T) {
			actual := fwSchema.CheckSettingIsValid(tc.Name, tc.Value, fwSchema.Spec.Schema)
			assert.Equal(t, tc.Expected, actual)
		})
	}
}
