package baremetalhost

import (
	"fmt"

	metal3v1alpha1 "github.com/metal3-io/baremetal-operator/pkg/apis/metal3/v1alpha1"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner"

	"github.com/go-logr/logr"

	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// hostStateMachine is a finite state machine that manages transitions between
// the states of a BareMetalHost.
type hostStateMachine struct {
	Host                *metal3v1alpha1.BareMetalHost
	NextState           metal3v1alpha1.ProvisioningState
	Reconciler          *ReconcileBareMetalHost
	Provisioner         provisioner.Provisioner
	RequeueDespiteError bool
}

func newHostStateMachine(host *metal3v1alpha1.BareMetalHost,
	reconciler *ReconcileBareMetalHost,
	provisioner provisioner.Provisioner) *hostStateMachine {
	currentState := host.Status.Provisioning.State
	r := hostStateMachine{
		Host:        host,
		NextState:   currentState,
		Reconciler:  reconciler,
		Provisioner: provisioner,
	}
	return &r
}

type stateHandler func(*reconcileInfo) (reconcile.Result, error)

func (hsm *hostStateMachine) handlers() map[metal3v1alpha1.ProvisioningState]stateHandler {
	return map[metal3v1alpha1.ProvisioningState]stateHandler{
		metal3v1alpha1.StateNone:                  hsm.handleNone,
		metal3v1alpha1.StateRegistering:           hsm.handleRegistering,
		metal3v1alpha1.StateRegistrationError:     hsm.handleRegistrationError,
		metal3v1alpha1.StateInspecting:            hsm.handleInspecting,
		metal3v1alpha1.StateExternallyProvisioned: hsm.handleExternallyProvisioned,
		metal3v1alpha1.StateMatchProfile:          hsm.handleMatchProfile,
		metal3v1alpha1.StateReady:                 hsm.handleReady,
		metal3v1alpha1.StateProvisioning:          hsm.handleProvisioning,
		metal3v1alpha1.StateProvisioningError:     hsm.handleProvisioningError,
		metal3v1alpha1.StateProvisioned:           hsm.handleProvisioned,
		metal3v1alpha1.StatePowerManagementError:  hsm.handlePowerManagementError,
		metal3v1alpha1.StateDeprovisioning:        hsm.handleDeprovisioning,
		metal3v1alpha1.StateDeleting:              hsm.handleDeleting,
	}
}

func (hsm *hostStateMachine) updateHostStateFrom(initialState metal3v1alpha1.ProvisioningState,
	result *reconcile.Result,
	log logr.Logger) {
	if hsm.NextState != initialState {
		log.Info("changing provisioning state",
			"old", initialState,
			"new", hsm.NextState)
		hsm.Host.Status.Provisioning.State = hsm.NextState
		result.Requeue = true
	}
}

func (hsm *hostStateMachine) ReconcileState(info *reconcileInfo) (result reconcile.Result, err error) {
	initialState := hsm.Host.Status.Provisioning.State
	defer hsm.updateHostStateFrom(initialState, &result, info.log)

	if hsm.shouldInitiateDelete() {
		info.log.Info("Initiating host deletion")
		return
	}
	// TODO: In future we should always re-register the host if required,
	// rather than initiate a transistion back to the Registering state.
	if hsm.shouldInitiateRegister(info) {
		info.log.Info("Initiating host registration")
		return
	}

	if stateHandler, found := hsm.handlers()[initialState]; found {
		result, err = stateHandler(info)
	} else {
		info.log.Info("No handler found for state", "state", initialState)
		err = fmt.Errorf("No handler found for state \"%s\"", initialState)
		return
	}

	return
}

func (hsm *hostStateMachine) shouldInitiateDelete() bool {
	if hsm.Host.DeletionTimestamp.IsZero() {
		// Delete not requested
		return false
	}

	hsm.RequeueDespiteError = true
	switch hsm.NextState {
	default:
		hsm.NextState = metal3v1alpha1.StateDeleting
	case metal3v1alpha1.StateProvisioning, metal3v1alpha1.StateProvisioningError, metal3v1alpha1.StateProvisioned:
		hsm.NextState = metal3v1alpha1.StateDeprovisioning
	case metal3v1alpha1.StateDeprovisioning:
		// Allow state machine to run to continue deprovisioning.
		return false
	case metal3v1alpha1.StateDeleting:
		// Already in deleting state. Allow state machine to run.
		return false
	}
	return true
}

func (hsm *hostStateMachine) shouldInitiateRegister(info *reconcileInfo) bool {
	changeState := false
	if hsm.Host.DeletionTimestamp.IsZero() {
		switch hsm.NextState {
		default:
			changeState = !hsm.Host.Status.GoodCredentials.Match(*info.bmcCredsSecret)
		case metal3v1alpha1.StateNone:
		case metal3v1alpha1.StateRegistering, metal3v1alpha1.StateRegistrationError:
		case metal3v1alpha1.StateDeleting:
		}
	}
	if changeState {
		hsm.NextState = metal3v1alpha1.StateRegistering
	}
	return changeState
}

func (hsm *hostStateMachine) handleNone(info *reconcileInfo) (result reconcile.Result, err error) {
	// Running the state machine at all means we have successfully validated
	// the BMC credentials once, so we can move to the Registering state.
	hsm.Host.ClearError()
	hsm.NextState = metal3v1alpha1.StateRegistering
	return
}

func (hsm *hostStateMachine) handleRegistering(info *reconcileInfo) (result reconcile.Result, err error) {
	actResult := hsm.Reconciler.actionRegistering(hsm.Provisioner, info)

	switch actResult.(type) {
	case actionComplete:
		// TODO: In future this state should only occur before the host is
		// registered the first time (though we must always check and
		// re-register the host regardless of the current state). That will
		// eliminate the need to determine which state we came from here.
		switch {
		case hsm.Host.Spec.ExternallyProvisioned:
			hsm.NextState = metal3v1alpha1.StateExternallyProvisioned
		case hsm.Host.WasProvisioned():
			hsm.NextState = metal3v1alpha1.StateProvisioned
		case hsm.Host.NeedsHardwareInspection():
			hsm.NextState = metal3v1alpha1.StateInspecting
		case hsm.Host.NeedsHardwareProfile():
			hsm.NextState = metal3v1alpha1.StateMatchProfile
		default:
			hsm.NextState = metal3v1alpha1.StateReady
		}
	}
	return actResult.Result()
}

func (hsm *hostStateMachine) handleRegistrationError(info *reconcileInfo) (result reconcile.Result, err error) {
	if !hsm.Host.Status.TriedCredentials.Match(*info.bmcCredsSecret) {
		info.log.Info("Modified credentials detected; will retry registration")
		hsm.RequeueDespiteError = true
		hsm.NextState = metal3v1alpha1.StateRegistering
	}
	return
}

func (hsm *hostStateMachine) handleInspecting(info *reconcileInfo) (result reconcile.Result, err error) {
	actResult := hsm.Reconciler.actionInspecting(hsm.Provisioner, info)
	if _, complete := actResult.(actionComplete); complete {
		hsm.NextState = metal3v1alpha1.StateMatchProfile
	}
	return actResult.Result()
}

func (hsm *hostStateMachine) handleMatchProfile(info *reconcileInfo) (result reconcile.Result, err error) {
	actResult := hsm.Reconciler.actionMatchProfile(hsm.Provisioner, info)
	if _, complete := actResult.(actionComplete); complete {
		hsm.NextState = metal3v1alpha1.StateReady
	}
	return actResult.Result()
}

func (hsm *hostStateMachine) handleExternallyProvisioned(info *reconcileInfo) (result reconcile.Result, err error) {
	if hsm.Host.Spec.ExternallyProvisioned {
		result, err = hsm.Reconciler.actionManageSteadyState(hsm.Provisioner, info)
	} else {
		switch {
		case hsm.Host.NeedsHardwareInspection():
			hsm.NextState = metal3v1alpha1.StateInspecting
		case hsm.Host.NeedsHardwareProfile():
			hsm.NextState = metal3v1alpha1.StateMatchProfile
		default:
			hsm.NextState = metal3v1alpha1.StateReady
		}
	}
	return
}

func (hsm *hostStateMachine) handleReady(info *reconcileInfo) (result reconcile.Result, err error) {
	switch {
	case hsm.Host.Spec.ExternallyProvisioned:
		hsm.NextState = metal3v1alpha1.StateExternallyProvisioned
	case hsm.Host.NeedsProvisioning():
		hsm.NextState = metal3v1alpha1.StateProvisioning
	default:
		result, err = hsm.Reconciler.actionManageReady(hsm.Provisioner, info)
	}
	return
}

func (hsm *hostStateMachine) handleProvisioning(info *reconcileInfo) (result reconcile.Result, err error) {
	if hsm.Host.NeedsDeprovisioning() {
		hsm.NextState = metal3v1alpha1.StateDeprovisioning
		return
	}

	actResult := hsm.Reconciler.actionProvisioning(hsm.Provisioner, info)
	switch actResult.(type) {
	case actionComplete:
		hsm.NextState = metal3v1alpha1.StateProvisioned
	case actionFailed:
		hsm.NextState = metal3v1alpha1.StateProvisioningError
	}
	return actResult.Result()
}

func (hsm *hostStateMachine) handleProvisioningError(info *reconcileInfo) (result reconcile.Result, err error) {
	hsm.RequeueDespiteError = true
	switch {
	case hsm.Host.Spec.ExternallyProvisioned:
		hsm.NextState = metal3v1alpha1.StateExternallyProvisioned
	default:
		hsm.NextState = metal3v1alpha1.StateDeprovisioning
	}
	return
}

func (hsm *hostStateMachine) handleProvisioned(info *reconcileInfo) (result reconcile.Result, err error) {
	if hsm.Host.NeedsDeprovisioning() {
		hsm.NextState = metal3v1alpha1.StateDeprovisioning
	} else {
		result, err = hsm.Reconciler.actionManageSteadyState(hsm.Provisioner, info)
	}
	return
}

func (hsm *hostStateMachine) handlePowerManagementError(info *reconcileInfo) (result reconcile.Result, err error) {
	hsm.RequeueDespiteError = true
	switch {
	case hsm.Host.Spec.ExternallyProvisioned:
		hsm.NextState = metal3v1alpha1.StateExternallyProvisioned
	case hsm.Host.WasProvisioned():
		hsm.NextState = metal3v1alpha1.StateProvisioned
	default:
		hsm.NextState = metal3v1alpha1.StateReady
	}
	return
}

func (hsm *hostStateMachine) handleDeprovisioning(info *reconcileInfo) (result reconcile.Result, err error) {
	result, err = hsm.Reconciler.actionDeprovisioning(hsm.Provisioner, info)
	if hsm.Host.Status.Provisioning.Image.URL == "" {
		if !hsm.Host.DeletionTimestamp.IsZero() {
			hsm.NextState = metal3v1alpha1.StateDeleting
		} else {
			hsm.NextState = metal3v1alpha1.StateReady
		}
	} else if hsm.Host.HasError() {
		if !hsm.Host.DeletionTimestamp.IsZero() {
			// If the provisioner gives up deprovisioning and
			// deletion has been requested, continue to delete.
			// Note that this is entirely theoretical, as the
			// Ironic provisioner currently never gives up
			// trying to deprovision.
			hsm.RequeueDespiteError = true
			hsm.NextState = metal3v1alpha1.StateDeleting
		} else {
			hsm.NextState = metal3v1alpha1.StateProvisioningError
		}
	}
	return
}

func (hsm *hostStateMachine) handleDeleting(info *reconcileInfo) (result reconcile.Result, err error) {
	result, err = hsm.Reconciler.actionDeleting(hsm.Provisioner, info)
	return
}
