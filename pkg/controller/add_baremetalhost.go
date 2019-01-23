package controller

import (
	"github.com/metalkube/bare-metal-operator/pkg/controller/baremetalhost"
)

func init() {
	// AddToManagerFuncs is a list of functions to create controllers and add them to a manager.
	AddToManagerFuncs = append(AddToManagerFuncs, baremetalhost.Add)
}
