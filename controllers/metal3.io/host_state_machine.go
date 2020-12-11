package controllers

import (
	"fmt"

	metal3v1alpha1 "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// hostStateMachine is a finite state machine that manages transitions between
// the states of a BareMetalHost.
type hostStateMachine struct {
	Host        *metal3v1alpha1.BareMetalHost
	NextState   metal3v1alpha1.ProvisioningState
	Reconciler  *BareMetalHostReconciler
	Provisioner provisioner.Provisioner
	haveCreds   bool
}

func newHostStateMachine(host *metal3v1alpha1.BareMetalHost,
	reconciler *BareMetalHostReconciler,
	provisioner provisioner.Provisioner,
	haveCreds bool) *hostStateMachine {
	currentState := host.Status.Provisioning.State
	r := hostStateMachine{
		Host:        host,
		NextState:   currentState, // Remain in current state by default
		Reconciler:  reconciler,
		Provisioner: provisioner,
		haveCreds:   haveCreds,
	}
	return &r
}

type stateHandler func(*reconcileInfo) actionResult

func (hsm *hostStateMachine) handlers() map[metal3v1alpha1.ProvisioningState]stateHandler {
	return map[metal3v1alpha1.ProvisioningState]stateHandler{
		metal3v1alpha1.StateNone:                  hsm.handleNone,
		metal3v1alpha1.StateUnmanaged:             hsm.handleUnmanaged,
		metal3v1alpha1.StateRegistering:           hsm.handleRegistering,
		metal3v1alpha1.StateInspecting:            hsm.handleInspecting,
		metal3v1alpha1.StateExternallyProvisioned: hsm.handleExternallyProvisioned,
		metal3v1alpha1.StateMatchProfile:          hsm.handleMatchProfile,
		metal3v1alpha1.StateAvailable:             hsm.handleReady,
		metal3v1alpha1.StateReady:                 hsm.handleReady,
		metal3v1alpha1.StateProvisioning:          hsm.handleProvisioning,
		metal3v1alpha1.StateProvisioned:           hsm.handleProvisioned,
		metal3v1alpha1.StateDeprovisioning:        hsm.handleDeprovisioning,
		metal3v1alpha1.StateDeleting:              hsm.handleDeleting,
	}
}

func recordStateBegin(host *metal3v1alpha1.BareMetalHost, state metal3v1alpha1.ProvisioningState, time metav1.Time) {
	if nextMetric := host.OperationMetricForState(state); nextMetric != nil {
		if nextMetric.Start.IsZero() || !nextMetric.End.IsZero() {
			*nextMetric = metal3v1alpha1.OperationMetric{
				Start: time,
			}
		}
	}
}

func recordStateEnd(info *reconcileInfo, host *metal3v1alpha1.BareMetalHost, state metal3v1alpha1.ProvisioningState, time metav1.Time) {
	if prevMetric := host.OperationMetricForState(state); prevMetric != nil {
		if !prevMetric.Start.IsZero() && prevMetric.End.IsZero() {
			prevMetric.End = time
			info.postSaveCallbacks = append(info.postSaveCallbacks, func() {
				observer := stateTime[state].With(hostMetricLabels(info.request))
				observer.Observe(prevMetric.Duration().Seconds())
			})
		}
	}
}

func (hsm *hostStateMachine) updateHostStateFrom(initialState metal3v1alpha1.ProvisioningState,
	info *reconcileInfo) {
	if hsm.NextState != initialState {
		info.log.Info("changing provisioning state",
			"old", initialState,
			"new", hsm.NextState)
		now := metav1.Now()
		recordStateEnd(info, hsm.Host, initialState, now)
		recordStateBegin(hsm.Host, hsm.NextState, now)
		info.postSaveCallbacks = append(info.postSaveCallbacks, func() {
			stateChanges.With(stateChangeMetricLabels(initialState, hsm.NextState)).Inc()
		})
		hsm.Host.Status.Provisioning.State = hsm.NextState
		// Here we assume that if we're being asked to change the
		// state, the return value of ReconcileState (our caller) is
		// set up to ensure the change in the host is written back to
		// the API. That means we can safely update any status fields
		// along with the state.
		switch hsm.NextState {
		case metal3v1alpha1.StateInspecting,
			metal3v1alpha1.StateProvisioning:
			// TODO: When the user-selectable profile field is
			// removed, move saveHostProvisioningSettings() from the
			// controller to this point. We can't move it yet because
			// it needs error handling logic that we can't support in
			// this function.
			if updateBootModeStatus(hsm.Host) {
				info.log.Info("saving boot mode",
					"new mode", hsm.Host.Status.Provisioning.BootMode)
			}
		}
	}
}

func (hsm *hostStateMachine) ReconcileState(info *reconcileInfo) actionResult {
	initialState := hsm.Host.Status.Provisioning.State
	defer hsm.updateHostStateFrom(initialState, info)

	if hsm.checkInitiateDelete() {
		info.log.Info("Initiating host deletion")
		return actionComplete{}
	}

	if registerResult := hsm.ensureRegistered(info); registerResult != nil {
		hostRegistrationRequired.Inc()
		return registerResult
	}

	if stateHandler, found := hsm.handlers()[initialState]; found {
		return stateHandler(info)
	}

	info.log.Info("No handler found for state", "state", initialState)
	return actionError{fmt.Errorf("No handler found for state \"%s\"", initialState)}
}

func updateBootModeStatus(host *metal3v1alpha1.BareMetalHost) bool {
	// Make sure we have saved the current boot mode value.
	bootMode := host.BootMode()
	if bootMode == host.Status.Provisioning.BootMode {
		return false
	}
	host.Status.Provisioning.BootMode = bootMode
	return true
}

func (hsm *hostStateMachine) checkInitiateDelete() bool {
	if hsm.Host.DeletionTimestamp.IsZero() {
		// Delete not requested
		return false
	}

	switch hsm.NextState {
	default:
		hsm.NextState = metal3v1alpha1.StateDeleting
	case metal3v1alpha1.StateProvisioning, metal3v1alpha1.StateProvisioned:
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

func (hsm *hostStateMachine) ensureRegistered(info *reconcileInfo) (result actionResult) {
	if !hsm.haveCreds {
		// If we are in the process of deletion (which may start with
		// deprovisioning) and we have been unable to obtain any credentials,
		// don't attempt to re-register the Host as this will always fail.
		return
	}

	needsReregister := false

	switch hsm.NextState {
	case metal3v1alpha1.StateNone, metal3v1alpha1.StateUnmanaged:
		// We haven't yet reached the Registration state, so don't attempt
		// to register the Host.
		return
	case metal3v1alpha1.StateDeleting:
		// In the deleting state the whole idea is to de-register the host
		return
	case metal3v1alpha1.StateRegistering:
	default:
		needsReregister = (hsm.Host.Status.ErrorType == metal3v1alpha1.RegistrationError ||
			!hsm.Host.Status.GoodCredentials.Match(*info.bmcCredsSecret))
		if needsReregister {
			info.log.Info("Retrying registration")
			recordStateBegin(hsm.Host, metal3v1alpha1.StateRegistering, metav1.Now())
		}
	}

	result = hsm.Reconciler.actionRegistering(hsm.Provisioner, info)
	if _, complete := result.(actionComplete); complete {
		if hsm.NextState != metal3v1alpha1.StateRegistering {
			recordStateEnd(info, hsm.Host, metal3v1alpha1.StateRegistering, metav1.Now())
		}
		if needsReregister {
			// Host was re-registered, so requeue and run the state machine on
			// the next reconcile
			result = actionContinue{}
		} else {
			// Allow the state machine to run, either because we were just
			// reconfirming an existing registration, or because we are in the
			// Registering state
			result = nil
		}
	}
	return
}

func (hsm *hostStateMachine) handleNone(info *reconcileInfo) actionResult {
	// No state is set, so immediately move to either Registering or Unmanaged
	if hsm.Host.HasBMCDetails() {
		hsm.NextState = metal3v1alpha1.StateRegistering
	} else {
		info.publishEvent("Discovered", "Discovered host with no BMC details")
		hsm.Host.SetOperationalStatus(metal3v1alpha1.OperationalStatusDiscovered)
		hsm.NextState = metal3v1alpha1.StateUnmanaged
		hostUnmanaged.Inc()
	}
	return actionComplete{}
}

func (hsm *hostStateMachine) handleUnmanaged(info *reconcileInfo) actionResult {
	actResult := hsm.Reconciler.actionUnmanaged(hsm.Provisioner, info)
	if _, complete := actResult.(actionComplete); complete {
		hsm.NextState = metal3v1alpha1.StateRegistering
	}
	return actResult
}

func (hsm *hostStateMachine) handleRegistering(info *reconcileInfo) actionResult {
	// Getting to the state handler at all means we have successfully
	// registered using the current BMC credentials, so we can move to the
	// next state. We will not return to the Registering state, even
	// if the credentials change and the Host must be re-registered.
	if hsm.Host.Spec.ExternallyProvisioned {
		hsm.NextState = metal3v1alpha1.StateExternallyProvisioned
	} else {
		hsm.NextState = metal3v1alpha1.StateInspecting
	}
	hsm.Host.Status.ErrorCount = 0
	return actionComplete{}
}

func (hsm *hostStateMachine) handleInspecting(info *reconcileInfo) actionResult {
	actResult := hsm.Reconciler.actionInspecting(hsm.Provisioner, info)
	if _, complete := actResult.(actionComplete); complete {
		hsm.NextState = metal3v1alpha1.StateMatchProfile
		hsm.Host.Status.ErrorCount = 0
	}
	return actResult
}

func (hsm *hostStateMachine) handleMatchProfile(info *reconcileInfo) actionResult {
	actResult := hsm.Reconciler.actionMatchProfile(hsm.Provisioner, info)
	if _, complete := actResult.(actionComplete); complete {
		hsm.NextState = metal3v1alpha1.StateReady
		hsm.Host.Status.ErrorCount = 0
	}
	return actResult
}

func (hsm *hostStateMachine) handleExternallyProvisioned(info *reconcileInfo) actionResult {
	if hsm.Host.Spec.ExternallyProvisioned {
		// ErrorCount is cleared when appropriate inside actionManageSteadyState
		return hsm.Reconciler.actionManageSteadyState(hsm.Provisioner, info)
	}

	switch {
	case hsm.Host.NeedsHardwareInspection():
		hsm.NextState = metal3v1alpha1.StateInspecting
	case hsm.Host.NeedsHardwareProfile():
		hsm.NextState = metal3v1alpha1.StateMatchProfile
	default:
		hsm.NextState = metal3v1alpha1.StateReady
	}
	return actionComplete{}
}

func (hsm *hostStateMachine) handleReady(info *reconcileInfo) actionResult {
	if hsm.Host.Spec.ExternallyProvisioned {
		hsm.NextState = metal3v1alpha1.StateExternallyProvisioned
		return actionComplete{}
	}

	// ErrorCount is cleared when appropriate inside actionManageReady
	actResult := hsm.Reconciler.actionManageReady(hsm.Provisioner, info)
	if _, complete := actResult.(actionComplete); complete {
		hsm.NextState = metal3v1alpha1.StateProvisioning
	}
	return actResult
}

func (hsm *hostStateMachine) provisioningCancelled() bool {
	if hsm.Host.Status.ErrorMessage != "" {
		return true
	}
	if hsm.Host.Spec.Image == nil {
		return true
	}
	if hsm.Host.Spec.Image.URL == "" {
		return true
	}
	if hsm.Host.Status.Provisioning.Image.URL == "" {
		return false
	}
	if hsm.Host.Spec.Image.URL != hsm.Host.Status.Provisioning.Image.URL {
		return true
	}
	return false
}

func (hsm *hostStateMachine) handleProvisioning(info *reconcileInfo) actionResult {
	if hsm.provisioningCancelled() {
		hsm.NextState = metal3v1alpha1.StateDeprovisioning
		return actionComplete{}
	}

	actResult := hsm.Reconciler.actionProvisioning(hsm.Provisioner, info)
	if _, complete := actResult.(actionComplete); complete {
		hsm.NextState = metal3v1alpha1.StateProvisioned
		hsm.Host.Status.ErrorCount = 0
	}
	return actResult
}

func (hsm *hostStateMachine) handleProvisioned(info *reconcileInfo) actionResult {
	if hsm.provisioningCancelled() {
		hsm.NextState = metal3v1alpha1.StateDeprovisioning
		return actionComplete{}
	}

	// ErrorCount is cleared when appropriate inside actionManageSteadyState
	return hsm.Reconciler.actionManageSteadyState(hsm.Provisioner, info)
}

func (hsm *hostStateMachine) handleDeprovisioning(info *reconcileInfo) actionResult {
	actResult := hsm.Reconciler.actionDeprovisioning(hsm.Provisioner, info)

	if hsm.Host.DeletionTimestamp.IsZero() {
		if _, complete := actResult.(actionComplete); complete {
			hsm.NextState = metal3v1alpha1.StateReady
			hsm.Host.Status.ErrorCount = 0
		}
	} else {
		skipToDelete := func() actionResult {
			hsm.NextState = metal3v1alpha1.StateDeleting
			info.postSaveCallbacks = append(info.postSaveCallbacks, deleteWithoutDeprov.Inc)
			return actionComplete{}
		}

		switch r := actResult.(type) {
		case actionComplete:
			hsm.NextState = metal3v1alpha1.StateDeleting
			hsm.Host.Status.ErrorCount = 0
		case actionFailed:
			// If the provisioner gives up deprovisioning and
			// deletion has been requested, continue to delete.
			if hsm.Host.Status.ErrorCount > 3 {
				return skipToDelete()
			}
		case actionError:
			if r.NeedsRegistration() && !hsm.haveCreds {
				// If the host is not registered as a node in Ironic and we
				// lack the credentials to deprovision it, just continue to
				// delete.
				return skipToDelete()
			}
		}
	}
	return actResult
}

func (hsm *hostStateMachine) handleDeleting(info *reconcileInfo) actionResult {
	return hsm.Reconciler.actionDeleting(hsm.Provisioner, info)
}
