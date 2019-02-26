package baremetalhost

import (
	"context"
	"time"

	metalkubev1alpha1 "github.com/metalkube/baremetal-operator/pkg/apis/metalkube/v1alpha1"
	"github.com/metalkube/baremetal-operator/pkg/bmc"
	"github.com/metalkube/baremetal-operator/pkg/provisioning"
	"github.com/metalkube/baremetal-operator/pkg/utils"

	corev1 "k8s.io/api/core/v1"

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
	return &ReconcileBareMetalHost{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
		Provisioner: &provisioning.Provisioner{
			DeprovisionRequeueDelay: time.Second * 10,
		},
	}
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
	Client client.Client
	Scheme *runtime.Scheme

	// Provisioner handles interacting with the provisioning system.
	Provisioner *provisioning.Provisioner
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

	var dirty bool // have we updated the host status but not saved it?

	reqLogger := log.WithValues("Request.Namespace",
		request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling BareMetalHost")

	// Fetch the BareMetalHost
	host := &metalkubev1alpha1.BareMetalHost{}
	err := r.Client.Get(context.TODO(), request.NamespacedName, host)
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
		err := r.Client.Update(context.TODO(), host)
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

		if dirty, err = r.Provisioner.Deprovision(host); err != nil {
			reqLogger.Error(err, "failed to deprovision")
			return reconcile.Result{}, err
		}
		if dirty {
			if err := r.saveStatus(host); err != nil {
				reqLogger.Error(err, "failed to clear host status on deprovision")
				return reconcile.Result{}, err
			}
			// Go back into the queue and wait for the Deprovision() method
			// to return false, indicating that it has no more work to
			// do.
			return reconcile.Result{RequeueAfter: r.Provisioner.DeprovisionRequeueDelay}, nil
		}

		// Remove finalizer to allow deletion
		reqLogger.Info("cleanup is complete, removing finalizer")
		host.ObjectMeta.Finalizers = utils.FilterStringFromList(
			host.ObjectMeta.Finalizers, metalkubev1alpha1.BareMetalHostFinalizer)
		if err := r.Client.Update(context.TODO(), host); err != nil {
			reqLogger.Error(err, "failed to remove finalizer")
			return reconcile.Result{}, err
		}
		return reconcile.Result{}, nil // done
	}

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
	bmcCredsSecret := &corev1.Secret{}
	err = r.Client.Get(context.TODO(), secretKey, bmcCredsSecret)
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

		if dirty, err = r.Provisioner.ValidateManagementAccess(host); err != nil {
			reqLogger.Error(err, "failed to validate BMC access")
			return reconcile.Result{}, err
		}

		if dirty {
			reqLogger.Info("saving status after validating management access")
			if err := r.saveStatus(host); err != nil {
				reqLogger.Error(err, "failed to save provisioning host status")
				return reconcile.Result{}, err
			}
			// Not returning here because it seems we can update the
			// status multiple times safely?
		}

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

	// Set the hardware profile name.
	if host.Status.HardwareDetails == nil {
		if dirty, err = r.Provisioner.InspectHardware(host); err != nil {
			reqLogger.Error(err, "failed to inspect hardware")
			return reconcile.Result{}, err
		}
		if dirty {
			reqLogger.Info("saving host after inspecting hardware")
			if err := r.Client.Update(context.TODO(), host); err != nil {
				reqLogger.Error(err, "failed to save host during inspection")
				return reconcile.Result{}, err
			}
			if host.Status.HardwareDetails != nil {
				reqLogger.Info("saving hardware details")
				if err := r.saveStatus(host); err != nil {
					reqLogger.Error(err, "failed to save hardware details during inspection")
					return reconcile.Result{}, err
				}
			}
			return reconcile.Result{Requeue: true}, err
		}
	}

	// FIXME(dhellmann): Insert logic to match hardware profiles here.
	hardwareProfile := "unknown"
	if host.SetHardwareProfile(hardwareProfile) {
		reqLogger.Info("updating hardware profile", "profile", hardwareProfile)
		if err := r.Client.Update(context.TODO(), host); err != nil {
			reqLogger.Error(err, "failed to update hardware profile")
			return reconcile.Result{}, err
		}
		return reconcile.Result{Requeue: true}, nil
	}

	// Update the operational status of the host.
	newOpStatus := metalkubev1alpha1.OperationalStatusOffline
	if host.Spec.Online {
		reqLogger.Info("ensuring host is powered on")
		dirty, err = r.Provisioner.PowerOn(host)
		if err != nil {
			reqLogger.Error(err, "failed to power on the host")
			return reconcile.Result{}, err
		}
		newOpStatus = metalkubev1alpha1.OperationalStatusOnline
	} else {
		reqLogger.Info("ensuring host is powered off")
		dirty, err = r.Provisioner.PowerOff(host)
		if err != nil {
			reqLogger.Error(err, "failed to power off the host")
			return reconcile.Result{}, err
		}
	}
	if dirty {
		if err := r.saveStatus(host); err != nil {
			reqLogger.Error(err, "failed to save power status")
			return reconcile.Result{}, err
		}
	}
	if host.SetOperationalStatus(newOpStatus) {
		reqLogger.Info(
			"setting operational status",
			"newStatus", newOpStatus,
		)
		if err := r.Client.Update(context.TODO(), host); err != nil {
			reqLogger.Error(err, "failed to update operational status")
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
	return r.Client.Status().Update(context.TODO(), host)
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
		if err := r.Client.Update(context.TODO(), host); err != nil {
			reqLogger.Error(err, "failed to update operational status")
			return err
		}
	}

	return nil
}

func (r *ReconcileBareMetalHost) setBMCCredentialsSecretOwner(request reconcile.Request, host *metalkubev1alpha1.BareMetalHost, secret *corev1.Secret) (err error) {
	reqLogger := log.WithValues("Request.Namespace",
		request.Namespace, "Request.Name", request.Name)
	if metav1.IsControlledBy(secret, host) {
		return nil
	}
	reqLogger.Info("updating owner of secret")
	err = controllerutil.SetControllerReference(host, secret, r.Scheme)
	if err != nil {
		reqLogger.Error(err, "failed to set owner")
		return err
	}
	err = r.Client.Update(context.TODO(), secret)
	if err != nil {
		reqLogger.Error(err, "failed to save owner")
		return err
	}
	return nil
}
