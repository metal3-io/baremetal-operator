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
	"encoding/json"
	"fmt"
	"strings"

	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	"github.com/metal3-io/baremetal-operator/pkg/secretutils"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// KubeHostResolver fetches BMH data on demand via client.Get + SecretManager.
type KubeHostResolver struct {
	Client        client.Client
	SecretManager secretutils.SecretManager
}

// ReadHostSecret resolves a BMH spec secret field to its string content; "" when unset.
func (r *KubeHostResolver) ReadHostSecret(ctx context.Context, namespace, name, field string) (string, error) {
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

	ns := ref.Namespace
	if ns == "" {
		ns = namespace
	}
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

// ReadHostSpec returns BareMetalHost.Spec as a read-only map[string]any.
func (r *KubeHostResolver) ReadHostSpec(ctx context.Context, namespace, name string) (map[string]any, error) {
	host := &metal3api.BareMetalHost{}
	if err := r.Client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, host); err != nil {
		return nil, fmt.Errorf("get BareMetalHost: %w", err)
	}

	data, err := json.Marshal(host.Spec)
	if err != nil {
		return nil, fmt.Errorf("marshal spec: %w", err)
	}

	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("unmarshal spec: %w", err)
	}

	return m, nil
}
