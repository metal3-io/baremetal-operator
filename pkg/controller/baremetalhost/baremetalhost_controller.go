package baremetalhost

import (
	"context"
	"fmt"
	"time"

	metalkubev1alpha1 "github.com/metalkube/baremetal-operator/pkg/apis/metalkube/v1alpha1"
	"github.com/metalkube/baremetal-operator/pkg/bmc"
	"github.com/metalkube/baremetal-operator/pkg/utils"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
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

	return nil
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

	// Fetch the BareMetalHost instance
	instance := &metalkubev1alpha1.BareMetalHost{}
	err := r.client.Get(context.TODO(), request.NamespacedName, instance)
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
	if instance.ObjectMeta.DeletionTimestamp.IsZero() &&
		!utils.StringInList(instance.ObjectMeta.Finalizers, metalkubev1alpha1.BareMetalHostFinalizer) {
		reqLogger.Info(
			"adding finalizer",
			"existingFinalizers", instance.ObjectMeta.Finalizers,
			"newValue", metalkubev1alpha1.BareMetalHostFinalizer,
		)
		instance.Finalizers = append(instance.Finalizers,
			metalkubev1alpha1.BareMetalHostFinalizer)
		err := r.client.Update(context.TODO(), instance)
		if err != nil {
			reqLogger.Error(err, "failed to add finalizer")
			return reconcile.Result{}, err
		}
		return reconcile.Result{Requeue: true}, nil
	}

	// Handle delete operations.
	if !instance.ObjectMeta.DeletionTimestamp.IsZero() {
		reqLogger.Info(
			"marked to be deleted",
			"timestamp", instance.ObjectMeta.DeletionTimestamp,
		)
		// no-op if finalizer has been removed.
		if !utils.StringInList(instance.ObjectMeta.Finalizers, metalkubev1alpha1.BareMetalHostFinalizer) {
			reqLogger.Info("BareMetalHost is ready to be deleted")
			return reconcile.Result{}, nil
		}

		// NOTE(dhellmann): This is where we would do something with
		// external resources not managed through CRs (those are
		// deleted automatically), like telling ironic to wipe the
		// host.

		// Remove finalizer to allow deletion
		reqLogger.Info("cleanup is complete, removing finalizer")
		instance.ObjectMeta.Finalizers = utils.FilterStringFromList(
			instance.ObjectMeta.Finalizers, metalkubev1alpha1.BareMetalHostFinalizer)
		if err := r.client.Update(context.TODO(), instance); err != nil {
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
	if instance.Spec.BMC.IP == "" {
		reqLogger.Info(bmc.MissingIPMsg)
		err := r.setErrorCondition(request, instance, bmc.MissingIPMsg)
		// Without the BMC IP there's no more we can do, so we're
		// going to return the emtpy Result anyway, and don't need to
		// check err.
		return reconcile.Result{}, err
	}

	if instance.Spec.BMC.Credentials == nil {
		reqLogger.Info(bmc.MissingCredentialsMsg)
		err := r.setErrorCondition(request, instance, bmc.MissingCredentialsMsg)
		// Without the BMC credentials there's no more we can do, so we're
		// going to return the emtpy Result anyway, and don't need to
		// check err.
		return reconcile.Result{}, err
	} else {

		bmcCreds, err := r.getBMCCredentials(request, instance)
		if err != nil {
			// If we can't fetch the BMC settings there's no more we can do.
			reqLogger.Info("missing BMC Credentials")
			return reconcile.Result{}, err
		}
		validCreds, reason := bmcCreds.AreValid()
		if !validCreds {
			reqLogger.Info("invalid BMC Credentials", "reason", reason)
			err := r.setErrorCondition(request, instance, reason)
			// We won't see a change to the secret, so put ourselves
			// back into the queue after a brief wait.
			return reconcile.Result{RequeueAfter: time.Second * 10}, err
		}

		// FIXME(dhellmann): Need to ensure the power state matches
		// the desired state here before updating the status label.

		newOpStatus := metalkubev1alpha1.OperationalStatusOffline
		if instance.Spec.Online {
			newOpStatus = metalkubev1alpha1.OperationalStatusOnline
		}
		if instance.SetOperationalStatus(newOpStatus) {
			reqLogger.Info(
				"setting operational status",
				"newStatus", newOpStatus,
			)
			if err := r.client.Update(context.TODO(), instance); err != nil {
				reqLogger.Error(err, "failed to update operational status")
				return reconcile.Result{}, err
			}
			return reconcile.Result{Requeue: true}, nil
		}
	}

	// Set the hardware profile name.
	//
	// FIXME(dhellmann): This should pull data from Ironic and compare
	// it against known profiles.
	if instance.SetLabel(metalkubev1alpha1.HardwareProfileLabel, "unknown") {
		if err := r.client.Update(context.TODO(), instance); err != nil {
			reqLogger.Error(err, "failed to update hardware profile")
			return reconcile.Result{}, err
		}
		return reconcile.Result{Requeue: true}, nil
	}

	// If we reach this point we haven't encountered any issues
	// communicating with the host, so ensure the error message field
	// is cleared.
	if instance.SetErrorMessage("") {
		reqLogger.Info("clearing error message")
		if err := r.saveStatus(instance); err != nil {
			reqLogger.Error(err, "failed to clear error message")
			return reconcile.Result{}, err
		}
	}

	// If we have nothing else to do and there is no LastUpdated
	// timestamp set, set one.
	if instance.Status.LastUpdated.IsZero() {
		reqLogger.Info("initializing status")
		if err := r.saveStatus(instance); err != nil {
			reqLogger.Error(err, "failed to initialize status block")
			return reconcile.Result{}, err
		}
	}

	// Pod already exists - don't requeue
	reqLogger.Info("Done with reconcile")
	return reconcile.Result{}, nil
}

func (r *ReconcileBareMetalHost) saveStatus(instance *metalkubev1alpha1.BareMetalHost) error {
	t := metav1.Now()
	instance.Status.LastUpdated = &t
	return r.client.Status().Update(context.TODO(), instance)
}

func (r *ReconcileBareMetalHost) setErrorCondition(request reconcile.Request, instance *metalkubev1alpha1.BareMetalHost, message string) error {
	reqLogger := log.WithValues("Request.Namespace",
		request.Namespace, "Request.Name", request.Name)

	if instance.SetErrorMessage(message) {
		reqLogger.Info(
			"adding error message",
			"message", message,
		)
		if err := r.saveStatus(instance); err != nil {
			reqLogger.Error(err, "failed to update error message")
			return err
		}
	}

	if instance.SetOperationalStatus(metalkubev1alpha1.OperationalStatusError) {
		reqLogger.Info(
			"setting operational status",
			"newStatus", metalkubev1alpha1.OperationalStatusError,
		)
		if err := r.client.Update(context.TODO(), instance); err != nil {
			reqLogger.Error(err, "failed to update operational status")
			return err
		}
	}

	return nil
}

func (r *ReconcileBareMetalHost) getBMCCredentials(request reconcile.Request, instance *metalkubev1alpha1.BareMetalHost) (creds bmc.Credentials, err error) {
	reqLogger := log.WithValues("Request.Namespace",
		request.Namespace, "Request.Name", request.Name)

	if instance.Spec.BMC.Credentials.Name == "" {
		return creds, fmt.Errorf(bmc.MissingCredentialsMsg)
	}

	// Fetch the named secret.
	secretKey := types.NamespacedName{
		Name: instance.Spec.BMC.Credentials.Name,
	}
	secretKey.Namespace = instance.Spec.BMC.Credentials.Namespace
	if secretKey.Namespace == "" {
		secretKey.Namespace = request.Namespace
	}
	reqLogger.Info("looking for secret", "secretKey", secretKey)

	bmcSecret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretKey.Name,
			Namespace: secretKey.Namespace,
		},
	}
	err = r.client.Get(context.TODO(), secretKey, bmcSecret)
	if err != nil {
		reqLogger.Error(err, "failed to fetch BMC credentials from secret reference")
		return creds, err
	}
	creds.Username = string(bmcSecret.Data["username"])
	creds.Password = string(bmcSecret.Data["password"])

	return creds, nil
}
