package baremetalhost

import (
	"context"
	"flag"

	"github.com/pkg/errors"

	metalkubev1alpha1 "github.com/metalkube/baremetal-operator/pkg/apis/metalkube/v1alpha1"
	"github.com/metalkube/baremetal-operator/pkg/bmc"
	"github.com/metalkube/baremetal-operator/pkg/provisioner"
	"github.com/metalkube/baremetal-operator/pkg/provisioner/fixture"
	"github.com/metalkube/baremetal-operator/pkg/provisioner/ironic"
	"github.com/metalkube/baremetal-operator/pkg/utils"

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

var runInTestMode bool

func init() {
	flag.BoolVar(&runInTestMode, "test-mode", false, "disable ironic communication")
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
	if runInTestMode {
		log.Info("USING TEST MODE")
		provisionerFactory = fixture.New
	} else {
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
	client             client.Client
	scheme             *runtime.Scheme
	provisionerFactory provisioner.Factory
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

	var dirty bool                    // have we updated the host status but not saved it?
	var provResult provisioner.Result // result of any provisioner call

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
	prov, err := r.provisionerFactory(host, *bmcCreds)
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

		if provResult, err = prov.Deprovision(true); err != nil {
			return reconcile.Result{}, errors.Wrap(err, "failed to deprovision")
		}
		if provResult.Dirty {
			if err := r.saveStatus(host); err != nil {
				return reconcile.Result{}, errors.Wrap(err,
					"failed to clear host status on deprovision")
			}
			// Go back into the queue and wait for the Deprovision() method
			// to return false, indicating that it has no more work to
			// do.
			return reconcile.Result{RequeueAfter: provResult.RequeueAfter}, nil
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

	// Test the credentials by connecting to the management controller.
	if host.CredentialsNeedValidation(*bmcCredsSecret) {

		provResult, err = prov.ValidateManagementAccess()
		if err != nil {
			return reconcile.Result{}, errors.Wrap(err, "failed to validate BMC access")
		}

		// We may have changed the host during validation or when we
		// cleared the error state above. In either case, save now.
		reqLogger.Info("after validation", "provResult", provResult)
		if provResult.Dirty {
			reqLogger.Info("saving status after validating management access")
			if err := r.saveStatus(host); err != nil {
				return reconcile.Result{}, errors.Wrap(err,
					"failed to save provisioning host status")
			}
			// We need to wait before checking with the provisioner
			// again.
			reqLogger.Info("host not ready", "delay", provResult.RequeueAfter)
			result := reconcile.Result{
				Requeue:      true,
				RequeueAfter: provResult.RequeueAfter,
			}
			return result, nil
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

	// Ensure we have the information about the hardware on the host.
	if host.Status.HardwareDetails == nil {
		provResult, err = prov.InspectHardware()
		if err != nil {
			return reconcile.Result{}, errors.Wrap(err, "hardware inspection failed")
		}
		if provResult.Dirty || dirty {
			reqLogger.Info("saving hardware details after inspecting hardware")
			if err := r.saveStatus(host); err != nil {
				return reconcile.Result{}, errors.Wrap(err,
					"failed to save hardware details after inspection")
			}
			res := reconcile.Result{
				Requeue:      true,
				RequeueAfter: provResult.RequeueAfter,
			}
			return res, nil
		}
	}

	// FIXME(dhellmann): Insert logic to match hardware profiles here.
	hardwareProfile := "unknown"
	if host.SetHardwareProfile(hardwareProfile) {
		reqLogger.Info("updating hardware profile", "profile", hardwareProfile)
		if err := r.saveStatus(host); err != nil {
			return reconcile.Result{}, errors.Wrap(err,
				"failed to save host after updating hardware profile")
		}
		return reconcile.Result{Requeue: true}, nil
	}

	// Start/continue provisioning if we need to.
	if host.NeedsProvisioning() {
		var userData string

		// FIXME(dhellmann): Maybe instead of loading this every time
		// through the loop we want to provide a callback for
		// Provision() to invoke when it actually needs the data.
		if host.Spec.UserData != nil {
			reqLogger.Info("fetching user data before provisioning")
			userDataSecret := &corev1.Secret{}
			key := types.NamespacedName{
				Name:      host.Spec.UserData.Name,
				Namespace: host.Spec.UserData.Namespace,
			}
			err = r.client.Get(context.TODO(), key, userDataSecret)
			if err != nil {
				return reconcile.Result{}, errors.Wrap(err,
					"failed to fetch user data from secret reference")
			}
			userData = string(userDataSecret.Data["userData"])
		}

		reqLogger.Info("provisioning")

		provResult, err = prov.Provision(userData)
		if err != nil {
			return reconcile.Result{}, errors.Wrap(err, "failed to provision")
		}
		if provResult.Dirty || dirty {
			if err := r.saveStatus(host); err != nil {
				return reconcile.Result{}, errors.Wrap(err,
					"failed to save host status after provisioning")
			}
			// Go back into the queue and wait for the Provision() method
			// to return false, indicating that it has no more work to
			// do.
			res := reconcile.Result{
				Requeue:      true,
				RequeueAfter: provResult.RequeueAfter,
			}
			return res, nil
		}
		if host.HasError() {
			reqLogger.Info("needs provisioning but has error")
			return reconcile.Result{}, nil
		}
	}

	// Start/continue deprovisioning if we need to.
	if host.NeedsDeprovisioning() {
		if provResult, err = prov.Deprovision(false); err != nil {
			return reconcile.Result{}, errors.Wrap(err, "failed to deprovision")
		}
		if provResult.Dirty {
			if err := r.saveStatus(host); err != nil {
				return reconcile.Result{}, errors.Wrap(err,
					"failed to clear host status on deprovision")
			}
			// Go back into the queue and wait for the Deprovision() method
			// to return false, indicating that it has no more work to
			// do.
			return reconcile.Result{RequeueAfter: provResult.RequeueAfter}, nil
		}
	}

	// If we reach this point we haven't encountered any issues
	// communicating with the host, so ensure the error message field
	// is cleared.
	if host.ClearError() {
		if err := r.saveStatus(host); err != nil {
			return reconcile.Result{}, errors.Wrap(err, "failed to clear error")
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

// Make sure the credentials for the management controller look
// right. This does not actually try to use the credentials.
func (r *ReconcileBareMetalHost) getValidBMCCredentials(request reconcile.Request, host *metalkubev1alpha1.BareMetalHost) (bmcCreds *bmc.Credentials, bmcCredsSecret *corev1.Secret, err error) {
	reqLogger := log.WithValues("Request.Namespace",
		request.Namespace, "Request.Name", request.Name)

	// If we do not have all of the needed BMC credentials, set our
	// operational status to indicate missing information.
	if host.Spec.BMC.Address == "" {
		reqLogger.Info(bmc.MissingAddressMsg)
		err := r.setErrorCondition(request, host, bmc.MissingAddressMsg)
		// Without the BMC address there's no more we can do, so we're
		// going to return the emtpy Result anyway, and don't need to
		// check err.
		return nil, nil, errors.Wrap(err, "failed to set error condition")
	}

	// Load the secret containing the credentials for talking to the
	// BMC.
	if host.Spec.BMC.CredentialsName == "" {
		// We have no name to use to load the secrets.
		reqLogger.Info(bmc.MissingCredentialsMsg)
		err := r.setErrorCondition(request, host, bmc.MissingCredentialsMsg)
		return nil, nil, errors.Wrap(err, "failed to set error condition")
	}
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
