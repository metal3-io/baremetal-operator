package clients

import (
	"bytes"
	"testing"

	"github.com/go-logr/logr"
	"github.com/go-logr/logr/funcr"
	"github.com/gophercloud/gophercloud/v2/openstack/baremetal/v1/nodes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

func TestOptionValueEqual(t *testing.T) {
	cases := []struct {
		Name   string
		Before any
		After  any
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
			Before: map[string]any{"foo": "foo"},
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
			Before: map[string]any{"foo": "foo"},
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
			Before: map[string]any{"foo": "bar"},
			After:  map[string]string{"foo": "bar"},
			Equal:  true,
		},
		{
			Name:   "unequal map string-typed",
			Before: map[string]any{"foo": "bar"},
			After:  map[string]string{"foo": "baz"},
			Equal:  false,
		},
		{
			Name:   "equal map int-typed",
			Before: map[string]any{"foo": 42},
			After:  map[string]int{"foo": 42},
			Equal:  true,
		},
		{
			Name:   "unequal map int-typed",
			Before: map[string]any{"foo": "bar"},
			After:  map[string]int{"foo": 42},
			Equal:  false,
		},
		{
			Name:   "equal map",
			Before: map[string]any{"foo": "bar", "42": 42},
			After:  map[string]any{"foo": "bar", "42": 42},
			Equal:  true,
		},
		{
			Name:   "unequal map",
			Before: map[string]any{"foo": "bar", "42": 42},
			After:  map[string]any{"foo": "bar", "42": 27},
			Equal:  false,
		},
		{
			Name:   "equal map empty string",
			Before: map[string]any{"foo": ""},
			After:  map[string]any{"foo": ""},
			Equal:  true,
		},
		{
			Name:   "unequal map replace empty string",
			Before: map[string]any{"foo": ""},
			After:  map[string]any{"foo": "bar"},
			Equal:  false,
		},
		{
			Name:   "unequal map replace with empty string",
			Before: map[string]any{"foo": "bar"},
			After:  map[string]any{"foo": ""},
			Equal:  false,
		},
		{
			Name:   "shorter map",
			Before: map[string]any{"foo": "bar", "42": 42},
			After:  map[string]any{"foo": "bar"},
			Equal:  false,
		},
		{
			Name:   "longer map",
			Before: map[string]any{"foo": "bar"},
			After:  map[string]any{"foo": "bar", "42": 42},
			Equal:  false,
		},
		{
			Name:   "different map",
			Before: map[string]any{"foo": "bar"},
			After:  map[string]any{"baz": "bar"},
			Equal:  false,
		},
		{
			Name:   "equal list string-typed",
			Before: []any{"foo", "bar"},
			After:  []string{"foo", "bar"},
			Equal:  true,
		},
		{
			Name:   "unequal list string-typed",
			Before: []any{"foo", "bar"},
			After:  []string{"foo", "baz"},
			Equal:  false,
		},
		{
			Name:   "equal list",
			Before: []any{"foo", 42},
			After:  []any{"foo", 42},
			Equal:  true,
		},
		{
			Name:   "unequal list",
			Before: []any{"foo", 42},
			After:  []any{"foo", 27},
			Equal:  false,
		},
		{
			Name:   "shorter list",
			Before: []any{"foo", 42},
			After:  []any{"foo"},
			Equal:  false,
		},
		{
			Name:   "longer list",
			Before: []any{"foo"},
			After:  []any{"foo", 42},
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
	existingData := map[string]any{
		"foo":  "bar",
		"foop": &barExist,
		"nil":  nilp,
	}
	cases := []struct {
		Name          string
		Field         string
		NewValue      any
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
			path := "test/" + c.Field
			updateOp := getUpdateOperation(
				c.Field, existingData,
				c.NewValue,
				path, logr.Logger{})

			switch c.ExpectedOp {
			case nodes.AddOp, nodes.ReplaceOp:
				var ev string
				var ok bool
				assert.NotNil(t, updateOp)
				ev = c.ExpectedValue
				if ev == "" {
					ev, ok = c.NewValue.(string)
					require.True(t, ok, "expected NewValue to be a string")
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
	op, ok := ops[0].(nodes.UpdateOperation)
	require.True(t, ok, "expected UpdateOperation")
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
		Properties: map[string]any{
			"foo": "bar",
		},
	}

	u := UpdateOptsBuilder(logf.Log)
	u.SetPropertiesOpts(newValues, &node)
	ops := u.Updates
	assert.Len(t, ops, 1)
	op, ok := ops[0].(nodes.UpdateOperation)
	require.True(t, ok, "expected UpdateOperation")
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
		InstanceInfo: map[string]any{
			"foo": "bar",
		},
	}

	u := UpdateOptsBuilder(logf.Log)
	u.SetInstanceInfoOpts(newValues, &node)
	ops := u.Updates
	assert.Len(t, ops, 1)
	op, ok := ops[0].(nodes.UpdateOperation)
	require.True(t, ok, "expected UpdateOperation")
	assert.Equal(t, nodes.AddOp, op.Op)
	assert.Equal(t, "quux", op.Value)
	assert.Equal(t, "/instance_info/baz", op.Path)
}

func TestSanitisedValue(t *testing.T) {
	assert.Nil(t, sanitisedValue(nil))

	unchanged := []any{
		"foo",
		42,
		true,
		[]string{"bar", "baz"},
		map[string]string{"foo": "bar"},
		map[string][]string{"foo": {"bar", "baz"}},
		map[string]any{"foo": []string{"bar", "baz"}, "bar": 42},
	}

	for _, u := range unchanged {
		assert.Exactly(t, u, sanitisedValue(u))
	}

	unsafe := map[string]any{
		"foo":               "bar",
		"password":          "secret",
		"ipmi_password":     "secret",
		"image_pull_secret": "user:pass",
	}
	safe := map[string]any{
		"foo":               "bar",
		"password":          "<redacted>",
		"ipmi_password":     "<redacted>",
		"image_pull_secret": "<redacted>",
	}
	assert.Exactly(t, safe, sanitisedValue(unsafe))

	// Key matching is case-insensitive, so RedactSensitiveMap and the option
	// logging cannot disagree about what names a credential.
	mixedCase := map[string]any{"IPMI_Password": "secret"}
	assert.Exactly(t, map[string]any{"IPMI_Password": "<redacted>"},
		sanitisedValue(mixedCase))

	// A value that is not named as a credential can still be a URL carrying
	// one.
	withURL := map[string]any{
		"deploy_kernel": "https://user:pass@example.com/ipa.kernel?X-Amz-Signature=abc",
	}
	safeURL := map[string]any{
		"deploy_kernel": "https://example.com/ipa.kernel?X-Amz-Signature=REDACTED",
	}
	assert.Exactly(t, safeURL, sanitisedValue(withURL))
}

// A map-valued option must not smuggle a signed URL into the log: Register
// passes the whole DriverInfo map when the credentials or BMC address change.
func TestGetUpdateOperationRedactsMapLogValues(t *testing.T) {
	var logged bytes.Buffer
	log := funcr.New(func(_, args string) {
		logged.WriteString(args)
	}, funcr.Options{})

	currentData := map[string]any{
		"driver_info": map[string]any{
			"deploy_kernel": "https://example.com/ipa.kernel?X-Amz-Signature=oldsignature",
			"ipmi_password": "oldpassword",
		},
	}
	desiredValue := map[string]any{
		"deploy_kernel": "https://example.com/ipa.kernel?X-Amz-Signature=newsignature",
		"ipmi_password": "newpassword",
	}

	updateOp := getUpdateOperation("driver_info", currentData, desiredValue,
		"/driver_info", log)
	require.NotNil(t, updateOp)

	out := logged.String()
	assert.NotContains(t, out, "oldsignature")
	assert.NotContains(t, out, "newsignature")
	assert.NotContains(t, out, "oldpassword")
	assert.NotContains(t, out, "newpassword")
	assert.Contains(t, out, "REDACTED")
	// Only the log is redacted, the update itself carries the real values.
	assert.Equal(t, desiredValue, updateOp.Value)
}

func TestRedactSensitiveURL(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "empty is unchanged",
			in:   "",
			want: "",
		},
		{
			name: "plain URL without secrets is unchanged",
			in:   "https://example.com/images/image.qcow2",
			want: "https://example.com/images/image.qcow2",
		},
		{
			name: "benign query params are preserved as-is",
			in:   "https://example.com/image.qcow2?version=2&format=qcow2",
			want: "https://example.com/image.qcow2?version=2&format=qcow2",
		},
		{
			name: "embedded userinfo credentials are removed",
			in:   "https://user:pass@example.com/image.qcow2",
			want: "https://example.com/image.qcow2",
		},
		{
			name: "username-only userinfo is removed",
			in:   "https://user@example.com/image.qcow2",
			want: "https://example.com/image.qcow2",
		},
		{
			name: "AWS signature query param is redacted",
			in:   "https://example.com/image.qcow2?X-Amz-Signature=abc123&foo=bar",
			want: "https://example.com/image.qcow2?X-Amz-Signature=REDACTED&foo=bar",
		},
		{
			name: "token query param is redacted",
			in:   "https://example.com/image.qcow2?token=supersecret",
			want: "https://example.com/image.qcow2?token=REDACTED",
		},
		{
			name: "access_token query param is redacted",
			in:   "https://example.com/image.qcow2?access_token=supersecret",
			want: "https://example.com/image.qcow2?access_token=REDACTED",
		},
		{
			name: "password query param is redacted",
			in:   "https://example.com/image.qcow2?password=hunter2",
			want: "https://example.com/image.qcow2?password=REDACTED",
		},
		{
			name: "secret query param is redacted",
			in:   "https://example.com/image.qcow2?secret=hunter2&keep=1",
			want: "https://example.com/image.qcow2?keep=1&secret=REDACTED",
		},
		{
			name: "redaction is case-insensitive on param name",
			in:   "https://example.com/image.qcow2?Signature=abc",
			want: "https://example.com/image.qcow2?Signature=REDACTED",
		},
		{
			name: "both userinfo and signed params are handled",
			in:   "https://user:pass@example.com/image.qcow2?sig=abc&keep=1",
			want: "https://example.com/image.qcow2?keep=1&sig=REDACTED",
		},
		{
			name: "GCS signed URL params are redacted",
			in:   "https://storage.googleapis.com/bucket/image.qcow2?X-Goog-Signature=abc&X-Goog-Credential=def",
			want: "https://storage.googleapis.com/bucket/image.qcow2?X-Goog-Credential=REDACTED&X-Goog-Signature=REDACTED",
		},
		{
			name: "Swift temp_url_sig is redacted",
			in:   "https://swift.example.com/v1/img.qcow2?temp_url_sig=abc&temp_url_expires=123",
			want: "https://swift.example.com/v1/img.qcow2?temp_url_expires=123&temp_url_sig=REDACTED",
		},
		{
			name: "percent-encoded param name is redacted",
			in:   "https://example.com/image.qcow2?%74oken=supersecret",
			want: "https://example.com/image.qcow2?token=REDACTED",
		},
		{
			name: "an apostrophe in the userinfo does not keep the password",
			in:   "https://user:'hunter2'@example.com/image.qcow2",
			want: "https://example.com/image.qcow2",
		},
		{
			name: "an apostrophe in a sensitive query value does not keep it",
			in:   "https://example.com/image.qcow2?token='quote-secret'",
			want: "https://example.com/image.qcow2?token=REDACTED",
		},
		{
			name: "a prefixed sensitive param name is redacted",
			in:   "https://example.com/image.qcow2?my_token=abc&keep=1",
			want: "https://example.com/image.qcow2?keep=1&my_token=REDACTED",
		},
		{
			name: "unparsable input is redacted wholesale",
			in:   "://not a url",
			want: "<redacted>",
		},
		{
			name: "malformed query is redacted wholesale",
			in:   "https://example.com/image.qcow2?token=secret;evil=1",
			want: "https://example.com/image.qcow2?REDACTED",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, RedactSensitiveURL(tt.in))
		})
	}
}

func TestRedactSensitiveText(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "empty is unchanged",
			in:   "",
			want: "",
		},
		{
			name: "text without a URL is unchanged",
			in:   "Deploy failed: could not connect to BMC",
			want: "Deploy failed: could not connect to BMC",
		},
		{
			name: "URL embedded in an error message is redacted",
			in:   "failed to download image from https://example.com/image.qcow2?X-Amz-Signature=abc123 (404)",
			want: "failed to download image from https://example.com/image.qcow2?X-Amz-Signature=REDACTED (404)",
		},
		{
			name: "embedded userinfo is stripped",
			in:   "error fetching https://user:pass@example.com/image.qcow2 for node",
			want: "error fetching https://example.com/image.qcow2 for node",
		},
		{
			name: "multiple URLs are each redacted",
			in:   "primary https://example.com/a?token=one secondary https://example.com/b?sig=two",
			want: "primary https://example.com/a?token=REDACTED secondary https://example.com/b?sig=REDACTED",
		},
		{
			name: "uppercase scheme is still redacted",
			in:   "failed to download HTTPS://user:pass@example.com/image.qcow2?token=abc",
			want: "failed to download https://example.com/image.qcow2?token=REDACTED",
		},
		{
			name: "mixed-case scheme is still redacted",
			in:   "failed to download Https://example.com/image.qcow2?X-Amz-Signature=abc",
			want: "failed to download https://example.com/image.qcow2?X-Amz-Signature=REDACTED",
		},
		{
			name: "uppercase OCI scheme is still redacted",
			in:   "pull failed from OCI://user:pass@registry.test/image:tag",
			want: "pull failed from oci://registry.test/image:tag",
		},
		{
			name: "a checksum URL is redacted",
			in:   "https://user:pass@example.com/image.qcow2.md5sum?token=abc",
			want: "https://example.com/image.qcow2.md5sum?token=REDACTED",
		},
		{
			name: "a bare checksum digest is left untouched",
			in:   "97830b5e5b6d4d1c1f0f0d0e7a0f4f7e",
			want: "97830b5e5b6d4d1c1f0f0d0e7a0f4f7e",
		},
		{
			name: "a prefixed checksum digest is left untouched",
			in:   "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
			want: "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		},
		{
			name: "trailing sentence punctuation is not treated as part of the URL",
			in:   "failed to download https://example.com/image.qcow2?token=abc.",
			want: "failed to download https://example.com/image.qcow2?token=REDACTED.",
		},
		{
			name: "a parenthesised URL keeps its closing bracket",
			in:   "image (https://example.com/image.qcow2?sig=abc) is gone",
			want: "image (https://example.com/image.qcow2?sig=REDACTED) is gone",
		},
		{
			name: "scheme-less urllib3 request path is redacted",
			in:   "HTTPSConnectionPool(host='example.com', port=443): Max retries exceeded with url: /image.qcow2?X-Amz-Signature=abc123",
			want: "HTTPSConnectionPool(host='example.com', port=443): Max retries exceeded with url: /image.qcow2?X-Amz-Signature=REDACTED",
		},
		{
			name: "a bare sensitive param pair is redacted",
			in:   "deploy failed, request was token=supersecret",
			want: "deploy failed, request was token=REDACTED",
		},
		{
			name: "a bare password pair is redacted",
			in:   "deploy failed, request was password=hunter2",
			want: "deploy failed, request was password=REDACTED",
		},
		{
			name: "a benign param pair is left untouched",
			in:   "deploy failed, request was version=2",
			want: "deploy failed, request was version=2",
		},
		{
			name: "an apostrophe in the userinfo does not truncate the URL match",
			in:   "failed to download https://user:'hunter2'@example.com/image.qcow2 (403)",
			want: "failed to download https://example.com/image.qcow2 (403)",
		},
		{
			name: "an apostrophe in a sensitive query value does not truncate it",
			in:   "failed to download https://example.com/image.qcow2?token='quote-secret'",
			want: "failed to download https://example.com/image.qcow2?token=REDACTED'",
		},
		{
			name: "a quoted URL keeps its surrounding quotes",
			in:   "failed to download 'https://example.com/image.qcow2?sig=abc', giving up",
			want: "failed to download 'https://example.com/image.qcow2?sig=REDACTED', giving up",
		},
		{
			name: "a quoted scheme-less param value is redacted",
			in:   "deploy failed, request was token='quote-secret'",
			want: "deploy failed, request was token=REDACTED",
		},
		{
			name: "a double-quoted param value is redacted",
			in:   `deploy failed, request was password="hunter2" (403)`,
			want: "deploy failed, request was password=REDACTED (403)",
		},
		{
			name: "an unterminated quoted value fails closed",
			in:   `deploy failed, request was password="hunter2 and more`,
			want: "deploy failed, request was password=REDACTED",
		},
		{
			name: "an escaped quote does not end a quoted value early",
			in:   `deploy failed, request was password="hun\"ter2" (403)`,
			want: "deploy failed, request was password=REDACTED (403)",
		},
		{
			name: "an escaped quote in a Python repr value is redacted",
			in:   `deploy failed, request was token='hun\'ter2', retrying`,
			want: "deploy failed, request was token=REDACTED, retrying",
		},
		{
			name: "a path containing = cannot swallow a signed param",
			in:   "Max retries exceeded with url: /images/os=linux/image.qcow2?X-Amz-Signature=supersecret",
			want: "Max retries exceeded with url: /images/os=linux/image.qcow2?X-Amz-Signature=REDACTED",
		},
		{
			name: "a benign name containing a denylisted substring is left untouched",
			in:   "deploy failed, request was design=modern",
			want: "deploy failed, request was design=modern",
		},
		{
			name: "a percent-encoded param name in a request path is redacted",
			in:   "HTTPSConnectionPool(host='example.com', port=443): Max retries exceeded with url: /image.qcow2?%74oken=supersecret",
			want: "HTTPSConnectionPool(host='example.com', port=443): Max retries exceeded with url: /image.qcow2?%74oken=REDACTED",
		},
		{
			name: "percent-encoded separators in a param name are redacted",
			in:   "Max retries exceeded with url: /image.qcow2?X%2DAmz%2DSignature=abc123",
			want: "Max retries exceeded with url: /image.qcow2?X%2DAmz%2DSignature=REDACTED",
		},
		{
			name: "an uppercase-encoded param name is redacted",
			in:   "Max retries exceeded with url: /image?%54oken=supersecret",
			want: "Max retries exceeded with url: /image?%54oken=REDACTED",
		},
		{
			name: "an encoded separator before a param name is redacted",
			in:   "Max retries exceeded with url: /image?my%5Ftoken=supersecret",
			want: "Max retries exceeded with url: /image?my%5Ftoken=REDACTED",
		},
		{
			name: "an uppercase-encoded letter inside a prefixed name is redacted",
			in:   "Max retries exceeded with url: /image?X-Amz-%53ignature=supersecret",
			want: "Max retries exceeded with url: /image?X-Amz-%53ignature=REDACTED",
		},
		{
			name: "a prefixed scheme-less param name is redacted",
			in:   "Max retries exceeded with url: /image.qcow2?my_token=abc123",
			want: "Max retries exceeded with url: /image.qcow2?my_token=REDACTED",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, RedactSensitiveText(tt.in))
		})
	}
}

func TestRedactSensitiveMap(t *testing.T) {
	assert.Nil(t, RedactSensitiveMap(nil))

	iinfo := map[string]any{
		"image_source":      "https://user:pass@example.com/image.qcow2?X-Amz-Signature=abc&keep=1",
		"boot_iso":          "HTTPS://example.com/boot.iso?token=secret",
		"image_pull_secret": "dockerconfigjson-contents",
		"image_type":        "whole-disk",
		"root_device":       map[string]string{"name": "/dev/sda"},
		"capabilities":      nil,
	}

	safe := RedactSensitiveMap(iinfo)

	assert.Equal(t, "https://example.com/image.qcow2?X-Amz-Signature=REDACTED&keep=1", safe["image_source"])
	assert.Equal(t, "https://example.com/boot.iso?token=REDACTED", safe["boot_iso"])
	assert.Equal(t, "<redacted>", safe["image_pull_secret"])
	// Non-sensitive values are passed through untouched.
	assert.Equal(t, "whole-disk", safe["image_type"])
	assert.Equal(t, map[string]string{"name": "/dev/sda"}, safe["root_device"])
	assert.Nil(t, safe["capabilities"])

	// The original map must not be modified.
	assert.Equal(t, "https://user:pass@example.com/image.qcow2?X-Amz-Signature=abc&keep=1", iinfo["image_source"])
	assert.Equal(t, "dockerconfigjson-contents", iinfo["image_pull_secret"])
}
