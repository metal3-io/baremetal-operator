package baremetalhost

import (
	"context"

	metalkubev1alpha1 "github.com/metalkube/baremetal-operator/pkg/apis/metalkube/v1alpha1"
	"github.com/metalkube/baremetal-operator/pkg/bmc"
	"github.com/metalkube/baremetal-operator/pkg/utils"

	v1 "k8s.io/api/core/v1"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log.WithName("controller_baremetalhost")

// Add creates a new BareMetalHost Controller and adds it to the
// Manager. The Manager will set fields on the Controller and Start it
// when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileBareMetalHost{client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("baremetalhost-controller", mgr,
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
	err = c.Watch(&source.Kind{Type: &v1.Secret{}},
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
	client client.Client
	scheme *runtime.Scheme
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
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after
			// reconcile request.  Owned objects are automatically
			// garbage collected. For additional cleanup logic use
			// finalizers.  Return and don't requeue
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	// Add a finalizer to newly created objects.
	if host.ObjectMeta.DeletionTimestamp.IsZero() &&
		!utils.StringInList(host.ObjectMeta.Finalizers, metalkubev1alpha1.BareMetalHostFinalizer) {
		reqLogger.Info(
			"adding finalizer",
			"existingFinalizers", host.ObjectMeta.Finalizers,
			"newValue", metalkubev1alpha1.BareMetalHostFinalizer,
		)
		host.Finalizers = append(host.Finalizers,
			metalkubev1alpha1.BareMetalHostFinalizer)
		err := r.client.Update(context.TODO(), host)
		if err != nil {
			reqLogger.Error(err, "failed to add finalizer")
			return reconcile.Result{}, err
		}
		return reconcile.Result{Requeue: true}, nil
	}

	// Handle delete operations.
	if !host.ObjectMeta.DeletionTimestamp.IsZero() {
		reqLogger.Info(
			"marked to be deleted",
			"timestamp", host.ObjectMeta.DeletionTimestamp,
		)
		// no-op if finalizer has been removed.
		if !utils.StringInList(host.ObjectMeta.Finalizers, metalkubev1alpha1.BareMetalHostFinalizer) {
			reqLogger.Info("BareMetalHost is ready to be deleted")
			return reconcile.Result{}, nil
		}

		// NOTE(dhellmann): This is where we would do something with
		// external resources not managed through CRs (those are
		// deleted automatically), like telling ironic to wipe the
		// host.

		// Remove finalizer to allow deletion
		reqLogger.Info("cleanup is complete, removing finalizer")
		host.ObjectMeta.Finalizers = utils.FilterStringFromList(
			host.ObjectMeta.Finalizers, metalkubev1alpha1.BareMetalHostFinalizer)
		if err := r.client.Update(context.TODO(), host); err != nil {
			reqLogger.Error(err, "failed to remove finalizer")
			return reconcile.Result{}, err
		}
		return reconcile.Result{}, nil // done
	}

	// FIXME(dhellmann): There are likely to be many more cases where
	// we need to look for errors. Do we want to chain them all here
	// in if/elif blocks?

	// If we do not have all of the needed BMC credentials, set our
	// operational status to indicate missing information.
	if host.Spec.BMC.IP == "" {
		reqLogger.Info(bmc.MissingIPMsg)
		err := r.setErrorCondition(request, host, bmc.MissingIPMsg)
		// Without the BMC IP there's no more we can do, so we're
		// going to return the emtpy Result anyway, and don't need to
		// check err.
		return reconcile.Result{}, err
	}

	// Load the secret containing the credentials for talking to the
	// BMC.
	if host.Spec.BMC.CredentialsName == "" {
		// We have no name to use to load the secrets.
		reqLogger.Error(err, "BMC.CredentialsName is not set")
		err := r.setErrorCondition(request, host, bmc.MissingCredentialsMsg)
		return reconcile.Result{}, err
	}
	secretKey := host.CredentialsKey()
	bmcCredsSecret := &v1.Secret{}
	err = r.client.Get(context.TODO(), secretKey, bmcCredsSecret)
	if err != nil {
		reqLogger.Error(err, "failed to fetch BMC credentials from secret reference")
		return reconcile.Result{}, err
	}
	bmcCreds := bmc.Credentials{
		Username: string(bmcCredsSecret.Data["username"]),
		Password: string(bmcCredsSecret.Data["password"]),
	}

	// Make sure the secret has the correct owner.
	err = r.setBMCCredentialsSecretOwner(request, host, bmcCredsSecret)
	if err != nil {
		// FIXME: Set error condition?
		reqLogger.Error(err, "could not update owner of credentials secret")
		return reconcile.Result{}, err
	}

	// Verify that the secret contains the expected info.
	validCreds, reason := bmcCreds.AreValid()
	if !validCreds {
		reqLogger.Info("invalid BMC Credentials", "reason", reason)
		err := r.setErrorCondition(request, host, reason)
		return reconcile.Result{}, err
	}

	// Update the success info for the credentails.
	if host.CredentialsNeedValidation(*bmcCredsSecret) {

		// FIXME(dhellmann): Test using the credentials to get into
		// the BMC, and record error status if it fails.

		// Reaching this point means the credentials are valid and
		// worked, so record that in the status block.
		reqLogger.Info("updating credentials success status fields")
		host.UpdateGoodCredentials(*bmcCredsSecret)
		if err := r.saveStatus(host); err != nil {
			reqLogger.Error(err, "failed to update credentials success status fields")
			return reconcile.Result{}, err
		}
		return reconcile.Result{Requeue: true}, nil
	}

	// Update the operational status of the host.
	newOpStatus := metalkubev1alpha1.OperationalStatusOffline
	if host.Spec.Online {
		newOpStatus = metalkubev1alpha1.OperationalStatusOnline
	}
	if host.SetOperationalStatus(newOpStatus) {
		reqLogger.Info(
			"setting operational status",
			"newStatus", newOpStatus,
		)
		if err := r.client.Update(context.TODO(), host); err != nil {
			reqLogger.Error(err, "failed to update operational status")
			return reconcile.Result{}, err
		}
		return reconcile.Result{Requeue: true}, nil
	}

	// FIXME(dhellmann): Need to ensure the power state matches
	// the desired state here.

	// Set the hardware profile name.
	//
	// FIXME(dhellmann): This should pull data from Ironic and compare
	// it against known profiles.
	hardwareProfile := "unknown"
	if host.SetHardwareProfile(hardwareProfile) {
		reqLogger.Info("updating hardware profile", "profile", hardwareProfile)
		if err := r.client.Update(context.TODO(), host); err != nil {
			reqLogger.Error(err, "failed to update hardware profile")
			return reconcile.Result{}, err
		}
		return reconcile.Result{Requeue: true}, nil
	}

	// If we reach this point we haven't encountered any issues
	// communicating with the host, so ensure the error message field
	// is cleared.
	if host.SetErrorMessage("") {
		reqLogger.Info("clearing error message")
		if err := r.saveStatus(host); err != nil {
			reqLogger.Error(err, "failed to clear error message")
			return reconcile.Result{}, err
		}
	}

	// If we have nothing else to do and there is no LastUpdated
	// timestamp set, set one.
	if host.Status.LastUpdated.IsZero() {
		reqLogger.Info("initializing status")
		if err := r.saveStatus(host); err != nil {
			reqLogger.Error(err, "failed to initialize status block")
			return reconcile.Result{}, err
		}
	}

	// Pod already exists - don't requeue
	reqLogger.Info("Done with reconcile")
	return reconcile.Result{}, nil
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
			reqLogger.Error(err, "failed to update error message")
			return err
		}
	}

	if host.SetOperationalStatus(metalkubev1alpha1.OperationalStatusError) {
		reqLogger.Info(
			"setting operational status",
			"newStatus", metalkubev1alpha1.OperationalStatusError,
		)
		if err := r.client.Update(context.TODO(), host); err != nil {
			reqLogger.Error(err, "failed to update operational status")
			return err
		}
	}

	return nil
}

func (r *ReconcileBareMetalHost) setBMCCredentialsSecretOwner(request reconcile.Request, host *metalkubev1alpha1.BareMetalHost, secret *v1.Secret) (err error) {
	reqLogger := log.WithValues("Request.Namespace",
		request.Namespace, "Request.Name", request.Name)
	if metav1.IsControlledBy(secret, host) {
		return nil
	}
	reqLogger.Info("updating owner of secret")
	err = controllerutil.SetControllerReference(host, secret, r.scheme)
	if err != nil {
		reqLogger.Error(err, "failed to set owner")
		return err
	}
	err = r.client.Update(context.TODO(), secret)
	if err != nil {
		reqLogger.Error(err, "failed to save owner")
		return err
	}
	return nil
}
