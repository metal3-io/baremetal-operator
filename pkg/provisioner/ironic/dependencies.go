package ironic

import (
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack/baremetal/v1/drivers"
	"github.com/pkg/errors"

	metal3v1alpha1 "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	"github.com/metal3-io/baremetal-operator/pkg/bmc"
)

const (
	checkRequeueDelay = time.Second * 10
)

type ironicDependenciesChecker struct {
	client    *gophercloud.ServiceClient
	inspector *gophercloud.ServiceClient
	host      *metal3v1alpha1.BareMetalHost
	log       logr.Logger
}

func newIronicDependenciesChecker(client *gophercloud.ServiceClient, inspector *gophercloud.ServiceClient, host *metal3v1alpha1.BareMetalHost,
	log logr.Logger) *ironicDependenciesChecker {
	return &ironicDependenciesChecker{
		client:    client,
		inspector: inspector,
		host:      host,
		log:       log,
	}
}

func (i *ironicDependenciesChecker) IsReady() (result bool, err error) {

	ready, err := i.checkIronic()
	if ready && err == nil {
		ready = i.checkIronicInspector()
	}

	return ready, err
}

func (i *ironicDependenciesChecker) checkEndpoint(client *gophercloud.ServiceClient) (ready bool) {

	// NOTE: Some versions of Ironic inspector returns 404 for /v1/ but 200 for /v1,
	// which seems to be the default behavior for Flask. Remove the trailing slash
	// from the client endpoint.
	endpoint := strings.TrimSuffix(client.Endpoint, "/")

	_, err := client.Get(endpoint, nil, nil)
	if err != nil {
		log.Info("error caught while checking endpoint", "endpoint", client.Endpoint, "error", err)
	}

	return err == nil
}

func (i *ironicDependenciesChecker) checkIronic() (ready bool, err error) {
	ready = i.checkEndpoint(i.client)
	if ready {
		ready, err = i.checkIronicConductor()
	}
	return ready, err
}

func (i *ironicDependenciesChecker) checkIronicConductor() (bool, error) {

	bmcAccess, err := bmc.NewAccessDetails(i.host.Spec.BMC.Address, i.host.Spec.BMC.DisableCertificateVerification)
	if err != nil {
		return false, errors.Wrap(err, "failed to parse BMC address information")
	}

	pager := drivers.ListDrivers(i.client, drivers.ListDriversOpts{
		Detail: false,
	})
	if pager.Err != nil {
		return false, pager.Err
	}

	page, err := pager.AllPages()
	if err != nil {
		return false, err
	}
	allDrivers, err := drivers.ExtractDrivers(page)
	if err != nil {
		return false, err
	}

	for _, driver := range allDrivers {
		if driver.Name == bmcAccess.Driver() {
			return true, nil
		}
	}

	return false, nil
}

func (i *ironicDependenciesChecker) checkIronicInspector() (ready bool) {
	return i.checkEndpoint(i.inspector)
}
