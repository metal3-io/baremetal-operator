/*
Copyright 2025 The Metal3 Authors.

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

package hostclaim

import (
	"context"
	"crypto/rand"
	"errors"
	"math/big"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/go-logr/logr"
	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	"github.com/metal3-io/baremetal-operator/pkg/secretutils"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/apimachinery/pkg/types"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

type HostManager struct {
	client    client.Client
	HostClaim *metal3api.HostClaim
	Log       logr.Logger
	APIReader client.Reader
}

const (
	// PausedAnnotationValue is the value used to mark a BareMetalHost as paused by
	// a HostClaim.
	PausedAnnotationValue = "metal3.io/hostclaim"
	// rebootDomain is the domain of metal3 reboot annotations.
	rebootDomain = "reboot.metal3.io"
	// Requeueing after 0 is in fact not requeuing.
	TerminalReueueDelay time.Duration = 0
	// Standard delay when waiting for other to settle.
	HostClaimRequeueDelay = time.Second * 30
	// Small delay on conflict error.
	ConflictRequeueDelay = time.Millisecond * 100
	// Factor up to which the conflict requeue delay may be randomly increased.
	ConflictJitterFactor = 9.0
	// FailureDomainLabelName is a label name for FailureDomains.
	FailureDomainLabelName = "infrastructure.cluster.x-k8s.io/failure-domain"
	// HostClaimKind is the name of the kind.
	HostClaimKind = "HostClaim"
)

var (
	// An error used when there is no BMH satisfying the constraints.
	ErrNoAvailableBMH = errors.New("no available BareMetalHost")
	// An error raised when the secret to synchronize does not exists.
	ErrNoSecret = errors.New("no Secret found")
	// ErrNoBMH raised when the BareMetalHost is not found.
	ErrNoBMH = errors.New("no BareMetalHost")
)

func NewHostManager(client client.Client, log logr.Logger, claim *metal3api.HostClaim, apireader client.Reader) (*HostManager, error) {
	return &HostManager{
		client:    client,
		HostClaim: claim,
		Log:       log,
		APIReader: apireader,
	}, nil
}

// SetConditionHostToFalse sets Host condition status to False.
func (m *HostManager) SetConditionHostToFalse(
	t string,
	reason string,
	message string,
) {
	conditions.Set(m.HostClaim, metav1.Condition{Type: t, Status: metav1.ConditionFalse, Reason: reason, Message: message})
}

// SetConditionHostToTrue sets Host condition status to True.
func (m *HostManager) SetConditionHostToTrue(
	t string,
	reason string,
) {
	conditions.Set(m.HostClaim, metav1.Condition{Type: t, Status: metav1.ConditionTrue, Reason: reason, Message: ""})
}

// Associate associates a BareMetalHost to the HostClaim. It chooses a BareMetalHost in a namespace that accepts
// the current claim (through HostDeployPolicy) and that fulfills the constraints set in the claim.
// If no suitable host was found, returns a requeueAfterDelay error so that the controller can request a requeue
// after a delay.
// If a suitable host is found, update its consumer ref, to ensure it is booked. If an error
// prevents the update of the status of the claim at the end of the reconciliation logic, the choice logic will
// ensure that the BareMetalHost already marked is chosen.
func (m *HostManager) Associate(ctx context.Context) error {
	m.Log.Info("Associating host")

	// load and validate the config
	if m.HostClaim == nil {
		// Should have been picked earlier. Do not requeue
		m.Log.Info("No hostclaim in Associate")
		return nil
	}

	bmh, err := m.chooseBMH(ctx)
	if err != nil {
		if ok, _ := IsRequeueAfterError(err); !ok {
			m.SetConditionHostToFalse(
				metal3api.AssociatedCondition, metal3api.NoBareMetalHostReason,
				"Failed to pick a BaremetalHost for the Host")
		}
		if errors.Is(err, ErrNoAvailableBMH) {
			m.Log.Info("No available host found. Requeuing.")
			m.SetConditionHostToFalse(
				metal3api.AssociatedCondition, metal3api.NoBareMetalHostReason,
				"No available host found: requeuing.")
			return RequeueAfterError{RequeueAfter: HostClaimRequeueDelay}
		}
		return err
	}
	m.Log.Info("Associating hostClaim with host", "bmh", bmh.Name, "bmhNamespace", bmh.Namespace)

	// First we record the association in the BMH. If we fail, we must redo the
	// whole selection process.
	bmh.Spec.ConsumerRef = &corev1.ObjectReference{
		Kind:       HostClaimKind,
		Name:       m.HostClaim.Name,
		Namespace:  m.HostClaim.Namespace,
		APIVersion: metal3api.GroupVersion.Identifier(),
	}

	if err = m.client.Update(ctx, bmh); err != nil {
		m.Log.Error(err, "Error while updating the consumerRef on BMH")
		m.SetConditionHostToFalse(
			metal3api.AssociatedCondition, metal3api.BareMetalHostNotSynchronizedReason,
			"Failed to set consumer Reference on BareMetalHost")
		return hideConflictError(err)
	}

	// Then we record the commitment to this given BMH.
	m.HostClaim.Status.BareMetalHost = &metal3api.ObjectReference{
		Namespace: bmh.Namespace,
		Name:      bmh.Name,
	}

	// From here the hostClaim is definitely associated
	m.SetConditionHostToTrue(metal3api.AssociatedCondition, metal3api.BareMetalHostAssociatedReason)

	return nil
}

// Update updates a hostclaim and is invoked by the HostClaim Controller.
func (m *HostManager) Update(ctx context.Context) error {
	m.Log.V(1).Info("Updating HostClaim")

	bmh, err := m.getBmh(ctx)
	if err != nil && !errors.Is(err, ErrNoBMH) {
		m.SetConditionHostToFalse(
			metal3api.AssociatedCondition, metal3api.MissingBareMetalHostReason,
			"Failed to get a BareMetalHost for the Host")
		return err
	}
	if bmh == nil {
		m.SetConditionHostToFalse(
			metal3api.AssociatedCondition, metal3api.MissingBareMetalHostReason,
			"BareMetalHost associated to the claim not found")
		return errors.New("BareMetalHost not found")
	}

	// ensure that the BMH specs are correctly set.
	updated, err := m.setBmhSpec(ctx, bmh)
	if err != nil {
		return err
	}

	if bmh.Annotations == nil {
		bmh.Annotations = map[string]string{}
	}

	if syncReboot(m.HostClaim.Annotations, bmh.Annotations) {
		updated = true
	}
	if updated {
		m.Log.Info("Update the BareMetalHost spec: changes detected.")
		err = m.client.Update(ctx, bmh)
	}

	if err != nil {
		sanitizedErr := hideConflictError(err)
		if !errors.As(err, &RequeueAfterError{}) {
			m.SetConditionHostToFalse(
				metal3api.SynchronizedCondition, metal3api.BareMetalHostNotSynchronizedReason,
				"Failed to update BareMetalHost")
		}
		m.Log.Error(err, "Error while patching the BareMetalHost")
		return sanitizedErr
	}

	// transient rebootAnnotation was successfully transmitted. We can delete it on HostClaim.
	delete(m.HostClaim.Annotations, rebootDomain)

	m.updateHostClaimStatus(bmh)

	m.Log.V(1).Info("Finished updating HostClaim")
	return nil
}

// getBmh gets the associated BareMetalHost by looking for the status of the HostClaim.
// Returns ErrNoBMH if the BareMetalHost is not found.
func (m *HostManager) getBmh(ctx context.Context) (*metal3api.BareMetalHost, error) {
	hostClaim := m.HostClaim
	bmhRef := hostClaim.Status.BareMetalHost
	if bmhRef == nil {
		return nil, ErrNoBMH
	}

	bmh := metal3api.BareMetalHost{}
	key := types.NamespacedName{
		Name:      bmhRef.Name,
		Namespace: bmhRef.Namespace,
	}
	err := m.client.Get(ctx, key, &bmh)
	if k8serrors.IsNotFound(err) {
		m.Log.Info("Linked host not found", "bmh", bmhRef.Name, "bmhNamespace", bmhRef.Namespace)
		hostClaim.Status.BareMetalHost = nil
		return nil, ErrNoBMH
	} else if err != nil {
		return nil, err
	}
	if !consumerRefMatches(bmh.Spec.ConsumerRef, hostClaim) {
		m.Log.Info("The consumer ref does not point to the hostClaim", "consumerRef", bmh.Spec.ConsumerRef)
		hostClaim.Status.BareMetalHost = nil
		return nil, ErrNoBMH
	}
	return &bmh, nil
}

func hideConflictError(err error) error {
	var aggr kerrors.Aggregate
	if ok := errors.As(err, &aggr); ok {
		if slices.ContainsFunc(aggr.Errors(), k8serrors.IsConflict) {
			return RequeueAfterError{RequeueAfter: wait.Jitter(ConflictRequeueDelay, ConflictJitterFactor)}
		}
	}
	return err
}

// setBmhSpec will ensure the host's Spec is set according to the hostclaim's
// details.
func (m *HostManager) setBmhSpec(ctx context.Context, bmh *metal3api.BareMetalHost) (bool, error) {
	updated := false
	secretManager := secretutils.NewSecretManager(m.Log, m.client, m.APIReader)
	ref, err := m.synchronizeDataSecret(ctx, secretManager, bmh, "userdata", m.HostClaim.Spec.UserData, m.HostClaim.Namespace, m.HostClaim.Name)
	if err != nil && !errors.Is(err, ErrNoSecret) {
		m.SetConditionHostToFalse(
			metal3api.SynchronizedCondition, metal3api.BadUserDataSecretReason, err.Error(),
		)
		return false, err
	}
	if referencesDiffer(bmh.Spec.UserData, ref) {
		bmh.Spec.UserData = ref
		updated = true
	}
	ref, err = m.synchronizeDataSecret(ctx, secretManager, bmh, "metadata", m.HostClaim.Spec.MetaData, m.HostClaim.Namespace, m.HostClaim.Name)
	if err != nil && !errors.Is(err, ErrNoSecret) {
		m.SetConditionHostToFalse(
			metal3api.SynchronizedCondition, metal3api.BadMetaDataSecretReason, err.Error(),
		)
		return false, err
	}
	if referencesDiffer(bmh.Spec.MetaData, ref) {
		bmh.Spec.MetaData = ref
		updated = true
	}
	ref, err = m.synchronizeDataSecret(ctx, secretManager, bmh, "networkdata", m.HostClaim.Spec.NetworkData, m.HostClaim.Namespace, m.HostClaim.Name)
	if err != nil && !errors.Is(err, ErrNoSecret) {
		m.SetConditionHostToFalse(
			metal3api.SynchronizedCondition, metal3api.BadNetworkDataSecretReason, err.Error(),
		)
		return false, err
	}
	if referencesDiffer(bmh.Spec.NetworkData, ref) {
		bmh.Spec.NetworkData = ref
		updated = true
	}
	// A host with an existing image is already provisioned and
	// upgrades are not supported at this time. To re-provision a
	// host, we must fully deprovision it and then provision it again.
	if bmh.Spec.Image == nil && m.HostClaim.Spec.Image != nil {
		updated = true
		bmh.Spec.Image = m.HostClaim.Spec.Image.DeepCopy()
	} else if m.HostClaim.Spec.Image == nil {
		if bmh.Spec.Image != nil {
			updated = true
			bmh.Spec.Image = nil
		}
	}

	// Propagate custom deploy.
	if m.HostClaim.Spec.CustomDeploy == nil {
		if bmh.Spec.CustomDeploy != nil {
			updated = true
			bmh.Spec.CustomDeploy = nil
		}
	} else {
		if bmh.Spec.CustomDeploy == nil {
			updated = true
			bmh.Spec.CustomDeploy = &metal3api.CustomDeploy{Method: m.HostClaim.Spec.CustomDeploy.Method}
		} else if bmh.Spec.CustomDeploy.Method != m.HostClaim.Spec.CustomDeploy.Method {
			updated = true
			bmh.Spec.CustomDeploy.Method = m.HostClaim.Spec.CustomDeploy.Method
		}
	}

	// Set automatedCleaningMode to disabled as long as the hostclaim exists
	if bmh.Spec.AutomatedCleaningMode != metal3api.CleaningModeDisabled {
		updated = true
		bmh.Spec.AutomatedCleaningMode = metal3api.CleaningModeDisabled
	}
	if bmh.Spec.Online != m.HostClaim.Spec.PoweredOn {
		updated = true
		bmh.Spec.Online = m.HostClaim.Spec.PoweredOn
	}

	m.SetConditionHostToTrue(metal3api.SynchronizedCondition, metal3api.ConfigurationSyncedReason)
	return updated, nil
}

func referencesDiffer(ref1, ref2 *corev1.SecretReference) bool {
	if ref1 == nil {
		return ref2 != nil
	}
	return ref2 == nil || ref1.Name != ref2.Name
}

func (m *HostManager) synchronizeDataSecret(
	ctx context.Context,
	secretManager secretutils.SecretManager,
	bmh *metal3api.BareMetalHost,
	role string,
	sourceRef *corev1.SecretReference,
	namespace string,
	hostName string,
) (*corev1.SecretReference, error) {
	if namespace == bmh.Namespace {
		return sourceRef.DeepCopy(), nil
	}
	log := m.Log.WithValues("hostclaimName", hostName, "hostclaimNamespace", namespace, "secret-type", role)
	secretName := bmh.Name + "-" + role
	if sourceRef == nil {
		key := client.ObjectKey{Name: secretName, Namespace: bmh.Namespace}
		targetSecret := corev1.Secret{}
		err := m.client.Get(ctx, key, &targetSecret)
		if err != nil {
			if !k8serrors.IsNotFound(err) {
				log.Error(err, "Failed to access secret associated to BareMetalHost", "secretName", secretName, "namespace", bmh.Namespace)
				return nil, err
			}
			return nil, ErrNoSecret
		}
		log.V(1).Info("no configuration secret in hostclaim: destroying in bmh")
		err = m.client.Delete(ctx, &targetSecret)
		if err != nil {
			log.Error(err, "Failed to delete secret associated to BareMetalHost", "secretName", secretName, "namespace", bmh.Namespace)
		}
		return nil, err
	}
	// For the same reason as bmh, we ignore namespace value in the ref.
	sourceKey := client.ObjectKey{Name: sourceRef.Name, Namespace: namespace}
	sourceSecret, err := secretManager.AcquireSecret(ctx, sourceKey, m.HostClaim, false)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			log.Error(err, "Missing source Secret for synchronization from claim to BMH", "source", sourceKey)
			return nil, RequeueAfterError{RequeueAfter: HostClaimRequeueDelay}
		}
		log.Error(err, "Cannot get source Secret for synchronization from claim to BMH", "source", sourceKey)
		return nil, err
	}
	log.V(1).Info("Updating bmh secret with hostclaim secret content")
	targetSecret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: bmh.Namespace,
		},
	}
	_, err = controllerutil.CreateOrUpdate(ctx, m.client, &targetSecret,
		func() error {
			if targetSecret.Labels == nil {
				targetSecret.Labels = map[string]string{}
			}
			targetSecret.Labels[secretutils.LabelEnvironmentName] = secretutils.LabelEnvironmentValue
			if err = controllerutil.SetOwnerReference(bmh, &targetSecret, m.client.Scheme()); err != nil {
				return err
			}
			targetSecret.Data = sourceSecret.DeepCopy().Data
			return nil
		},
	)
	if err != nil {
		log.Error(err, "cannot copy/update secret", "source", sourceKey, "target", secretName)
		return nil, err
	}
	return &corev1.SecretReference{Name: secretName, Namespace: bmh.Namespace}, nil
}

// consumerRefMatches returns a boolean based on whether the consumer
// reference and bareMetalHost metadata match.
func consumerRefMatches(consumer *corev1.ObjectReference, claim *metal3api.HostClaim) bool {
	if consumer == nil || claim == nil {
		return false
	}
	if consumer.Name != claim.Name {
		return false
	}
	if consumer.Namespace != claim.Namespace {
		return false
	}
	if consumer.Kind != HostClaimKind {
		return false
	}
	if consumer.GroupVersionKind().Group != metal3api.GroupVersion.Group {
		return false
	}
	return true
}

func (m *HostManager) selectBMH(
	availableHosts []*metal3api.BareMetalHost,
) (*metal3api.BareMetalHost, error) {
	// choose a host.
	var err error
	var chosenHost *metal3api.BareMetalHost

	// If there are hosts with nodeReuseLabelName:
	chosenHost, err = m.pickHost(availableHosts)
	if err != nil {
		m.Log.Error(err, "Failed to choose host, not choosing host")
		return nil, err
	}

	return chosenHost, nil
}

// Picks host from list of available hosts, if failureDomain is set, tries to choose from hosts in failureDomain.
// When none available in failureDomain it chooses from all available hosts.
func (m *HostManager) pickHost(availableHosts []*metal3api.BareMetalHost) (*metal3api.BareMetalHost, error) {
	var chosenHost *metal3api.BareMetalHost
	var availableHostsInFailureDomain []*metal3api.BareMetalHost

	// When failureDomain is set, create a list from available hosts in failureDomain
	if m.HostClaim.Spec.FailureDomain != "" {
		labelSelector := labels.NewSelector()
		var reqs labels.Requirements
		var r *labels.Requirement
		r, err := labels.NewRequirement(FailureDomainLabelName, selection.Equals, []string{m.HostClaim.Spec.FailureDomain})

		if err != nil {
			m.Log.Error(err, "Failed to create FailureDomain MatchLabel requirement, not choosing host")
			return nil, err
		}
		reqs = append(reqs, *r)
		labelSelector = labelSelector.Add(reqs...)

		for _, host := range availableHosts {
			if labelSelector.Matches(labels.Set(host.ObjectMeta.Labels)) {
				availableHostsInFailureDomain = append(availableHostsInFailureDomain, host)
			}
		}
		if len(availableHostsInFailureDomain) == 0 {
			m.Log.Info("No available hosts in FailureDomain", m.HostClaim.Spec.FailureDomain, "choosing from other available hosts")
		}
	}

	if len(availableHostsInFailureDomain) > 0 {
		rHost, _ := rand.Int(rand.Reader, big.NewInt(int64(len(availableHostsInFailureDomain))))
		randomHost := rHost.Int64()
		chosenHost = availableHostsInFailureDomain[randomHost]
	} else {
		rHost, _ := rand.Int(rand.Reader, big.NewInt(int64(len(availableHosts))))
		randomHost := rHost.Int64()
		chosenHost = availableHosts[randomHost]
	}

	return chosenHost, nil
}

// For a given HostClaim in a given namespace, computes all the namespaces that can contain BMH
// that can be bound to the HostClaim.
//
// The function lists the HostDeployPolicy and keeps the namespaces of the ones that
// match the namespace of the claim. If the namespace argument is not empty,
// HostDeployPolicies are only listed in that namespace and the result is either
// an empty set or a singleton containing that namespace.
func (m *HostManager) acceptableNamespaces(ctx context.Context, namespace string) (Set[string], error) {
	m.Log.V(1).Info("Searching for suitable namespaces")
	hostdeploypolicies := metal3api.HostDeployPolicyList{}
	options := []client.ListOption{}
	if namespace != "" {
		options = append(options, client.InNamespace(namespace))
	}
	err := m.client.List(ctx, &hostdeploypolicies, options...)
	if err != nil {
		m.Log.Error(err, "cannot list the HostDeployPolicies")
		return nil, err
	}
	hostNs := m.HostClaim.Namespace
	hostNsResource := &corev1.Namespace{}
	err = m.client.Get(ctx, client.ObjectKey{Name: hostNs}, hostNsResource)
	if err != nil {
		m.Log.Error(err, "cannot access the namespace of the claim")
		return nil, err
	}
	nsLabels := hostNsResource.Labels
	if nsLabels == nil {
		nsLabels = map[string]string{}
	}
	namespaces := NewSet[string]()
LOOP_POLICY:
	for _, hostDeployPolicy := range hostdeploypolicies.Items {
		log := m.Log.WithValues("policyNamespace", hostDeployPolicy.Namespace, "policyName", hostDeployPolicy.Name)
		if SetContains(namespaces, hostDeployPolicy.Namespace) {
			// namespace already added, no reason to check this hdp.
			continue
		}
		constraints := hostDeployPolicy.Spec.HostClaimNamespaces
		if constraints == nil {
			log.V(1).Info("Ignoring HostDeployPolicy without constraint")
			continue
		}
		if constraints.Names != nil && !slices.Contains(constraints.Names, hostNs) {
			log.V(1).Info("Ignoring HostDeployPolicy because claim namespace not in names", "names", constraints.Names)
			continue
		}
		if constraints.NameMatches != "" {
			b, err := regexp.MatchString(constraints.NameMatches, hostNs)
			if err != nil {
				log.Error(
					err, "Error during regexp matching on HostClaim namespace (bad regexp)",
					"regexp", constraints.NameMatches)
				return nil, err
			}
			if !b {
				log.V(1).Info("Ignoring HostDeployPolicy because claim namespace does not match regex")
				continue
			}
		}
		// Behaves as a 'forall' on labels
		for _, pair := range constraints.HasLabels {
			if v, ok := nsLabels[pair.Name]; ok {
				if pair.Value != "" && v != pair.Value {
					log.V(1).Info(
						"Ignoring HostDeployPolicy because claim namespace labels does not have correct value",
						"label", pair.Name, "value", v, "expected", pair.Value)
					continue LOOP_POLICY
				}
			} else {
				log.V(1).Info(
					"Ignoring HostDeployPolicy because claim namespace labels does not have a required label",
					"label", pair.Name)
				continue LOOP_POLICY
			}
		}
		log.V(1).Info("Accepting namespace because of HostDeployPolicy", "namespace", hostDeployPolicy.Namespace)
		AddSet(namespaces, hostDeployPolicy.Namespace)
	}
	m.Log.Info("Acceptable namespaces", "namespaces", namespaces)
	return namespaces, nil
}

func (m *HostManager) hostLabelSelectorForHostClaim() (labels.Selector, error) {
	labelSelector := labels.NewSelector()

	for labelKey, labelVal := range m.HostClaim.Spec.HostSelector.MatchLabels {
		r, err := labels.NewRequirement(labelKey, selection.Equals, []string{labelVal})
		if err != nil {
			m.Log.Error(err, "Failed to create MatchLabel requirement, not choosing host")
			return nil, err
		}
		labelSelector = labelSelector.Add(*r)
	}

	for _, req := range m.HostClaim.Spec.HostSelector.MatchExpressions {
		lowercaseOperator := selection.Operator(strings.ToLower(string(req.Operator)))
		r, err := labels.NewRequirement(req.Key, lowercaseOperator, req.Values)
		if err != nil {
			m.Log.Error(err, "Failed to create MatchExpression requirement, not choosing host")
			return nil, err
		}
		labelSelector = labelSelector.Add(*r)
	}
	return labelSelector, nil
}

// chooseBMH iterates through known bare-metal hosts and returns one that can be
// associated with the HostClaim. It searches all hosts in case one already has an
// association with this HostClaim.
func (m *HostManager) chooseBMH(ctx context.Context) (*metal3api.BareMetalHost, error) {
	namespaces, err := m.acceptableNamespaces(ctx, m.HostClaim.Spec.HostSelector.InNamespace)
	if err != nil {
		return nil, err
	}
	if len(namespaces) == 0 {
		return nil, ErrNoAvailableBMH
	}
	// get list of BMH.
	labelSelector, err := m.hostLabelSelectorForHostClaim()
	if err != nil {
		return nil, err
	}

	// Different from M3M: We do not restrict to a single namespace (namespace of Metal3Machine)

	availableHosts := []*metal3api.BareMetalHost{}

	for namespace := range namespaces {
		bmhs := metal3api.BareMetalHostList{}
		m.Log.V(1).Info("Looking for BMHs in namespace", "namespace", namespace)
		err = m.client.List(ctx, &bmhs, client.MatchingLabelsSelector{Selector: labelSelector}, client.InNamespace(namespace))
		if err != nil {
			return nil, err
		}
		for i, bmh := range bmhs.Items {
			if bmh.Spec.ConsumerRef != nil && consumerRefMatches(bmh.Spec.ConsumerRef, m.HostClaim) {
				m.Log.Info("Found host with existing ConsumerRef", "bmh", bmh.Name, "bmhNamespace", bmh.Namespace)
				return &bmh, nil
			}

			if bmh.Spec.ConsumerRef != nil {
				continue
			}
			if bmh.GetDeletionTimestamp() != nil {
				continue
			}
			// continue if BaremetalHost is paused or marked with UnhealthyAnnotation.
			annotations := bmh.GetAnnotations()
			if annotations != nil {
				if _, ok := annotations[metal3api.PausedAnnotation]; ok {
					continue
				}
			}

			switch bmh.Status.Provisioning.State {
			case metal3api.StateReady, metal3api.StateAvailable:
			default:
				continue
			}

			if bmh.Status.ErrorMessage != "" {
				m.Log.Info("Found an available host with an error message (should not occur)",
					"bmh", bmh.Name, "bmhNamespace", bmh.Namespace)
				continue
			}

			m.Log.Info("Host matched hostSelector for Host, adding it to availableHosts list",
				"bmh", bmh.Name, "bmhNamespace", bmh.Namespace)
			availableHosts = append(availableHosts, &bmhs.Items[i])
		}
	}

	m.Log.Info("Host count available while choosing host for HostClaim", "hostcount", len(availableHosts))
	if len(availableHosts) == 0 {
		return nil, ErrNoAvailableBMH
	}

	chosenHost, err := m.selectBMH(availableHosts)
	if err != nil {
		m.Log.Error(err, "Failed to select a Host")
		return nil, err
	}

	return chosenHost, err
}

// updateHostClaimStatus updates the status of the HostClaim with information from BareMetalHost.
func (m *HostManager) updateHostClaimStatus(bmh *metal3api.BareMetalHost) {
	hostOld := m.HostClaim.Status.DeepCopy()

	// synchronize power status
	m.HostClaim.Status.PoweredOn = bmh.Status.PoweredOn
	m.HostClaim.Status.HardwareData = &metal3api.ObjectReference{
		Namespace: bmh.Namespace,
		Name:      bmh.Name,
	}
	conditions.SetMirrorCondition(bmh, m.HostClaim, metal3api.AvailableForProvisioningCondition)
	conditions.SetMirrorCondition(bmh, m.HostClaim, metal3api.ProvisionedCondition)
	m.SetConditionHostToTrue(metal3api.AssociatedCondition, metal3api.BareMetalHostAssociatedReason)

	if !equality.Semantic.DeepEqual(m.HostClaim.Status, hostOld) {
		m.Log.Info("Status of HostClaim changed")
		now := metav1.Now()
		m.HostClaim.Status.LastUpdated = &now
	}
}

// synchronize reboot annotations from hostMap to bmhMap.
func syncReboot(hostMap, bmhMap map[string]string) bool {
	updated := false
	// We propagate first deletion of reboot annotations on BMH
	for key := range bmhMap {
		elts := strings.Split(key, "/")
		// Propagates down unless it is just reboot and already propagated.
		if elts[0] == rebootDomain && len(elts) == 2 {
			if _, ok := hostMap[key]; !ok {
				updated = true
				delete(bmhMap, key)
			}
		}
	}

	// We propagate reboot annotations to the bmh when it appears.
	for key, v := range hostMap {
		elts := strings.Split(key, "/")
		if elts[0] == rebootDomain {
			if bmhMap[key] != v {
				updated = true
				bmhMap[key] = v
			}
		}
	}
	return updated
}

type Set[T comparable] = map[T]struct{}

func NewSet[T comparable]() Set[T] {
	return Set[T]{}
}

func AddSet[T comparable](m Set[T], s T) {
	m[s] = struct{}{}
}

func SetContains[T comparable](m Set[T], s T) bool {
	_, ok := m[s]
	return ok
}
