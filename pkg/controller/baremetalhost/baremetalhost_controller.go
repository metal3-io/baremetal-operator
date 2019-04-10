package baremetalhost

import (
	"context"
	"flag"
	"fmt"
	"time"

	"github.com/pkg/errors"

	metalkubev1alpha1 "github.com/metalkube/baremetal-operator/pkg/apis/metalkube/v1alpha1"
	"github.com/metalkube/baremetal-operator/pkg/bmc"
	"github.com/metalkube/baremetal-operator/pkg/provisioner"
	"github.com/metalkube/baremetal-operator/pkg/provisioner/demo"
	"github.com/metalkube/baremetal-operator/pkg/provisioner/fixture"
	"github.com/metalkube/baremetal-operator/pkg/provisioner/ironic"
	"github.com/metalkube/baremetal-operator/pkg/utils"

	"github.com/go-logr/logr"

	corev1 "k8s.io/api/core/v1"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const (
	hostErrorRetryDelay = time.Second * 10
)

var runInTestMode bool
var runInDemoMode bool

func init() {
	flag.BoolVar(&runInTestMode, "test-mode", false, "disable ironic communication")
	flag.BoolVar(&runInDemoMode, "demo-mode", false,
		"use the demo provisioner to set host states")
}

var log = logf.Log.WithName("baremetalhost")

// Add creates a new BareMetalHost Controller and adds it to the
// Manager. The Manager will set fields on the Controller and Start it
// when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	var provisionerFactory provisioner.Factory
	switch {
	case runInTestMode:
		log.Info("USING TEST MODE")
		provisionerFactory = fixture.New
	case runInDemoMode:
		log.Info("USING DEMO MODE")
		provisionerFactory = demo.New
	default:
		provisionerFactory = ironic.New
	}
	return &ReconcileBareMetalHost{
		client:             mgr.GetClient(),
		scheme:             mgr.GetScheme(),
		provisionerFactory: provisionerFactory,
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("metalkube-baremetalhost-controller", mgr,
		controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource BareMetalHost
	err = c.Watch(&source.Kind{Type: &metalkubev1alpha1.BareMetalHost{}},
		&handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// Watch for changes to secrets being used by hosts
	err = c.Watch(&source.Kind{Type: &corev1.Secret{}},
		&handler.EnqueueRequestForOwner{
			IsController: true,
			OwnerType:    &metalkubev1alpha1.BareMetalHost{},
		})
	return err
}

var _ reconcile.Reconciler = &ReconcileBareMetalHost{}

// ReconcileBareMetalHost reconciles a BareMetalHost object
type ReconcileBareMetalHost struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client             client.Client
	scheme             *runtime.Scheme
	provisionerFactory provisioner.Factory
}

// Instead of passing a zillion arguments to the action of a phase,
// hold them in a context
type reconcileInfo struct {
	log            logr.Logger
	host           *metalkubev1alpha1.BareMetalHost
	request        reconcile.Request
	bmcCredsSecret *corev1.Secret
	events         []corev1.Event
	errorMessage   string
}

// match the provisioner.EventPublisher interface
func (info *reconcileInfo) publishEvent(reason, message string) {
	info.events = append(info.events, info.host.NewEvent(reason, message))
}

// Action for one step of reconciliation.
//
// - Return a result if the host should be saved and requeued without error.
// - Return error if there was an error.
// - Return double nil if nothing was done and processing should continue.
type reconcileAction func(info *reconcileInfo) (*reconcile.Result, error)

// One step of reconciliation
type reconcilePhase struct {
	name   string
	action reconcileAction
}

// Reconcile reads that state of the cluster for a BareMetalHost
// object and makes changes based on the state read and what is in the
// BareMetalHost.Spec TODO(user): Modify this Reconcile function to
// implement your Controller logic.  This example creates a Pod as an
// example Note: The Controller will requeue the Request to be
// processed again if the returned error is non-nil or Result.Requeue
// is true, otherwise upon completion it will remove the work from the
// queue.
func (r *ReconcileBareMetalHost) Reconcile(request reconcile.Request) (result reconcile.Result, err error) {

	reqLogger := log.WithValues("Request.Namespace",
		request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling BareMetalHost")

	// Fetch the BareMetalHost
	host := &metalkubev1alpha1.BareMetalHost{}
	err = r.client.Get(context.TODO(), request.NamespacedName, host)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			// Request object not found, could have been deleted after
			// reconcile request.  Owned objects are automatically
			// garbage collected. For additional cleanup logic use
			// finalizers.  Return and don't requeue
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, errors.Wrap(err, "could not load host data")
	}

	// NOTE(dhellmann): Handle a few steps outside of the phase
	// structure because they require extra data lookup (like the
	// credential checks) or have to be done "first" (like delete
	// handling) to avoid looping.

	// Add a finalizer to newly created objects.
	if host.DeletionTimestamp.IsZero() && !hostHasFinalizer(host) {
		reqLogger.Info(
			"adding finalizer",
			"existingFinalizers", host.Finalizers,
			"newValue", metalkubev1alpha1.BareMetalHostFinalizer,
		)
		host.Finalizers = append(host.Finalizers,
			metalkubev1alpha1.BareMetalHostFinalizer)
		err := r.client.Update(context.TODO(), host)
		if err != nil {
			return reconcile.Result{}, errors.Wrap(err, "failed to add finalizer")
		}
		return reconcile.Result{Requeue: true}, nil
	}

	// Handle delete operations
	if !host.DeletionTimestamp.IsZero() {
		result, err := r.deleteHost(request, host)
		return result, err
	}

	// Check for a "discovered" host vs. one that we have all the info for.
	if host.Spec.BMC.Address == "" {
		reqLogger.Info(bmc.MissingAddressMsg)
		dirty := host.SetOperationalStatus(metalkubev1alpha1.OperationalStatusDiscovered)
		if dirty {
			err = r.saveStatus(host)
			if err != nil {
				// Only publish the event if we do not have an error
				// after saving so that we only publish one time.
				r.publishEvent(request,
					host.NewEvent("Discovered", "Discovered host without BMC address"))
			}
			// Without the address we can't do any more so we return here
			// without checking for an error.
			return reconcile.Result{Requeue: true}, err
		}
		reqLogger.Info("nothing to do for discovered host without BMC address")
		return reconcile.Result{}, nil
	}
	if host.Spec.BMC.CredentialsName == "" {
		reqLogger.Info(bmc.MissingCredentialsMsg)
		dirty := host.SetOperationalStatus(metalkubev1alpha1.OperationalStatusDiscovered)
		if dirty {
			err = r.saveStatus(host)
			if err != nil {
				// Only publish the event if we do not have an error
				// after saving so that we only publish one time.
				r.publishEvent(request,
					host.NewEvent("Discovered", "Discovered host without BMC credentials"))
			}
			// Without any credentials we can't do any more so we return
			// here without checking for an error.
			return reconcile.Result{Requeue: true}, err
		}
		reqLogger.Info("nothing to do for discovered host without BMC credentials")
		return reconcile.Result{}, nil
	}

	// Load the credentials for talking to the management controller.
	bmcCreds, bmcCredsSecret, err := r.getValidBMCCredentials(request, host)
	if err != nil {
		return reconcile.Result{}, errors.Wrap(err, "BMC credentials are invalid")
	}
	if bmcCreds == nil {
		// We do not have valid credentials, but did not encounter a
		// retriable error in determining that. Reconciliation is
		// complete until something about the secrets change.
		return reconcile.Result{}, nil
	}

	// Pick the action to perform
	var actionName metalkubev1alpha1.ProvisioningState
	switch {
	case host.CredentialsNeedValidation(*bmcCredsSecret):
		actionName = metalkubev1alpha1.StateRegistering
	case host.NeedsHardwareInspection():
		actionName = metalkubev1alpha1.StateInspecting
	case host.HardwareProfile() == "":
		actionName = metalkubev1alpha1.StateMatchProfile
	case host.NeedsProvisioning():
		actionName = metalkubev1alpha1.StateProvisioning
	case host.NeedsDeprovisioning():
		actionName = metalkubev1alpha1.StateDeprovisioning
	case host.WasProvisioned():
		actionName = metalkubev1alpha1.StateProvisioned
	default:
		actionName = metalkubev1alpha1.StateReady
	}

	if actionName != host.Status.Provisioning.State {
		host.Status.Provisioning.State = actionName
		reqLogger.Info(fmt.Sprintf("setting provisioning state to %q", actionName))
		if err := r.saveStatus(host); err != nil {
			return reconcile.Result{}, errors.Wrap(err,
				fmt.Sprintf("failed to save host status after handling %q", actionName))
		}
		return reconcile.Result{Requeue: true}, nil
	}

	info := &reconcileInfo{
		log:            reqLogger.WithValues("actionName", actionName),
		host:           host,
		request:        request,
		bmcCredsSecret: bmcCredsSecret,
	}
	prov, err := r.provisionerFactory(host, *bmcCreds, info.publishEvent)
	if err != nil {
		return reconcile.Result{}, errors.Wrap(err, "failed to create provisioner")
	}

	switch actionName {
	case metalkubev1alpha1.StateRegistering:
		result, err = r.actionRegistering(prov, info)
	case metalkubev1alpha1.StateInspecting:
		result, err = r.actionInspecting(prov, info)
	case metalkubev1alpha1.StateMatchProfile:
		result, err = r.actionMatchProfile(prov, info)
	case metalkubev1alpha1.StateProvisioning:
		result, err = r.actionProvisioning(prov, info)
	case metalkubev1alpha1.StateDeprovisioning:
		result, err = r.actionDeprovisioning(prov, info)
	case metalkubev1alpha1.StateProvisioned:
		result, err = r.actionManageHostPower(prov, info)
	case metalkubev1alpha1.StateReady:
		result, err = r.actionManageHostPower(prov, info)
	default:
		// Probably a provisioning error state?
		return reconcile.Result{}, fmt.Errorf("Unrecognized action %q", actionName)
	}

	if err != nil {
		return reconcile.Result{}, errors.Wrap(err, fmt.Sprintf("action %q failed", actionName))
	}

	// Only save status when we're told to requeue, otherwise we
	// introduce an infinite loop reconciling the same object over and
	// over when there is an unrecoverable error (tracked through the
	// error state of the host).
	if result.Requeue {
		info.log.Info("saving host status",
			"operational status", host.OperationalStatus(),
			"provisioning state", host.Status.Provisioning.State)
		if err = r.saveStatus(host); err != nil {
			return reconcile.Result{}, errors.Wrap(err,
				fmt.Sprintf("failed to save host status after %q", actionName))
		}
	}

	for _, e := range info.events {
		r.publishEvent(request, e)
	}

	if host.HasError() {
		// We have tried to do something that failed in a way we
		// assume is not retryable, so do not proceed to any other
		// steps.
		info.log.Info("stopping on host error")
		return reconcile.Result{}, nil
	}

	info.log.Info("done",
		"requeue", result.Requeue,
		"after", result.RequeueAfter,
	)
	return result, nil
}

// Handle all delete cases
func (r *ReconcileBareMetalHost) deleteHost(request reconcile.Request, host *metalkubev1alpha1.BareMetalHost) (result reconcile.Result, err error) {

	reqLogger := log.WithValues("Request.Namespace",
		request.Namespace, "Request.Name", request.Name)

	reqLogger.Info(
		"marked to be deleted",
		"timestamp", host.DeletionTimestamp,
	)

	// no-op if finalizer has been removed.
	if !utils.StringInList(host.Finalizers, metalkubev1alpha1.BareMetalHostFinalizer) {
		reqLogger.Info("ready to be deleted")
		// There is nothing to save and no reason to requeue since we
		// are being deleted.
		return reconcile.Result{}, nil
	}

	bmcCreds, _, err := r.getValidBMCCredentials(request, host)
	// We ignore the error, because we are deleting this host anyway.
	if bmcCreds == nil {
		// There are no valid credentials, so create an empty
		// credentials object to give to the provisioner.
		bmcCreds = &bmc.Credentials{}
	}

	eventPublisher := func(reason, message string) {
		r.publishEvent(request, host.NewEvent(reason, message))
	}

	prov, err := r.provisionerFactory(host, *bmcCreds, eventPublisher)
	if err != nil {
		return result, errors.Wrap(err, "failed to create provisioner")
	}

	reqLogger.Info("deprovisioning")
	provResult, err := prov.Deprovision(true)
	if err != nil {
		return result, errors.Wrap(err, "failed to deprovision")
	}
	if provResult.Dirty {
		err = r.saveStatus(host)
		if err != nil {
			return result, errors.Wrap(err, "failed to save host after deprovisioning")
		}
		result.Requeue = true
		result.RequeueAfter = provResult.RequeueAfter
		return result, nil
	}

	// Remove finalizer to allow deletion
	host.Finalizers = utils.FilterStringFromList(
		host.Finalizers, metalkubev1alpha1.BareMetalHostFinalizer)
	reqLogger.Info("cleanup is complete, removed finalizer",
		"remaining", host.Finalizers)
	if err := r.client.Update(context.Background(), host); err != nil {
		return result, errors.Wrap(err, "failed to remove finalizer")
	}

	return result, nil
}

// Test the credentials by connecting to the management controller.
func (r *ReconcileBareMetalHost) actionRegistering(prov provisioner.Provisioner, info *reconcileInfo) (result reconcile.Result, err error) {
	var provResult provisioner.Result

	info.log.Info("registering and validating access to management controller")

	provResult, err = prov.ValidateManagementAccess()
	if err != nil {
		return result, errors.Wrap(err, "failed to validate BMC access")
	}

	if provResult.ErrorMessage != "" {
		info.host.Status.Provisioning.State = metalkubev1alpha1.StateRegistrationError
		info.host.SetErrorMessage(provResult.ErrorMessage)
		info.publishEvent("RegistrationError", provResult.ErrorMessage)
		return result, nil
	}

	if provResult.Dirty {
		// Set Requeue true as well as RequeueAfter in case the delay
		// is 0.
		info.log.Info("host not ready")
		info.host.ClearError()
		result.Requeue = true
		result.RequeueAfter = provResult.RequeueAfter
		return result, nil
	}

	// Reaching this point means the credentials are valid and worked,
	// so record that in the status block.
	info.log.Info("updating credentials success status fields")
	info.host.UpdateGoodCredentials(*info.bmcCredsSecret)

	info.publishEvent("BMCAccessValidated", "Verified access to BMC")

	result.Requeue = true
	result.RequeueAfter = provResult.RequeueAfter
	return result, nil
}

// Ensure we have the information about the hardware on the host.
func (r *ReconcileBareMetalHost) actionInspecting(prov provisioner.Provisioner, info *reconcileInfo) (result reconcile.Result, err error) {
	var provResult provisioner.Result

	info.log.Info("inspecting hardware")

	provResult, err = prov.InspectHardware()
	if err != nil {
		return result, errors.Wrap(err, "hardware inspection failed")
	}

	if provResult.ErrorMessage != "" {
		info.host.Status.Provisioning.State = metalkubev1alpha1.StateRegistrationError
		info.host.SetErrorMessage(provResult.ErrorMessage)
		info.publishEvent("RegistrationError", provResult.ErrorMessage)
		return result, nil
	}

	if provResult.Dirty {
		info.host.ClearError()
		result.Requeue = true
		result.RequeueAfter = provResult.RequeueAfter
		return result, nil
	}

	// FIXME(dhellmann): Since we test the HardwareDetails pointer
	// before calling function, perhaps it makes sense to have
	// InspectHardware() return a value and store it here in this
	// function. That would eliminate duplication in the provisioners
	// and make this phase consistent with the structure of others.

	// Line up a requeue if we could set the hardware profile
	result.Requeue = info.host.NeedsHardwareProfile()

	return result, nil
}

func (r *ReconcileBareMetalHost) actionMatchProfile(prov provisioner.Provisioner, info *reconcileInfo) (result reconcile.Result, err error) {

	// FIXME(dhellmann): Insert logic to match hardware profiles here.
	hardwareProfile := "unknown"
	if info.host.SetHardwareProfile(hardwareProfile) {
		info.log.Info("updating hardware profile", "profile", hardwareProfile)
		info.publishEvent("ProfileSet", fmt.Sprintf("Hardware profile set: %s", hardwareProfile))
		info.host.ClearError()
		result.Requeue = true
		return result, nil
	}

	// Line up a requeue if we could provision
	result.Requeue = info.host.NeedsProvisioning()

	return result, nil
}

// Start/continue provisioning if we need to.
func (r *ReconcileBareMetalHost) actionProvisioning(prov provisioner.Provisioner, info *reconcileInfo) (result reconcile.Result, err error) {
	var provResult provisioner.Result

	getUserData := func() (string, error) {
		info.log.Info("fetching user data before provisioning")
		userDataSecret := &corev1.Secret{}
		key := types.NamespacedName{
			Name:      info.host.Spec.UserData.Name,
			Namespace: info.host.Spec.UserData.Namespace,
		}
		err = r.client.Get(context.TODO(), key, userDataSecret)
		if err != nil {
			return "", errors.Wrap(err,
				"failed to fetch user data from secret reference")
		}
		return string(userDataSecret.Data["userData"]), nil
	}

	info.log.Info("provisioning")

	provResult, err = prov.Provision(getUserData)
	if err != nil {
		return result, errors.Wrap(err, "failed to provision")
	}

	if provResult.ErrorMessage != "" {
		info.log.Info("handling provisioning error in controller")
		info.host.Status.Provisioning.State = metalkubev1alpha1.StateProvisioningError
		if info.host.SetErrorMessage(provResult.ErrorMessage) {
			info.publishEvent("ProvisioningError", provResult.ErrorMessage)
			result.Requeue = true
		}
		return result, nil
	}

	if provResult.Dirty {
		// Go back into the queue and wait for the Provision() method
		// to return false, indicating that it has no more work to
		// do.
		info.host.ClearError()
		result.Requeue = true
		result.RequeueAfter = provResult.RequeueAfter
		return result, nil
	}

	// If the provisioner had no work, ensure the image settings match.
	if info.host.Status.Provisioning.Image != *(info.host.Spec.Image) {
		info.log.Info("updating deployed image in status")
		info.host.Status.Provisioning.Image = *(info.host.Spec.Image)
	}

	// After provisioning we always requeue to ensure we enter the
	// "provisioned" state and start monitoring power status.
	result.Requeue = true

	return result, nil
}

func (r *ReconcileBareMetalHost) actionDeprovisioning(prov provisioner.Provisioner, info *reconcileInfo) (result reconcile.Result, err error) {
	var provResult provisioner.Result

	info.log.Info("deprovisioning")

	if provResult, err = prov.Deprovision(false); err != nil {
		return result, errors.Wrap(err, "failed to deprovision")
	}

	if provResult.ErrorMessage != "" {
		info.host.Status.Provisioning.State = metalkubev1alpha1.StateProvisioningError
		if info.host.SetErrorMessage(provResult.ErrorMessage) {
			info.publishEvent("ProvisioningError", provResult.ErrorMessage)
			result.Requeue = true
		}
		return result, nil
	}

	if provResult.Dirty {
		info.host.ClearError()
		result.Requeue = true
		result.RequeueAfter = provResult.RequeueAfter
		return result, nil
	}

	// After the provisioner is done, clear the image settings so we
	// transition to the next state.
	info.host.Status.Provisioning.Image = metalkubev1alpha1.Image{}

	// After deprovisioning we always requeue to ensure we enter the
	// "ready" state and start monitoring power status.
	result.Requeue = true

	return result, nil
}

// Check the current power status against the desired power status.
func (r *ReconcileBareMetalHost) actionManageHostPower(prov provisioner.Provisioner, info *reconcileInfo) (result reconcile.Result, err error) {
	var provResult provisioner.Result

	// Check the current status and save it before trying to update it.
	if provResult, err = prov.UpdateHardwareState(); err != nil {
		return result, errors.Wrap(err, "failed to update the hardware status")
	}

	if provResult.ErrorMessage != "" {
		info.host.Status.Provisioning.State = metalkubev1alpha1.StatePowerManagementError
		if info.host.SetErrorMessage(provResult.ErrorMessage) {
			info.publishEvent("PowerManagementError", provResult.ErrorMessage)
			result.Requeue = true
		}
		return result, nil
	}

	if provResult.Dirty {
		info.host.ClearError()
		result.Requeue = true
		result.RequeueAfter = provResult.RequeueAfter
		return result, nil
	}

	// Power state needs to be monitored regularly, so if we leave
	// this function without an error we always want to requeue after
	// a delay.
	result.RequeueAfter = time.Second * 60

	if info.host.Status.PoweredOn == info.host.Spec.Online {
		return result, nil
	}

	info.log.Info("power state change needed",
		"expected", info.host.Spec.Online,
		"actual", info.host.Status.PoweredOn)

	if info.host.Spec.Online {
		provResult, err = prov.PowerOn()
	} else {
		provResult, err = prov.PowerOff()
	}
	if err != nil {
		return result, errors.Wrap(err, "failed to manage power state of host")
	}

	if provResult.ErrorMessage != "" {
		info.host.Status.Provisioning.State = metalkubev1alpha1.StatePowerManagementError
		if info.host.SetErrorMessage(provResult.ErrorMessage) {
			info.publishEvent("PowerManagementError", provResult.ErrorMessage)
			result.Requeue = true
		}
		return result, nil
	}

	if provResult.Dirty {
		info.host.ClearError()
		result.Requeue = true
		result.RequeueAfter = provResult.RequeueAfter
		return result, nil
	}

	// The provisioner did not have to do anything to change the power
	// state and there were no errors, so reflect the new state in the
	// host status field.
	info.host.Status.PoweredOn = info.host.Spec.Online
	result.Requeue = true

	return result, nil

}

func (r *ReconcileBareMetalHost) saveStatus(host *metalkubev1alpha1.BareMetalHost) error {
	t := metav1.Now()
	host.Status.LastUpdated = &t
	return r.client.Status().Update(context.TODO(), host)
}

func (r *ReconcileBareMetalHost) setErrorCondition(request reconcile.Request, host *metalkubev1alpha1.BareMetalHost, message string) error {
	reqLogger := log.WithValues("Request.Namespace",
		request.Namespace, "Request.Name", request.Name)

	if host.SetErrorMessage(message) {
		reqLogger.Info(
			"adding error message",
			"message", message,
		)
		if err := r.saveStatus(host); err != nil {
			return errors.Wrap(err, "failed to update error message")
		}
	}

	return nil
}

// Make sure the credentials for the management controller look
// right. This does not actually try to use the credentials.
func (r *ReconcileBareMetalHost) getValidBMCCredentials(request reconcile.Request, host *metalkubev1alpha1.BareMetalHost) (bmcCreds *bmc.Credentials, bmcCredsSecret *corev1.Secret, err error) {
	reqLogger := log.WithValues("Request.Namespace",
		request.Namespace, "Request.Name", request.Name)

	// Load the secret containing the credentials for talking to the
	// BMC. This assumes we have a reference to the secret, otherwise
	// Reconcile() should not have let us be called.
	secretKey := host.CredentialsKey()
	bmcCredsSecret = &corev1.Secret{}
	err = r.client.Get(context.TODO(), secretKey, bmcCredsSecret)
	if err != nil {
		return nil, nil, errors.Wrap(err,
			"failed to fetch BMC credentials from secret reference")
	}
	bmcCreds = &bmc.Credentials{
		Username: string(bmcCredsSecret.Data["username"]),
		Password: string(bmcCredsSecret.Data["password"]),
	}

	// Verify that the secret contains the expected info.
	if validCreds, reason := bmcCreds.AreValid(); !validCreds {
		reqLogger.Info("invalid BMC Credentials", "reason", reason)
		err := r.setErrorCondition(request, host, reason)
		if err != nil {
			return nil, nil, errors.Wrap(err, "failed to set error condition")
		}

		// Only publish the event if we do not have an error
		// after saving so that we only publish one time.
		r.publishEvent(request, host.NewEvent("BMCCredentialError", reason))

		// This is not an error we can retry from, so stop reconciling.
		return nil, nil, nil
	}

	// Make sure the secret has the correct owner.
	if err = r.setBMCCredentialsSecretOwner(request, host, bmcCredsSecret); err != nil {
		return nil, nil, errors.Wrap(err,
			"failed to update owner of credentials secret")
	}

	return bmcCreds, bmcCredsSecret, nil
}

func (r *ReconcileBareMetalHost) setBMCCredentialsSecretOwner(request reconcile.Request, host *metalkubev1alpha1.BareMetalHost, secret *corev1.Secret) (err error) {
	reqLogger := log.WithValues("Request.Namespace",
		request.Namespace, "Request.Name", request.Name)
	if metav1.IsControlledBy(secret, host) {
		return nil
	}
	reqLogger.Info("updating owner of secret")
	err = controllerutil.SetControllerReference(host, secret, r.scheme)
	if err != nil {
		return errors.Wrap(err, "failed to set owner")
	}
	err = r.client.Update(context.TODO(), secret)
	if err != nil {
		return errors.Wrap(err, "failed to save owner")
	}
	return nil
}

func (r *ReconcileBareMetalHost) publishEvent(request reconcile.Request, event corev1.Event) {
	reqLogger := log.WithValues("Request.Namespace",
		request.Namespace, "Request.Name", request.Name)
	log.Info("publishing event", "reason", event.Reason, "message", event.Message)
	err := r.client.Create(context.TODO(), &event)
	if err != nil {
		reqLogger.Info("failed to record event, ignoring",
			"reason", event.Reason, "message", event.Message, "error", err)
	}
	return
}

func hostHasFinalizer(host *metalkubev1alpha1.BareMetalHost) bool {
	return utils.StringInList(host.Finalizers, metalkubev1alpha1.BareMetalHostFinalizer)
}
