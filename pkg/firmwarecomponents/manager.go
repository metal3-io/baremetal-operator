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

package firmwarecomponents

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	"github.com/go-logr/logr"
	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	"github.com/metal3-io/baremetal-operator/pkg/logging"
	"github.com/metal3-io/baremetal-operator/pkg/mgrutils"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// Manager manages HostFirmwareComponents sub-resources.
type Manager struct {
	client      client.Client
	scheme      *runtime.Scheme
	provisioner provisioner.Provisioner
	log         logr.Logger
}

// ManagerInterface defines operations on a host's firmware components sub-resource.
type ManagerInterface interface {
	// EnsureComponents creates HostFirmwareComponents with an owner reference if it does
	// not already exist, and adds the owner reference if the resource was created manually.
	EnsureComponents(ctx context.Context, host *metal3api.BareMetalHost) error

	// GetComponentsChanges returns dirty=true when HostFirmwareComponents has valid pending
	// changes. The hfc object is returned regardless of validity so callers can inspect
	// spec contents.
	GetComponentsChanges(ctx context.Context, host *metal3api.BareMetalHost) (dirty bool, hfc *metal3api.HostFirmwareComponents, err error)

	// ApplyError clears pending updates from HostFirmwareComponents status after a
	// Prepare or Service provisioner error. No-op if dirty is false, hfc is nil, or
	// there are no pending updates.
	ApplyError(ctx context.Context, hfc *metal3api.HostFirmwareComponents, dirty bool) error

	// ApplyResult syncs HostFirmwareComponents status after a successful Prepare or
	// Service provisioner call. No-op if dirty or started is false, or if status already
	// matches spec. Copies spec updates to status and refreshes component versions from
	// the provisioner.
	ApplyResult(ctx context.Context, hfc *metal3api.HostFirmwareComponents, dirty, started bool) error
}

// NewManager returns a new Manager for firmware components operations.
// client and scheme must not be nil. Provisioner must not be nil when
// ApplyResult will be called; it may be nil in tests that exercise other methods.
func NewManager(c client.Client, scheme *runtime.Scheme, prov provisioner.Provisioner, log logr.Logger) ManagerInterface {
	if c == nil {
		panic("firmwarecomponents.NewManager: client must not be nil")
	}
	if scheme == nil {
		panic("firmwarecomponents.NewManager: scheme must not be nil")
	}
	return &Manager{client: c, scheme: scheme, provisioner: prov, log: log}
}

// GetUpdatesDifference returns the firmware updates in spec that are not yet reflected in status.
func GetUpdatesDifference(specUpdates, statusUpdates []metal3api.FirmwareUpdate) []metal3api.FirmwareUpdate {
	diff := []metal3api.FirmwareUpdate{}
	applied := make(map[string]string, len(statusUpdates))
	for _, s := range statusUpdates {
		applied[s.Component] = s.URL
	}
	for _, fw := range specUpdates {
		if url, ok := applied[fw.Component]; !ok || fw.URL != url {
			diff = append(diff, fw)
		}
	}
	return diff
}

func (m *Manager) EnsureComponents(ctx context.Context, host *metal3api.BareMetalHost) error {
	hostKey := client.ObjectKey{Name: host.Name, Namespace: host.Namespace}
	hfc := &metal3api.HostFirmwareComponents{}
	if err := m.client.Get(ctx, hostKey, hfc); err != nil {
		if !k8serrors.IsNotFound(err) {
			return fmt.Errorf("could not load hostFirmwareComponents resource: %w", err)
		}
		hfc.ObjectMeta = metav1.ObjectMeta{Name: host.Name, Namespace: host.Namespace}
		hfc.Spec = metal3api.HostFirmwareComponentsSpec{Updates: []metal3api.FirmwareUpdate{}}
		if err = controllerutil.SetOwnerReference(host, hfc, m.scheme); err != nil {
			return fmt.Errorf("could not set bmh as owner for hostFirmwareComponents: %w", err)
		}
		if err = m.client.Create(ctx, hfc); err != nil {
			return fmt.Errorf("failure creating hostFirmwareComponents resource: %w", err)
		}
		m.log.V(logging.VerbosityLevelDebug).Info("created new hostFirmwareComponents resource")
		return nil
	}
	if !mgrutils.OwnerReferenceExists(host, hfc) {
		if err := controllerutil.SetOwnerReference(host, hfc, m.scheme); err != nil {
			return fmt.Errorf("could not set bmh as owner for hostFirmwareComponents: %w", err)
		}
		if err := m.client.Update(ctx, hfc); err != nil {
			return fmt.Errorf("failure updating hostFirmwareComponents resource: %w", err)
		}
	}
	return nil
}

func (m *Manager) GetComponentsChanges(ctx context.Context, host *metal3api.BareMetalHost) (dirty bool, hfc *metal3api.HostFirmwareComponents, err error) {
	hostKey := client.ObjectKey{Name: host.Name, Namespace: host.Namespace}
	hfc = &metal3api.HostFirmwareComponents{}
	if err = m.client.Get(ctx, hostKey, hfc); err != nil {
		if !k8serrors.IsNotFound(err) {
			return false, nil, fmt.Errorf("could not load host firmware components: %w", err)
		}
		m.log.V(logging.VerbosityLevelDebug).Info("could not get hostFirmwareComponents", "name", host.Name, "namespace", host.Namespace)
		return false, nil, nil
	}

	changed, valid, err := mgrutils.ObjectHasChanges(hfc.Status.Conditions,
		string(metal3api.HostFirmwareComponentsChangeDetected),
		string(metal3api.HostFirmwareComponentsValid),
		hfc.GetGeneration())
	if err != nil {
		return false, nil, fmt.Errorf("hostFirmwareComponents not ready yet: %w", err)
	}
	if !valid {
		m.log.Info("hostFirmwareComponents not valid", "name", host.Name, "namespace", host.Namespace)
		return false, hfc, nil
	}
	if changed {
		m.log.Info("hostFirmwareComponents indicating ChangeDetected", "name", host.Name, "namespace", host.Namespace)
		return true, hfc, nil
	}
	m.log.V(logging.VerbosityLevelTrace).Info("hostFirmwareComponents no updates", "name", host.Name, "namespace", host.Namespace)
	return false, hfc, nil
}

func (m *Manager) ApplyError(ctx context.Context, hfc *metal3api.HostFirmwareComponents, dirty bool) error {
	if !dirty || hfc == nil || hfc.Status.Updates == nil {
		return nil
	}
	hfc.Status.Updates = nil
	return m.client.Status().Update(ctx, hfc)
}

func (m *Manager) ApplyResult(ctx context.Context, hfc *metal3api.HostFirmwareComponents, dirty, started bool) error {
	if !dirty || !started {
		return nil
	}
	if reflect.DeepEqual(hfc.Status.Updates, hfc.Spec.Updates) {
		m.log.V(logging.VerbosityLevelDebug).Info("not saving hostFirmwareComponents information since it is not necessary")
		return nil
	}
	if m.provisioner == nil {
		return errors.New("provisioner required for ApplyResult")
	}
	m.log.V(logging.VerbosityLevelDebug).Info("saving hostFirmwareComponents information",
		"specUpdates", hfc.Spec.Updates,
		"statusUpdates", hfc.Status.Updates)
	hfc.Status.Updates = append([]metal3api.FirmwareUpdate(nil), hfc.Spec.Updates...)
	components, err := m.provisioner.GetFirmwareComponents(ctx)
	if err != nil {
		m.log.Error(err, "failed to get new information for firmware components in ironic")
		return fmt.Errorf("failed to get firmware components: %w", err)
	}
	if !reflect.DeepEqual(hfc.Status.Components, components) {
		for _, fwc := range components {
			m.log.Info("firmware component added for host", "component", fwc.Component)
		}
		hfc.Status.Components = components
	}
	return m.client.Status().Update(ctx, hfc)
}
