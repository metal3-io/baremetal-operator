//go:build vbmctl
// +build vbmctl

package network

import (
	"context"
	"errors"
	"fmt"
	"log"

	vbmctlapi "github.com/metal3-io/baremetal-operator/test/vbmctl/pkg/api"
	"github.com/vishvananda/netlink"
)

var ErrVethExistsWithWrongParams = errors.New("veth interface exists with different parameters")

func ConnectWithVeth(_ context.Context, network1 string, network2 string, vethpeer1 string, vethpeer2 string) error {
	// Get masters
	master1, err := netlink.LinkByName(network1)
	if err != nil {
		return fmt.Errorf("failed to get network interface %s: %w", network1, err)
	}
	master2, err := netlink.LinkByName(network2)
	if err != nil {
		return fmt.Errorf("failed to get network interface %s: %w", network2, err)
	}

	// Check if pair exists and has correct masters
	veth1, err1 := netlink.LinkByName(vethpeer1)
	veth2, err2 := netlink.LinkByName(vethpeer2)
	if err1 == nil && err2 == nil {
		// Return error if the masters is wrong. Someone has created and
		// configured the interfaces already, and we don't want to break their
		// configuration by modifying the interfaces.
		if veth1.Attrs().MasterIndex != master1.Attrs().Index {
			return ErrVethExistsWithWrongParams
		}
		if veth2.Attrs().MasterIndex != master2.Attrs().Index {
			return ErrVethExistsWithWrongParams
		}
		// Veth pair exists and has correct masters
		return nil
	}

	// Check for unknown errors
	var notFound netlink.LinkNotFoundError
	if !errors.As(err1, &notFound) || !errors.As(err2, &notFound) {
		return fmt.Errorf("error checking interfaces: %w AND %w", err1, err2)
	}

	la := netlink.NewLinkAttrs()
	la.Name = vethpeer1
	veth := &netlink.Veth{
		LinkAttrs: la,
		PeerName:  vethpeer2,
	}
	err = netlink.LinkAdd(veth)
	if err != nil {
		return fmt.Errorf("could not add vethpair %s: %w", la.Name, err)
	}

	// Get the newly created interfaces and configure them
	veth1, err = netlink.LinkByName(vethpeer1)
	if err != nil {
		return fmt.Errorf("failed to get veth interface %s: %w", vethpeer1, err)
	}
	veth2, err = netlink.LinkByName(vethpeer2)
	if err != nil {
		return fmt.Errorf("failed to get veth interface %s: %w", vethpeer2, err)
	}

	if err := netlink.LinkSetUp(veth1); err != nil {
		return fmt.Errorf("failed to bring up veth interface %s: %w", vethpeer1, err)
	}
	if err := netlink.LinkSetUp(veth2); err != nil {
		return fmt.Errorf("failed to bring up veth interface %s: %w", vethpeer2, err)
	}

	if err := netlink.LinkSetMaster(veth1, master1); err != nil {
		return fmt.Errorf("failed to set master %s for veth %s: %w", network1, vethpeer1, err)
	}
	if err := netlink.LinkSetMaster(veth2, master2); err != nil {
		return fmt.Errorf("failed to set master %s for veth %s: %w", network2, vethpeer2, err)
	}
	return nil
}

func ConnectAllWithVeth(ctx context.Context, vethPairs []vbmctlapi.VethPair) error {
	createdPairs := make([]vbmctlapi.VethPair, 0, len(vethPairs))
	for _, pair := range vethPairs {
		err := ConnectWithVeth(ctx, pair.Master1, pair.Master2, pair.Veth1, pair.Veth2)
		if err != nil {
			// Clean up previously created pairs
			log.Printf("Failed to create veth pair %s, cleaning up %d previously created pair(s)\n", pair.Veth1, len(createdPairs))
			for _, created := range createdPairs {
				if delErr := DeleteLink(ctx, created.Veth1); delErr != nil {
					log.Printf("Warning: failed to clean up veth pair %s: %v\n", created.Veth1, delErr)
				}
			}
			return fmt.Errorf("failed to create veth pair %s: %w", pair.Veth1, err)
		}
		createdPairs = append(createdPairs, pair)
	}
	return nil
}

func DeleteLink(_ context.Context, link string) error {
	l, err := netlink.LinkByName(link)
	var notFound netlink.LinkNotFoundError
	if errors.As(err, &notFound) {
		log.Printf("cannot delete network interface, interface %s does not exist", link)
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to get network interface %s: %w", link, err)
	}

	if err := netlink.LinkDel(l); err != nil {
		return fmt.Errorf("failed to delete network interface %s: %w", link, err)
	}
	return nil
}

func DeleteAllVeth(ctx context.Context, vethPairs []vbmctlapi.VethPair) error {
	var lastErr error
	for _, pair := range vethPairs {
		if err := DeleteLink(ctx, pair.Veth1); err != nil {
			log.Printf("Error deleting veth pair %s: %v\n", pair.Veth1, err)
			lastErr = err
		}
	}
	return lastErr
}
