//go:build unit

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

package testing

import (
	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/cluster-api/util/conditions"
)

type WithFinalizers []string
type WithLabels map[string]string

type WithAnnotations map[string]string

type NamespaceOption interface {
	EnhanceNamespace(*corev1.Namespace)
}

// Namespace definitions.
func NewNamespace(name string, options ...NamespaceOption) *corev1.Namespace {
	ns := &corev1.Namespace{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Namespace",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
	for _, option := range options {
		option.EnhanceNamespace(ns)
	}
	return ns
}

const (
	HostclaimName      = "hostclaim"
	HostclaimNamespace = "hcNs"
)

func (wl WithLabels) EnhanceNamespace(ns *corev1.Namespace) {
	ns.Labels = wl
}

// Secret definitions.

type SecretOption interface {
	EnhanceSecret(*corev1.Secret)
}

func NewSecret(name, namespace string, options ...SecretOption) *corev1.Secret {
	sec := &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: "v1",
		},

		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
	for _, option := range options {
		option.EnhanceSecret(sec)
	}
	return sec
}

type WithData map[string][]byte

func (wd WithData) EnhanceSecret(sec *corev1.Secret) {
	sec.Data = wd
}

// BareMetalHost definitions.
type BaremetalhostOption interface {
	EnhanceBmh(*metal3api.BareMetalHost)
}

func NewBaremetalhost(name, namespace string, state metal3api.ProvisioningState, options ...BaremetalhostOption) *metal3api.BareMetalHost {
	bmh := &metal3api.BareMetalHost{
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
	}
	for _, option := range options {
		option.EnhanceBmh(bmh)
	}
	return bmh
}

func (bl WithLabels) EnhanceBmh(bmh *metal3api.BareMetalHost) {
	bmh.Labels = bl
}

func (ba WithAnnotations) EnhanceBmh(bmh *metal3api.BareMetalHost) {
	bmh.Annotations = ba
}

type PoweredOnTrue struct{}

func (ot PoweredOnTrue) EnhanceBmh(bmh *metal3api.BareMetalHost) {
	bmh.Spec.Online = true
}

type WithConsumerRef corev1.ObjectReference

func (wcr WithConsumerRef) EnhanceBmh(bmh *metal3api.BareMetalHost) {
	c := corev1.ObjectReference(wcr)
	bmh.Spec.ConsumerRef = &c
}

type WithUserData string

func (wud WithUserData) EnhanceBmh(bmh *metal3api.BareMetalHost) {
	bmh.Spec.UserData = &corev1.SecretReference{Name: string(wud)}
}

type WithMetaData string

func (wud WithMetaData) EnhanceBmh(bmh *metal3api.BareMetalHost) {
	bmh.Spec.MetaData = &corev1.SecretReference{Name: string(wud)}
}

type WithNetworkData string

func (wud WithNetworkData) EnhanceBmh(bmh *metal3api.BareMetalHost) {
	bmh.Spec.NetworkData = &corev1.SecretReference{Name: string(wud)}
}

type WithImage struct {
	metal3api.Image
}

func (wi WithImage) EnhanceBmh(bmh *metal3api.BareMetalHost) {
	bmh.Spec.Image = &wi.Image
}

type WithCleaningMode metal3api.AutomatedCleaningMode

func (wcm WithCleaningMode) EnhanceBmh(bmh *metal3api.BareMetalHost) {
	bmh.Spec.AutomatedCleaningMode = metal3api.AutomatedCleaningMode(wcm)
}

type WithCustomDeploy string

func (wcd WithCustomDeploy) EnhanceBmh(bmh *metal3api.BareMetalHost) {
	bmh.Spec.CustomDeploy = &metal3api.CustomDeploy{Method: string(wcd)}
}

// Hardware data.
func NewHardwareData(bmh *metal3api.BareMetalHost) *metal3api.HardwareData {
	return &metal3api.HardwareData{
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
	}
}

// Hostclaim definitions.
type HostclaimOption interface {
	EnhanceHostclaim(*metal3api.HostClaim)
}

func NewHostclaim(name string, options ...HostclaimOption) *metal3api.HostClaim {
	hc := &metal3api.HostClaim{
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
	}
	for _, option := range options {
		option.EnhanceHostclaim(hc)
	}
	return hc
}

func (wf WithFinalizers) EnhanceHostclaim(hc *metal3api.HostClaim) {
	hc.Finalizers = wf
}

func (wl WithLabels) EnhanceHostclaim(hc *metal3api.HostClaim) {
	hc.Labels = wl
}

func (wa WithAnnotations) EnhanceHostclaim(hc *metal3api.HostClaim) {
	hc.Annotations = wa
}

func (wud WithUserData) EnhanceHostclaim(hc *metal3api.HostClaim) {
	hc.Spec.UserData = &corev1.SecretReference{Name: string(wud)}
}

func (wud WithMetaData) EnhanceHostclaim(hc *metal3api.HostClaim) {
	hc.Spec.MetaData = &corev1.SecretReference{Name: string(wud)}
}

func (wud WithNetworkData) EnhanceHostclaim(hc *metal3api.HostClaim) {
	hc.Spec.NetworkData = &corev1.SecretReference{Name: string(wud)}
}

func (wi WithImage) EnhanceHostclaim(hc *metal3api.HostClaim) {
	hc.Spec.Image = &wi.Image
}

func (wcd WithCustomDeploy) EnhanceHostclaim(hc *metal3api.HostClaim) {
	hc.Spec.CustomDeploy = &metal3api.CustomDeploy{Method: string(wcd)}
}

func (ot PoweredOnTrue) EnhanceHostclaim(hc *metal3api.HostClaim) {
	hc.Spec.PoweredOn = true
}

func (wcd WithCustomDeploy) EnhanceHostClaim(hc *metal3api.HostClaim) {
	hc.Spec.CustomDeploy = &metal3api.CustomDeploy{Method: string(wcd)}
}

type WithTargetNamespace string

func (whs WithTargetNamespace) EnhanceHostclaim(hostclaim *metal3api.HostClaim) {
	hostclaim.Spec.HostSelector.InNamespace = string(whs)
}

type WithFailureDomain string

func (wfd WithFailureDomain) EnhanceHostclaim(hostclaim *metal3api.HostClaim) {
	hostclaim.Spec.FailureDomain = string(wfd)
}

type WithLabelSelector map[string]string

func (wls WithLabelSelector) EnhanceHostclaim(hostclaim *metal3api.HostClaim) {
	hostclaim.Spec.HostSelector.MatchLabels = wls
}

type WithMatchExprSelector []metal3api.HostSelectorRequirement

func (wmes WithMatchExprSelector) EnhanceHostclaim(hostclaim *metal3api.HostClaim) {
	hostclaim.Spec.HostSelector.MatchExpressions = wmes
}

type Condition struct {
	Type   string
	Status bool
	Reason string
}

func (c Condition) EnhanceHostclaim(hostclaim *metal3api.HostClaim) {
	conditions.Set(hostclaim, metav1.Condition{Type: c.Type, Status: conditions.BoolToStatus(c.Status), Reason: c.Reason})
}

// HostDeployPolicy definitions.
type HostdeploypolicyOption interface {
	EnhanceHDP(*metal3api.HostDeployPolicy)
}

func NewHostdeploypolicy(name, namespace string, options ...HostdeploypolicyOption) *metal3api.HostDeployPolicy {
	hdp := &metal3api.HostDeployPolicy{
		TypeMeta: metav1.TypeMeta{
			Kind:       "HostDeployPolicy",
			APIVersion: metal3api.GroupVersion.Identifier(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
	for _, option := range options {
		option.EnhanceHDP(hdp)
	}
	return hdp
}

type AcceptNames []string

func (an AcceptNames) EnhanceHDP(hdp *metal3api.HostDeployPolicy) {
	if hdp.Spec.HostClaimNamespaces == nil {
		hdp.Spec.HostClaimNamespaces = &metal3api.HostClaimNamespaces{}
	}
	hdp.Spec.HostClaimNamespaces.Names = an
}

type AcceptLabels []metal3api.NameValuePair

func (al AcceptLabels) EnhanceHDP(hdp *metal3api.HostDeployPolicy) {
	if hdp.Spec.HostClaimNamespaces == nil {
		hdp.Spec.HostClaimNamespaces = &metal3api.HostClaimNamespaces{}
	}
	hdp.Spec.HostClaimNamespaces.HasLabels = al
}

type AcceptRegexp string

func (ar AcceptRegexp) EnhanceHDP(hdp *metal3api.HostDeployPolicy) {
	if hdp.Spec.HostClaimNamespaces == nil {
		hdp.Spec.HostClaimNamespaces = &metal3api.HostClaimNamespaces{}
	}
	hdp.Spec.HostClaimNamespaces.NameMatches = string(ar)
}
