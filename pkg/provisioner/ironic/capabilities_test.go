package ironic

import (
	"testing"

	"github.com/gophercloud/gophercloud/openstack/baremetal/v1/nodes"
	metal3v1alpha1 "github.com/metal3-io/baremetal-operator/pkg/apis/metal3/v1alpha1"
	"github.com/stretchr/testify/assert"
)

func TestBuildCapabilitiesValue(t *testing.T) {
	cases := []struct {
		Scenario      string
		Node          nodes.Node
		Mode          metal3v1alpha1.BootMode
		ExpectedValue string
		ExpectedOp    nodes.UpdateOp
	}{
		{
			Scenario:      "unset",
			Node:          nodes.Node{},
			Mode:          metal3v1alpha1.UEFI,
			ExpectedValue: "boot_mode:uefi",
			ExpectedOp:    nodes.AddOp,
		},
		{
			Scenario: "empty",
			Node: nodes.Node{
				Properties: map[string]interface{}{
					"capabilities": "",
				},
			},
			Mode:          metal3v1alpha1.UEFI,
			ExpectedValue: "boot_mode:uefi",
			ExpectedOp:    nodes.ReplaceOp,
		},
		{
			Scenario: "not-there",
			Node: nodes.Node{
				Properties: map[string]interface{}{
					"capabilities": "cpu_vt:true,cpu_aes:true,cpu_hugepages:true,cpu_hugepages_1g:true",
				},
			},
			Mode:          metal3v1alpha1.UEFI,
			ExpectedValue: "cpu_vt:true,cpu_aes:true,cpu_hugepages:true,cpu_hugepages_1g:true,boot_mode:uefi",
			ExpectedOp:    nodes.ReplaceOp,
		},
		{
			Scenario: "uefi-to-uefi",
			Node: nodes.Node{
				Properties: map[string]interface{}{
					"capabilities": "boot_mode:uefi,cpu_vt:true,cpu_aes:true,cpu_hugepages:true,cpu_hugepages_1g:true",
				},
			},
			Mode:          metal3v1alpha1.UEFI,
			ExpectedValue: "boot_mode:uefi,cpu_vt:true,cpu_aes:true,cpu_hugepages:true,cpu_hugepages_1g:true",
			ExpectedOp:    nodes.ReplaceOp,
		},
		{
			Scenario: "bios-to-bios",
			Node: nodes.Node{
				Properties: map[string]interface{}{
					"capabilities": "boot_mode:bios,cpu_vt:true,cpu_aes:true,cpu_hugepages:true,cpu_hugepages_1g:true",
				},
			},
			Mode:          metal3v1alpha1.Legacy,
			ExpectedValue: "boot_mode:bios,cpu_vt:true,cpu_aes:true,cpu_hugepages:true,cpu_hugepages_1g:true",
			ExpectedOp:    nodes.ReplaceOp,
		},
		{
			Scenario: "bios-to-uefi",
			Node: nodes.Node{
				Properties: map[string]interface{}{
					"capabilities": "boot_mode:bios,cpu_vt:true,cpu_aes:true,cpu_hugepages:true,cpu_hugepages_1g:true",
				},
			},
			Mode:          metal3v1alpha1.UEFI,
			ExpectedValue: "boot_mode:uefi,cpu_vt:true,cpu_aes:true,cpu_hugepages:true,cpu_hugepages_1g:true",
			ExpectedOp:    nodes.ReplaceOp,
		},
		{
			Scenario: "uefi-to-bios",
			Node: nodes.Node{
				Properties: map[string]interface{}{
					"capabilities": "boot_mode:uefi,cpu_vt:true,cpu_aes:true,cpu_hugepages:true,cpu_hugepages_1g:true",
				},
			},
			Mode:          metal3v1alpha1.Legacy,
			ExpectedValue: "boot_mode:bios,cpu_vt:true,cpu_aes:true,cpu_hugepages:true,cpu_hugepages_1g:true",
			ExpectedOp:    nodes.ReplaceOp,
		},
	}

	for _, tc := range cases {
		t.Run(tc.Scenario, func(t *testing.T) {
			actualOp, actualVal := buildCapabilitiesValue(&tc.Node, tc.Mode)
			assert.Equal(t, tc.ExpectedOp, actualOp)
			assert.Equal(t, tc.ExpectedValue, actualVal)
		})
	}
}
