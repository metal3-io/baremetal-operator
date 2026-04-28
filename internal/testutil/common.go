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

package testutil

import (
	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/cluster-api/util/conditions"
)

// Namespace definitions.

const (
	HostclaimName      = "hostclaim"
	HostclaimNamespace = "hcNs"
)

type NamespaceBuilder struct {
	namespace corev1.Namespace
}

func NewNamespace(name string) *NamespaceBuilder {
	return &NamespaceBuilder{
		corev1.Namespace{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Namespace",
				APIVersion: "v1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			},
		},
	}
}

func (nb *NamespaceBuilder) SetLabels(labels map[string]string) *NamespaceBuilder {
	nb.namespace.Labels = labels
	return nb
}

func (nb *NamespaceBuilder) Build() *corev1.Namespace {
	return &nb.namespace
}

// Secret Builder

type SecretBuilder struct {
	secret corev1.Secret
}

func NewSecret(name, namespace string) *SecretBuilder {
	return &SecretBuilder{
		corev1.Secret{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Secret",
				APIVersion: "v1",
			},

			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
		},
	}
}

func (sb *SecretBuilder) Build() *corev1.Secret {
	return &sb.secret
}

func (sb *SecretBuilder) SetData(data map[string][]byte) *SecretBuilder {
	sb.secret.Data = data
	return sb
}

type BareMetalHostBuilder struct {
	bmh metal3api.BareMetalHost
}

func NewBaremetalhost(name, namespace string, state metal3api.ProvisioningState) *BareMetalHostBuilder {
	return &BareMetalHostBuilder{
		metal3api.BareMetalHost{
			TypeMeta: metav1.TypeMeta{
				Kind:       "BareMetalHost",
				APIVersion: metal3api.GroupVersion.Identifier(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
			Status: metal3api.BareMetalHostStatus{
				Provisioning: metal3api.ProvisionStatus{
					State: state,
				},
			},
		},
	}
}

func (bb *BareMetalHostBuilder) Build() *metal3api.BareMetalHost {
	return &bb.bmh
}

func (bb *BareMetalHostBuilder) SetLabels(labels map[string]string) *BareMetalHostBuilder {
	bb.bmh.Labels = labels
	return bb
}

func (bb *BareMetalHostBuilder) SetAnnotations(annotations map[string]string) *BareMetalHostBuilder {
	bb.bmh.Annotations = annotations
	return bb
}

func (bb *BareMetalHostBuilder) SetPowerOn() *BareMetalHostBuilder {
	bb.bmh.Spec.Online = true
	return bb
}

func (bb *BareMetalHostBuilder) SetConsumerRef(cref corev1.ObjectReference) *BareMetalHostBuilder {
	bb.bmh.Spec.ConsumerRef = &cref
	return bb
}

func (bb *BareMetalHostBuilder) SetUserData(udata string) *BareMetalHostBuilder {
	bb.bmh.Spec.UserData = &corev1.SecretReference{Name: udata}
	return bb
}

func (bb *BareMetalHostBuilder) SetMetaData(mdata string) *BareMetalHostBuilder {
	bb.bmh.Spec.MetaData = &corev1.SecretReference{Name: mdata}
	return bb
}

func (bb *BareMetalHostBuilder) SetNetworkData(ndata string) *BareMetalHostBuilder {
	bb.bmh.Spec.NetworkData = &corev1.SecretReference{Name: ndata}
	return bb
}

func (bb *BareMetalHostBuilder) SetImage(image metal3api.Image) *BareMetalHostBuilder {
	bb.bmh.Spec.Image = &image
	return bb
}

func (bb *BareMetalHostBuilder) SetCleaningMode(cmode metal3api.AutomatedCleaningMode) *BareMetalHostBuilder {
	bb.bmh.Spec.AutomatedCleaningMode = cmode
	return bb
}

func (bb *BareMetalHostBuilder) SetCustomDeploy(cd string) *BareMetalHostBuilder {
	bb.bmh.Spec.CustomDeploy = &metal3api.CustomDeploy{Method: cd}
	return bb
}

func (bb *BareMetalHostBuilder) SetCondition(typ string, status bool, reason string) *BareMetalHostBuilder {
	conditions.Set(&bb.bmh, metav1.Condition{Type: typ, Status: conditions.BoolToStatus(status), Reason: reason})
	return bb
}

// Hardware data.

type HardwareDataBuilder struct {
	hardwareData metal3api.HardwareData
}

func NewHardwareData(bmh *metal3api.BareMetalHost) *HardwareDataBuilder {
	return &HardwareDataBuilder{
		metal3api.HardwareData{
			TypeMeta: metav1.TypeMeta{
				Kind:       "HardwareData",
				APIVersion: metal3api.GroupVersion.Identifier(),
			},

			ObjectMeta: metav1.ObjectMeta{
				Name:      bmh.Name,
				Namespace: bmh.Namespace,
			},
			Spec: metal3api.HardwareDataSpec{
				HardwareDetails: &metal3api.HardwareDetails{},
			},
		},
	}
}

func (hb *HardwareDataBuilder) SetLabels(labels map[string]string) *HardwareDataBuilder {
	hb.hardwareData.Labels = labels
	return hb
}

func (hb *HardwareDataBuilder) Build() *metal3api.HardwareData {
	return &hb.hardwareData
}

type HostClaimBuilder struct {
	hostClaim metal3api.HostClaim
}

func NewHostclaim(name string) *HostClaimBuilder {
	return &HostClaimBuilder{
		metal3api.HostClaim{
			TypeMeta: metav1.TypeMeta{
				Kind:       "HostClaim",
				APIVersion: metal3api.GroupVersion.Identifier(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: HostclaimNamespace,
			},
			Spec: metal3api.HostClaimSpec{
				HostSelector: metal3api.HostSelector{},
			},
		},
	}
}

func (hb *HostClaimBuilder) Build() *metal3api.HostClaim {
	return &hb.hostClaim
}

func (hb *HostClaimBuilder) SetFinalizer(finalizers []string) *HostClaimBuilder {
	hb.hostClaim.Finalizers = finalizers
	return hb
}

func (hb *HostClaimBuilder) SetLabels(labels map[string]string) *HostClaimBuilder {
	hb.hostClaim.Labels = labels
	return hb
}

func (hb *HostClaimBuilder) SetAnnotations(annotations map[string]string) *HostClaimBuilder {
	hb.hostClaim.Annotations = annotations
	return hb
}

func (hb *HostClaimBuilder) SetConsumerRef(cref corev1.ObjectReference) *HostClaimBuilder {
	hb.hostClaim.Spec.ConsumerRef = &cref
	return hb
}

func (hb *HostClaimBuilder) SetUserData(udata string) *HostClaimBuilder {
	hb.hostClaim.Spec.UserData = &corev1.SecretReference{Name: udata}
	return hb
}

func (hb *HostClaimBuilder) SetMetaData(mdata string) *HostClaimBuilder {
	hb.hostClaim.Spec.MetaData = &corev1.SecretReference{Name: mdata}
	return hb
}

func (hb *HostClaimBuilder) SetNetworkData(ndata string) *HostClaimBuilder {
	hb.hostClaim.Spec.NetworkData = &corev1.SecretReference{Name: ndata}
	return hb
}

func (hb *HostClaimBuilder) SetImage(image metal3api.Image) *HostClaimBuilder {
	hb.hostClaim.Spec.Image = &image
	return hb
}

func (hb *HostClaimBuilder) SetCustomDeploy(cd string) *HostClaimBuilder {
	hb.hostClaim.Spec.CustomDeploy = &metal3api.CustomDeploy{Method: cd}
	return hb
}

func (hb *HostClaimBuilder) SetPowerOn() *HostClaimBuilder {
	hb.hostClaim.Spec.PoweredOn = true
	return hb
}

func (hb *HostClaimBuilder) SetFailureDomain(fd string) *HostClaimBuilder {
	hb.hostClaim.Spec.FailureDomain = fd
	return hb
}

func (hb *HostClaimBuilder) SetTargetNamespace(ns string) *HostClaimBuilder {
	hb.hostClaim.Spec.HostSelector.InNamespace = ns
	return hb
}

func (hb *HostClaimBuilder) SetLabelSelector(ls map[string]string) *HostClaimBuilder {
	hb.hostClaim.Spec.HostSelector.MatchLabels = ls
	return hb
}

func (hb *HostClaimBuilder) SetMatchExpressions(me []metal3api.HostSelectorRequirement) *HostClaimBuilder {
	hb.hostClaim.Spec.HostSelector.MatchExpressions = me
	return hb
}

func (hb *HostClaimBuilder) SetCondition(typ string, status bool, reason string) *HostClaimBuilder {
	conditions.Set(&hb.hostClaim, metav1.Condition{Type: typ, Status: conditions.BoolToStatus(status), Reason: reason})
	return hb
}

func (hb *HostClaimBuilder) SetAssociatedBMH(namespace, name string) *HostClaimBuilder {
	hb.hostClaim.Status.BareMetalHost = &metal3api.ObjectReference{Namespace: namespace, Name: name}
	return hb
}

type HostDeployPolicyBuilder struct {
	hostDeployPolicy metal3api.HostDeployPolicy
}

func NewHostdeploypolicy(name, namespace string) *HostDeployPolicyBuilder {
	return &HostDeployPolicyBuilder{
		metal3api.HostDeployPolicy{
			TypeMeta: metav1.TypeMeta{
				Kind:       "HostDeployPolicy",
				APIVersion: metal3api.GroupVersion.Identifier(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
		},
	}
}

func (hb *HostDeployPolicyBuilder) Build() *metal3api.HostDeployPolicy {
	return &hb.hostDeployPolicy
}

func (hb *HostDeployPolicyBuilder) AcceptNames(acn []string) *HostDeployPolicyBuilder {
	spec := &hb.hostDeployPolicy.Spec
	if spec.HostClaimNamespaces == nil {
		spec.HostClaimNamespaces = &metal3api.HostClaimNamespaces{}
	}
	spec.HostClaimNamespaces.Names = acn
	return hb
}

func (hb *HostDeployPolicyBuilder) AcceptLabels(acl []metal3api.NameValuePair) *HostDeployPolicyBuilder {
	spec := &hb.hostDeployPolicy.Spec
	if spec.HostClaimNamespaces == nil {
		spec.HostClaimNamespaces = &metal3api.HostClaimNamespaces{}
	}
	spec.HostClaimNamespaces.HasLabels = acl
	return hb
}

func (hb *HostDeployPolicyBuilder) AcceptRegexp(re string) *HostDeployPolicyBuilder {
	spec := &hb.hostDeployPolicy.Spec
	if spec.HostClaimNamespaces == nil {
		spec.HostClaimNamespaces = &metal3api.HostClaimNamespaces{}
	}
	spec.HostClaimNamespaces.NameMatches = re
	return hb
}

func (hb *HostDeployPolicyBuilder) AllowsDetachedMode() *HostDeployPolicyBuilder {
	hb.hostDeployPolicy.Spec.AllowsDetachedMode = true
	return hb
}
