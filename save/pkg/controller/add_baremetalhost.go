package controller

import (
	"github.com/metal3-io/baremetal-operator/pkg/controller/baremetalhost"
)

func init() {
	// AddToManagerFuncs is a list of functions to create controllers and add them to a manager.
	AddToManagerFuncs = append(AddToManagerFuncs, baremetalhost.Add)
}
