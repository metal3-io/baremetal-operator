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
	// "fmt"
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
func newHost(t *testing.T, ctx *framework.TestCtx, spec *metalkube.BareMetalHostSpec) *metalkube.BareMetalHost {
	namespace, err := ctx.GetNamespace()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Using namespace: %v\n", namespace)

	exampleName := "example-baremetalhost"
	host := &metalkube.BareMetalHost{
		TypeMeta: metav1.TypeMeta{
			Kind:       "BareMetalHost",
			APIVersion: "baremetalhosts.metalkube.org/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      exampleName,
			Namespace: namespace,
		},
		Spec: *spec,
	}

	return host
}

// Create a BareMetalHost and publish it to the test system.
func makeHost(t *testing.T, ctx *framework.TestCtx, spec *metalkube.BareMetalHostSpec) *metalkube.BareMetalHost {
	host := newHost(t, ctx, spec)

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

func TestAddFinalizers(t *testing.T) {
	ctx := setup(t)
	defer ctx.Cleanup()

	exampleHost := makeHost(t, ctx, &metalkube.BareMetalHostSpec{
		BMC: metalkube.BMCDetails{
			IP:       "192.168.100.100",
			Username: "user",
			Password: "pass",
		},
	})

	// get global framework variables
	f := framework.Global

	// Verify that the finalizers list is updated for the new host.
	namespacedName := types.NamespacedName{
		Namespace: exampleHost.ObjectMeta.Namespace,
		Name:      exampleHost.ObjectMeta.Name,
	}
	instance := &metalkube.BareMetalHost{}
	err := wait.Poll(retryInterval, timeout, func() (done bool, err error) {
		t.Log("polling host for updates")
		err = f.Client.Get(goctx.TODO(), namespacedName, instance)
		if err != nil {
			return false, err
		}
		t.Logf("finalizers: %v", instance.ObjectMeta.Finalizers)
		if utils.StringInList(instance.ObjectMeta.Finalizers, metalkube.BareMetalHostFinalizer) {
			return true, nil
		}
		return false, nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
