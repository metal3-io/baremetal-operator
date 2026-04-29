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
	"os"

	"github.com/metal3-io/baremetal-operator/pkg/provisioner"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner/ironic"
)

const pluginName = "ironic"

// PluginName is advertised to the host via plugin.Lookup.
func PluginName() string { return pluginName }

// NewProvisionerFactory is the exported symbol that BMO looks up in the plugin.
// It is resolved at runtime via plugin.Lookup from the host binary, so static
// analysis cannot see the reference.
func NewProvisionerFactory(config provisioner.PluginConfig) (provisioner.Factory, error) {
	logger := config.Logger.WithName(pluginName)

	ironicName := os.Getenv("IRONIC_NAME")
	ironicNamespace := os.Getenv("IRONIC_NAMESPACE")

	havePreprovImgBuilder := config.HasFeature(provisioner.FeaturePreprovImg)

	if config.K8sClient != nil && ironicName != "" && ironicNamespace != "" {
		return ironic.NewProvisionerFactoryWithClient(
			logger, havePreprovImgBuilder,
			config.K8sClient, config.APIReader,
			ironicName, ironicNamespace,
		)
	}

	return ironic.NewProvisionerFactory(logger, havePreprovImgBuilder)
}
