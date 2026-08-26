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

package main

import (
	"fmt"
	"os"

	"github.com/metal3-io/baremetal-operator/pkg/provisioner"
	"github.com/metal3-io/baremetal-operator/pkg/secretutils"
	starlarkprov "github.com/s3rj1k/starlark-provisioner"
)

const pluginName = "starlark"

// envStarlarkScript points at the Starlark script implementing the provisioner
// interface, replacing the former host flag as the ironic plugin does via env.
const envStarlarkScript = "STARLARK_PROVISIONER_SCRIPT"

// PluginName is advertised to the host via plugin.Lookup.
func PluginName() string { return pluginName }

// NewProvisionerFactory is the exported symbol BMO looks up in the plugin,
// resolved at runtime via plugin.Lookup so static analysis cannot see it.
func NewProvisionerFactory(config provisioner.PluginConfig) (provisioner.Factory, error) {
	scriptPath := os.Getenv(envStarlarkScript)
	if scriptPath == "" {
		return nil, fmt.Errorf("starlark provisioner requires %s to point at a script", envStarlarkScript)
	}

	logger := config.Logger.WithName(pluginName)
	logger.Info("using starlark provisioner", "script", scriptPath)

	// KubeHostResolver backs read_host_secret and read_host_spec. The client is
	// supplied by the host and may be nil, in which case only host reads fail.
	hostResolver := &starlarkprov.KubeHostResolver{
		Client:    config.K8sClient,
		APIReader: config.APIReader,
		SecretManager: secretutils.NewSecretManager(
			logger.WithName("host-resolver"),
			config.K8sClient,
			config.APIReader,
		),
		// The host already resolved this, so take it rather than re-deriving it
		// from POD_NAMESPACE and silently listing cluster wide when that is unset.
		PodNamespace: config.ProvisionerNamespace,
	}

	return starlarkprov.NewProvisionerFactory(scriptPath, hostResolver)
}
