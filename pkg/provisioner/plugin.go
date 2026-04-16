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
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// HostFeature identifies an optional capability the host binary has enabled,
// passed to plugins via PluginConfig so they can adapt without duplicating
// the host's flag plumbing.
type HostFeature string

const (
	// FeaturePreprovisioningImage indicates that a PreprovisioningImage
	// controller is running in the host manager.
	FeaturePreprovisioningImage HostFeature = "PreprovisioningImage"
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

// HostConfigureInput is passed to HostConfigure before the manager is built.
type HostConfigureInput struct {
	Logger   logr.Logger
	Features []HostFeature
}

// HostRequirements is returned by HostConfigure with all fields optional.
type HostRequirements struct {
	AddToScheme   func(*runtime.Scheme) error
	CacheByObject map[client.Object]cache.ByObject
}

// Plugin is a loaded provisioner plugin, Open does no K8s I/O.
type Plugin struct {
	path          string
	name          string
	factory       func(PluginConfig) (Factory, error)
	hostConfigure func(HostConfigureInput) (HostRequirements, error)
}

// Path returns the .so path.
func (p *Plugin) Path() string { return p.path }

// Name returns the plugin's self-reported name.
func (p *Plugin) Name() string { return p.name }

// NewFactory invokes the plugin's NewProvisionerFactory.
func (p *Plugin) NewFactory(config PluginConfig) (Factory, error) {
	return p.factory(config)
}

// HostConfigure invokes the optional HostConfigure symbol, yielding zero
// HostRequirements when the plugin omits it.
func (p *Plugin) HostConfigure(input HostConfigureInput) (HostRequirements, error) {
	if p.hostConfigure == nil {
		return HostRequirements{}, nil
	}

	return p.hostConfigure(input)
}

// Open loads the plugin at path and rejects a PluginName mismatch.
func Open(path, expectedName string) (*Plugin, error) {
	p, err := plugin.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open plugin %s: %w", path, err)
	}

	nameSym, err := p.Lookup("PluginName")
	if err != nil {
		return nil, fmt.Errorf("plugin %s does not export PluginName: %w", path, err)
	}

	nameFunc, ok := nameSym.(func() string)
	if !ok {
		return nil, fmt.Errorf("plugin %s: PluginName has wrong signature (want func() string)", path)
	}

	name := nameFunc()
	if expectedName != "" && name != expectedName {
		return nil, fmt.Errorf("plugin %s: PluginName() returned %q, expected %q", path, name, expectedName)
	}

	factorySym, err := p.Lookup("NewProvisionerFactory")
	if err != nil {
		return nil, fmt.Errorf("plugin %s does not export NewProvisionerFactory: %w", path, err)
	}

	factoryFunc, ok := factorySym.(func(PluginConfig) (Factory, error))
	if !ok {
		return nil, fmt.Errorf("plugin %s: NewProvisionerFactory has wrong signature (want func(PluginConfig) (Factory, error))", path)
	}

	loaded := &Plugin{path: path, name: name, factory: factoryFunc}

	// HostConfigure is optional, only a wrong signature is an error.
	var hostConfigureSym plugin.Symbol

	hostConfigureSym, err = p.Lookup("HostConfigure")
	if err == nil {
		hostConfigureFunc, ok := hostConfigureSym.(func(HostConfigureInput) (HostRequirements, error))
		if !ok {
			return nil, fmt.Errorf("plugin %s: HostConfigure has wrong signature (want func(HostConfigureInput) (HostRequirements, error))", path)
		}

		loaded.hostConfigure = hostConfigureFunc
	}

	return loaded, nil
}
