/*
Copyright 2023 The Metal3 Authors.

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

package main

import (
	"bytes"
	"crypto/tls"
	"testing"

	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
)

func TestAddToNamespaces(t *testing.T) {
	t.Run("adds a namespace to an empty map", func(t *testing.T) {
		g := NewWithT(t)
		result := addToNamespaces(nil, "baremetal-operator-system")
		g.Expect(result).To(Equal(map[string]cache.Config{
			"baremetal-operator-system": {},
		}))
	})

	t.Run("copies the existing namespaces and adds the new namespace", func(t *testing.T) {
		g := NewWithT(t)
		tenants := map[string]cache.Config{
			"tenant-a": {FieldSelector: fields.OneTermEqualSelector("metadata.name", "host-a")},
			"tenant-b": {},
		}

		result := addToNamespaces(tenants, "baremetal-operator-system")

		g.Expect(result).To(HaveLen(3))
		g.Expect(result["tenant-a"]).To(Equal(tenants["tenant-a"]))
		g.Expect(result).To(HaveKey("tenant-b"))
		g.Expect(result).To(HaveKey("baremetal-operator-system"))
		g.Expect(tenants).To(HaveLen(2))
		g.Expect(tenants).NotTo(HaveKey("baremetal-operator-system"))
	})

	t.Run("preserves config when namespace is already present", func(t *testing.T) {
		g := NewWithT(t)
		config := cache.Config{FieldSelector: fields.OneTermEqualSelector("metadata.name", "host-a")}
		watched := map[string]cache.Config{"baremetal-operator-system": config}

		result := addToNamespaces(watched, "baremetal-operator-system")

		g.Expect(result).To(Equal(watched))
	})

	t.Run("empty namespace leaves namespaces unchanged", func(t *testing.T) {
		g := NewWithT(t)
		watched := map[string]cache.Config{"tenant-a": {}}

		result := addToNamespaces(watched, "")

		g.Expect(result).To(Equal(watched))
		g.Expect(result).NotTo(HaveKey(cache.AllNamespaces))
	})
}

func TestResolveSwitchSecretsNamespace(t *testing.T) {
	tests := []struct {
		name            string
		ironicNamespace string
		podNamespace    string
		want            string
	}{
		{
			name:            "prefers Ironic namespace",
			ironicNamespace: "ironic-system",
			podNamespace:    "baremetal-operator-system",
			want:            "ironic-system",
		},
		{
			name:         "falls back to BMO pod namespace",
			podNamespace: "baremetal-operator-system",
			want:         "baremetal-operator-system",
		},
		{
			name: "returns empty when neither namespace is set",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			g.Expect(resolveSwitchSecretsNamespace(tt.ironicNamespace, tt.podNamespace)).To(Equal(tt.want))
		})
	}
}

func TestTLSInsecureCiperSuite(t *testing.T) {
	t.Run("test insecure cipher suite passed as TLS flag", func(t *testing.T) {
		g := NewWithT(t)
		tlsMockOptions := TLSOptions{
			TLSMaxVersion:   "TLS13",
			TLSMinVersion:   "TLS12",
			TLSCipherSuites: "TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA256",
		}
		ctrl.Log.WithName("setup")
		ctrl.SetLogger(klog.Background())

		bufWriter := bytes.NewBuffer(nil)
		klog.SetOutput(bufWriter)
		klog.LogToStderr(false) // this is important, because klog by default logs to stderr only
		_, err := GetTLSOptionOverrideFuncs(tlsMockOptions)
		g.Expect(err).ShouldNot(HaveOccurred())
		// Check for structured log format: key=value pairs
		g.Expect(bufWriter.String()).Should(ContainSubstring("use of insecure cipher detected"))
		g.Expect(bufWriter.String()).Should(ContainSubstring("TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA256"))
	})
}

func TestTLSMinAndMaxVersion(t *testing.T) {
	t.Run("should fail if TLS min version is greater than max version.", func(t *testing.T) {
		g := NewWithT(t)
		tlsMockOptions := TLSOptions{
			TLSMaxVersion: "TLS12",
			TLSMinVersion: "TLS13",
		}
		_, err := GetTLSOptionOverrideFuncs(tlsMockOptions)
		g.Expect(err.Error()).To(Equal("TLS version flag min version (TLS13) is greater than max version (TLS12)"))
	})
}

func Test13CipherSuite(t *testing.T) {
	t.Run("should reset ciphersuite flag if TLS min and max version are set to 1.3", func(t *testing.T) {
		g := NewWithT(t)

		// Here TLS_RSA_WITH_AES_128_CBC_SHA is a tls12 cipher suite.
		tlsMockOptions := TLSOptions{
			TLSMaxVersion:   "TLS13",
			TLSMinVersion:   "TLS13",
			TLSCipherSuites: "TLS_RSA_WITH_AES_128_CBC_SHA,TLS_AES_256_GCM_SHA384",
		}

		ctrl.Log.WithName("setup")
		ctrl.SetLogger(klog.Background())

		bufWriter := bytes.NewBuffer(nil)
		klog.SetOutput(bufWriter)
		klog.LogToStderr(false) // this is important, because klog by default logs to stderr only
		_, err := GetTLSOptionOverrideFuncs(tlsMockOptions)
		g.Expect(err).ShouldNot(HaveOccurred())
		g.Expect(bufWriter.String()).Should(ContainSubstring("warning: Cipher suites should not be set for TLS version 1.3. Ignoring ciphers"))
	})
}

func TestGetTLSOverrideFuncs(t *testing.T) {
	t.Run("should error out when incorrect min tls version passed", func(t *testing.T) {
		g := NewWithT(t)
		_, err := GetTLSOptionOverrideFuncs(TLSOptions{
			TLSMinVersion: "TLS11",
			TLSMaxVersion: "TLS12",
		})
		g.Expect(err.Error()).Should(Equal("unexpected TLS version \"TLS11\" (must be one of: TLS12, TLS13)"))
	})
	t.Run("should error out when incorrect max tls version passed", func(t *testing.T) {
		g := NewWithT(t)
		_, err := GetTLSOptionOverrideFuncs(TLSOptions{
			TLSMinVersion: "TLS12",
			TLSMaxVersion: "TLS11",
		})
		g.Expect(err.Error()).Should(Equal("unexpected TLS version \"TLS11\" (must be one of: TLS12, TLS13)"))
	})
	t.Run("should apply the requested TLS versions", func(t *testing.T) {
		g := NewWithT(t)
		tlsOptionOverrides, err := GetTLSOptionOverrideFuncs(TLSOptions{
			TLSMinVersion: "TLS12",
			TLSMaxVersion: "TLS13",
		})

		var tlsConfig tls.Config
		for _, apply := range tlsOptionOverrides {
			apply(&tlsConfig)
		}

		g.Expect(err).ShouldNot(HaveOccurred())
		g.Expect(tlsConfig.MinVersion).To(Equal(uint16(tls.VersionTLS12)))
		g.Expect(tlsConfig.MaxVersion).To(Equal(uint16(tls.VersionTLS13)))
	})
	t.Run("should apply the requested non-default TLS versions", func(t *testing.T) {
		g := NewWithT(t)
		tlsOptionOverrides, err := GetTLSOptionOverrideFuncs(TLSOptions{
			TLSMinVersion: "TLS13",
			TLSMaxVersion: "TLS13",
		})

		var tlsConfig tls.Config
		for _, apply := range tlsOptionOverrides {
			apply(&tlsConfig)
		}

		g.Expect(err).ShouldNot(HaveOccurred())
		g.Expect(tlsConfig.MinVersion).To(Equal(uint16(tls.VersionTLS13)))
		g.Expect(tlsConfig.MaxVersion).To(Equal(uint16(tls.VersionTLS13)))
	})
}

func TestTLSOptions(t *testing.T) {
	t.Run("should pass with all the correct options below with no error.", func(t *testing.T) {
		g := NewWithT(t)
		tlsMockOptions := TLSOptions{
			TLSMinVersion:   "TLS12",
			TLSMaxVersion:   "TLS13",
			TLSCipherSuites: "TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384",
		}
		_, err := GetTLSOptionOverrideFuncs(tlsMockOptions)
		g.Expect(err).ShouldNot(HaveOccurred())
	})
}

func TestTLSCurvePreferences(t *testing.T) {
	t.Run("valid curve preferences should be parsed correctly", func(t *testing.T) {
		g := NewWithT(t)
		tlsMockOptions := TLSOptions{
			TLSMinVersion:       "TLS12",
			TLSMaxVersion:       "TLS13",
			TLSCurvePreferences: "X25519,CurveP256,CurveP384",
		}
		tlsOptionOverrides, err := GetTLSOptionOverrideFuncs(tlsMockOptions)
		g.Expect(err).ShouldNot(HaveOccurred())

		var tlsConfig tls.Config
		for _, apply := range tlsOptionOverrides {
			apply(&tlsConfig)
		}

		g.Expect(tlsConfig.CurvePreferences).To(Equal([]tls.CurveID{
			tls.X25519,
			tls.CurveP256,
			tls.CurveP384,
		}))
	})
	t.Run("all supported curve names should be accepted", func(t *testing.T) {
		g := NewWithT(t)
		tlsMockOptions := TLSOptions{
			TLSMinVersion:       "TLS12",
			TLSMaxVersion:       "TLS13",
			TLSCurvePreferences: "X25519,CurveP256,CurveP384,CurveP521,X25519MLKEM768",
		}
		tlsOptionOverrides, err := GetTLSOptionOverrideFuncs(tlsMockOptions)
		g.Expect(err).ShouldNot(HaveOccurred())

		var tlsConfig tls.Config
		for _, apply := range tlsOptionOverrides {
			apply(&tlsConfig)
		}

		g.Expect(tlsConfig.CurvePreferences).To(Equal([]tls.CurveID{
			tls.X25519,
			tls.CurveP256,
			tls.CurveP384,
			tls.CurveP521,
			tls.X25519MLKEM768,
		}))
	})
	t.Run("empty curve preferences should have no effect", func(t *testing.T) {
		g := NewWithT(t)
		tlsMockOptions := TLSOptions{
			TLSMinVersion: "TLS12",
			TLSMaxVersion: "TLS13",
		}
		tlsOptionOverrides, err := GetTLSOptionOverrideFuncs(tlsMockOptions)
		g.Expect(err).ShouldNot(HaveOccurred())

		var tlsConfig tls.Config
		for _, apply := range tlsOptionOverrides {
			apply(&tlsConfig)
		}

		g.Expect(tlsConfig.CurvePreferences).To(BeNil())
	})
	t.Run("invalid curve name should return error", func(t *testing.T) {
		g := NewWithT(t)
		tlsMockOptions := TLSOptions{
			TLSMinVersion:       "TLS12",
			TLSMaxVersion:       "TLS13",
			TLSCurvePreferences: "X25519,InvalidCurve",
		}
		_, err := GetTLSOptionOverrideFuncs(tlsMockOptions)
		g.Expect(err).Should(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("unrecognized TLS curve preference"))
		g.Expect(err.Error()).To(ContainSubstring("InvalidCurve"))
	})
	t.Run("whitespace around names should be tolerated", func(t *testing.T) {
		g := NewWithT(t)
		tlsMockOptions := TLSOptions{
			TLSMinVersion:       "TLS12",
			TLSMaxVersion:       "TLS13",
			TLSCurvePreferences: "X25519, CurveP256, CurveP384",
		}
		tlsOptionOverrides, err := GetTLSOptionOverrideFuncs(tlsMockOptions)
		g.Expect(err).ShouldNot(HaveOccurred())

		var tlsConfig tls.Config
		for _, apply := range tlsOptionOverrides {
			apply(&tlsConfig)
		}

		g.Expect(tlsConfig.CurvePreferences).To(Equal([]tls.CurveID{
			tls.X25519,
			tls.CurveP256,
			tls.CurveP384,
		}))
	})
}
