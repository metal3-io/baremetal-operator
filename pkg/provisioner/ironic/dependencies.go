package ironic

import (
	"github.com/gophercloud/gophercloud/openstack/baremetal/v1/drivers"
	"github.com/gophercloud/gophercloud/pagination"
)

// IsReady checks if the provisioning backend is available
func (p *ironicProvisioner) IsReady() (result bool, err error) {
	p.debugLog.Info("verifying ironic provisioner dependencies")
	ready := p.checkEndpoint()
	if ready {
		ready, err = p.checkIronicConductor()
	}
	return ready, err
}

func (p *ironicProvisioner) checkEndpoint() (ready bool) {
	_, err := p.client.Get(p.client.Endpoint, nil, nil)
	if err != nil {
		p.log.Info("error caught while checking endpoint", "endpoint", p.client.Endpoint, "error", err)
	}

	return err == nil
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
	pager.EachPage(func(page pagination.Page) (bool, error) {
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
