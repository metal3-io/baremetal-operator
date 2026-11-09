package secretutils

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestAddSecretSelector_NilInput(t *testing.T) {
	result := AddSecretSelector(nil, nil)
	assert.Len(t, result, 1)
}

func TestAddSecretSelector_ExistingMap(t *testing.T) {
	existing := make(map[client.Object]cache.ByObject)
	result := AddSecretSelector(existing, nil)

	assert.Len(t, result, 1)
	// Verify it returns the same map reference (modified in place)
	assert.Equal(t, &existing, &result)
}

func TestAddSecretSelector_Namespaces(t *testing.T) {
	namespaces := map[string]cache.Config{
		"watched":  {},
		"operator": {},
	}

	result := AddSecretSelector(nil, namespaces)
	assert.Len(t, result, 1)
	for _, config := range result {
		assert.Equal(t, namespaces, config.Namespaces)
	}
}
