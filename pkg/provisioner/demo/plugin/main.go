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
	"github.com/metal3-io/baremetal-operator/pkg/provisioner"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner/demo"
)

// PluginName is advertised to the host via plugin.Lookup.
func PluginName() string { return "demo" }

// NewProvisionerFactory is the exported symbol that BMO looks up in the plugin.
// It is resolved at runtime via plugin.Lookup from the host binary, so static
// analysis cannot see the reference.
func NewProvisionerFactory(_ provisioner.PluginConfig) (provisioner.Factory, error) {
	return &demo.Demo{}, nil
}
