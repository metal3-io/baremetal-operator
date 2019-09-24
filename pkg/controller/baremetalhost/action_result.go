package baremetalhost

import (
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"time"
)

type actionResult interface {
	Result() (reconcile.Result, error)
	Dirty() bool
}

type actionContinue struct {
	delay time.Duration
}

func (r actionContinue) Result() (result reconcile.Result, err error) {
	result.RequeueAfter = r.delay
	// Set Requeue true as well as RequeueAfter in case the delay is 0.
	result.Requeue = true
	return
}

func (r actionContinue) Dirty() bool {
	return true
}

type actionComplete struct {
}

func (r actionComplete) Result() (result reconcile.Result, err error) {
	result.Requeue = true
	return
}

func (r actionComplete) Dirty() bool {
	return true
}

type deleteComplete struct {
	actionComplete
}

func (r deleteComplete) Result() (result reconcile.Result, err error) {
	// Don't requeue, since the CR has been successfully deleted
	return
}

func (r deleteComplete) Dirty() bool {
	return false
}

type actionError struct {
	err error
}

func (r actionError) Result() (result reconcile.Result, err error) {
	err = r.err
	return
}

func (r actionError) Dirty() bool {
	return false
}

type actionFailed struct {
	dirty bool
}

func (r actionFailed) Result() (result reconcile.Result, err error) {
	// We don't actually want to requeue after a failure, but the Requeue
	// field is overloaded since it's also used to determine whether the
	// Host object needs to be written to the database.
	// Once we've written the Host object, we ignore the Requeue flag when
	// returning the final response if the Host is in an operational error
	// state, unless it's needed to transition the Host out of that state
	// despite the error (e.g. to delete it).
	// We should be able to eliminate all of these extra layers of meaning
	// in the future by using the Dirty() method at the top level.
	result.Requeue = r.dirty
	return
}

func (r actionFailed) Dirty() bool {
	return r.dirty
}
