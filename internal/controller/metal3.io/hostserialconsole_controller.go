/*
Copyright 2026 The Metal3 Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	"errors"
	"fmt"
	"net"
	"reflect"
	"slices"
	"time"

	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	"github.com/metal3-io/baremetal-operator/pkg/provisioner"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	serialConsolePort            int32 = 2023
	serialConsoleRetryDelay            = 10 * time.Second
	serialConsolePollDelay             = time.Minute
	serialConsoleEndpointManager       = "hostserialconsole-controller.metal3.io"
)

var errConsoleAddressFamilyChanged = errors.New("console backend no longer has an address in the Service IP family")

// HostSerialConsoleReconciler exposes requested consoles through ClusterIP
// Services. Only this controller issues console operations; BMH remains the
// authority for registration, adoption and host teardown.
type HostSerialConsoleReconciler struct {
	client.Client
	APIReader          client.Reader
	ProvisionerFactory provisioner.Factory
	// LookupIPAddr defaults to the system resolver. Tests can supply DNS
	// answers without depending on the network or changing global state.
	LookupIPAddr func(context.Context, string) ([]net.IPAddr, error)
}

// +kubebuilder:rbac:groups=metal3.io,resources=hostserialconsoles,verbs=get;list;watch;update;patch;delete
// +kubebuilder:rbac:groups=metal3.io,resources=hostserialconsoles/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=metal3.io,resources=hostserialconsoles/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=discovery.k8s.io,resources=endpointslices,verbs=get;list;watch;create;update;patch;delete

// Reconcile manages the lifetime of one shared backend console.
func (r *HostSerialConsoleReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	console := &metal3api.HostSerialConsole{}
	if err := r.Get(ctx, req.NamespacedName, console); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	host := &metal3api.BareMetalHost{}
	hostErr := r.Get(ctx, req.NamespacedName, host)
	if hostErr != nil && !k8serrors.IsNotFound(hostErr) {
		return ctrl.Result{}, hostErr
	}
	missingHost := k8serrors.IsNotFound(hostErr)

	// Never let a request follow a different host recreated under the same name.
	if !missingHost {
		for _, owner := range console.OwnerReferences {
			if owner.Kind == "BareMetalHost" && owner.UID != host.UID {
				missingHost = true
			}
		}
	}
	if !console.DeletionTimestamp.IsZero() {
		return r.closeConsole(ctx, console, host, missingHost)
	}
	if missingHost {
		if err := r.removeConsoleExposure(ctx, console); err != nil {
			return ctrl.Result{}, err
		}
		if console.Status.ProvisionerID != "" || len(console.OwnerReferences) != 0 {
			return ctrl.Result{}, r.Delete(ctx, console)
		}
		return r.setConsoleCondition(ctx, console, "HostNotFound", "Waiting for the corresponding BareMetalHost", metav1.ConditionFalse)
	}

	workloadChanged := console.Status.HostDeprovisioningGeneration != nil &&
		*console.Status.HostDeprovisioningGeneration != host.Status.Provisioning.DeprovisioningGeneration
	if workloadChanged || !host.DeletionTimestamp.IsZero() || hasDetachedAnnotation(host) || host.Status.Provisioning.State == metal3api.StateDeprovisioning || host.Status.Provisioning.State == metal3api.StateDeleting || host.Status.Provisioning.State == metal3api.StatePoweringOffBeforeDelete {
		// A console request expires when its host leaves service. Do not reopen
		// it automatically for the next tenant or provisioned workload.
		return ctrl.Result{}, r.Delete(ctx, console)
	}
	if _, paused := host.Annotations[metal3api.PausedAnnotation]; paused {
		return r.setConsoleCondition(ctx, console, "Paused", "BareMetalHost reconciliation is paused; the last observed endpoint is retained", metav1.ConditionUnknown)
	}
	if !metav1.IsControlledBy(console, host) {
		if err := controllerutil.SetControllerReference(host, console, r.Scheme()); err != nil {
			return ctrl.Result{}, err
		}
		if err := r.Update(ctx, console); err != nil {
			return ctrl.Result{}, err
		}
	}
	if console.Status.ProvisionerID != "" && console.Status.ProvisionerID != host.Status.Provisioning.ID {
		// Clean the old node first, even when BMH has already registered a new one.
		return r.closeConsole(ctx, console, host, false)
	}
	if host.Status.Provisioning.ID == "" || !consoleHostReady(host) {
		if err := r.removeConsoleExposure(ctx, console); err != nil {
			return ctrl.Result{}, err
		}
		return r.setConsoleCondition(ctx, console, "HostNotReady", "Waiting for a registered host in a stable state", metav1.ConditionFalse)
	}
	// Ordinary node reads do not require BMC credentials. Even Secret outages
	// must not prevent closing a previously opened console.
	prov, err := r.ProvisionerFactory.NewProvisioner(ctx, provisioner.BuildHostDataNoBMC(*host), func(string, string) {})
	if err != nil {
		return r.consoleFailure(ctx, console, err)
	}
	consoleProv, supported := prov.(provisioner.SerialConsoleProvisioner)
	if !supported {
		return r.setConsoleCondition(ctx, console, "ConsoleUnsupported", "The selected provisioner does not support serial consoles", metav1.ConditionFalse)
	}
	// Unsupported provisioners must never acquire backend cleanup work. For
	// capable provisioners, persist the finalizer before any backend operation.
	if controllerutil.AddFinalizer(console, metal3api.HostSerialConsoleFinalizer) {
		if err = r.Update(ctx, console); err != nil {
			return ctrl.Result{}, err
		}
	}
	if console.Status.ProvisionerID == "" || console.Status.HostDeprovisioningGeneration == nil {
		// Bind cleanup to this node before either adopting an already-enabled
		// console or starting a new one. The host may disappear at any time.
		console.Status.ProvisionerID = host.Status.Provisioning.ID
		if console.Status.HostDeprovisioningGeneration == nil {
			console.Status.HostDeprovisioningGeneration = ptr.To(host.Status.Provisioning.DeprovisioningGeneration)
		}
		if err = r.Status().Update(ctx, console); err != nil {
			return ctrl.Result{}, err
		}
	}
	state, err := consoleProv.GetSerialConsoleState(ctx)
	if err != nil {
		return r.consoleFailure(ctx, console, err)
	}
	if state.Busy {
		return ctrl.Result{RequeueAfter: serialConsoleRetryDelay}, nil
	}
	if !state.Enabled {
		if err = r.removeConsoleExposure(ctx, console); err != nil {
			return ctrl.Result{}, err
		}
		if console.Status.BackendEndpoint != nil || console.Status.BackendID != "" {
			console.Status.BackendEndpoint = nil
			console.Status.BackendID = ""
			if err = r.Status().Update(ctx, console); err != nil {
				return ctrl.Result{}, err
			}
		}
		if _, err = r.setConsoleCondition(ctx, console, "Enabling", "Enabling the backend serial console", metav1.ConditionFalse); err != nil {
			return ctrl.Result{}, err
		}
		secret := &corev1.Secret{}
		reader := r.APIReader
		if reader == nil {
			reader = r.Client
		}
		if err = reader.Get(ctx, host.CredentialsKey(), secret); err != nil {
			return r.consoleFailure(ctx, console, err)
		}
		creds := credentialsFromSecret(secret)
		if (creds.SerialConsoleUsername == "") != (creds.SerialConsolePassword == "") {
			return r.consoleFailure(ctx, console, errors.New("serialConsoleUsername and serialConsolePassword must both be supplied"))
		}
		prov, err = r.ProvisionerFactory.NewProvisioner(ctx, provisioner.BuildHostData(*host, *creds), func(string, string) {})
		if err != nil {
			return r.consoleFailure(ctx, console, err)
		}
		consoleProv, supported = prov.(provisioner.SerialConsoleProvisioner)
		if !supported {
			return r.consoleFailure(ctx, console, errors.New("provisioner no longer supports serial consoles"))
		}
		if err = consoleProv.SetSerialConsole(ctx, true); err != nil {
			return r.consoleFailure(ctx, console, err)
		}
		return ctrl.Result{RequeueAfter: serialConsoleRetryDelay}, nil
	}
	if console.Status.BackendEndpoint == nil || console.Status.BackendID != state.ID {
		if err = r.removeConsoleExposure(ctx, console); err != nil {
			return ctrl.Result{}, err
		}
		endpoint, endpointErr := consoleProv.GetSerialConsole(ctx)
		if endpointErr != nil {
			return r.consoleFailure(ctx, console, endpointErr)
		}
		if endpoint == nil {
			return r.consoleFailure(ctx, console, provisioner.ErrNotReady)
		}
		console.Status.BackendEndpoint = endpoint
		console.Status.BackendID = state.ID
		console.Status.ProvisionerID = host.Status.Provisioning.ID
		// Checkpoint before publishing the Service, and before any retry can call
		// console show again. Subsequent reconciles only read ordinary node state.
		if err = r.Status().Update(ctx, console); err != nil {
			return ctrl.Result{}, err
		}
	}
	if err = r.exposeConsole(ctx, console); err != nil {
		return r.consoleFailure(ctx, console, err)
	}
	return r.setConsoleCondition(ctx, console, "ConsoleAvailable", "Connect to the console Service using a TCP client", metav1.ConditionTrue)
}

func consoleHostReady(host *metal3api.BareMetalHost) bool {
	switch host.Status.Provisioning.State {
	case metal3api.StateProvisioned, metal3api.StateExternallyProvisioned, metal3api.StateAvailable, metal3api.StateReady:
		return true
	default:
		return false
	}
}

func (r *HostSerialConsoleReconciler) consoleFailure(ctx context.Context, console *metal3api.HostSerialConsole, err error) (ctrl.Result, error) {
	if errors.Is(err, provisioner.ErrNodeIsBusy) {
		return ctrl.Result{RequeueAfter: serialConsoleRetryDelay}, nil
	}
	if errors.Is(err, provisioner.ErrNeedsRegistration) {
		// A confirmed missing node invalidates routing; an API outage does not.
		if cleanupErr := r.removeConsoleExposure(ctx, console); cleanupErr != nil {
			return ctrl.Result{}, cleanupErr
		}
		console.Status.BackendEndpoint = nil
		console.Status.BackendID = ""
	}
	if console.Status.ServiceName != "" {
		return r.setConsoleCondition(ctx, console, "ConsoleObservationFailed", err.Error(), metav1.ConditionUnknown)
	}
	return r.setConsoleCondition(ctx, console, "ConsoleUnavailable", err.Error(), metav1.ConditionFalse)
}

func (r *HostSerialConsoleReconciler) setConsoleCondition(ctx context.Context, console *metal3api.HostSerialConsole, reason, message string, condition metav1.ConditionStatus) (ctrl.Result, error) {
	delay := serialConsoleRetryDelay
	if condition == metav1.ConditionTrue {
		delay = serialConsolePollDelay
	}
	meta.SetStatusCondition(&console.Status.Conditions, metav1.Condition{Type: "Ready", Status: condition, Reason: reason, Message: message, ObservedGeneration: console.Generation})
	// Compare against persisted status, not the in-memory object: callers may
	// already have changed the Service URL or cleared a backend checkpoint.
	// An uncached read also avoids replacing status using a resourceVersion
	// from before a slow backend operation.
	reader := r.APIReader
	if reader == nil {
		reader = r.Client
	}
	current := &metal3api.HostSerialConsole{}
	if err := reader.Get(ctx, client.ObjectKeyFromObject(console), current); err != nil {
		return ctrl.Result{}, err
	}
	old := current.DeepCopy()
	current.Status = *console.Status.DeepCopy()
	if !reflect.DeepEqual(old.Status, current.Status) {
		if err := r.Status().Patch(ctx, current, client.MergeFrom(old)); err != nil {
			return ctrl.Result{}, err
		}
	}
	*console = *current
	return ctrl.Result{RequeueAfter: delay}, nil
}

func (r *HostSerialConsoleReconciler) closeConsole(ctx context.Context, console *metal3api.HostSerialConsole, host *metal3api.BareMetalHost, missingHost bool) (ctrl.Result, error) {
	// Attempt both forms of revocation independently. An ownership or API
	// error withdrawing routing must not leave the backend console enabled.
	routingErr := r.removeConsoleExposure(ctx, console)
	closed, backendErr := r.disableConsoleBackend(ctx, console, host, missingHost)
	if cleanupErr := errors.Join(routingErr, backendErr); cleanupErr != nil {
		result, statusErr := r.setConsoleCondition(ctx, console, "ConsoleUnavailable", cleanupErr.Error(), metav1.ConditionFalse)
		if routingErr != nil {
			return result, errors.Join(cleanupErr, statusErr)
		}
		return result, statusErr
	}
	if !closed {
		return r.setConsoleCondition(ctx, console, "Disabling", "Waiting for the backend console to stop", metav1.ConditionFalse)
	}
	if console.DeletionTimestamp.IsZero() {
		// Backend replacement: the request remains, but its old checkpoint is gone.
		console.Status.ProvisionerID, console.Status.BackendID = "", ""
		console.Status.BackendEndpoint = nil
		if err := r.Status().Update(ctx, console); err != nil {
			return ctrl.Result{}, err
		}
		return r.setConsoleCondition(ctx, console, "HostChanged", "The backend node changed; establishing a new console", metav1.ConditionFalse)
	}
	if controllerutil.RemoveFinalizer(console, metal3api.HostSerialConsoleFinalizer) {
		return ctrl.Result{}, r.Update(ctx, console)
	}
	return ctrl.Result{}, nil
}

// disableConsoleBackend reports true only after disable completion or removal
// of the backend node. It does not depend on routing cleanup succeeding.
func (r *HostSerialConsoleReconciler) disableConsoleBackend(ctx context.Context, console *metal3api.HostSerialConsole, host *metal3api.BareMetalHost, missingHost bool) (bool, error) {
	nodeID := console.Status.ProvisionerID
	if nodeID == "" && !missingHost {
		nodeID = host.Status.Provisioning.ID
	}
	if nodeID == "" || !controllerutil.ContainsFinalizer(console, metal3api.HostSerialConsoleFinalizer) {
		return true, nil
	}
	data := provisioner.HostData{ObjectMeta: console.ObjectMeta, ProvisionerID: nodeID}
	prov, err := r.ProvisionerFactory.NewProvisioner(ctx, data, func(string, string) {})
	if err != nil {
		return false, err
	}
	consoleProv, supported := prov.(provisioner.SerialConsoleProvisioner)
	if !supported {
		// Do not silently release an old console if the operator has switched
		// from a console-capable provisioner to an incompatible backend.
		return false, errors.New("current provisioner cannot clean up a previously managed console")
	}
	state, err := consoleProv.GetSerialConsoleState(ctx)
	if errors.Is(err, provisioner.ErrNeedsRegistration) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	if state.Busy {
		return false, nil
	}
	if !state.Enabled {
		return true, nil
	}
	err = consoleProv.SetSerialConsole(ctx, false)
	if errors.Is(err, provisioner.ErrNeedsRegistration) {
		return true, nil
	}
	if errors.Is(err, provisioner.ErrNodeIsBusy) {
		return false, nil
	}
	return false, err
}

func consoleServiceName(console *metal3api.HostSerialConsole) string {
	// UID guarantees a valid, short Service name even for long BMH names.
	return "serial-console-" + string(console.UID)
}

func (r *HostSerialConsoleReconciler) removeConsoleExposure(ctx context.Context, console *metal3api.HostSerialConsole) error {
	console.Status.ConsoleURL, console.Status.ServiceName = "", ""
	key := types.NamespacedName{Namespace: console.Namespace, Name: consoleServiceName(console)}
	var cleanupErrors []error
	for _, obj := range []client.Object{&discoveryv1.EndpointSlice{}, &corev1.Service{}} {
		if err := r.Get(ctx, key, obj); err != nil {
			if k8serrors.IsNotFound(err) {
				continue
			}
			cleanupErrors = append(cleanupErrors, err)
			continue
		}
		if !metav1.IsControlledBy(obj, console) {
			cleanupErrors = append(cleanupErrors, fmt.Errorf("refusing to delete unowned %T %s", obj, key))
			continue
		}
		if err := r.Delete(ctx, obj, client.Preconditions{UID: ptr.To(obj.GetUID())}); err != nil && !k8serrors.IsNotFound(err) {
			cleanupErrors = append(cleanupErrors, err)
		}
	}
	return errors.Join(cleanupErrors...)
}

func (r *HostSerialConsoleReconciler) exposeConsole(ctx context.Context, console *metal3api.HostSerialConsole) error {
	endpoint := console.Status.BackendEndpoint
	lookup := r.LookupIPAddr
	if lookup == nil {
		lookup = net.DefaultResolver.LookupIPAddr
	}
	addresses, err := lookup(ctx, endpoint.Host)
	if err != nil {
		return fmt.Errorf("resolve console endpoint: %w", err)
	}
	byFamily := make(map[corev1.IPFamily][]string)
	for _, address := range addresses {
		if !address.IP.IsGlobalUnicast() || address.IP.IsLoopback() {
			continue
		}
		family := corev1.IPv6Protocol
		if address.IP.To4() != nil {
			family = corev1.IPv4Protocol
		}
		byFamily[family] = append(byFamily[family], address.IP.String())
	}
	if len(byFamily) == 0 {
		return errors.New("console endpoint has no routable unicast addresses")
	}
	name := consoleServiceName(console)
	svc := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: console.Namespace}}
	if _, err = controllerutil.CreateOrUpdate(ctx, r.Client, svc, func() error {
		if !svc.CreationTimestamp.IsZero() && !metav1.IsControlledBy(svc, console) {
			return errors.New("refusing to adopt an unowned console Service")
		}
		if len(svc.Spec.IPFamilies) > 0 && len(byFamily[svc.Spec.IPFamilies[0]]) == 0 {
			return errConsoleAddressFamilyChanged
		}
		if err = controllerutil.SetControllerReference(console, svc, r.Scheme()); err != nil {
			return err
		}
		svc.Spec.Type = corev1.ServiceTypeClusterIP
		svc.Spec.Selector = nil
		svc.Spec.ExternalName = ""
		svc.Spec.ExternalIPs = nil
		svc.Spec.LoadBalancerIP = ""
		svc.Spec.LoadBalancerSourceRanges = nil
		// When DNS offers both families, let the API server choose the cluster's
		// default family. Preserve an existing Service's family and ClusterIP.
		// A single-family backend must explicitly request that family.
		if len(svc.Spec.IPFamilies) == 0 && len(byFamily) == 1 {
			for family := range byFamily {
				svc.Spec.IPFamilies = []corev1.IPFamily{family}
			}
		}
		svc.Spec.IPFamilyPolicy = ptr.To(corev1.IPFamilyPolicySingleStack)
		svc.Spec.Ports = []corev1.ServicePort{{Name: "console", Protocol: corev1.ProtocolTCP, Port: serialConsolePort, TargetPort: intstr.FromInt32(endpoint.Port)}}
		return nil
	}); err != nil {
		if errors.Is(err, errConsoleAddressFamilyChanged) {
			if cleanupErr := r.removeConsoleExposure(ctx, console); cleanupErr != nil {
				return cleanupErr
			}
			return fmt.Errorf("waiting for console Service address-family replacement: %w", provisioner.ErrNotReady)
		}
		return err
	}
	if len(svc.Spec.IPFamilies) == 0 {
		return errors.New("API server did not allocate a console Service IP family")
	}
	family := svc.Spec.IPFamilies[0]
	if len(byFamily[family]) == 0 {
		return errors.New("allocated console Service IP family has no backend address")
	}
	slice := &discoveryv1.EndpointSlice{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: console.Namespace}}
	if _, err = controllerutil.CreateOrUpdate(ctx, r.Client, slice, func() error {
		if !slice.CreationTimestamp.IsZero() && !metav1.IsControlledBy(slice, console) {
			return errors.New("refusing to adopt an unowned console EndpointSlice")
		}
		if err = controllerutil.SetControllerReference(console, slice, r.Scheme()); err != nil {
			return err
		}
		slice.Labels = map[string]string{discoveryv1.LabelServiceName: name, discoveryv1.LabelManagedBy: serialConsoleEndpointManager}
		slice.AddressType = discoveryv1.AddressType(family)
		slice.Ports = []discoveryv1.EndpointPort{{Name: ptr.To("console"), Protocol: ptr.To(corev1.ProtocolTCP), Port: ptr.To(endpoint.Port)}}
		address := byFamily[family][0]
		if len(slice.Endpoints) == 1 && len(slice.Endpoints[0].Addresses) == 1 && slices.Contains(byFamily[family], slice.Endpoints[0].Addresses[0]) {
			// Round-robin DNS ordering alone must not move a live console.
			address = slice.Endpoints[0].Addresses[0]
		}
		slice.Endpoints = []discoveryv1.Endpoint{{Addresses: []string{address}, Conditions: discoveryv1.EndpointConditions{Ready: ptr.To(true)}}}
		return nil
	}); err != nil {
		return err
	}
	console.Status.ServiceName = name
	console.Status.ConsoleURL = fmt.Sprintf("tcp://%s.%s.svc:%d", name, console.Namespace, serialConsolePort)
	return nil
}

// SetupWithManager watches host lifecycle changes and repairs deleted routing
// resources without accessing Ironic's disruptive endpoint API again.
func (r *HostSerialConsoleReconciler) SetupWithManager(mgr ctrl.Manager, maxConcurrentReconcile int) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&metal3api.HostSerialConsole{}, builder.WithPredicates(predicate.Funcs{UpdateFunc: serialConsoleRequestChanged})).
		Owns(&corev1.Service{}).Owns(&discoveryv1.EndpointSlice{}).
		Watches(&metal3api.BareMetalHost{}, handler.EnqueueRequestsFromMapFunc(func(_ context.Context, obj client.Object) []reconcile.Request {
			return []reconcile.Request{{NamespacedName: client.ObjectKeyFromObject(obj)}}
		})).WithOptions(controller.Options{MaxConcurrentReconciles: maxConcurrentReconcile}).Complete(r)
}

func serialConsoleRequestChanged(e event.UpdateEvent) bool {
	// Retry backend errors on the requested delay, not immediately on every
	// status write. Deletion and finalizer changes must still wake cleanup.
	return e.ObjectOld.GetGeneration() != e.ObjectNew.GetGeneration() ||
		!e.ObjectOld.GetDeletionTimestamp().Equal(e.ObjectNew.GetDeletionTimestamp()) ||
		!reflect.DeepEqual(e.ObjectOld.GetFinalizers(), e.ObjectNew.GetFinalizers())
}
