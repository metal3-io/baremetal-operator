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
// Secrets so that only those labelled as part of the baremetal environment get
// cached. The input may be nil.
func AddSecretSelector(selectors map[client.Object]cache.ByObject) map[client.Object]cache.ByObject {
	secret := &corev1.Secret{}
	newSelectors := map[client.Object]cache.ByObject{
		secret: {
			Label: labels.SelectorFromSet(
				labels.Set{
					LabelEnvironmentName: LabelEnvironmentValue,
				}),
		},
	}

	if selectors == nil {
		return newSelectors
	}

	selectors[secret] = newSelectors[secret]
	return selectors
}
