// SPDX-License-Identifier: Apache-2.0

// Package anaconda provisions a BareMetalHost by booting an anaconda installer
// ISO over Redfish virtual media and serving it a per host kickstart.
package anaconda

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner"
	"metal3.local/anaconda/internal/core"
	"metal3.local/anaconda/internal/httpapi"
	"metal3.local/anaconda/internal/redfish"
)

type HostStore interface {
	ReadInstallReport(ctx context.Context, namespace, name string) (*core.InstallReport, error)
	ClearInstallReport(ctx context.Context, namespace, name string) error
	InstallStartedAt(ctx context.Context, namespace, name string) (time.Time, error)
	MarkInstallStarted(ctx context.Context, namespace, name string, at time.Time) error
	HasKickstart(ctx context.Context, namespace, name string) (bool, error)
	HostInstallDisk(ctx context.Context, namespace, name string) (string, error)
	HostHardwareData(ctx context.Context, namespace, name string) (*metal3api.HardwareDetails, error)
}

type Factory struct {
	Store HostStore
	Cfg   core.Config
	// CallbackEnabled follows the configuration, not the bind. A listener asked
	// for and not running must fail installs, never pass them off as finished.
	CallbackEnabled bool
}

type Provisioner struct {
	Store     HostStore
	Publisher provisioner.EventPublisher
	HostData  provisioner.HostData
	Log       logr.Logger
	Cfg       core.Config

	CallbackEnabled bool
}

// NewProvisionerFactory starts the listener when one is configured and returns a
// Factory building per host provisioners. Interface args keep kube out of here.
func NewProvisionerFactory(cfg core.Config, store HostStore, resolver httpapi.ServerResolver) (provisioner.Factory, error) {
	f := &Factory{Cfg: cfg, Store: store}

	if !cfg.Enabled() {
		core.Log.Info("no listener address configured, kickstart and callback are disabled",
			"env", core.EnvListenAddr)

		return f, nil
	}

	server := &httpapi.PluginServer{
		Config:   cfg,
		Resolver: resolver,
		Log:      core.Log.WithName("http"),
	}

	// A failed bind leaves power management working and every install failing,
	// which beats reporting success against a listener serving no kickstart.
	f.CallbackEnabled = true

	if err := server.Start(context.Background()); err != nil {
		core.Log.Error(err, "plugin listener failed to start, no host can be installed",
			"listenAddr", cfg.ListenAddr)
	}

	return f, nil
}

// NewProvisioner creates a per host provisioner (ctx unused, present for the
// Factory interface).
func (f *Factory) NewProvisioner(
	_ context.Context,
	hostData provisioner.HostData,
	publisher provisioner.EventPublisher,
) (provisioner.Provisioner, error) {
	return &Provisioner{
		Cfg:             f.Cfg,
		Store:           f.Store,
		HostData:        hostData,
		Log:             core.Log.WithValues("host", hostData.ObjectMeta.Name),
		Publisher:       publisher,
		CallbackEnabled: f.CallbackEnabled,
	}, nil
}

func (p *Provisioner) Conn() (redfish.Conn, error) {
	addr, err := redfish.ParseRedfishAddress(p.HostData.BMCAddress)
	if err != nil {
		return redfish.Conn{}, err
	}

	return redfish.Conn{
		Endpoint: addr.Endpoint,
		SystemID: addr.SystemID,
		Username: p.HostData.BMCCredentials.Username,
		Password: p.HostData.BMCCredentials.Password,
	}, nil
}

func (p *Provisioner) Namespace() string { return p.HostData.ObjectMeta.Namespace }

func (p *Provisioner) Name() string { return p.HostData.ObjectMeta.Name }

func (p *Provisioner) Publish(reason, message string) {
	if p.Publisher != nil {
		p.Publisher(reason, message)
	}
}
