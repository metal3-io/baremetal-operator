// SPDX-License-Identifier: Apache-2.0

package main

import (
	"github.com/metal3-io/baremetal-operator/pkg/provisioner"
	anacondaprov "metal3.local/anaconda/internal/anaconda"
	"metal3.local/anaconda/internal/core"
	"metal3.local/anaconda/internal/kube"
)

const pluginName = "anaconda"

// PluginName is advertised to the host via plugin.Lookup.
func PluginName() string { return pluginName }

// NewProvisionerFactory is the exported symbol BMO looks up in the plugin,
// resolved at runtime via plugin.Lookup so static analysis cannot see it.
func NewProvisionerFactory(config provisioner.PluginConfig) (provisioner.Factory, error) {
	cfg := core.LoadConfig()

	logger := config.Logger.WithName(pluginName)
	logger.Info("using anaconda provisioner",
		"listenAddr", cfg.ListenAddr, "installTimeout", cfg.InstallTimeout)

	// The client is supplied by the host and may be nil, in which case only the
	// cluster backed parts (kickstart, callback, install state) fail.
	resolver := &kube.HostResolver{
		Client:    config.K8sClient,
		APIReader: config.APIReader,
	}

	return anacondaprov.NewProvisionerFactory(cfg, resolver, resolver)
}
