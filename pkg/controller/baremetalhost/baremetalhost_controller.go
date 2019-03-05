package baremetalhost

import (
	"context"
	"time"

	"github.com/pkg/errors"

	metalkubev1alpha1 "github.com/metalkube/baremetal-operator/pkg/apis/metalkube/v1alpha1"
	"github.com/metalkube/baremetal-operator/pkg/bmc"
	"github.com/metalkube/baremetal-operator/pkg/provisioner"
	"github.com/metalkube/baremetal-operator/pkg/provisioner/ironic"
	"github.com/metalkube/baremetal-operator/pkg/utils"

	corev1 "k8s.io/api/core/v1"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
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

// FIXME(dhellmann): These values should probably come from
// configuration settings and something that can tell the IP address
// of the ironic server.
const (
	instanceImageSource   = "http://172.22.0.1/images/redhat-coreos-maipo-latest.qcow2"
	instanceImageChecksum = "97830b21ed272a3d854615beb54cf004"
	ironicEndpoint        = "http://localhost:6385/v1/"
)

// Add creates a new BareMetalHost Controller and adds it to the
// Manager. The Manager will set fields on the Controller and Start it
// when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileBareMetalHost{
		client: mgr.GetClient(),
		scheme: mgr.GetScheme(),
		provisionerFactory: ironic.NewFactory(
			ironicEndpoint,
			instanceImageSource,
			instanceImageChecksum,
		),
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
	client client.Client
	scheme *runtime.Scheme
	// Provisioner handles interacting with the provisioning system.
	provisionerFactory provisioner.ProvisionerFactory
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

	var dirty bool               // have we updated the host status but not saved it?
	var retryDelay time.Duration // how long to wait before trying reconcile again

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
			return reconcile.Result{}, errors.Wrap(err, "failed to add finalizer")
		}
		return reconcile.Result{Requeue: true}, nil
	}

	// If we do not have all of the needed BMC credentials, set our
	// operational status to indicate missing information.
	if host.Spec.BMC.Address == "" {
		reqLogger.Info(bmc.MissingAddressMsg)
		err := r.setErrorCondition(request, host, bmc.MissingAddressMsg)
		// Without the BMC address there's no more we can do, so we're
		// going to return the emtpy Result anyway, and don't need to
		// check err.
		return reconcile.Result{}, errors.Wrap(err, "failed to set error condition")
	}

	// Load the secret containing the credentials for talking to the
	// BMC.
	if host.Spec.BMC.CredentialsName == "" {
		// We have no name to use to load the secrets.
		reqLogger.Info(bmc.MissingCredentialsMsg)
		err := r.setErrorCondition(request, host, bmc.MissingCredentialsMsg)
		return reconcile.Result{}, errors.Wrap(err, "failed to set error condition")
	}
	secretKey := host.CredentialsKey()
	bmcCredsSecret := &corev1.Secret{}
	err = r.client.Get(context.TODO(), secretKey, bmcCredsSecret)
	if err != nil {
		return reconcile.Result{}, errors.Wrap(err,
			"failed to fetch BMC credentials from secret reference")
	}
	bmcCreds := bmc.Credentials{
		Username: string(bmcCredsSecret.Data["username"]),
		Password: string(bmcCredsSecret.Data["password"]),
	}

	// Verify that the secret contains the expected info.
	validCreds, reason := bmcCreds.AreValid()
	if !validCreds {
		reqLogger.Info("invalid BMC Credentials", "reason", reason)
		err := r.setErrorCondition(request, host, reason)
		return reconcile.Result{}, errors.Wrap(err, "failed to set error condition")
	}

	// Past this point we may need a provisioner, so create one.
	prov, err := r.provisionerFactory.New(host, bmcCreds)
	if err != nil {
		return reconcile.Result{}, errors.Wrap(err, "failed to create provisioner")
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

		if dirty, retryDelay, err = prov.Deprovision(); err != nil {
			return reconcile.Result{}, errors.Wrap(err, "failed to deprovision")
		}
		if dirty {
			if err := r.saveStatus(host); err != nil {
				return reconcile.Result{}, errors.Wrap(err,
					"failed to clear host status on deprovision")
			}
			// Go back into the queue and wait for the Deprovision() method
			// to return false, indicating that it has no more work to
			// do.
			return reconcile.Result{RequeueAfter: retryDelay}, nil
		}

		// Remove finalizer to allow deletion
		reqLogger.Info("cleanup is complete, removing finalizer")
		host.ObjectMeta.Finalizers = utils.FilterStringFromList(
			host.ObjectMeta.Finalizers, metalkubev1alpha1.BareMetalHostFinalizer)
		if err := r.client.Update(context.TODO(), host); err != nil {
			return reconcile.Result{}, errors.Wrap(err, "failed to remove finalizer")
		}
		return reconcile.Result{}, nil // done
	}

	// Make sure the secret has the correct owner.
	err = r.setBMCCredentialsSecretOwner(request, host, bmcCredsSecret)
	if err != nil {
		// FIXME: Set error condition?
		return reconcile.Result{}, errors.Wrap(err,
			"failed to update owner of credentials secret")
	}

	// Clear any error on the host so we can recalculate it in one of
	// the next phases. This may make the host dirty, so remember that
	// in case nothing else does.
	dirty = host.ClearError()

	// Update the success info for the credentails.
	if host.CredentialsNeedValidation(*bmcCredsSecret) {

		changed, err := prov.ValidateManagementAccess()
		if err != nil {
			return reconcile.Result{}, errors.Wrap(err, "failed to validate BMC access")
		}

		// We may have changed the host during validation or when we
		// cleared the error state above. In either case, save now.
		reqLogger.Info("after validation", "changed", changed, "dirty", dirty)
		if changed || dirty {
			reqLogger.Info("saving status after validating management access")
			if err := r.saveStatus(host); err != nil {
				return reconcile.Result{}, errors.Wrap(err,
					"failed to save provisioning host status")
			}
			// If we don't have an error status, we just need to wait
			// before checking with the provisioner again. If the
			// provisioner set an error, fall through to the next
			// stanza where we stop reconciling.
			if !host.HasError() {
				reqLogger.Info("waiting before checking provisioning status again")
				return reconcile.Result{RequeueAfter: time.Second * 5}, nil
			}
		}

		if host.HasError() {
			reqLogger.Info("host failed credential validation, stopping")
			return reconcile.Result{}, nil
		}

		// Reaching this point means the credentials are valid and
		// worked, so record that in the status block.
		reqLogger.Info("updating credentials success status fields")
		host.UpdateGoodCredentials(*bmcCredsSecret)
		if err := r.saveStatus(host); err != nil {
			return reconcile.Result{}, errors.Wrap(err,
				"failed to update credentials success status fields")
		}
		return reconcile.Result{Requeue: true}, nil
	}

	// Set the hardware profile name.
	if host.Status.HardwareDetails == nil {
		changed, err := prov.InspectHardware()
		if err != nil {
			return reconcile.Result{}, errors.Wrap(err, "hardware inspection failed")
		}
		if changed || dirty {
			reqLogger.Info("saving host after inspecting hardware")
			if err := r.client.Update(context.TODO(), host); err != nil {
				return reconcile.Result{}, errors.Wrap(err,
					"failed to save host after inspection")
			}
			if host.Status.HardwareDetails != nil {
				reqLogger.Info("saving hardware details")
				if err := r.saveStatus(host); err != nil {
					return reconcile.Result{}, errors.Wrap(err,
						"failed to save hardware details after inspection")
				}
			}
			return reconcile.Result{Requeue: true}, nil
		}
	}

	// FIXME(dhellmann): Insert logic to match hardware profiles here.
	hardwareProfile := "unknown"
	if host.SetHardwareProfile(hardwareProfile) {
		reqLogger.Info("updating hardware profile", "profile", hardwareProfile)
		if err := r.client.Update(context.TODO(), host); err != nil {
			return reconcile.Result{}, errors.Wrap(err,
				"failed to save host after updating hardware profile")
		}
		return reconcile.Result{Requeue: true}, nil
	}

	// Update the operational status of the host.
	newOpStatus := metalkubev1alpha1.OperationalStatusOffline
	if host.Spec.Online {
		reqLogger.Info("ensuring host is powered on")
		changed, err := prov.PowerOn()
		if err != nil {
			return reconcile.Result{}, errors.Wrap(err, "failed to power on the host")
		}
		newOpStatus = metalkubev1alpha1.OperationalStatusOnline
		dirty = dirty || changed
	} else {
		reqLogger.Info("ensuring host is powered off")
		changed, err := prov.PowerOff()
		if err != nil {
			return reconcile.Result{}, errors.Wrap(err, "failed to power off the host")
		}
		dirty = dirty || changed
	}
	if dirty {
		if err := r.saveStatus(host); err != nil {
			return reconcile.Result{}, errors.Wrap(err, "failed to save power status")
		}
	}
	if host.SetOperationalStatus(newOpStatus) {
		reqLogger.Info(
			"setting operational status",
			"newStatus", newOpStatus,
		)
		if err := r.client.Update(context.TODO(), host); err != nil {
			return reconcile.Result{}, errors.Wrap(err, "failed to update operational status")
		}
		return reconcile.Result{Requeue: true}, nil
	}

	// If we reach this point we haven't encountered any issues
	// communicating with the host, so ensure the error message field
	// is cleared.
	if host.ClearError() {
		reqLogger.Info("clearing error message")
		if err := r.saveStatus(host); err != nil {
			return reconcile.Result{}, errors.Wrap(err, "failed to clear error message")
		}
	}

	// If we have nothing else to do and there is no LastUpdated
	// timestamp set, set one.
	if host.Status.LastUpdated.IsZero() {
		reqLogger.Info("initializing status")
		if err := r.saveStatus(host); err != nil {
			return reconcile.Result{}, errors.Wrap(err, "failed to initialize status block")
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
			return errors.Wrap(err, "failed to update error message")
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
