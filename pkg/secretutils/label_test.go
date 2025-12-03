package secretutils

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestAddSecretSelector_NilInput(t *testing.T) {
	result := AddSecretSelector(nil)

	assert.NotNil(t, result)
	assert.Len(t, result, 1)
}

func TestAddSecretSelector_ExistingMap(t *testing.T) {
	existing := make(map[client.Object]cache.ByObject)
	result := AddSecretSelector(existing)

	assert.NotNil(t, result)
	assert.Len(t, result, 1)
	// Verify it returns the same map reference (modified in place)
	assert.Equal(t, &existing, &result)
}

func TestLabelConstants(t *testing.T) {
	assert.Equal(t, "environment.metal3.io", LabelEnvironmentName)
	assert.Equal(t, "baremetal", LabelEnvironmentValue)
}
