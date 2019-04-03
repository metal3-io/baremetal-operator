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

var log = logf.Log.WithName("controller_baremetalhost")

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
	provisioner    provisioner.Provisioner
	request        reconcile.Request
	publisher      provisioner.EventPublisher
	bmcCredsSecret *corev1.Secret
}

// Action for one step of reconciliation.
//
// - Return a result if the host should be saved and requeued without error.
// - Return error if there was an error.
// - Return double nil if nothing was done and processing should continue.
type reconcileAction func(info reconcileInfo) (*reconcile.Result, error)

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
func (r *ReconcileBareMetalHost) Reconcile(request reconcile.Request) (reconcile.Result, error) {

	reqLogger := log.WithValues("Request.Namespace",
		request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling BareMetalHost")

	// Fetch the BareMetalHost
	host := &metalkubev1alpha1.BareMetalHost{}
	err := r.client.Get(context.TODO(), request.NamespacedName, host)
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
	if host.ObjectMeta.DeletionTimestamp.IsZero() && !hostHasFinalizer(host) {
		reqLogger.Info(
			"adding finalizer",
			"existingFinalizers", host.ObjectMeta.Finalizers,
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

	// Handle delete operations for discovered hosts separately
	// because we cannot connect to the BMC to deprovision them.
	if !host.ObjectMeta.DeletionTimestamp.IsZero() && host.Status.OperationalStatus == metalkubev1alpha1.OperationalStatusDiscovered {
		reqLogger.Info(
			"discovered host marked to be deleted",
			"timestamp", host.ObjectMeta.DeletionTimestamp,
		)
		if hostHasFinalizer(host) {
			reqLogger.Info("removing finalizer from discovered host without cleanup")
			host.ObjectMeta.Finalizers = utils.FilterStringFromList(
				host.ObjectMeta.Finalizers, metalkubev1alpha1.BareMetalHostFinalizer)
			if err := r.client.Update(context.TODO(), host); err != nil {
				return reconcile.Result{}, errors.Wrap(err, "failed to remove finalizer")
			}
			return reconcile.Result{Requeue: true}, nil
		}
		reqLogger.Info("discovered host is ready to be deleted")
		return reconcile.Result{}, nil
	}

	// Clear any error so we can recompute it
	host.ClearError()

	// Check for a "discovered" host vs. one that we have all the info for.
	if host.Spec.BMC.Address == "" {
		reqLogger.Info(bmc.MissingAddressMsg)
		dirty := host.SetOperationalStatus(metalkubev1alpha1.OperationalStatusDiscovered)
		if dirty {
			r.publishEvent(request, host, "Discovered", "Discovered host without BMC address")
			err = r.saveStatus(host)
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
			r.publishEvent(request, host, "Discovered", "Discovered host without BMC credentials")
			err = r.saveStatus(host)
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

	// Past this point we may need a provisioner, so create one.
	publisher := func(reason, message string) {
		r.publishEvent(request, host, reason, message)
	}
	prov, err := r.provisionerFactory(host, *bmcCreds, publisher)
	if err != nil {
		return reconcile.Result{}, errors.Wrap(err, "failed to create provisioner")
	}

	info := reconcileInfo{
		log:            reqLogger,
		host:           host,
		provisioner:    prov,
		request:        request,
		publisher:      publisher,
		bmcCredsSecret: bmcCredsSecret,
	}

	phases := []reconcilePhase{
		{name: "delete", action: r.phaseDelete},
		{name: "validate access", action: r.phaseValidateAccess},
		{name: "inspect hardware", action: r.phaseInspectHardware},
		{name: "hardware profile", action: r.phaseSetHardwareProfile},
		{name: "provisioning", action: r.phaseProvisioning},
		{name: "deprovisioning", action: r.phaseDeprovisioning},
		{name: "check hardware state", action: r.phaseCheckHardwareState},
		{name: "manage power", action: r.phaseManagePower},
	}
	for _, phase := range phases {
		info.log = reqLogger.WithValues("phase", phase.name)

		info.log.Info("starting")
		phaseResult, err := phase.action(info)
		if err != nil {
			return reconcile.Result{}, errors.Wrap(err, fmt.Sprintf("phase %s failed", phase.name))
		}
		if phaseResult == nil {
			continue
		}

		info.log.Info("saving host status")
		if err = r.saveStatus(host); err != nil {
			return reconcile.Result{}, errors.Wrap(err,
				fmt.Sprintf("failed to save host status after %s phase", phase.name))
		}

		if host.HasError() {
			// We have tried to do something that failed in a way we
			// assume is not retryable, so do not proceed to any other
			// steps.
			info.log.Info("stopping on host error")
			return reconcile.Result{}, nil
		}

		info.log.Info("phase done",
			"requeue", (*phaseResult).Requeue,
			"after", (*phaseResult).RequeueAfter,
		)
		return *phaseResult, nil
	}

	// If we have nothing else to do and there is no LastUpdated
	// timestamp set, set one.
	if host.Status.LastUpdated.IsZero() {
		reqLogger.Info("initializing status")
		if err := r.saveStatus(host); err != nil {
			return reconcile.Result{}, errors.Wrap(err, "failed to initialize status block")
		}
	}

	// Come back in 60 seconds to keep an eye on the power state
	reqLogger.Info("Done with reconcile")
	return reconcile.Result{RequeueAfter: time.Second * 60}, nil
}

// Handle delete operations.
func (r *ReconcileBareMetalHost) phaseDelete(info reconcileInfo) (result *reconcile.Result, err error) {
	var provResult provisioner.Result

	if info.host.ObjectMeta.DeletionTimestamp.IsZero() {
		return nil, nil
	}

	info.log.Info(
		"marked to be deleted",
		"timestamp", info.host.ObjectMeta.DeletionTimestamp,
	)

	// no-op if finalizer has been removed.
	if !utils.StringInList(info.host.ObjectMeta.Finalizers, metalkubev1alpha1.BareMetalHostFinalizer) {
		info.log.Info("ready to be deleted")
		// There is nothing to save and no reason to requeue since we
		// are being deleted.
		return &reconcile.Result{}, nil
	}

	info.log.Info("deprovisioning")
	if provResult, err = info.provisioner.Deprovision(true); err != nil {
		return nil, errors.Wrap(err, "failed to deprovision")
	}
	if provResult.Dirty {
		result = &reconcile.Result{
			Requeue:      true,
			RequeueAfter: provResult.RequeueAfter,
		}
		return result, nil
	}

	// Remove finalizer to allow deletion
	info.log.Info("cleanup is complete, removing finalizer")
	info.host.ObjectMeta.Finalizers = utils.FilterStringFromList(
		info.host.ObjectMeta.Finalizers, metalkubev1alpha1.BareMetalHostFinalizer)
	if err := r.client.Update(context.Background(), info.host); err != nil {
		return nil, errors.Wrap(err, "failed to remove finalizer")
	}

	return &reconcile.Result{}, nil
}

// Test the credentials by connecting to the management controller.
func (r *ReconcileBareMetalHost) phaseValidateAccess(info reconcileInfo) (result *reconcile.Result, err error) {
	var provResult provisioner.Result

	if !info.host.CredentialsNeedValidation(*info.bmcCredsSecret) {
		return nil, nil
	}

	info.log.Info("validating access to management controller")

	provResult, err = info.provisioner.ValidateManagementAccess()
	if err != nil {
		return nil, errors.Wrap(err, "failed to validate BMC access")
	}

	if info.host.Status.Provisioning.State == provisioner.StateRegistrationError {
		// We have tried to register and validate the host and that
		// failed. We have to return Requeue=true to ensure the host
		// object is saved before the main loop stops when it sees the
		// host error.
		info.log.Info("registration error")
		return &reconcile.Result{Requeue: true}, nil
	}

	if provResult.Dirty {
		// Set Requeue true as well as RequeueAfter in case the delay
		// is 0.
		info.log.Info("host not ready")
		result = &reconcile.Result{
			Requeue:      true,
			RequeueAfter: provResult.RequeueAfter,
		}
		return result, nil
	}

	// Reaching this point means the credentials are valid and worked,
	// so record that in the status block.
	info.log.Info("updating credentials success status fields")
	info.host.UpdateGoodCredentials(*info.bmcCredsSecret)

	result = &reconcile.Result{
		Requeue:      true,
		RequeueAfter: provResult.RequeueAfter,
	}
	return result, nil
}

// Ensure we have the information about the hardware on the host.
func (r *ReconcileBareMetalHost) phaseInspectHardware(info reconcileInfo) (result *reconcile.Result, err error) {
	var provResult provisioner.Result

	if info.host.Status.HardwareDetails != nil {
		return nil, nil
	}

	info.log.Info("inspecting hardware")

	provResult, err = info.provisioner.InspectHardware()
	if err != nil {
		return nil, errors.Wrap(err, "hardware inspection failed")
	}
	if provResult.Dirty {
		res := &reconcile.Result{
			Requeue:      true,
			RequeueAfter: provResult.RequeueAfter,
		}
		return res, nil
	}

	// FIXME(dhellmann): Since we test the HardwareDetails pointer
	// here in this function, perhaps it makes sense to have
	// InspectHardware() return a value and store it here in this
	// function. That would eliminate duplication in the provisioners
	// and make this phase consistent with the structure of others.
	return nil, nil
}

func (r *ReconcileBareMetalHost) phaseSetHardwareProfile(info reconcileInfo) (result *reconcile.Result, err error) {

	// FIXME(dhellmann): Insert logic to match hardware profiles here.
	hardwareProfile := "unknown"
	if info.host.SetHardwareProfile(hardwareProfile) {
		info.log.Info("updating hardware profile", "profile", hardwareProfile)
		info.publisher("ProfileSet", "Hardware profile set")
		return &reconcile.Result{Requeue: true}, nil
	}

	return nil, nil
}

// Start/continue provisioning if we need to.
func (r *ReconcileBareMetalHost) phaseProvisioning(info reconcileInfo) (result *reconcile.Result, err error) {
	var provResult provisioner.Result

	if !info.host.NeedsProvisioning() {
		return nil, nil
	}

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

	provResult, err = info.provisioner.Provision(getUserData)
	if err != nil {
		return nil, errors.Wrap(err, "failed to provision")
	}
	if provResult.Dirty {
		// Go back into the queue and wait for the Provision() method
		// to return false, indicating that it has no more work to
		// do.
		result = &reconcile.Result{
			Requeue:      true,
			RequeueAfter: provResult.RequeueAfter,
		}
		return result, nil
	}

	return nil, nil
}

func (r *ReconcileBareMetalHost) phaseDeprovisioning(info reconcileInfo) (result *reconcile.Result, err error) {
	var provResult provisioner.Result

	if !info.host.NeedsDeprovisioning() {
		return nil, nil
	}

	info.log.Info("deprovisioning")

	if provResult, err = info.provisioner.Deprovision(false); err != nil {
		return nil, errors.Wrap(err, "failed to deprovision")
	}
	if provResult.Dirty {
		result = &reconcile.Result{
			Requeue:      true,
			RequeueAfter: provResult.RequeueAfter,
		}
		return result, nil
	}

	return nil, nil
}

// Ask the backend about the current state of the hardware.
func (r *ReconcileBareMetalHost) phaseCheckHardwareState(info reconcileInfo) (result *reconcile.Result, err error) {
	var provResult provisioner.Result

	if provResult, err = info.provisioner.UpdateHardwareState(); err != nil {
		return nil, errors.Wrap(err, "failed to update the hardware status")
	}

	if provResult.Dirty {
		result = &reconcile.Result{
			Requeue:      true,
			RequeueAfter: provResult.RequeueAfter,
		}
		return result, nil
	}

	return nil, nil
}

// Check the current power status against the desired power status.
func (r *ReconcileBareMetalHost) phaseManagePower(info reconcileInfo) (result *reconcile.Result, err error) {
	var provResult provisioner.Result

	if info.host.Status.PoweredOn == info.host.Spec.Online {
		return nil, nil
	}

	info.log.Info("power state change needed",
		"expected", info.host.Spec.Online,
		"actual", info.host.Status.PoweredOn)

	if info.host.Spec.Online {
		provResult, err = info.provisioner.PowerOn()
	} else {
		provResult, err = info.provisioner.PowerOff()
	}
	if err != nil {
		return nil, errors.Wrap(err, "failed to manage power state of host")
	}

	if provResult.Dirty {
		result = &reconcile.Result{
			Requeue:      true,
			RequeueAfter: provResult.RequeueAfter,
		}
		return result, nil
	}

	return nil, nil
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
		r.publishEvent(request, host, "BMCCredentialError", reason)
		err := r.setErrorCondition(request, host, reason)
		return nil, nil, errors.Wrap(err, "failed to set error condition")
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

func (r *ReconcileBareMetalHost) publishEvent(request reconcile.Request, host *metalkubev1alpha1.BareMetalHost, reason, message string) {
	reqLogger := log.WithValues("Request.Namespace",
		request.Namespace, "Request.Name", request.Name)
	event := host.NewEvent(reason, message)
	log.Info("publishing event", "reason", reason, "message", message)
	err := r.client.Create(context.TODO(), &event)
	if err != nil {
		reqLogger.Info("failed to record event",
			"reason", reason, "message", message, "error", err)
	}
	return
}

func hostHasFinalizer(host *metalkubev1alpha1.BareMetalHost) bool {
	return utils.StringInList(host.ObjectMeta.Finalizers, metalkubev1alpha1.BareMetalHostFinalizer)
}
