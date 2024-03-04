package ironic

import (
	"context"

	"github.com/gophercloud/gophercloud/v2/openstack/baremetal/v1/drivers"
	"github.com/gophercloud/gophercloud/v2/pagination"

	"github.com/metal3-io/baremetal-operator/pkg/provisioner/ironic/clients"
)

// TryInit checks if the provisioning backend is available.
func (p *ironicProvisioner) TryInit() (ready bool, err error) {
	p.debugLog.Info("verifying ironic provisioner dependencies")

	p.availableFeatures, err = clients.GetAvailableFeatures(p.ctx, p.client)
	if err != nil {
		p.log.Info("error caught while checking endpoint, will retry", "endpoint", p.client.Endpoint, "error", err)
		return false, nil
	}

	p.client.Microversion = p.availableFeatures.ChooseMicroversion()
	p.availableFeatures.Log(p.debugLog)

	return p.checkIronicConductor()
}

func (p *ironicProvisioner) checkIronicConductor() (ready bool, err error) {
	pager := drivers.ListDrivers(p.client, drivers.ListDriversOpts{
		Detail: false,
	})
	err = pager.Err

	if err != nil {
		return ready, err
	}

	driverCount := 0
	pager.EachPage(p.ctx, func(_ context.Context, page pagination.Page) (bool, error) {
		actual, driverErr := drivers.ExtractDrivers(page)
		if driverErr != nil {
			return false, driverErr
		}
		driverCount += len(actual)
		return true, nil
	})
	// If we have any drivers, conductor is up.
	ready = driverCount > 0

	return ready, err
}
