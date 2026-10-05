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

package firmwaresettings

import (
	"context"
	"errors"
	"fmt"

	"github.com/go-logr/logr"
	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	"github.com/metal3-io/baremetal-operator/pkg/logging"
	"github.com/metal3-io/baremetal-operator/pkg/mgrutils"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// Manager manages HostFirmwareSettings sub-resources.
type Manager struct {
	client client.Client
	scheme *runtime.Scheme
	log    logr.Logger
}

// ManagerInterface defines operations on a host's firmware settings sub-resource.
type ManagerInterface interface {
	// EnsureSettings creates HostFirmwareSettings with an owner reference if it does not
	// already exist, and adds the owner reference if the resource was created manually.
	EnsureSettings(ctx context.Context, host *metal3api.BareMetalHost) error

	// GetSettingsChanges returns dirty=true when HostFirmwareSettings has valid pending
	// changes. The hfs object is returned regardless of validity so callers can inspect
	// spec contents.
	GetSettingsChanges(ctx context.Context, host *metal3api.BareMetalHost) (dirty bool, hfs *metal3api.HostFirmwareSettings, err error)
}

// NewManager returns a new Manager for firmware settings operations.
// client and scheme must not be nil.
func NewManager(c client.Client, scheme *runtime.Scheme, log logr.Logger) ManagerInterface {
	if c == nil {
		panic("firmwaresettings.NewManager: client must not be nil")
	}
	if scheme == nil {
		panic("firmwaresettings.NewManager: scheme must not be nil")
	}
	return &Manager{client: c, scheme: scheme, log: log}
}

func (m *Manager) EnsureSettings(ctx context.Context, host *metal3api.BareMetalHost) error {
	hostKey := client.ObjectKey{Name: host.Name, Namespace: host.Namespace}
	hfs := &metal3api.HostFirmwareSettings{}
	if err := m.client.Get(ctx, hostKey, hfs); err != nil {
		if !k8serrors.IsNotFound(err) {
			return fmt.Errorf("could not load hostFirmwareSettings resource: %w", err)
		}
		hfs.ObjectMeta = metav1.ObjectMeta{Name: host.Name, Namespace: host.Namespace}
		hfs.Status.Settings = make(metal3api.SettingsMap)
		hfs.Spec.Settings = make(metal3api.DesiredSettingsMap)
		if err = controllerutil.SetOwnerReference(host, hfs, m.scheme); err != nil {
			return fmt.Errorf("could not set bmh as owner for hostFirmwareSettings: %w", err)
		}
		if err = m.client.Create(ctx, hfs); err != nil {
			return fmt.Errorf("failure creating hostFirmwareSettings resource: %w", err)
		}
		m.log.V(logging.VerbosityLevelDebug).Info("created new hostFirmwareSettings resource")
		return nil
	}
	if !mgrutils.OwnerReferenceExists(host, hfs) {
		if err := controllerutil.SetOwnerReference(host, hfs, m.scheme); err != nil {
			return fmt.Errorf("could not set bmh as owner for hostFirmwareSettings: %w", err)
		}
		if err := m.client.Update(ctx, hfs); err != nil {
			return fmt.Errorf("failure updating hostFirmwareSettings resource: %w", err)
		}
	}
	return nil
}

func (m *Manager) GetSettingsChanges(ctx context.Context, host *metal3api.BareMetalHost) (dirty bool, hfs *metal3api.HostFirmwareSettings, err error) {
	hostKey := client.ObjectKey{Name: host.Name, Namespace: host.Namespace}
	hfs = &metal3api.HostFirmwareSettings{}
	if err = m.client.Get(ctx, hostKey, hfs); err != nil {
		if !k8serrors.IsNotFound(err) {
			return false, nil, fmt.Errorf("could not load host firmware settings: %w", err)
		}
		m.log.V(logging.VerbosityLevelDebug).Info("could not get hostFirmwareSettings", "name", host.Name, "namespace", host.Namespace)
		return false, nil, nil
	}

	changed, valid, err := mgrutils.ObjectHasChanges(hfs.Status.Conditions,
		string(metal3api.FirmwareSettingsChangeDetected),
		string(metal3api.FirmwareSettingsValid),
		hfs.GetGeneration())
	if err != nil {
		return false, nil, fmt.Errorf("hostFirmwareSettings not ready yet: %w", err)
	}
	if !valid {
		m.log.Info("hostFirmwareSettings not valid", "name", host.Name, "namespace", host.Namespace)
		return false, hfs, nil
	}
	if changed {
		if len(hfs.Status.Settings) == 0 {
			return false, nil, errors.New("host firmware status settings not available")
		}
		m.log.Info("hostFirmwareSettings indicating ChangeDetected", "name", host.Name, "namespace", host.Namespace)
		return true, hfs, nil
	}
	m.log.V(logging.VerbosityLevelTrace).Info("hostFirmwareSettings no updates", "name", host.Name, "namespace", host.Namespace)
	return false, hfs, nil
}
