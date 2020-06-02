package discovery

import (
	"context"
	"time"

	"github.com/pkg/errors"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack/baremetal/v1/nodes"
	"github.com/gophercloud/gophercloud/pagination"

	metal3v1alpha1 "github.com/metal3-io/baremetal-operator/pkg/apis/metal3/v1alpha1"
	ironicclient "github.com/metal3-io/baremetal-operator/pkg/provisioner/ironic/client"
)

var log = logf.Log.WithName("baremetalhost-discovery")

// Scanner returns a new manager for identifying hosts that have been
// seen by ironic but do not have a matching BareMetalHost resource.
func Scanner(mgr manager.Manager, period time.Duration) (scanner manager.Runnable, err error) {
	ironic, err := ironicclient.New()
	if err != nil {
		return nil, errors.Wrap(err, "failed to create ironic client")
	}
	inspector, err := ironicclient.NewInspector()
	if err != nil {
		return nil, errors.Wrap(err, "failed to create ironic-inspector client")
	}
	scanner = &discoveryScanner{
		client:    mgr.GetClient(),
		period:    period,
		ironic:    ironic,
		inspector: inspector,
	}
	return scanner, nil
}

type discoveryScanner struct {
	// kubernetes API client
	client client.Client
	// scanning interval (number of seconds)
	period time.Duration
	// a client for talking to ironic
	ironic *gophercloud.ServiceClient
	// a client for talking to ironic-inspector
	inspector *gophercloud.ServiceClient
}

func (scanner *discoveryScanner) Start(done <-chan struct{}) error {
	for {
		select {
		case <-done:
			return nil
		case <-time.After(scanner.period):
			scanner.poll()
		}
	}
}

func (scanner *discoveryScanner) poll() {
	log.Info("polling")

	ctx := context.TODO()
	hostList := metal3v1alpha1.BareMetalHostList{}
	err := scanner.client.List(ctx, &hostList)
	if err != nil {
		log.Error(err, "failed to fetch list of hosts")
		return
	}
	log.Info("got hosts", "count", len(hostList.Items))

	// Organize the data to make it easier to find existing hosts
	// based on data Ironic will have.
	byUUID := make(map[string]metal3v1alpha1.BareMetalHost)
	byName := make(map[string]metal3v1alpha1.BareMetalHost)
	for _, host := range hostList.Items {
		byName[host.Name] = host
		if host.Status.Provisioning.ID != "" {
			byUUID[host.Status.Provisioning.ID] = host
		}
	}

	// FIXME: Should we constrain this list at all? Maybe only
	// look for hosts that are in a particular state?
	nodePages := nodes.ListDetail(scanner.ironic, nodes.ListOpts{})
	nodePages.EachPage(func(p pagination.Page) (bool, error) {
		nodeList, err := nodes.ExtractNodes(p)
		if err != nil {
			return false, err
		}
		for _, node := range nodeList {
			var ok bool
			log.Info("looking for ironic node", "uuid", node.UUID, "name", node.Name)
			_, ok = byUUID[node.UUID]
			if ok {
				log.Info("host is known")
				continue
			}
			_, ok = byName[node.Name]
			if ok {
				log.Info("host is known by name")
				continue
			}
			log.Info("host is unknown")
		}
		return true, nil
	})
}
