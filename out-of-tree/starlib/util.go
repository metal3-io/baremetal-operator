/*

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package starlib

import (
	"crypto/hmac"
	"crypto/md5"  //nolint:gosec // md5 offered only for legacy checksum interop
	"crypto/sha1" //nolint:gosec // sha1 offered only for legacy checksum interop
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"

	"github.com/google/uuid"
	"go.starlark.net/starlark"
)

// BuiltinSHA256 returns the hex SHA256 digest of the input string.
func BuiltinSHA256(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var data string
	if err := starlark.UnpackArgs("sha256", args, kwargs, "data", &data); err != nil {
		return starlark.None, err
	}

	sum := sha256.Sum256([]byte(data))

	return starlark.String(hex.EncodeToString(sum[:])), nil
}

// BuiltinSHA1 returns the hex SHA1 digest of the input string.
func BuiltinSHA1(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var data string
	if err := starlark.UnpackArgs("sha1", args, kwargs, "data", &data); err != nil {
		return starlark.None, err
	}

	sum := sha1.Sum([]byte(data)) //nolint:gosec // legacy checksum interop only

	return starlark.String(hex.EncodeToString(sum[:])), nil
}

// BuiltinMD5 returns the hex MD5 digest of the input string.
func BuiltinMD5(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var data string
	if err := starlark.UnpackArgs("md5", args, kwargs, "data", &data); err != nil {
		return starlark.None, err
	}

	sum := md5.Sum([]byte(data)) //nolint:gosec // legacy checksum interop only

	return starlark.String(hex.EncodeToString(sum[:])), nil
}

// BuiltinHMACSHA256 returns the hex HMAC SHA256 of data keyed by key.
func BuiltinHMACSHA256(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var key, data string
	if err := starlark.UnpackArgs("hmac_sha256", args, kwargs, "key", &key, "data", &data); err != nil {
		return starlark.None, err
	}

	mac := hmac.New(sha256.New, []byte(key))
	_, _ = mac.Write([]byte(data))

	return starlark.String(hex.EncodeToString(mac.Sum(nil))), nil
}

// BuiltinHexEncode returns the hex encoding of the input string.
func BuiltinHexEncode(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var data string
	if err := starlark.UnpackArgs("hex_encode", args, kwargs, "data", &data); err != nil {
		return starlark.None, err
	}

	return starlark.String(hex.EncodeToString([]byte(data))), nil
}

// BuiltinHexDecode decodes a hex string back to its raw bytes.
func BuiltinHexDecode(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var s string
	if err := starlark.UnpackArgs("hex_decode", args, kwargs, "hex", &s); err != nil {
		return starlark.None, err
	}

	b, err := hex.DecodeString(s)
	if err != nil {
		return starlark.None, fmt.Errorf("hex_decode: %w", err)
	}

	return starlark.String(b), nil
}

// BuiltinUUID returns a new random RFC 4122 UUID string.
func BuiltinUUID(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if err := starlark.UnpackArgs("uuid", args, kwargs); err != nil {
		return starlark.None, err
	}

	return starlark.String(uuid.NewString()), nil
}

// BuiltinURLEncode percent encodes a string for safe use in a URL query.
func BuiltinURLEncode(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var s string
	if err := starlark.UnpackArgs("url_encode", args, kwargs, "s", &s); err != nil {
		return starlark.None, err
	}

	return starlark.String(url.QueryEscape(s)), nil
}

// BuiltinURLDecode reverses url_encode, erroring on malformed input.
func BuiltinURLDecode(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var s string
	if err := starlark.UnpackArgs("url_decode", args, kwargs, "s", &s); err != nil {
		return starlark.None, err
	}

	d, err := url.QueryUnescape(s)
	if err != nil {
		return starlark.None, fmt.Errorf("url_decode: %w", err)
	}

	return starlark.String(d), nil
}

// UtilBuiltins exposes hashing, hex, uuid, and url helpers to scripts.
func UtilBuiltins() starlark.StringDict {
	return starlark.StringDict{
		"sha256":      starlark.NewBuiltin("sha256", BuiltinSHA256),
		"sha1":        starlark.NewBuiltin("sha1", BuiltinSHA1),
		"md5":         starlark.NewBuiltin("md5", BuiltinMD5),
		"hmac_sha256": starlark.NewBuiltin("hmac_sha256", BuiltinHMACSHA256),
		"hex_encode":  starlark.NewBuiltin("hex_encode", BuiltinHexEncode),
		"hex_decode":  starlark.NewBuiltin("hex_decode", BuiltinHexDecode),
		"uuid":        starlark.NewBuiltin("uuid", BuiltinUUID),
		"url_encode":  starlark.NewBuiltin("url_encode", BuiltinURLEncode),
		"url_decode":  starlark.NewBuiltin("url_decode", BuiltinURLDecode),
	}
}
