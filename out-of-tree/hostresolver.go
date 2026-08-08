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
	"cmp"
	"context"
	"errors"
	"fmt"
	"strings"

	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	"github.com/metal3-io/baremetal-operator/pkg/secretutils"
	"github.com/s3rj1k/starlark-provisioner/starlib"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// KubeHostResolver fetches BMH data on demand via client.Get + SecretManager.
type KubeHostResolver struct {
	Client client.Client
	// APIReader is the uncached reader used when a get bypasses the informer cache.
	APIReader     client.Reader
	SecretManager secretutils.SecretManager
}

// ReadHostSecret resolves a BMH spec secret field to its string content. Returns "" when unset.
func (r *KubeHostResolver) ReadHostSecret(ctx context.Context, namespace, name, field string) (string, error) {
	// The client is optional at construction and required only for host reads.
	if r.Client == nil {
		return "", errors.New("read_host_secret requires a Kubernetes client")
	}

	host := &metal3api.BareMetalHost{}
	if err := r.Client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, host); err != nil {
		return "", fmt.Errorf("get BareMetalHost: %w", err)
	}

	var ref *corev1.SecretReference
	var dataKey string

	switch strings.ToLower(field) {
	case "userdata":
		ref, dataKey = host.Spec.UserData, "userData"
	case "networkdata":
		ref, dataKey = host.Spec.NetworkData, "networkData"
	case "metadata":
		ref, dataKey = host.Spec.MetaData, "metaData"
	case "preprovisioningnetworkdata":
		if host.Spec.PreprovisioningNetworkDataName != "" {
			ref = &corev1.SecretReference{Name: host.Spec.PreprovisioningNetworkDataName}
		}
		dataKey = "networkData"
	default:
		return "", fmt.Errorf("unknown field %q", field)
	}

	if ref == nil {
		return "", nil
	}

	ns := cmp.Or(ref.Namespace, namespace)
	if ns != namespace {
		return "", fmt.Errorf("%s secret must be in BMH namespace %s", dataKey, namespace)
	}

	sec, err := r.SecretManager.ObtainSecret(ctx, types.NamespacedName{Name: ref.Name, Namespace: ns})
	if err != nil {
		return "", err
	}

	if v, ok := sec.Data[dataKey]; ok {
		return string(v), nil
	}
	if v, ok := sec.Data["value"]; ok {
		return string(v), nil
	}
	return "", nil
}

// ReadHostSpec returns BareMetalHost.Spec as a readonly map[string]any.
func (r *KubeHostResolver) ReadHostSpec(ctx context.Context, namespace, name string) (map[string]any, error) {
	// The client is optional at construction and required only for host reads.
	if r.Client == nil {
		return nil, errors.New("read_host_spec requires a Kubernetes client")
	}

	host := &metal3api.BareMetalHost{}
	if err := r.Client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, host); err != nil {
		return nil, fmt.Errorf("get BareMetalHost: %w", err)
	}

	// StructToMap uses UseNumber so whole numbers reach scripts as ints, matching HostArgs and k8s_get.
	m, err := starlib.StructToMap(host.Spec)
	if err != nil {
		return nil, fmt.Errorf("marshal spec: %w", err)
	}

	return m, nil
}

// ReadHostStatus returns BareMetalHost.Status as a readonly map[string]any.
func (r *KubeHostResolver) ReadHostStatus(ctx context.Context, namespace, name string) (map[string]any, error) {
	// The client is optional at construction and required only for host reads.
	if r.Client == nil {
		return nil, errors.New("read_host_status requires a Kubernetes client")
	}

	host := &metal3api.BareMetalHost{}
	if err := r.Client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, host); err != nil {
		return nil, fmt.Errorf("get BareMetalHost: %w", err)
	}

	// StructToMap uses UseNumber so whole numbers reach scripts as ints, matching HostArgs and k8s_get.
	m, err := starlib.StructToMap(host.Status)
	if err != nil {
		return nil, fmt.Errorf("marshal status: %w", err)
	}

	return m, nil
}

// kubeGVK builds a GroupVersionKind from an apiVersion and kind, blocking Secrets.
func kubeGVK(apiVersion, kind string) (schema.GroupVersionKind, error) {
	if apiVersion == "" {
		return schema.GroupVersionKind{}, errors.New("api_version is required")
	}

	if kind == "" {
		return schema.GroupVersionKind{}, errors.New("kind is required")
	}

	gv, err := schema.ParseGroupVersion(apiVersion)
	if err != nil {
		return schema.GroupVersionKind{}, fmt.Errorf("parse apiVersion %q: %w", apiVersion, err)
	}

	if gv.Version == "" {
		return schema.GroupVersionKind{}, fmt.Errorf("apiVersion %q must include a version", apiVersion)
	}

	return gv.WithKind(kind), nil
}

// GetObject reads one object in the namespace, returning nil when it is absent.
// A true uncached reads straight from the API, bypassing the informer cache.
func (r *KubeHostResolver) GetObject(ctx context.Context, namespace, apiVersion, kind, name string, uncached bool) (map[string]any, error) {
	var reader client.Reader = r.Client
	if uncached && r.APIReader != nil {
		reader = r.APIReader
	}

	if reader == nil {
		return nil, errors.New("k8s_get requires a Kubernetes client")
	}

	gvk, err := kubeGVK(apiVersion, kind)
	if err != nil {
		return nil, err
	}

	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(gvk)

	if err := reader.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, obj); err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, nil //nolint:nilnil // absent object is a valid None result
		}

		return nil, err
	}

	return obj.Object, nil
}

// ListObjects lists objects of a kind in the namespace, optionally by label selector.
func (r *KubeHostResolver) ListObjects(ctx context.Context, namespace, apiVersion, kind, labelSelector string) ([]map[string]any, error) {
	if r.Client == nil {
		return nil, errors.New("k8s_list requires a Kubernetes client")
	}

	gvk, err := kubeGVK(apiVersion, kind)
	if err != nil {
		return nil, err
	}

	list := &unstructured.UnstructuredList{}
	list.SetGroupVersionKind(gvk.GroupVersion().WithKind(gvk.Kind + "List"))

	opts := []client.ListOption{client.InNamespace(namespace)}
	if labelSelector != "" {
		sel, serr := labels.Parse(labelSelector)
		if serr != nil {
			return nil, fmt.Errorf("parse label selector: %w", serr)
		}

		opts = append(opts, client.MatchingLabelsSelector{Selector: sel})
	}

	if err := r.Client.List(ctx, list, opts...); err != nil {
		return nil, err
	}

	out := make([]map[string]any, len(list.Items))
	for i := range list.Items {
		out[i] = list.Items[i].Object
	}

	return out, nil
}

// ApplyObject server side applies an object, forcing it into the namespace.
func (r *KubeHostResolver) ApplyObject(ctx context.Context, namespace string, obj map[string]any, fieldManager string, force bool) (map[string]any, error) {
	if r.Client == nil {
		return nil, errors.New("k8s_apply requires a Kubernetes client")
	}

	u := &unstructured.Unstructured{Object: obj}
	if _, err := kubeGVK(u.GetAPIVersion(), u.GetKind()); err != nil {
		return nil, err
	}

	if u.GetName() == "" {
		return nil, errors.New("apply object requires metadata.name")
	}

	if ns := u.GetNamespace(); ns != "" && ns != namespace {
		return nil, fmt.Errorf("object namespace %q must match host namespace %q", ns, namespace)
	}

	u.SetNamespace(namespace)

	fieldManager = cmp.Or(fieldManager, starlib.DefaultFieldManager)

	opts := []client.PatchOption{client.FieldOwner(fieldManager)}
	if force {
		opts = append(opts, client.ForceOwnership)
	}

	if err := r.Client.Patch(ctx, u, client.Apply, opts...); err != nil {
		return nil, err
	}

	return u.Object, nil
}

// PatchObject applies a merge or json patch to an object by name in the namespace.
func (r *KubeHostResolver) PatchObject(ctx context.Context, namespace, apiVersion, kind, name, patchType string, data []byte, status bool) (map[string]any, error) {
	if r.Client == nil {
		return nil, errors.New("k8s_patch requires a Kubernetes client")
	}

	gvk, err := kubeGVK(apiVersion, kind)
	if err != nil {
		return nil, err
	}

	var pt types.PatchType
	switch patchType {
	case "", "merge":
		pt = types.MergePatchType
	case "json":
		pt = types.JSONPatchType
	default:
		return nil, fmt.Errorf("unknown patch_type %q", patchType)
	}

	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(gvk)
	u.SetNamespace(namespace)
	u.SetName(name)

	patch := client.RawPatch(pt, data)

	if status {
		err = r.Client.Status().Patch(ctx, u, patch)
	} else {
		err = r.Client.Patch(ctx, u, patch)
	}

	if err != nil {
		return nil, err
	}

	return u.Object, nil
}

// DeleteObject deletes an object by name in the namespace, treating absence as success.
func (r *KubeHostResolver) DeleteObject(ctx context.Context, namespace, apiVersion, kind, name string) error {
	if r.Client == nil {
		return errors.New("k8s_delete requires a Kubernetes client")
	}

	gvk, err := kubeGVK(apiVersion, kind)
	if err != nil {
		return err
	}

	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(gvk)
	u.SetNamespace(namespace)
	u.SetName(name)

	if err := r.Client.Delete(ctx, u); err != nil && !k8serrors.IsNotFound(err) {
		return err
	}

	return nil
}
