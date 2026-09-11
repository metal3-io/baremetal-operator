// SPDX-License-Identifier: Apache-2.0

package core_test

import (
	"testing"

	"metal3.local/anaconda/internal/core"
)

func TestLoadConfig(t *testing.T) {
	t.Setenv(core.EnvListenAddr, ":9080")
	t.Setenv(core.EnvBaseURL, "http://cb:9080")

	cfg := core.LoadConfig()

	if cfg.ListenAddr != ":9080" || cfg.BaseURL != "http://cb:9080" {
		t.Errorf("LoadConfig = %+v", cfg)
	}

	// An unset timeout must land on the default, a zero one would fail every
	// install the moment it started.
	if cfg.InstallTimeout != core.DefaultInstallTimeout {
		t.Errorf("install timeout = %s, want the default", cfg.InstallTimeout)
	}

	if !cfg.Enabled() {
		t.Error("a config with an address should be enabled")
	}
}

func TestConfigDisabledWithoutAddr(t *testing.T) {
	t.Setenv(core.EnvListenAddr, "")

	cfg := core.LoadConfig()

	if cfg.Enabled() {
		t.Error("a config without an address must be disabled")
	}
}

func TestLoadConfigIgnoresBadDurations(t *testing.T) {
	t.Setenv(core.EnvInstallTimeout, "not-a-duration")

	cfg := core.LoadConfig()

	if cfg.InstallTimeout != core.DefaultInstallTimeout {
		t.Errorf("install timeout = %s, want the default", cfg.InstallTimeout)
	}
}

// An unset base URL used to render a kickstart with an empty .CallbackURL, so
// the host installed, never reported, and failed the whole install timeout.
func TestLoadConfigDerivesABaseURL(t *testing.T) {
	cases := map[string]struct{ listen, want string }{
		"loopback bind": {":9080", "http://127.0.0.1:9080"},
		"wildcard bind": {"0.0.0.0:8080", "http://127.0.0.1:8080"},
		"explicit bind": {"10.0.0.5:8080", "http://10.0.0.5:8080"},
		"unparseable":   {"garbage", ""},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Setenv(core.EnvListenAddr, tc.listen)
			t.Setenv(core.EnvBaseURL, "")

			if got := core.LoadConfig().BaseURL; got != tc.want {
				t.Errorf("BaseURL = %q, want %q", got, tc.want)
			}
		})
	}
}

// A configured base URL is never second guessed, it is the only value that can
// be right when the listener sits behind a proxy.
func TestLoadConfigKeepsAConfiguredBaseURL(t *testing.T) {
	t.Setenv(core.EnvListenAddr, "127.0.0.1:9080")
	t.Setenv(core.EnvBaseURL, "http://node:8080")

	if got := core.LoadConfig().BaseURL; got != "http://node:8080" {
		t.Errorf("BaseURL = %q, want the configured value", got)
	}
}
