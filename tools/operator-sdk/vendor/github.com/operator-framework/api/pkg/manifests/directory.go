package manifests

import (
	"fmt"

	internal "github.com/operator-framework/api/pkg/internal"
	"github.com/operator-framework/api/pkg/validation"
	"github.com/operator-framework/api/pkg/validation/errors"

	"github.com/operator-framework/operator-registry/pkg/registry"
)

// GetManifestsDir parses all bundles and a package manifest from dir, which
// are returned if found along with any errors or warnings encountered while
// parsing/validating found manifests.
func GetManifestsDir(dir string) (registry.PackageManifest, []*registry.Bundle, []errors.ManifestResult) {
	manifests, err := internal.ManifestsStoreForDir(dir)
	if err != nil {
		result := errors.ManifestResult{}
		result.Add(errors.ErrInvalidParse(fmt.Sprintf("parse manifests from %q", dir), err))
		return registry.PackageManifest{}, nil, []errors.ManifestResult{result}
	}
	pkg := manifests.GetPackageManifest()
	bundles := manifests.GetBundles()
	objs := []interface{}{}
	for _, obj := range bundles {
		objs = append(objs, obj)
	}
	results := validation.AllValidators.Validate(objs...)
	return pkg, bundles, results
}
