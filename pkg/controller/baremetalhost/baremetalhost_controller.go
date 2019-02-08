package baremetalhost

import (
	"context"

	metalkubev1alpha1 "github.com/metalkube/baremetal-operator/pkg/apis/metalkube/v1alpha1"
	"github.com/metalkube/baremetal-operator/pkg/utils"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
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

	saveStatus := false

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

	reqLogger.Info("found BMH instance", "instance", instance)

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
		if err := r.client.Update(context.Background(), instance); err != nil {
			reqLogger.Error(err, "failed to remove finalizer")
			return reconcile.Result{}, err
		}
		return reconcile.Result{}, nil // done
	}

	// If we've never recorded an update, assume we're going to need
	// to do that.
	if instance.Status.LastUpdated.IsZero() {
		saveStatus = true
	}

	// If we do not have all of the needed BMC credentials, set our
	// operational status to indicate missing information.
	if instance.Spec.BMC.IP == "" || instance.Spec.BMC.Username == "" || instance.Spec.BMC.Password == "" {
		if !instance.Status.OperationalState.IsError() {
			reqLogger.Info("missing BMC connection details", "BMC", instance.Spec.BMC)
			instance.Status.OperationalState.SetError("Missing BMC connection details")
			saveStatus = true
		}
	} else {
		if instance.Status.OperationalState.IsError() {
			// Clear the error
			reqLogger.Info("clearing operational status error")
			instance.Status.OperationalState.SetOK("")
			saveStatus = true
		}
	}

	if saveStatus {
		t := metav1.Now()
		instance.Status.LastUpdated = &t
		if err = r.client.Status().Update(context.Background(), instance); err != nil {
			reqLogger.Error(err, "failed to update host status")
			return reconcile.Result{}, err
		} else {
			reqLogger.Info("successfully updated host status")
		}
	}

	// Pod already exists - don't requeue
	reqLogger.Info("Done with reconcile")
	return reconcile.Result{}, nil
}
