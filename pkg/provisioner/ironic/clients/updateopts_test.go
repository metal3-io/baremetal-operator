package clients

import (
	"fmt"
	"testing"

	"github.com/go-logr/logr"
	"github.com/gophercloud/gophercloud/v2/openstack/baremetal/v1/nodes"
	"github.com/stretchr/testify/assert"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

func TestOptionValueEqual(t *testing.T) {
	cases := []struct {
		Name   string
		Before interface{}
		After  interface{}
		Equal  bool
	}{
		{
			Name:   "nil interface",
			Before: nil,
			After:  "foo",
			Equal:  false,
		},
		{
			Name:   "equal string",
			Before: "foo",
			After:  "foo",
			Equal:  true,
		},
		{
			Name:   "unequal string",
			Before: "foo",
			After:  "bar",
			Equal:  false,
		},
		{
			Name:   "equal true",
			Before: true,
			After:  true,
			Equal:  true,
		},
		{
			Name:   "equal false",
			Before: false,
			After:  false,
			Equal:  true,
		},
		{
			Name:   "unequal true",
			Before: true,
			After:  false,
			Equal:  false,
		},
		{
			Name:   "unequal false",
			Before: false,
			After:  true,
			Equal:  false,
		},
		{
			Name:   "equal int",
			Before: 42,
			After:  42,
			Equal:  true,
		},
		{
			Name:   "unequal int",
			Before: 27,
			After:  42,
			Equal:  false,
		},
		{
			Name:   "string int",
			Before: "42",
			After:  42,
			Equal:  false,
		},
		{
			Name:   "int string",
			Before: 42,
			After:  "42",
			Equal:  false,
		},
		{
			Name:   "bool int",
			Before: false,
			After:  0,
			Equal:  false,
		},
		{
			Name:   "int bool",
			Before: 1,
			After:  true,
			Equal:  false,
		},
		{
			Name:   "string map",
			Before: "foo",
			After:  map[string]string{"foo": "foo"},
			Equal:  false,
		},
		{
			Name:   "map string",
			Before: map[string]interface{}{"foo": "foo"},
			After:  "foo",
			Equal:  false,
		},
		{
			Name:   "string list",
			Before: "foo",
			After:  []string{"foo"},
			Equal:  false,
		},
		{
			Name:   "list string",
			Before: []string{"foo"},
			After:  "foo",
			Equal:  false,
		},
		{
			Name:   "map list",
			Before: map[string]interface{}{"foo": "foo"},
			After:  []string{"foo"},
			Equal:  false,
		},
		{
			Name:   "list map",
			Before: []string{"foo"},
			After:  map[string]string{"foo": "foo"},
			Equal:  false,
		},
		{
			Name:   "equal map string-typed",
			Before: map[string]interface{}{"foo": "bar"},
			After:  map[string]string{"foo": "bar"},
			Equal:  true,
		},
		{
			Name:   "unequal map string-typed",
			Before: map[string]interface{}{"foo": "bar"},
			After:  map[string]string{"foo": "baz"},
			Equal:  false,
		},
		{
			Name:   "equal map int-typed",
			Before: map[string]interface{}{"foo": 42},
			After:  map[string]int{"foo": 42},
			Equal:  true,
		},
		{
			Name:   "unequal map int-typed",
			Before: map[string]interface{}{"foo": "bar"},
			After:  map[string]int{"foo": 42},
			Equal:  false,
		},
		{
			Name:   "equal map",
			Before: map[string]interface{}{"foo": "bar", "42": 42},
			After:  map[string]interface{}{"foo": "bar", "42": 42},
			Equal:  true,
		},
		{
			Name:   "unequal map",
			Before: map[string]interface{}{"foo": "bar", "42": 42},
			After:  map[string]interface{}{"foo": "bar", "42": 27},
			Equal:  false,
		},
		{
			Name:   "equal map empty string",
			Before: map[string]interface{}{"foo": ""},
			After:  map[string]interface{}{"foo": ""},
			Equal:  true,
		},
		{
			Name:   "unequal map replace empty string",
			Before: map[string]interface{}{"foo": ""},
			After:  map[string]interface{}{"foo": "bar"},
			Equal:  false,
		},
		{
			Name:   "unequal map replace with empty string",
			Before: map[string]interface{}{"foo": "bar"},
			After:  map[string]interface{}{"foo": ""},
			Equal:  false,
		},
		{
			Name:   "shorter map",
			Before: map[string]interface{}{"foo": "bar", "42": 42},
			After:  map[string]interface{}{"foo": "bar"},
			Equal:  false,
		},
		{
			Name:   "longer map",
			Before: map[string]interface{}{"foo": "bar"},
			After:  map[string]interface{}{"foo": "bar", "42": 42},
			Equal:  false,
		},
		{
			Name:   "different map",
			Before: map[string]interface{}{"foo": "bar"},
			After:  map[string]interface{}{"baz": "bar"},
			Equal:  false,
		},
		{
			Name:   "equal list string-typed",
			Before: []interface{}{"foo", "bar"},
			After:  []string{"foo", "bar"},
			Equal:  true,
		},
		{
			Name:   "unequal list string-typed",
			Before: []interface{}{"foo", "bar"},
			After:  []string{"foo", "baz"},
			Equal:  false,
		},
		{
			Name:   "equal list",
			Before: []interface{}{"foo", 42},
			After:  []interface{}{"foo", 42},
			Equal:  true,
		},
		{
			Name:   "unequal list",
			Before: []interface{}{"foo", 42},
			After:  []interface{}{"foo", 27},
			Equal:  false,
		},
		{
			Name:   "shorter list",
			Before: []interface{}{"foo", 42},
			After:  []interface{}{"foo"},
			Equal:  false,
		},
		{
			Name:   "longer list",
			Before: []interface{}{"foo"},
			After:  []interface{}{"foo", 42},
			Equal:  false,
		},
	}

	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			var ne = "="
			if c.Equal {
				ne = "!"
			}
			assert.Equal(t, c.Equal, optionValueEqual(c.Before, c.After),
				"%v %s= %v", c.Before, ne, c.After)
		})
	}
}

func TestGetUpdateOperation(t *testing.T) {
	var nilp *string
	barExist := "bar"
	bar := "bar"
	baz := "baz"
	existingData := map[string]interface{}{
		"foo":  "bar",
		"foop": &barExist,
		"nil":  nilp,
	}
	cases := []struct {
		Name          string
		Field         string
		NewValue      interface{}
		ExpectedOp    nodes.UpdateOp
		ExpectedValue string
	}{
		{
			Name:       "add value",
			Field:      "baz",
			NewValue:   "quux",
			ExpectedOp: nodes.AddOp,
		},
		{
			Name:          "add value pointer",
			Field:         "baz",
			NewValue:      &bar,
			ExpectedValue: bar,
			ExpectedOp:    nodes.AddOp,
		},
		{
			Name:          "add pointer value pointer",
			Field:         "nil",
			NewValue:      &bar,
			ExpectedValue: bar,
			ExpectedOp:    nodes.AddOp,
		},
		{
			Name:     "keep value",
			Field:    "foo",
			NewValue: "bar",
		},
		{
			Name:     "keep pointer value",
			Field:    "foop",
			NewValue: "bar",
		},
		{
			Name:     "keep value pointer",
			Field:    "foo",
			NewValue: &bar,
		},
		{
			Name:     "keep pointer value pointer",
			Field:    "foop",
			NewValue: &bar,
		},
		{
			Name:       "change value",
			Field:      "foo",
			NewValue:   "baz",
			ExpectedOp: nodes.AddOp,
		},
		{
			Name:       "change pointer value",
			Field:      "foop",
			NewValue:   "baz",
			ExpectedOp: nodes.AddOp,
		},
		{
			Name:          "change value pointer",
			Field:         "foo",
			NewValue:      &baz,
			ExpectedValue: baz,
			ExpectedOp:    nodes.AddOp,
		},
		{
			Name:          "change pointer value pointer",
			Field:         "foop",
			NewValue:      &baz,
			ExpectedValue: baz,
			ExpectedOp:    nodes.AddOp,
		},
		{
			Name:       "delete value",
			Field:      "foo",
			NewValue:   nil,
			ExpectedOp: nodes.RemoveOp,
		},
		{
			Name:       "delete value pointer",
			Field:      "foo",
			NewValue:   nilp,
			ExpectedOp: nodes.RemoveOp,
		},
		{
			Name:     "nonexistent value",
			Field:    "bar",
			NewValue: nil,
		},
	}

	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			path := fmt.Sprintf("test/%s", c.Field)
			updateOp := getUpdateOperation(
				c.Field, existingData,
				c.NewValue,
				path, logr.Logger{})

			switch c.ExpectedOp {
			case nodes.AddOp, nodes.ReplaceOp:
				assert.NotNil(t, updateOp)
				ev := c.ExpectedValue
				if ev == "" {
					ev = c.NewValue.(string)
				}
				assert.Equal(t, c.ExpectedOp, updateOp.Op)
				assert.Equal(t, ev, updateOp.Value)
				assert.Equal(t, path, updateOp.Path)
			case nodes.RemoveOp:
				assert.NotNil(t, updateOp)
				assert.Equal(t, c.ExpectedOp, updateOp.Op)
				assert.Equal(t, path, updateOp.Path)
			default:
				assert.Nil(t, updateOp)
			}
		})
	}
}

func TestTopLevelUpdateOpt(t *testing.T) {
	u := UpdateOptsBuilder(logf.Log)
	u.SetTopLevelOpt("foo", "baz", "bar")
	ops := u.Updates
	assert.Len(t, ops, 1)
	op := ops[0].(nodes.UpdateOperation)
	assert.Equal(t, nodes.AddOp, op.Op)
	assert.Equal(t, "baz", op.Value)
	assert.Equal(t, "/foo", op.Path)

	u = UpdateOptsBuilder(logf.Log)
	u.SetTopLevelOpt("foo", "bar", "bar")
	assert.Empty(t, u.Updates)
}

func TestPropertiesUpdateOpts(t *testing.T) {
	newValues := UpdateOptsData{
		"foo": "bar",
		"baz": "quux",
	}
	node := nodes.Node{
		Properties: map[string]interface{}{
			"foo": "bar",
		},
	}

	u := UpdateOptsBuilder(logf.Log)
	u.SetPropertiesOpts(newValues, &node)
	ops := u.Updates
	assert.Len(t, ops, 1)
	op := ops[0].(nodes.UpdateOperation)
	assert.Equal(t, nodes.AddOp, op.Op)
	assert.Equal(t, "quux", op.Value)
	assert.Equal(t, "/properties/baz", op.Path)
}

func TestInstanceInfoUpdateOpts(t *testing.T) {
	newValues := UpdateOptsData{
		"foo": "bar",
		"baz": "quux",
	}
	node := nodes.Node{
		InstanceInfo: map[string]interface{}{
			"foo": "bar",
		},
	}

	u := UpdateOptsBuilder(logf.Log)
	u.SetInstanceInfoOpts(newValues, &node)
	ops := u.Updates
	assert.Len(t, ops, 1)
	op := ops[0].(nodes.UpdateOperation)
	assert.Equal(t, nodes.AddOp, op.Op)
	assert.Equal(t, "quux", op.Value)
	assert.Equal(t, "/instance_info/baz", op.Path)
}

func TestSanitisedValue(t *testing.T) {
	unchanged := []interface{}{
		"foo",
		42,
		true,
		[]string{"bar", "baz"},
		map[string]string{"foo": "bar"},
		map[string][]string{"foo": {"bar", "baz"}},
		map[string]interface{}{"foo": []string{"bar", "baz"}, "bar": 42},
	}

	for _, u := range unchanged {
		assert.Exactly(t, u, sanitisedValue(u))
	}

	unsafe := map[string]interface{}{
		"foo":           "bar",
		"password":      "secret",
		"ipmi_password": "secret",
	}
	safe := map[string]interface{}{
		"foo":           "bar",
		"password":      "<redacted>",
		"ipmi_password": "<redacted>",
	}
	assert.Exactly(t, safe, sanitisedValue(unsafe))
}
