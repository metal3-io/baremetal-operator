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
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"strings"

	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	"github.com/metal3-io/baremetal-operator/pkg/secretutils"
	"github.com/s3rj1k/starlark-provisioner/starlib"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// KubeHostResolver fetches BMH data on demand via client.Get + SecretManager.
type KubeHostResolver struct {
	Client client.Client
	// APIReader is the uncached reader used when a get bypasses the informer cache.
	APIReader     client.Reader
	SecretManager secretutils.SecretManager
	// PodNamespace is the namespace the operator runs in, supplied by the host as
	// PluginConfig.ProvisionerNamespace. Empty falls back to the POD_NAMESPACE env.
	PodNamespace string
}

// Namespace returns the namespace the plugin's own objects live in. Empty would
// turn every scoped list cluster wide, so the host supplied value wins.
func (r *KubeHostResolver) Namespace() string {
	return cmp.Or(r.PodNamespace, PodNamespace())
}

// getHost fetches the BareMetalHost behind a script call. The caller name goes
// into the error when no Kubernetes client is configured.
func (r *KubeHostResolver) getHost(ctx context.Context, caller, namespace, name string) (*metal3api.BareMetalHost, error) {
	if r.Client == nil {
		return nil, fmt.Errorf("%s requires a Kubernetes client", caller)
	}

	host := &metal3api.BareMetalHost{}
	if err := r.Client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, host); err != nil {
		return nil, fmt.Errorf("get BareMetalHost: %w", err)
	}

	return host, nil
}

// ReadHostSecret resolves a BMH spec secret field to its string content. Returns "" when unset.
func (r *KubeHostResolver) ReadHostSecret(ctx context.Context, namespace, name, field string) (string, error) {
	host, err := r.getHost(ctx, "read_host_secret", namespace, name)
	if err != nil {
		return "", err
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

	// A referenced Secret carrying neither key is a misconfiguration, and BMO's own
	// reader errors here too. Returning "" would silently deploy a host with no data.
	return "", fmt.Errorf("secret %s/%s has no %q or \"value\" key", ns, ref.Name, dataKey)
}

// ReadHostSpec returns BareMetalHost.Spec as a readonly map[string]any.
func (r *KubeHostResolver) ReadHostSpec(ctx context.Context, namespace, name string) (map[string]any, error) {
	host, err := r.getHost(ctx, "read_host_spec", namespace, name)
	if err != nil {
		return nil, err
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
	host, err := r.getHost(ctx, "read_host_status", namespace, name)
	if err != nil {
		return nil, err
	}

	// StructToMap uses UseNumber so whole numbers reach scripts as ints, matching HostArgs and k8s_get.
	m, err := starlib.StructToMap(host.Status)
	if err != nil {
		return nil, fmt.Errorf("marshal status: %w", err)
	}

	return m, nil
}

// KubeGVK builds a GroupVersionKind from an apiVersion and kind, blocking core
// Secrets so a script cannot read the BMC password around the output masking.
func KubeGVK(apiVersion, kind string) (schema.GroupVersionKind, error) {
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

	gvk := gv.WithKind(kind)
	if IsSecretGVK(gvk) {
		return schema.GroupVersionKind{}, errors.New("access to Secrets is not allowed, use read_host_secret")
	}

	return gvk, nil
}

// IsSecretGVK reports whether a GroupVersionKind addresses core Secrets, in
// either the singular or the list form a caller may pass as a kind.
func IsSecretGVK(gvk schema.GroupVersionKind) bool {
	if gvk.Group != "" {
		return false
	}

	return gvk.Kind == "Secret" || gvk.Kind == "SecretList"
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

	gvk, err := KubeGVK(apiVersion, kind)
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

	gvk, err := KubeGVK(apiVersion, kind)
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
	if _, err := KubeGVK(u.GetAPIVersion(), u.GetKind()); err != nil {
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

	gvk, err := KubeGVK(apiVersion, kind)
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

	gvk, err := KubeGVK(apiVersion, kind)
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

// HostRef identifies a BareMetalHost resolved from a request. Everything lives in
// the operator's namespace, so the name and UID are the whole identity.
type HostRef struct {
	Name string
	UID  string
}

// hostRef builds a HostRef from a host.
func hostRef(host *metal3api.BareMetalHost) HostRef {
	return HostRef{Name: host.Name, UID: string(host.UID)}
}

// FindHostsByMAC returns the hosts owning any of the given MACs, boot MAC matches
// first then inspected NICs. More than one result means the addresses collide.
func (r *KubeHostResolver) FindHostsByMAC(ctx context.Context, macs []string) ([]HostRef, error) {
	if r.Client == nil {
		return nil, errors.New("kickstart lookup requires a Kubernetes client")
	}

	want := make(map[string]bool, len(macs))

	for _, m := range macs {
		if n := NormalizeMAC(m); n != "" {
			want[n] = true
		}
	}

	if len(want) == 0 {
		return nil, nil
	}

	list := &metal3api.BareMetalHostList{}
	if err := r.Client.List(ctx, list, client.InNamespace(r.Namespace())); err != nil {
		return nil, err
	}

	var boot, nics []HostRef

	for i := range list.Items {
		host := &list.Items[i]

		if want[NormalizeMAC(host.Spec.BootMACAddress)] {
			boot = append(boot, hostRef(host))

			continue
		}

		if host.Status.HardwareDetails == nil {
			continue
		}

		for _, nic := range host.Status.HardwareDetails.NIC {
			if want[NormalizeMAC(nic.MAC)] {
				nics = append(nics, hostRef(host))

				break
			}
		}
	}

	return append(boot, nics...), nil
}

// BMCCredentials returns the BMH's BMC username and password, trimmed to match HostData.
func (r *KubeHostResolver) BMCCredentials(ctx context.Context, namespace, name string) (username, password string, err error) {
	host, err := r.getHost(ctx, "callback", namespace, name)
	if err != nil {
		return "", "", err
	}

	if host.Spec.BMC.CredentialsName == "" {
		return "", "", errors.New("host has no BMC credentials")
	}

	sec, err := r.SecretManager.ObtainSecret(ctx, types.NamespacedName{Namespace: namespace, Name: host.Spec.BMC.CredentialsName})
	if err != nil {
		return "", "", err
	}

	// Trim to match how the controller decodes BMC credentials into HostData.
	user := strings.TrimSpace(string(sec.Data["username"]))
	pass := strings.TrimSpace(string(sec.Data["password"]))

	// An empty password would key the token HMAC over public data, so reject it.
	if pass == "" {
		return "", "", errors.New("host BMC password is empty")
	}

	return user, pass, nil
}

// CallbackReader returns the uncached reader when available so callback Secrets
// stay visible despite the label filtered informer cache.
func (r *KubeHostResolver) CallbackReader() client.Reader {
	if r.APIReader != nil {
		return r.APIReader
	}

	return r.Client
}

// Secret data keys, namespaced per Starlark use case so they share one host Secret.
const (
	callbackDataKey       = "callback.data"
	callbackReceivedAtKey = "callback.receivedAt"
	serveRegistrationsKey = "serve.registrations"
)

// ServeUIDLabel lets the serve handler find a host's Secret by BMH UID.
const ServeUIDLabel = "starlark.metal3.io/serve-uid"

// hostSecretMaxAttempts bounds the read modify write retry on the shared Secret.
const hostSecretMaxAttempts = 5

// decodeServeRegistrations parses the stored JSON map, keeping numbers as literals.
func decodeServeRegistrations(raw []byte) (map[string]starlib.ServeRegistration, error) {
	regs := map[string]starlib.ServeRegistration{}
	if len(raw) == 0 {
		return regs, nil
	}

	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()

	if err := dec.Decode(&regs); err != nil {
		return nil, fmt.Errorf("decode serve registrations: %w", err)
	}

	return regs, nil
}

// mutateHostSecret applies mutate to the single per host Starlark Secret, retrying on
// write conflicts since callback and serve share it. Empty result deletes the Secret.
func (r *KubeHostResolver) mutateHostSecret(ctx context.Context, namespace, name string, mutate func(data map[string][]byte) error) error {
	if r.Client == nil {
		return errors.New("host secret write requires a Kubernetes client")
	}

	secretName := starlib.HostSecretName(name)
	key := types.NamespacedName{Namespace: namespace, Name: secretName}

	for range hostSecretMaxAttempts {
		// Read uncached, the Secret may not be in the label filtered informer cache yet.
		sec := &corev1.Secret{}
		getErr := r.CallbackReader().Get(ctx, key, sec)

		create := k8serrors.IsNotFound(getErr)
		if getErr != nil && !create {
			return getErr
		}

		data := make(map[string][]byte, len(sec.Data))
		maps.Copy(data, sec.Data)

		if err := mutate(data); err != nil {
			return err
		}

		// Nothing changed, so skip the write. Callers upsert the same registration
		// on every reconcile and an unconditional Update would churn the API server.
		if !create && maps.EqualFunc(data, sec.Data, bytes.Equal) {
			return nil
		}

		if len(data) == 0 {
			if create {
				return nil
			}

			// Precondition on the read version so a concurrent writer that added a
			// key between the read and here is not silently wiped.
			delOpts := client.Preconditions{UID: &sec.UID, ResourceVersion: &sec.ResourceVersion}
			if err := r.Client.Delete(ctx, sec, delOpts); err != nil {
				if k8serrors.IsNotFound(err) {
					return nil
				}

				if k8serrors.IsConflict(err) {
					continue
				}

				return err
			}

			return nil
		}

		if create {
			host := &metal3api.BareMetalHost{}
			if err := r.CallbackReader().Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, host); err != nil {
				return fmt.Errorf("get BareMetalHost: %w", err)
			}

			sec.Namespace = namespace
			sec.Name = secretName
			sec.Type = corev1.SecretTypeOpaque
			// Cache label plus the UID label the serve handler looks the Secret up by.
			metav1.SetMetaDataLabel(&sec.ObjectMeta, secretutils.LabelEnvironmentName, secretutils.LabelEnvironmentValue)
			metav1.SetMetaDataLabel(&sec.ObjectMeta, ServeUIDLabel, string(host.UID))

			// Own the Secret so it is garbage collected when the host is deleted.
			if err := controllerutil.SetOwnerReference(host, sec, r.Client.Scheme()); err != nil {
				return err
			}
		}

		sec.Data = data

		var werr error
		if create {
			werr = r.Client.Create(ctx, sec)
		} else {
			werr = r.Client.Update(ctx, sec)
		}

		if werr == nil {
			return nil
		}

		// Another writer changed the shared Secret, reread and retry.
		if k8serrors.IsConflict(werr) || k8serrors.IsAlreadyExists(werr) {
			continue
		}

		return werr
	}

	return fmt.Errorf("host secret %s update conflict retries exhausted", secretName)
}

// WriteCallback stores a received callback body under the callback keys of the host Secret.
func (r *KubeHostResolver) WriteCallback(ctx context.Context, namespace, name string, body []byte, receivedAt string) error {
	return r.mutateHostSecret(ctx, namespace, name, func(data map[string][]byte) error {
		data[callbackDataKey] = body
		data[callbackReceivedAtKey] = []byte(receivedAt)

		return nil
	})
}

// ReadCallback returns the stored callback data for a host, or nil when none exists.
func (r *KubeHostResolver) ReadCallback(ctx context.Context, namespace, name string) (map[string]any, error) {
	if r.Client == nil {
		return nil, errors.New("callback read requires a Kubernetes client")
	}

	sec := &corev1.Secret{}
	key := types.NamespacedName{Namespace: namespace, Name: starlib.HostSecretName(name)}

	if err := r.CallbackReader().Get(ctx, key, sec); err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, nil //nolint:nilnil // no callback yet is a valid None result
		}

		return nil, err
	}

	raw, ok := sec.Data[callbackDataKey]
	if !ok {
		return nil, nil //nolint:nilnil // Secret exists for another use case, no callback yet
	}

	return map[string]any{
		"data":       string(raw),
		"receivedAt": string(sec.Data[callbackReceivedAtKey]),
	}, nil
}

// DeleteCallback clears the callback keys, deleting the Secret if nothing else uses it.
func (r *KubeHostResolver) DeleteCallback(ctx context.Context, namespace, name string) error {
	return r.mutateHostSecret(ctx, namespace, name, func(data map[string][]byte) error {
		delete(data, callbackDataKey)
		delete(data, callbackReceivedAtKey)

		return nil
	})
}

// ConfigMapValue returns a ConfigMap key value, with found false when the
// ConfigMap or the key is absent.
func (r *KubeHostResolver) ConfigMapValue(ctx context.Context, namespace, name, key string) (string, bool, error) {
	if r.Client == nil {
		return "", false, errors.New("serve requires a Kubernetes client")
	}

	// Read uncached. A cached get would start a cluster wide ConfigMap informer,
	// which needs list and watch cluster wide, and a plain get in one namespace does not.
	cm := &corev1.ConfigMap{}
	if err := r.CallbackReader().Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, cm); err != nil {
		if k8serrors.IsNotFound(err) {
			return "", false, nil
		}

		return "", false, err
	}

	if v, ok := cm.Data[key]; ok {
		return v, true, nil
	}

	if v, ok := cm.BinaryData[key]; ok {
		return string(v), true, nil
	}

	return "", false, nil
}

// WriteServe upserts one route into the serve key of the host Secret. The read side
// lists only the operator namespace, so a host outside it is refused here.
func (r *KubeHostResolver) WriteServe(ctx context.Context, namespace, name, id string, reg starlib.ServeRegistration) error {
	if ns := r.Namespace(); ns != "" && namespace != ns {
		return fmt.Errorf("serve routes are only supported for hosts in %s, %s/%s is elsewhere", ns, namespace, name)
	}

	return r.mutateHostSecret(ctx, namespace, name, func(data map[string][]byte) error {
		regs, err := decodeServeRegistrations(data[serveRegistrationsKey])
		if err != nil {
			return err
		}

		regs[id] = reg

		encoded, err := json.Marshal(regs)
		if err != nil {
			return fmt.Errorf("encode serve registrations: %w", err)
		}

		data[serveRegistrationsKey] = encoded

		return nil
	})
}

// DeleteServe removes one route, dropping the serve key once its last route is gone.
func (r *KubeHostResolver) DeleteServe(ctx context.Context, namespace, name, id string) error {
	return r.mutateHostSecret(ctx, namespace, name, func(data map[string][]byte) error {
		regs, err := decodeServeRegistrations(data[serveRegistrationsKey])
		if err != nil {
			return err
		}

		delete(regs, id)

		if len(regs) == 0 {
			delete(data, serveRegistrationsKey)

			return nil
		}

		encoded, err := json.Marshal(regs)
		if err != nil {
			return fmt.Errorf("encode serve registrations: %w", err)
		}

		data[serveRegistrationsKey] = encoded

		return nil
	})
}

// DeleteServeHost drops all serve routes for a host on teardown, keeping other keys.
func (r *KubeHostResolver) DeleteServeHost(ctx context.Context, namespace, name string) error {
	return r.mutateHostSecret(ctx, namespace, name, func(data map[string][]byte) error {
		delete(data, serveRegistrationsKey)

		return nil
	})
}

// ReadServe returns a route registration found by BMH UID.
func (r *KubeHostResolver) ReadServe(ctx context.Context, uid, id string) (starlib.ServeRegistration, bool, error) {
	if r.Client == nil {
		return starlib.ServeRegistration{}, false, errors.New("serve read requires a Kubernetes client")
	}

	list := &corev1.SecretList{}

	opts := []client.ListOption{client.InNamespace(r.Namespace()), client.MatchingLabels{ServeUIDLabel: uid}}
	if err := r.Client.List(ctx, list, opts...); err != nil {
		return starlib.ServeRegistration{}, false, err
	}

	for i := range list.Items {
		sec := &list.Items[i]

		regs, err := decodeServeRegistrations(sec.Data[serveRegistrationsKey])
		if err != nil {
			return starlib.ServeRegistration{}, false, err
		}

		if reg, ok := regs[id]; ok {
			return reg, true, nil
		}
	}

	return starlib.ServeRegistration{}, false, nil
}
