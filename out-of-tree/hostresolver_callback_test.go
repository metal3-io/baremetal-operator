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

package starlark

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	"github.com/metal3-io/baremetal-operator/pkg/secretutils"
	"github.com/s3rj1k/starlark-provisioner/starlib"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// newCallbackResolver builds a KubeHostResolver backed by a fake client seeded with objs.
func newCallbackResolver(t *testing.T, objs ...client.Object) *KubeHostResolver {
	t.Helper()

	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("add corev1 to scheme: %v", err)
	}

	if err := metal3api.AddToScheme(scheme); err != nil {
		t.Fatalf("add metal3api to scheme: %v", err)
	}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()

	return &KubeHostResolver{
		Client:        c,
		APIReader:     c,
		SecretManager: secretutils.NewSecretManager(logr.Discard(), c, c),
	}
}

// hostWithBMC returns a BareMetalHost referencing the named BMC credentials Secret.
func hostWithBMC(namespace, name, credsName string) *metal3api.BareMetalHost {
	return &metal3api.BareMetalHost{
		ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: name, UID: "bmh-uid"},
		Spec:       metal3api.BareMetalHostSpec{BMC: metal3api.BMCDetails{CredentialsName: credsName}},
	}
}

func TestBMCCredentialsTrims(t *testing.T) {
	host := hostWithBMC("ns", "node-1", "bmc-creds")
	creds := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "bmc-creds"},
		Data:       map[string][]byte{"username": []byte("  admin\n"), "password": []byte(" s3cret ")},
	}
	r := newCallbackResolver(t, host, creds)

	user, pass, err := r.BMCCredentials(context.Background(), "ns", "node-1")
	if err != nil {
		t.Fatalf("BMCCredentials: %v", err)
	}

	if user != "admin" || pass != "s3cret" {
		t.Errorf("got (%q, %q), want (admin, s3cret)", user, pass)
	}
}

func TestBMCCredentialsEmptyPassword(t *testing.T) {
	host := hostWithBMC("ns", "node-1", "bmc-creds")
	creds := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "bmc-creds"},
		Data:       map[string][]byte{"username": []byte("admin"), "password": []byte("   ")},
	}
	r := newCallbackResolver(t, host, creds)

	if _, _, err := r.BMCCredentials(context.Background(), "ns", "node-1"); err == nil {
		t.Error("an empty BMC password must be rejected")
	}
}

func TestWriteReadDeleteCallback(t *testing.T) {
	ctx := context.Background()
	r := newCallbackResolver(t, hostWithBMC("ns", "node-1", "bmc-creds"))

	if err := r.WriteCallback(ctx, "ns", "node-1", []byte(`{"n":1}`), "2026-01-02T03:04:05Z"); err != nil {
		t.Fatalf("WriteCallback: %v", err)
	}

	sec := &corev1.Secret{}
	key := types.NamespacedName{Namespace: "ns", Name: starlib.HostSecretName("node-1")}
	if err := r.Client.Get(ctx, key, sec); err != nil {
		t.Fatalf("get stored secret: %v", err)
	}

	if got := string(sec.Data[callbackDataKey]); got != `{"n":1}` {
		t.Errorf("stored data = %q", got)
	}

	if got := string(sec.Data[callbackReceivedAtKey]); got != "2026-01-02T03:04:05Z" {
		t.Errorf("stored receivedAt = %q", got)
	}

	if sec.Labels[secretutils.LabelEnvironmentName] != secretutils.LabelEnvironmentValue {
		t.Error("callback secret is missing the baremetal cache label")
	}

	if len(sec.OwnerReferences) != 1 || sec.OwnerReferences[0].UID != "bmh-uid" || sec.OwnerReferences[0].Kind != "BareMetalHost" {
		t.Errorf("owner references = %+v, want one ref back to the host", sec.OwnerReferences)
	}

	got, err := r.ReadCallback(ctx, "ns", "node-1")
	if err != nil {
		t.Fatalf("ReadCallback: %v", err)
	}

	if got["data"] != `{"n":1}` || got["receivedAt"] != "2026-01-02T03:04:05Z" {
		t.Errorf("ReadCallback = %v", got)
	}

	// A second write updates the same secret instead of failing on AlreadyExists.
	if err := r.WriteCallback(ctx, "ns", "node-1", []byte(`{"n":2}`), "later"); err != nil {
		t.Fatalf("second WriteCallback: %v", err)
	}

	if got, _ = r.ReadCallback(ctx, "ns", "node-1"); got["data"] != `{"n":2}` {
		t.Errorf("second write not persisted, data = %v", got["data"])
	}

	if err := r.DeleteCallback(ctx, "ns", "node-1"); err != nil {
		t.Fatalf("DeleteCallback: %v", err)
	}

	if got, err = r.ReadCallback(ctx, "ns", "node-1"); err != nil || got != nil {
		t.Errorf("after delete ReadCallback = (%v, %v), want (nil, nil)", got, err)
	}
}

func TestReadCallbackMissing(t *testing.T) {
	r := newCallbackResolver(t)

	got, err := r.ReadCallback(context.Background(), "ns", "absent")
	if err != nil || got != nil {
		t.Errorf("ReadCallback = (%v, %v), want (nil, nil)", got, err)
	}
}

func TestHostSecretShared(t *testing.T) {
	ctx := context.Background()
	r := newCallbackResolver(t, hostWithBMC("ns", "node-1", "bmc-creds"))

	if err := r.WriteCallback(ctx, "ns", "node-1", []byte("cbdata"), "t0"); err != nil {
		t.Fatalf("WriteCallback: %v", err)
	}

	if err := r.WriteServe(ctx, "ns", "node-1", "ks", starlib.ServeRegistration{ConfigMap: "cm", Key: "k"}); err != nil {
		t.Fatalf("WriteServe: %v", err)
	}

	// Both use cases share one Secret with distinct keys.
	sec := &corev1.Secret{}
	key := types.NamespacedName{Namespace: "ns", Name: starlib.HostSecretName("node-1")}
	if err := r.Client.Get(ctx, key, sec); err != nil {
		t.Fatalf("get host secret: %v", err)
	}

	if _, ok := sec.Data[callbackDataKey]; !ok {
		t.Error("callback data missing from shared secret")
	}

	if _, ok := sec.Data[serveRegistrationsKey]; !ok {
		t.Error("serve registrations missing from shared secret")
	}

	// Clearing callback keeps serve and the Secret.
	if err := r.DeleteCallback(ctx, "ns", "node-1"); err != nil {
		t.Fatalf("DeleteCallback: %v", err)
	}

	if cb, _ := r.ReadCallback(ctx, "ns", "node-1"); cb != nil {
		t.Error("callback should be cleared")
	}

	if _, _, found, _ := r.ReadServe(ctx, "bmh-uid", "ks"); !found {
		t.Error("serve route should survive a callback clear")
	}

	// Clearing serve too removes the now empty Secret.
	if err := r.DeleteServeHost(ctx, "ns", "node-1"); err != nil {
		t.Fatalf("DeleteServeHost: %v", err)
	}

	if err := r.Client.Get(ctx, key, sec); !k8serrors.IsNotFound(err) {
		t.Errorf("empty shared secret should be deleted, err = %v", err)
	}
}

func TestServeStoreRoundTrip(t *testing.T) {
	ctx := context.Background()
	r := newCallbackResolver(t, hostWithBMC("ns", "node-1", "bmc-creds"))

	if err := r.WriteServe(ctx, "ns", "node-1", "ks",
		starlib.ServeRegistration{ConfigMap: "ks-cm", Key: "ks.cfg", Render: true, Vars: map[string]any{"disk": "/dev/sda"}}); err != nil {
		t.Fatalf("WriteServe: %v", err)
	}

	sec := &corev1.Secret{}
	key := types.NamespacedName{Namespace: "ns", Name: starlib.HostSecretName("node-1")}
	if err := r.Client.Get(ctx, key, sec); err != nil {
		t.Fatalf("get serve secret: %v", err)
	}

	if sec.Labels[secretutils.LabelEnvironmentName] != secretutils.LabelEnvironmentValue || sec.Labels[ServeUIDLabel] != "bmh-uid" {
		t.Errorf("labels = %v, want env + uid labels", sec.Labels)
	}

	if len(sec.OwnerReferences) != 1 || sec.OwnerReferences[0].UID != "bmh-uid" || sec.OwnerReferences[0].Kind != "BareMetalHost" {
		t.Errorf("owner references = %+v, want one ref back to the host", sec.OwnerReferences)
	}

	reg, ns, found, err := r.ReadServe(ctx, "bmh-uid", "ks")
	if err != nil || !found || ns != "ns" || reg.ConfigMap != "ks-cm" || reg.Key != "ks.cfg" || !reg.Render || reg.Vars["disk"] != "/dev/sda" {
		t.Fatalf("ReadServe = (%+v, %q, %v, %v)", reg, ns, found, err)
	}

	// A second id shares the host Secret.
	if err := r.WriteServe(ctx, "ns", "node-1", "post", starlib.ServeRegistration{ConfigMap: "cm2", Key: "k2"}); err != nil {
		t.Fatalf("second WriteServe: %v", err)
	}

	if _, _, found, _ := r.ReadServe(ctx, "bmh-uid", "post"); !found {
		t.Error("second registration should be readable")
	}

	// Deleting one id keeps the other and the Secret.
	if err := r.DeleteServe(ctx, "ns", "node-1", "ks"); err != nil {
		t.Fatalf("DeleteServe: %v", err)
	}

	if _, _, found, _ := r.ReadServe(ctx, "bmh-uid", "ks"); found {
		t.Error("ks should be gone after DeleteServe")
	}

	if _, _, found, _ := r.ReadServe(ctx, "bmh-uid", "post"); !found {
		t.Error("post should remain after deleting ks")
	}

	// Deleting the last id removes the Secret.
	if err := r.DeleteServe(ctx, "ns", "node-1", "post"); err != nil {
		t.Fatalf("DeleteServe last: %v", err)
	}

	if err := r.Client.Get(ctx, key, sec); !k8serrors.IsNotFound(err) {
		t.Errorf("serve secret should be deleted once empty, get err = %v", err)
	}
}

func TestReadServeMissing(t *testing.T) {
	r := newCallbackResolver(t)

	if _, _, found, err := r.ReadServe(context.Background(), "no-uid", "ks"); err != nil || found {
		t.Errorf("ReadServe = (found=%v, err=%v), want (false, nil)", found, err)
	}
}

func TestDeleteServeHost(t *testing.T) {
	ctx := context.Background()
	r := newCallbackResolver(t, hostWithBMC("ns", "node-1", "bmc-creds"))

	if err := r.WriteServe(ctx, "ns", "node-1", "ks", starlib.ServeRegistration{ConfigMap: "cm", Key: "k"}); err != nil {
		t.Fatalf("WriteServe: %v", err)
	}

	if err := r.DeleteServeHost(ctx, "ns", "node-1"); err != nil {
		t.Fatalf("DeleteServeHost: %v", err)
	}

	if _, _, found, _ := r.ReadServe(ctx, "bmh-uid", "ks"); found {
		t.Error("all routes should be gone after DeleteServeHost")
	}

	// Deleting an absent host Secret still succeeds.
	if err := r.DeleteServeHost(ctx, "ns", "node-1"); err != nil {
		t.Errorf("DeleteServeHost on absent secret: %v", err)
	}
}

func TestConfigMapValue(t *testing.T) {
	ctx := context.Background()
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "cm"},
		Data:       map[string]string{"ks.cfg": "hello"},
		BinaryData: map[string][]byte{"blob": []byte("bin")},
	}
	r := newCallbackResolver(t, cm)

	if v, ok, err := r.ConfigMapValue(ctx, "ns", "cm", "ks.cfg"); err != nil || !ok || v != "hello" {
		t.Errorf("Data key = (%q, %v, %v)", v, ok, err)
	}

	if v, ok, err := r.ConfigMapValue(ctx, "ns", "cm", "blob"); err != nil || !ok || v != "bin" {
		t.Errorf("BinaryData key = (%q, %v, %v)", v, ok, err)
	}

	if _, ok, err := r.ConfigMapValue(ctx, "ns", "cm", "nope"); err != nil || ok {
		t.Errorf("missing key: ok=%v err=%v, want ok=false err=nil", ok, err)
	}

	if _, ok, err := r.ConfigMapValue(ctx, "ns", "absent", "k"); err != nil || ok {
		t.Errorf("missing configmap: ok=%v err=%v, want ok=false err=nil", ok, err)
	}
}
