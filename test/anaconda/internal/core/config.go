// SPDX-License-Identifier: Apache-2.0

// Every knob the plugin reads from the environment, resolved once at load.

package core

import (
	"net"
	"os"
	"strings"
	"time"
)

// Environment variable names, all prefixed so they cannot collide with BMO's own.
const (
	EnvListenAddr     = "ANACONDA_LISTEN_ADDR"
	EnvBaseURL        = "ANACONDA_BASE_URL"
	EnvInstallTimeout = "ANACONDA_INSTALL_TIMEOUT"
)

// NormalizeDisk drops the /dev prefix anaconda does not want, so a hint written
// as /dev/vda still reaches ignoredisk and clearpart as vda.
func NormalizeDisk(s string) string {
	return strings.TrimPrefix(strings.TrimSpace(s), "/dev/")
}

func EnvDuration(name string, def time.Duration) time.Duration {
	raw := os.Getenv(name)
	if raw == "" {
		return def
	}

	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		Log.Error(err, "ignoring invalid duration, using the default", "env", name, "value", raw, "default", def)

		return def
	}

	return d
}

const DefaultInstallTimeout = time.Hour

// InstallPollInterval is how often the wait for the anaconda callback requeues.
// Nothing reports faster than a machine installs, so this is not worth tuning.
const InstallPollInterval = 30 * time.Second

type Config struct {
	// ListenAddr set starts the plain HTTP listener serving /ks and /callback.
	// Unset leaves both off, a host that can be powered but never installed.
	ListenAddr string

	// BaseURL is what a host dials back on. Derived from ListenAddr when unset,
	// because an empty one renders a kickstart that can never report in.
	BaseURL string

	// InstallTimeout bounds the wait for the anaconda callback, after which the
	// provision fails instead of requeueing forever.
	InstallTimeout time.Duration
}

func (c Config) Enabled() bool { return c.ListenAddr != "" }

func LoadConfig() Config {
	cfg := Config{
		ListenAddr: os.Getenv(EnvListenAddr),
		BaseURL:    os.Getenv(EnvBaseURL),

		InstallTimeout: EnvDuration(EnvInstallTimeout, DefaultInstallTimeout),
	}

	if cfg.BaseURL == "" && cfg.ListenAddr != "" {
		cfg.BaseURL = BaseURLFor(cfg.ListenAddr)

		// A derived base is only right when the listener binds an address the
		// host can route to, so say what was guessed rather than guess silently.
		Log.Info("no callback base URL configured, derived one from the listen address",
			"env", EnvBaseURL, "baseURL", cfg.BaseURL)
	}

	return cfg
}

// BaseURLFor derives a callback base from a listen address. A wildcard or empty
// bind names no dialable host, so it falls back to loopback rather than empty.
func BaseURLFor(listenAddr string) string {
	host, port, err := net.SplitHostPort(listenAddr)
	if err != nil {
		return ""
	}

	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}

	return "http://" + net.JoinHostPort(host, port)
}
