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

// plugin-load-test opens a provisioner plugin .so, calls each exported entry
// point including a NewProvisioner + GetHealth round trip, and exits
// non-zero on any failure. Used to verify that an out-of-tree plugin build
// remains compatible with the host's plugin contract.
package main

import (
	"context"
	"log"
	"os"
	"strings"

	"github.com/go-logr/logr"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner"
)

const (
	minArgs       = 3
	maxArgs       = 4
	usageExitCode = 2
)

func main() {
	log.SetFlags(0)

	if len(os.Args) < minArgs || len(os.Args) > maxArgs {
		log.Println("usage: plugin-load-test <plugin-path> <expected-name> [<gethealth-contains>]")
		os.Exit(usageExitCode)
	}

	pluginPath, expectedName := os.Args[1], os.Args[2]
	var healthContains string
	if len(os.Args) == maxArgs {
		healthContains = os.Args[3]
	}

	p, err := provisioner.Open(pluginPath, expectedName)
	if err != nil {
		log.Fatalf("Open: %v", err)
	}
	log.Printf("Open OK          name=%s path=%s", p.Name(), p.Path())

	reqs, err := p.HostConfigure(provisioner.HostConfigureInput{Logger: logr.Discard()})
	if err != nil {
		log.Fatalf("HostConfigure: %v", err)
	}

	log.Printf("HostConfigure OK addToScheme=%v cacheByObject=%d",
		reqs.AddToScheme != nil, len(reqs.CacheByObject))

	factory, err := p.NewFactory(provisioner.PluginConfig{Logger: logr.Discard()})
	if err != nil {
		log.Fatalf("NewFactory: %v", err)
	}
	if factory == nil {
		log.Fatal("NewFactory returned nil factory")
	}

	log.Println("NewFactory OK")

	ctx := context.Background()

	prov, err := factory.NewProvisioner(ctx, provisioner.HostData{}, func(_, _ string) {})
	if err != nil {
		log.Fatalf("NewProvisioner: %v", err)
	}
	if prov == nil {
		log.Fatal("NewProvisioner returned nil")
	}

	health := prov.GetHealth(ctx)
	log.Printf("GetHealth OK     output=%q", health)

	if healthContains != "" && !strings.Contains(health, healthContains) {
		log.Fatalf("GetHealth output %q does not contain %q", health, healthContains)
	}

	log.Println("plugin-load-test PASSED")
}
