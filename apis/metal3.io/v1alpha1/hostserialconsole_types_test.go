package v1alpha1

import (
	"testing"

	metal3apiv1beta1 "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1beta1"
	"github.com/stretchr/testify/assert"
)

func TestSerialConsoleAddressConversion(t *testing.T) {
	host := &BareMetalHost{}
	host.Spec.BMC.SerialConsoleAddress = "ipmi://192.0.2.10:623?cipher_suite=3"
	host.Status.Provisioning.DeprovisioningGeneration = 7
	hub := &metal3apiv1beta1.BareMetalHost{}
	if err := host.ConvertTo(hub); err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, host.Spec.BMC.SerialConsoleAddress, hub.Spec.BMC.SerialConsoleAddress)
	assert.Equal(t, host.Status.Provisioning.DeprovisioningGeneration, hub.Status.Provisioning.DeprovisioningGeneration)
	restored := &BareMetalHost{}
	if err := restored.ConvertFrom(hub); err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, host.Spec.BMC.SerialConsoleAddress, restored.Spec.BMC.SerialConsoleAddress)
	assert.Equal(t, host.Status.Provisioning.DeprovisioningGeneration, restored.Status.Provisioning.DeprovisioningGeneration)
}
