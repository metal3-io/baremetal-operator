package ironic

import (
	"testing"

	"github.com/gophercloud/gophercloud/openstack/baremetal/v1/nodes"
	"github.com/stretchr/testify/assert"

	metal3v1alpha1 "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
)

func TestBuildCapabilitiesValue(t *testing.T) {
	cases := []struct {
		Scenario      string
		Node          nodes.Node
		Mode          metal3v1alpha1.BootMode
		ExpectedValue string
	}{
		{
			Scenario:      "unset",
			Node:          nodes.Node{},
			Mode:          metal3v1alpha1.UEFI,
			ExpectedValue: "boot_mode:uefi",
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
		},
		{
			Scenario: "add-secure-boot",
			Node: nodes.Node{
				Properties: map[string]interface{}{
					"capabilities": "cpu_vt:true,cpu_aes:true,cpu_hugepages:true,cpu_hugepages_1g:true",
				},
			},
			Mode:          metal3v1alpha1.UEFISecureBoot,
			ExpectedValue: "cpu_vt:true,cpu_aes:true,cpu_hugepages:true,cpu_hugepages_1g:true,boot_mode:uefi,secure_boot:true",
		},
		{
			Scenario: "uefi-to-uefi",
			Node: nodes.Node{
				Properties: map[string]interface{}{
					"capabilities": "boot_mode:uefi,cpu_vt:true,cpu_aes:true,cpu_hugepages:true,cpu_hugepages_1g:true",
				},
			},
			Mode:          metal3v1alpha1.UEFI,
			ExpectedValue: "cpu_vt:true,cpu_aes:true,cpu_hugepages:true,cpu_hugepages_1g:true,boot_mode:uefi",
		},
		{
			Scenario: "bios-to-bios",
			Node: nodes.Node{
				Properties: map[string]interface{}{
					"capabilities": "boot_mode:bios,cpu_vt:true,cpu_aes:true,cpu_hugepages:true,cpu_hugepages_1g:true",
				},
			},
			Mode:          metal3v1alpha1.Legacy,
			ExpectedValue: "cpu_vt:true,cpu_aes:true,cpu_hugepages:true,cpu_hugepages_1g:true,boot_mode:bios",
		},
		{
			Scenario: "bios-to-uefi",
			Node: nodes.Node{
				Properties: map[string]interface{}{
					"capabilities": "boot_mode:bios,cpu_vt:true,cpu_aes:true,cpu_hugepages:true,cpu_hugepages_1g:true",
				},
			},
			Mode:          metal3v1alpha1.UEFI,
			ExpectedValue: "cpu_vt:true,cpu_aes:true,cpu_hugepages:true,cpu_hugepages_1g:true,boot_mode:uefi",
		},
		{
			Scenario: "uefi-to-bios",
			Node: nodes.Node{
				Properties: map[string]interface{}{
					"capabilities": "boot_mode:uefi,cpu_vt:true,cpu_aes:true,cpu_hugepages:true,cpu_hugepages_1g:true",
				},
			},
			Mode:          metal3v1alpha1.Legacy,
			ExpectedValue: "cpu_vt:true,cpu_aes:true,cpu_hugepages:true,cpu_hugepages_1g:true,boot_mode:bios",
		},
		{
			Scenario: "uefi-to-secure",
			Node: nodes.Node{
				Properties: map[string]interface{}{
					"capabilities": "boot_mode:uefi,cpu_vt:true,cpu_aes:true,cpu_hugepages:true,cpu_hugepages_1g:true",
				},
			},
			Mode:          metal3v1alpha1.UEFISecureBoot,
			ExpectedValue: "cpu_vt:true,cpu_aes:true,cpu_hugepages:true,cpu_hugepages_1g:true,boot_mode:uefi,secure_boot:true",
		},
		{
			Scenario: "secure-to-uefi",
			Node: nodes.Node{
				Properties: map[string]interface{}{
					"capabilities": "boot_mode:uefi,cpu_vt:true,cpu_aes:true,cpu_hugepages:true,cpu_hugepages_1g:true,secure_boot:true",
				},
			},
			Mode:          metal3v1alpha1.UEFI,
			ExpectedValue: "cpu_vt:true,cpu_aes:true,cpu_hugepages:true,cpu_hugepages_1g:true,boot_mode:uefi",
		},
	}

	for _, tc := range cases {
		t.Run(tc.Scenario, func(t *testing.T) {
			actualVal := buildCapabilitiesValue(&tc.Node, tc.Mode)
			assert.Equal(t, tc.ExpectedValue, actualVal)
		})
	}
}
