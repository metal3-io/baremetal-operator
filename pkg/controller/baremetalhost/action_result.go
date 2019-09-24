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
	return
}

func (r actionFailed) Dirty() bool {
	return r.dirty
}
