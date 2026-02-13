//go:build vbmctl
// +build vbmctl

package libvirt

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"

	"github.com/metal3-io/baremetal-operator/test/vbmctl/pkg/api"
	"libvirt.org/go/libvirt"
)

const (
	// filePerm is the default permission for pool directories.
	filePerm = 0755
)

// PoolManager manages storage pools and volumes through libvirt.
type PoolManager struct {
	conn     *libvirt.Connect
	renderer *TemplateRenderer
}

// NewPoolManager creates a new pool manager.
func NewPoolManager(conn *libvirt.Connect) *PoolManager {
	renderer, _ := NewTemplateRenderer()
	return &PoolManager{
		conn:     conn,
		renderer: renderer,
	}
}

// EnsurePool ensures a storage pool exists, creating it if necessary.
func (m *PoolManager) EnsurePool(_ context.Context, cfg api.PoolConfig) (*api.Pool, error) {
	// Check if pool already exists
	existingPool, err := m.conn.LookupStoragePoolByName(cfg.Name)
	if err == nil {
		return m.ensurePoolActive(existingPool, cfg)
	}

	// Create pool directory
	err = os.MkdirAll(cfg.Path, filePerm)
	if err != nil && !os.IsExist(err) {
		return nil, fmt.Errorf("failed to create pool directory %s: %w", cfg.Path, err)
	}

	// Render pool XML
	poolXML, err := m.renderer.RenderPool(PoolTemplateData{
		PoolName: cfg.Name,
		PoolPath: cfg.Path,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to render pool template: %w", err)
	}

	// Define the pool
	pool, err := m.conn.StoragePoolDefineXML(poolXML, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to define storage pool: %w", err)
	}
	defer func() { _ = pool.Free() }()

	// Set autostart
	if err = pool.SetAutostart(true); err != nil {
		log.Printf("Warning: failed to set pool autostart: %v\n", err)
	}

	// Start the pool
	if err = pool.Create(0); err != nil {
		return nil, fmt.Errorf("failed to start storage pool: %w", err)
	}

	log.Printf("Created storage pool %s at %s\n", cfg.Name, cfg.Path)
	return m.getPoolInfo(pool, cfg)
}

// ensurePoolActive ensures an existing pool is active and returns its info.
func (m *PoolManager) ensurePoolActive(existingPool *libvirt.StoragePool, cfg api.PoolConfig) (*api.Pool, error) {
	defer func() { _ = existingPool.Free() }()

	active, err := existingPool.IsActive()
	if err != nil {
		return nil, fmt.Errorf("failed to check pool status: %w", err)
	}

	if !active {
		if err = existingPool.Create(0); err != nil {
			return nil, fmt.Errorf("failed to activate existing pool: %w", err)
		}
	}

	log.Printf("Pool %s already exists\n", cfg.Name)
	return m.getPoolInfo(existingPool, cfg)
}

// CreateVolume creates a new volume in the specified pool.
func (m *PoolManager) CreateVolume(_ context.Context, poolName, volumeName string, sizeGB int) error {
	pool, err := m.conn.LookupStoragePoolByName(poolName)
	if err != nil {
		return fmt.Errorf("failed to lookup pool %s: %w", poolName, err)
	}
	defer func() { _ = pool.Free() }()

	// Check if volume already exists
	existingVol, err := pool.LookupStorageVolByName(volumeName + ".qcow2")
	if err == nil {
		_ = existingVol.Free()
		log.Printf("Volume %s already exists\n", volumeName)
		return nil
	}

	// Render volume XML
	volumeXML, err := m.renderer.RenderVolume(VolumeTemplateData{
		VolumeName:         volumeName,
		VolumeCapacityInGB: sizeGB,
	})
	if err != nil {
		return fmt.Errorf("failed to render volume template: %w", err)
	}

	// Create the volume
	volume, err := pool.StorageVolCreateXML(volumeXML, 0)
	if err != nil {
		return fmt.Errorf("failed to create volume: %w", err)
	}
	defer func() { _ = volume.Free() }()

	log.Printf("Created volume %s in pool %s\n", volumeName, poolName)
	return nil
}

// DeleteVolume deletes a volume from the specified pool.
func (m *PoolManager) DeleteVolume(_ context.Context, poolName, volumeName string) error {
	pool, err := m.conn.LookupStoragePoolByName(poolName)
	if err != nil {
		var libvirtErr libvirt.Error
		if errors.As(err, &libvirtErr) {
			if libvirtErr.Code == libvirt.ERR_NO_STORAGE_POOL {
				log.Printf("Pool %s does not exist, skipping volume deletion\n", poolName)
				return nil
			}
		}
		return fmt.Errorf("failed to lookup pool %s: %w", poolName, err)
	}
	defer func() { _ = pool.Free() }()

	// Try both with and without .qcow2 extension
	volNames := []string{volumeName, volumeName + ".qcow2"}
	for _, name := range volNames {
		if deleted, err := m.tryDeleteVolume(pool, poolName, name); err != nil {
			return err
		} else if deleted {
			return nil
		}
	}

	log.Printf("Volume %s not found in pool %s, skipping\n", volumeName, poolName)
	return nil
}

// tryDeleteVolume attempts to delete a volume by name and returns true if successful.
func (m *PoolManager) tryDeleteVolume(pool *libvirt.StoragePool, poolName, volumeName string) (bool, error) {
	volume, err := pool.LookupStorageVolByName(volumeName)
	if err != nil {
		return false, nil
	}
	defer func() { _ = volume.Free() }()

	if err := volume.Delete(0); err != nil {
		return false, fmt.Errorf("failed to delete volume %s: %w", volumeName, err)
	}

	log.Printf("Deleted volume %s from pool %s\n", volumeName, poolName)
	return true, nil
}

// ListVolumes lists all volumes in a storage pool.
func (m *PoolManager) ListVolumes(_ context.Context, poolName string) ([]*api.Volume, error) {
	pool, err := m.conn.LookupStoragePoolByName(poolName)
	if err != nil {
		return nil, fmt.Errorf("failed to lookup pool %s: %w", poolName, err)
	}
	defer func() { _ = pool.Free() }()

	volumes, err := pool.ListAllStorageVolumes(0)
	if err != nil {
		return nil, fmt.Errorf("failed to list volumes: %w", err)
	}

	result := make([]*api.Volume, 0, len(volumes))
	for _, vol := range volumes {
		name, err := vol.GetName()
		if err != nil {
			_ = vol.Free()
			continue
		}

		path, _ := vol.GetPath()
		info, _ := vol.GetInfo()

		result = append(result, &api.Volume{
			Config: api.VolumeConfig{
				Name: name,
			},
			Path:       path,
			Capacity:   info.Capacity,
			Allocation: info.Allocation,
		})

		_ = vol.Free()
	}

	return result, nil
}

// getPoolInfo extracts pool information from a libvirt pool.
func (m *PoolManager) getPoolInfo(pool *libvirt.StoragePool, cfg api.PoolConfig) (*api.Pool, error) {
	uuid, err := pool.GetUUIDString()
	if err != nil {
		return nil, fmt.Errorf("failed to get pool UUID: %w", err)
	}

	active, err := pool.IsActive()
	if err != nil {
		return nil, fmt.Errorf("failed to check pool status: %w", err)
	}

	info, err := pool.GetInfo()
	if err != nil {
		return nil, fmt.Errorf("failed to get pool info: %w", err)
	}

	return &api.Pool{
		Config:    cfg,
		UUID:      uuid,
		Active:    active,
		Capacity:  info.Capacity,
		Available: info.Available,
	}, nil
}
