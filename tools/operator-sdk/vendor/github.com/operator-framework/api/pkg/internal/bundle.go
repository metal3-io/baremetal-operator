package manifests

import (
	"encoding/json"

	"github.com/blang/semver"
	operatorsv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	"github.com/operator-framework/operator-registry/pkg/registry"
	"github.com/operator-framework/operator-registry/pkg/sqlite"
	"github.com/pkg/errors"
)

// TODO: use internal version of registry.Bundle/registry.PackageManifest so
// operator-registry can use validation library.

// manifestsLoad loads a manifests directory from disk.
type manifestsLoad struct {
	dir     string
	pkg     registry.PackageManifest
	bundles map[string]*registry.Bundle
}

// Ensure manifestsLoad implements registry.Load.
var _ registry.Load = &manifestsLoad{}

// populate uses operator-registry's sqlite.NewSQLLoaderForDirectory to load
// l.dir's manifests. Note that this method does not call any functions that
// use SQL drivers.
func (l *manifestsLoad) populate() error {
	loader := sqlite.NewSQLLoaderForDirectory(l, l.dir)
	if err := loader.Populate(); err != nil {
		return errors.Wrapf(err, "error getting bundles from manifests dir %q", l.dir)
	}
	return nil
}

// AddOperatorBundle adds a bundle to l.
func (l *manifestsLoad) AddOperatorBundle(bundle *registry.Bundle) error {
	csvRaw, err := bundle.ClusterServiceVersion()
	if err != nil {
		return errors.Wrap(err, "error getting bundle CSV")
	}
	csvSpec := operatorsv1alpha1.ClusterServiceVersionSpec{}
	if err := json.Unmarshal(csvRaw.Spec, &csvSpec); err != nil {
		return errors.Wrap(err, "error unmarshaling CSV spec")
	}
	bundle.Name = csvSpec.Version.String()
	l.bundles[csvSpec.Version.String()] = bundle
	return nil
}

// AddOperatorBundle adds the package manifest to l.
func (l *manifestsLoad) AddPackageChannels(pkg registry.PackageManifest) error {
	l.pkg = pkg
	return nil
}

// AddBundlePackageChannels is a no-op to implement the registry.Load interface.
func (l *manifestsLoad) AddBundlePackageChannels(manifest registry.PackageManifest, bundle registry.Bundle) error {
	return nil
}

// RmPackageName is a no-op to implement the registry.Load interface.
func (l *manifestsLoad) RmPackageName(packageName string) error {
	return nil
}

// ClearNonDefaultBundles is a no-op to implement the registry.Load interface.
func (l *manifestsLoad) ClearNonDefaultBundles(packageName string) error {
	return nil
}

// ManifestsStore knows how to query for an operator's package manifest and
// related bundles.
type ManifestsStore interface {
	// GetPackageManifest returns the ManifestsStore's registry.PackageManifest.
	// The returned object is assumed to be valid.
	GetPackageManifest() registry.PackageManifest
	// GetBundles returns the ManifestsStore's set of registry.Bundle. These
	// bundles are unique by CSV version, since only one operator type should
	// exist in one manifests dir.
	// The returned objects are assumed to be valid.
	GetBundles() []*registry.Bundle
	// GetBundleForVersion returns the ManifestsStore's registry.Bundle for a
	// given version string. An error should be returned if the passed version
	// does not exist in the store.
	// The returned object is assumed to be valid.
	GetBundleForVersion(string) (*registry.Bundle, error)
}

// manifests implements ManifestsStore
type manifests struct {
	pkg     registry.PackageManifest
	bundles map[string]*registry.Bundle
}

// ManifestsStoreForDir populates a ManifestsStore from the metadata in dir.
// Each bundle and the package manifest are statically validated, and will
// return an error if any are not valid.
func ManifestsStoreForDir(dir string) (ManifestsStore, error) {
	load := &manifestsLoad{
		dir:     dir,
		bundles: map[string]*registry.Bundle{},
	}
	if err := load.populate(); err != nil {
		return nil, err
	}
	return &manifests{
		pkg:     load.pkg,
		bundles: load.bundles,
	}, nil
}

func (l manifests) GetPackageManifest() registry.PackageManifest {
	return l.pkg
}

func (l manifests) GetBundles() (bundles []*registry.Bundle) {
	for _, bundle := range l.bundles {
		bundles = append(bundles, bundle)
	}
	return bundles
}

func (l manifests) GetBundleForVersion(version string) (*registry.Bundle, error) {
	if _, err := semver.Parse(version); err != nil {
		return nil, errors.Wrapf(err, "error getting bundle for version %q", version)
	}
	bundle, ok := l.bundles[version]
	if !ok {
		return nil, errors.Errorf("bundle for version %q does not exist", version)
	}
	return bundle, nil
}
