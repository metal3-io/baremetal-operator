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
	"cmp"
	"os"

	"github.com/metal3-io/baremetal-operator/pkg/provisioner"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner/ironic"
	ironicv1alpha1 "github.com/metal3-io/ironic-standalone-operator/api/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const pluginName = "ironic"

const (
	envIronicName      = "IRONIC_NAME"
	envIronicNamespace = "IRONIC_NAMESPACE"
)

// PluginName is advertised to the host via plugin.Lookup.
func PluginName() string { return pluginName }

// HostConfigure registers the Ironic CRD scheme and scopes the cache to the
// Ironic CR namespace.
//
//nolint:unparam // signature dictated by the plugin contract
func HostConfigure(input provisioner.HostConfigureInput) (provisioner.HostRequirements, error) {
	reqs := provisioner.HostRequirements{
		AddToScheme: ironicv1alpha1.AddToScheme,
	}

	ironicName := os.Getenv(envIronicName)
	ironicNamespace := cmp.Or(os.Getenv(envIronicNamespace), input.ProvisionerNamespace)
	if ironicName != "" && ironicNamespace != "" {
		reqs.CacheByObject = map[client.Object]cache.ByObject{
			&ironicv1alpha1.Ironic{}: {
				Namespaces: map[string]cache.Config{ironicNamespace: {}},
			},
		}
	}

	return reqs, nil
}

// NewProvisionerFactory is the exported symbol BMO looks up in the plugin,
// resolved at runtime via plugin.Lookup so static analysis cannot see the
// reference.
func NewProvisionerFactory(config provisioner.PluginConfig) (provisioner.Factory, error) {
	logger := config.Logger.WithName(pluginName)
	havePreprovImgBuilder := config.HasFeature(provisioner.FeaturePreprovisioningImage)

	ironicName := os.Getenv(envIronicName)
	ironicNamespace := cmp.Or(os.Getenv(envIronicNamespace), config.ProvisionerNamespace)

	if config.K8sClient != nil && ironicName != "" && ironicNamespace != "" {
		return ironic.NewProvisionerFactoryWithClient(
			logger, havePreprovImgBuilder,
			config.K8sClient, config.APIReader,
			ironicName, ironicNamespace,
		)
	}

	return ironic.NewProvisionerFactory(logger, havePreprovImgBuilder)
}
