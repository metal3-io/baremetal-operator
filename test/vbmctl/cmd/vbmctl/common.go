//go:build vbmctl
// +build vbmctl

// Provides common utilities and shared code for vbmctl commands.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/metal3-io/baremetal-operator/test/vbmctl/pkg/config"
)

var (
	// Global flags.
	cfgFile     string
	libvirtURI  string
	showVersion bool
)

// loadConfig loads the configuration from file or returns defaults.
func loadConfig() (*config.Config, error) {
	var (
		cfg *config.Config
		err error
	)

	// Use explicit config file if provided
	if cfgFile != "" {
		cfg, err = config.Load(cfgFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load config from %s: %w", cfgFile, err)
		}
	} else if found := config.FindConfigFile(); found != "" {
		// Try to find config file in standard locations
		cfg, err = config.Load(found)
		if err != nil {
			return nil, fmt.Errorf("failed to load config from %s: %w", found, err)
		}
	} else {
		// Use defaults
		cfg = config.Default()
	}

	// Apply command-line overrides, then defaults, then validate.
	applyFlagOverrides(cfg)
	cfg.ApplyDefaults()

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return cfg, nil
}

// applyFlagOverrides applies command-line flag overrides to the config.
func applyFlagOverrides(cfg *config.Config) {
	if libvirtURI != "" {
		cfg.Spec.Libvirt.URI = libvirtURI
	}
}

// contextWithSignal creates a context that is canceled on SIGINT or SIGTERM.
func contextWithSignal() (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		defer signal.Stop(sigChan)
		select {
		case <-sigChan:
			cancel()
		case <-ctx.Done():
		}
	}()

	return ctx, cancel
}
