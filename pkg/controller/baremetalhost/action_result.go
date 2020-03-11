package baremetalhost

import (
	metal3 "github.com/metal3-io/baremetal-operator/pkg/apis/metal3/v1alpha1"

	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"time"
)

// actionResult is an interface that encapsulates the result of a Reconcile
// call, as returned by the action corresponding to the current state.
type actionResult interface {
	Result() (reconcile.Result, error)
	Dirty() bool
}

// actionContinue is a result indicating that the current action is still
// in progress, and that the resource should remain in the same provisioning
// state.
type actionContinue struct {
	delay time.Duration
	dirty bool
}

func (r actionContinue) Result() (result reconcile.Result, err error) {
	result.RequeueAfter = r.delay
	// Set Requeue true as well as RequeueAfter in case the delay is 0.
	result.Requeue = true
	return
}

func (r actionContinue) Dirty() bool {
	return r.dirty
}

// actionComplete is a result indicating that the current action has completed,
// and that the resource should transition to the next state.
type actionComplete struct {
}

func (r actionComplete) Result() (result reconcile.Result, err error) {
	result.Requeue = true
	return
}

func (r actionComplete) Dirty() bool {
	return true
}

// deleteComplete is a result indicating that the deletion action has
// completed, and that the resource has now been deleted.
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

// actionError is a result indicating that an error occurred while attempting
// to advance the current action, and that reconciliation should be retried.
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

// actionFailed is a result indicating that the current action has failed,
// and that the resource should be marked as in error.
type actionFailed struct {
	dirty     bool
	ErrorType metal3.ErrorType
}

func (r actionFailed) Result() (result reconcile.Result, err error) {
	return
}

func (r actionFailed) Dirty() bool {
	return r.dirty
}
