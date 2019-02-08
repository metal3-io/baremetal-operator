// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package e2e

import (
	goctx "context"
	"fmt"
	"testing"
	"time"

	apis "github.com/metalkube/baremetal-operator/pkg/apis"
	metalkube "github.com/metalkube/baremetal-operator/pkg/apis/metalkube/v1alpha1"
	// operator "github.com/metalkube/baremetal-operator/pkg/controller/baremetalhost"
	"github.com/metalkube/baremetal-operator/pkg/utils"

	framework "github.com/operator-framework/operator-sdk/pkg/test"
	// "github.com/operator-framework/operator-sdk/pkg/test/e2eutil"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
)

var (
	retryInterval        = time.Second * 5
	timeout              = time.Second * 60
	cleanupRetryInterval = time.Second * 1
	cleanupTimeout       = time.Second * 5
)

// Set up the test system to know about our types and return a
// context.
func setup(t *testing.T) *framework.TestCtx {
	bmhList := &metalkube.BareMetalHostList{
		TypeMeta: metav1.TypeMeta{
			Kind:       "BareMetalHost",
			APIVersion: "baremetalhosts.metalkube.org/v1alpha1",
		},
	}
	err := framework.AddToFrameworkScheme(apis.AddToScheme, bmhList)
	if err != nil {
		t.Fatalf("failed to add custom resource scheme to framework: %v", err)
	}

	t.Parallel()
	ctx := framework.NewTestCtx(t)

	err = ctx.InitializeClusterResources(&framework.CleanupOptions{TestContext: ctx, Timeout: cleanupTimeout, RetryInterval: cleanupRetryInterval})
	if err != nil {
		t.Fatalf("failed to initialize cluster resources: %v", err)
	}
	t.Log("Initialized cluster resources")

	return ctx
}

// Create a new BareMetalHost instance.
func newHost(t *testing.T, ctx *framework.TestCtx, name string, spec *metalkube.BareMetalHostSpec) *metalkube.BareMetalHost {
	namespace, err := ctx.GetNamespace()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Using namespace: %v\n", namespace)

	host := &metalkube.BareMetalHost{
		TypeMeta: metav1.TypeMeta{
			Kind:       "BareMetalHost",
			APIVersion: "baremetalhosts.metalkube.org/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-%s", ctx.GetID(), name),
			Namespace: namespace,
		},
		Spec: *spec,
	}

	return host
}

// Create a BareMetalHost and publish it to the test system.
func makeHost(t *testing.T, ctx *framework.TestCtx, name string, spec *metalkube.BareMetalHostSpec) *metalkube.BareMetalHost {
	host := newHost(t, ctx, name, spec)

	// get global framework variables
	f := framework.Global

	// use TestCtx's create helper to create the object and add a
	// cleanup function for the new object
	err := f.Client.Create(
		goctx.TODO(),
		host,
		&framework.CleanupOptions{
			TestContext:   ctx,
			Timeout:       cleanupTimeout,
			RetryInterval: cleanupRetryInterval})
	if err != nil {
		t.Fatal(err)
	}

	return host
}

type DoneFunc func(host *metalkube.BareMetalHost) (bool, error)

func refreshHost(host *metalkube.BareMetalHost) error {
	f := framework.Global
	namespacedName := types.NamespacedName{
		Namespace: host.ObjectMeta.Namespace,
		Name:      host.ObjectMeta.Name,
	}
	return f.Client.Get(goctx.TODO(), namespacedName, host)
}

func waitForHostStateChange(t *testing.T, host *metalkube.BareMetalHost, isDone DoneFunc) *metalkube.BareMetalHost {
	instance := &metalkube.BareMetalHost{}
	instance.ObjectMeta = host.ObjectMeta

	err := wait.Poll(retryInterval, timeout, func() (done bool, err error) {
		t.Log("polling host for updates")
		refreshHost(instance)
		if err != nil {
			return false, err
		}
		done, err = isDone(instance)
		return done, err
	})
	if err != nil {
		t.Fatal(err)
	}

	return instance
}

func waitForErrorStatus(t *testing.T, host *metalkube.BareMetalHost) {
	waitForHostStateChange(t, host, func(host *metalkube.BareMetalHost) (done bool, err error) {
		state := host.Labels[metalkube.OperationalStatusLabel]
		t.Logf("OperationalState: %s", state)
		if state == metalkube.OperationalStatusError {
			return true, nil
		}
		return false, nil
	})
}

func TestAddFinalizers(t *testing.T) {
	ctx := setup(t)
	defer ctx.Cleanup()

	exampleHost := makeHost(t, ctx, "gets-finalizers",
		&metalkube.BareMetalHostSpec{
			BMC: metalkube.BMCDetails{
				IP:       "192.168.100.100",
				Username: "user",
				Password: "pass",
			},
		})

	waitForHostStateChange(t, exampleHost, func(host *metalkube.BareMetalHost) (done bool, err error) {
		t.Logf("finalizers: %v", host.ObjectMeta.Finalizers)
		if utils.StringInList(host.ObjectMeta.Finalizers, metalkube.BareMetalHostFinalizer) {
			return true, nil
		}
		return false, nil
	})
}

func TestSetLastUpdated(t *testing.T) {
	ctx := setup(t)
	defer ctx.Cleanup()

	exampleHost := makeHost(t, ctx, "gets-last-updated",
		&metalkube.BareMetalHostSpec{
			BMC: metalkube.BMCDetails{
				IP:       "192.168.100.100",
				Username: "user",
				Password: "pass",
			},
		})

	waitForHostStateChange(t, exampleHost, func(host *metalkube.BareMetalHost) (done bool, err error) {
		t.Logf("LastUpdated: %v", host.Status.LastUpdated)
		if !host.Status.LastUpdated.IsZero() {
			return true, nil
		}
		return false, nil
	})
}

func TestMissingBMCParameters(t *testing.T) {
	ctx := setup(t)
	defer ctx.Cleanup()

	no_ip := makeHost(t, ctx, "missing-bmc-ip",
		&metalkube.BareMetalHostSpec{
			BMC: metalkube.BMCDetails{
				IP:       "",
				Username: "user",
				Password: "pass",
			},
		})
	waitForErrorStatus(t, no_ip)

	no_username := makeHost(t, ctx, "missing-bmc-username",
		&metalkube.BareMetalHostSpec{
			BMC: metalkube.BMCDetails{
				IP:       "192.168.100.100",
				Username: "",
				Password: "pass",
			},
		})
	waitForErrorStatus(t, no_username)

	no_password := makeHost(t, ctx, "missing-bmc-password",
		&metalkube.BareMetalHostSpec{
			BMC: metalkube.BMCDetails{
				IP:       "192.168.100.100",
				Username: "user",
				Password: "",
			},
		})
	waitForErrorStatus(t, no_password)
}

func TestSetOffline(t *testing.T) {
	ctx := setup(t)
	defer ctx.Cleanup()

	exampleHost := makeHost(t, ctx, "gets-last-updated",
		&metalkube.BareMetalHostSpec{
			BMC: metalkube.BMCDetails{
				IP:       "192.168.100.100",
				Username: "user",
				Password: "pass",
			},
			Online: true,
		})

	waitForHostStateChange(t, exampleHost, func(host *metalkube.BareMetalHost) (done bool, err error) {
		state := host.Labels[metalkube.OperationalStatusLabel]
		t.Logf("OperationalState before toggle: %s", state)
		if state == metalkube.OperationalStatusOnline {
			return true, nil
		}
		return false, nil
	})

	refreshHost(exampleHost)
	exampleHost.Spec.Online = false
	f := framework.Global
	err := f.Client.Update(goctx.TODO(), exampleHost)
	if err != nil {
		t.Fatal(err)
	}

	waitForHostStateChange(t, exampleHost, func(host *metalkube.BareMetalHost) (done bool, err error) {
		state := host.Labels[metalkube.OperationalStatusLabel]
		t.Logf("OperationalState after toggle: %s", state)
		if state == metalkube.OperationalStatusOffline {
			return true, nil
		}
		return false, nil
	})

}

func TestSetOnline(t *testing.T) {
	ctx := setup(t)
	defer ctx.Cleanup()

	exampleHost := makeHost(t, ctx, "gets-last-updated",
		&metalkube.BareMetalHostSpec{
			BMC: metalkube.BMCDetails{
				IP:       "192.168.100.100",
				Username: "user",
				Password: "pass",
			},
			Online: false,
		})

	waitForHostStateChange(t, exampleHost, func(host *metalkube.BareMetalHost) (done bool, err error) {
		state := host.Labels[metalkube.OperationalStatusLabel]
		t.Logf("OperationalState before toggle: %s", state)
		if state == metalkube.OperationalStatusOffline {
			return true, nil
		}
		return false, nil
	})

	refreshHost(exampleHost)
	exampleHost.Spec.Online = true
	f := framework.Global
	err := f.Client.Update(goctx.TODO(), exampleHost)
	if err != nil {
		t.Fatal(err)
	}

	waitForHostStateChange(t, exampleHost, func(host *metalkube.BareMetalHost) (done bool, err error) {
		state := host.Labels[metalkube.OperationalStatusLabel]
		t.Logf("OperationalState after toggle: %s", state)
		if state == metalkube.OperationalStatusOnline {
			return true, nil
		}
		return false, nil
	})

}

func TestSetHardwareProfileLabel(t *testing.T) {
	ctx := setup(t)
	defer ctx.Cleanup()

	exampleHost := makeHost(t, ctx, "hardware-profile",
		&metalkube.BareMetalHostSpec{
			BMC: metalkube.BMCDetails{
				IP:       "192.168.100.100",
				Username: "user",
				Password: "pass",
			},
		})

	waitForHostStateChange(t, exampleHost, func(host *metalkube.BareMetalHost) (done bool, err error) {
		t.Logf("labels: %v", host.ObjectMeta.Labels)
		if host.ObjectMeta.Labels[metalkube.HardwareProfileLabel] != "" {
			return true, nil
		}
		return false, nil
	})
}
