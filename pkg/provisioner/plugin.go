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

package provisioner

import (
	"fmt"
	"plugin"
	"slices"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// HostFeature identifies an optional capability that the host binary has
// enabled. The host passes the set of active features to the plugin via
// PluginConfig so the plugin can adapt its behavior without duplicating the
// host's flag plumbing.
type HostFeature string

const (
	// FeaturePreprovImg indicates that a PreprovisioningImage
	// controller is running in the host manager.
	FeaturePreprovImg HostFeature = "PreprovImg"
)

// PluginConfig carries initialization parameters from the host binary to a provisioner plugin.
type PluginConfig struct {
	Logger    logr.Logger
	Features  []HostFeature
	K8sClient client.Client
	APIReader client.Reader
}

// HasFeature reports whether the host enabled the given feature.
func (c PluginConfig) HasFeature(f HostFeature) bool {
	return slices.Contains(c.Features, f)
}

// LoadProvisionerPlugin opens a Go plugin .so and looks up the exported
// PluginName (string) and NewProvisionerFactory (function) symbols. The
// returned name is whatever the plugin advertises for itself.
func LoadProvisionerPlugin(path string, config PluginConfig) (Factory, string, error) {
	p, err := plugin.Open(path)
	if err != nil {
		return nil, "", fmt.Errorf("failed to open plugin %s: %w", path, err)
	}

	nameSym, err := p.Lookup("PluginName")
	if err != nil {
		return nil, "", fmt.Errorf("plugin %s does not export PluginName: %w", path, err)
	}

	nameFunc, ok := nameSym.(func() string)
	if !ok {
		return nil, "", fmt.Errorf("plugin %s: PluginName has wrong signature (want func() string)", path)
	}

	factorySym, err := p.Lookup("NewProvisionerFactory")
	if err != nil {
		return nil, "", fmt.Errorf("plugin %s does not export NewProvisionerFactory: %w", path, err)
	}

	factoryFunc, ok := factorySym.(func(PluginConfig) (Factory, error))
	if !ok {
		return nil, "", fmt.Errorf("plugin %s: NewProvisionerFactory has wrong signature (want func(PluginConfig) (Factory, error))", path)
	}

	factory, err := factoryFunc(config)
	if err != nil {
		return nil, "", err
	}

	return factory, nameFunc(), nil
}
