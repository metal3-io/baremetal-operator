package secretutils

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	LabelEnvironmentName  = "environment.metal3.io"
	LabelEnvironmentValue = "baremetal"
)

// AddSecretSelector adds a selector to a cache.SelectorsByObject that filters
// Secrets so that only those labeled as part of the baremetal environment get
// cached. When namespaces is non-empty, it limits the Secret cache to those
// namespaces.
func AddSecretSelector(selectors map[client.Object]cache.ByObject, namespaces map[string]cache.Config) map[client.Object]cache.ByObject {
	secret := &corev1.Secret{}
	secretConfig := cache.ByObject{
		Label: labels.SelectorFromSet(
			labels.Set{
				LabelEnvironmentName: LabelEnvironmentValue,
			}),
	}
	if len(namespaces) > 0 {
		secretConfig.Namespaces = namespaces
	}

	newSelectors := map[client.Object]cache.ByObject{
		secret: secretConfig,
	}

	if selectors == nil {
		return newSelectors
	}

	selectors[secret] = newSelectors[secret]
	return selectors
}
