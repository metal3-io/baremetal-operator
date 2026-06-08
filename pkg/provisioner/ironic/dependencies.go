package ironic

import (
	"context"
	"fmt"

	"github.com/gophercloud/gophercloud/v2/openstack/baremetal/v1/drivers"
	"github.com/gophercloud/gophercloud/v2/pagination"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner/ironic/clients"
)

func (p *ironicProvisioner) init(ctx context.Context) error {
	p.debugLog.Info("verifying ironic provisioner dependencies")

	var err error
	p.availableFeatures, err = clients.GetAvailableFeatures(ctx, p.client)
	if err != nil {
		p.log.Info("error caught while checking endpoint, will retry", "endpoint", p.client.Endpoint, "error", err)
		return fmt.Errorf("%w: cannot reach ironic endpoint", provisioner.ErrNotReady)
	}

	p.client.Microversion = p.availableFeatures.ChooseMicroversion()
	p.availableFeatures.Log(p.debugLog)

	return p.checkIronicConductor(ctx)
}

func (p *ironicProvisioner) checkIronicConductor(ctx context.Context) error {
	pager := drivers.ListDrivers(p.client, drivers.ListDriversOpts{
		Detail: false,
	})
	if pager.Err != nil {
		return pager.Err
	}

	driverCount := 0
	err := pager.EachPage(ctx, func(_ context.Context, page pagination.Page) (bool, error) {
		actual, driverErr := drivers.ExtractDrivers(page)
		if driverErr != nil {
			return false, driverErr
		}
		driverCount += len(actual)
		return true, nil
	})
	if err != nil {
		p.log.Error(err, "Unexpected error from the drivers API, still initializing?")
		return fmt.Errorf("%w: unexpected error from the drivers API", provisioner.ErrNotReady)
	}

	if driverCount == 0 {
		return fmt.Errorf("%w: no drivers loaded in ironic", provisioner.ErrNotReady)
	}

	return nil
}
