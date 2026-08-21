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
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.starlark.net/starlark"
)

func TestUtilBuiltins(t *testing.T) {
	th := &starlark.Thread{Name: "t"}
	abc := starlark.Tuple{starlark.String("abc")}

	str := func(v starlark.Value, err error) string {
		t.Helper()

		if err != nil {
			t.Fatalf("builtin error: %v", err)
		}

		s, ok := starlark.AsString(v)
		if !ok {
			t.Fatalf("builtin returned %T, want string", v)
		}

		return s
	}

	cases := []struct {
		name string
		got  string
		want string
	}{
		{"sha256", str(BuiltinSHA256(th, nil, abc, nil)), "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"},
		{"sha1", str(BuiltinSHA1(th, nil, abc, nil)), "a9993e364706816aba3e25717850c26c9cd0d89d"},
		{"md5", str(BuiltinMD5(th, nil, abc, nil)), "900150983cd24fb0d6963f7d28e17f72"},
		{"hex_encode", str(BuiltinHexEncode(th, nil, abc, nil)), "616263"},
		{"hex_decode", str(BuiltinHexDecode(th, nil, starlark.Tuple{starlark.String("616263")}, nil)), "abc"},
		{"url_encode", str(BuiltinURLEncode(th, nil, starlark.Tuple{starlark.String("a b&c")}, nil)), "a+b%26c"},
		{"url_decode", str(BuiltinURLDecode(th, nil, starlark.Tuple{starlark.String("a+b%26c")}, nil)), "a b&c"},
		{"hmac_sha256", str(BuiltinHMACSHA256(th, nil, starlark.Tuple{starlark.String("key"), starlark.String("The quick brown fox jumps over the lazy dog")}, nil)), "f7bc83f430538424b13298e6aa6fb143ef4d59a14946175997479dbc2d1a3cd8"},
	}

	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s = %q, want %q", c.name, c.got, c.want)
		}
	}

	id := str(BuiltinUUID(th, nil, starlark.Tuple{}, nil))
	if len(id) != 36 || strings.Count(id, "-") != 4 {
		t.Errorf("uuid = %q, want a 36 char hyphenated UUID", id)
	}

	if _, err := BuiltinHexDecode(th, nil, starlark.Tuple{starlark.String("zz")}, nil); err == nil {
		t.Error("hex_decode should reject non hex input")
	}
}

func TestBuiltinsIncludesUtilAndModules(t *testing.T) {
	b := Builtins()
	for _, name := range []string{"sha256", "sha1", "md5", "hmac_sha256", "hex_encode", "hex_decode", "uuid", "url_encode", "url_decode", "time", "math"} {
		if _, ok := b[name]; !ok {
			t.Errorf("Builtins is missing %q", name)
		}
	}
}

func TestHTTPResponseCap(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("0123456789"))
	}))
	defer srv.Close()

	saved := MaxHTTPResponseBytes
	MaxHTTPResponseBytes = 4
	defer func() { MaxHTTPResponseBytes = saved }()

	th := &starlark.Thread{Name: "t"}
	args := starlark.Tuple{
		starlark.String("GET"), starlark.String(srv.URL),
		starlark.String(""), starlark.String(""),
		starlark.Bool(false), starlark.MakeInt(5),
		starlark.String(""),
	}

	if _, err := BuiltinHTTPRequest(th, nil, args, nil); err == nil {
		t.Error("http_request_raw should error when the response exceeds the cap")
	}
}
