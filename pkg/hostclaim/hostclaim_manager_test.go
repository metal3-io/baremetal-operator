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
	"errors"
	"maps"
	"reflect"
	"strings"
	"testing"

	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	. "github.com/metal3-io/baremetal-operator/internal/testutil"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/selection"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// setupSchemes configures schemes.
func setupScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		panic(err)
	}

	if err := metal3api.AddToScheme(scheme); err != nil {
		panic(err)
	}

	return scheme
}

var _ = Describe("HostClaim manager", func() {
	var defaultConsumerRef = corev1.ObjectReference{
		Name:       HostclaimName,
		Namespace:  HostclaimNamespace,
		Kind:       HostClaimKind,
		APIVersion: metal3api.GroupVersion.String(),
	}

	var (
		defaultImage = metal3api.Image{URL: "url"}
	)

	type testCaseChooseBMH struct {
		HostClaim          *metal3api.HostClaim
		HostDeployPolicies []*metal3api.HostDeployPolicy
		BareMetalHosts     []*metal3api.BareMetalHost
		Namespaces         []*corev1.Namespace
		ExpectedBmhName    string
		ExpectRequeue      bool
		ExpectFail         bool
	}

	var (
		hcNs                    = NewNamespace(HostclaimNamespace).Build()
		ns1                     = NewNamespace("ns1").Build()
		ns2                     = NewNamespace("ns2").Build()
		hcNsLabelled            = NewNamespace(HostclaimNamespace).SetLabels(map[string]string{"l": "v"}).Build()
		defaultBmhLabels        = map[string]string{"default-selector": "default-value"}
		defaultFailureBmhLabels = map[string]string{"default-selector": "default-value", "infrastructure.cluster.x-k8s.io/failure-domain": "zone"}
		bmhns1                  = NewBaremetalhost("bmh1", "ns1", metal3api.StateAvailable).SetLabels(defaultBmhLabels).Build()
		bmhns2                  = NewBaremetalhost("bmh2", "ns2", metal3api.StateAvailable).SetLabels(defaultBmhLabels).Build()
		bmh2ns1FailureDomain    = NewBaremetalhost("bmh2", "ns1", metal3api.StateAvailable).SetLabels(defaultFailureBmhLabels).Build()
		bmhns1BadLabel          = NewBaremetalhost("nolabel-bmh1", "ns1", metal3api.StateAvailable).Build()
		bmhns1NotAvail          = NewBaremetalhost("notavail-bmh1", "ns1", metal3api.StateRegistering).SetLabels(defaultBmhLabels).Build()
		bmhns1Paused            = NewBaremetalhost("paused-bmh1", "ns1", metal3api.StateRegistering).SetLabels(defaultBmhLabels).
					SetAnnotations(map[string]string{metal3api.PausedAnnotation: PausedAnnotationValue}).Build()
		bmhns1Consumed = NewBaremetalhost(
			"bmh-consumed", "ns1", metal3api.StateAvailable).SetLabels(defaultBmhLabels).
			SetConsumerRef(corev1.ObjectReference{Kind: HostClaimKind, Namespace: HostclaimNamespace,
				APIVersion: metal3api.GroupVersion.String(), Name: HostclaimName}).Build()
		bmhns1ConsOther = NewBaremetalhost(
			"bmh-cons-other", "ns1", metal3api.StateAvailable).SetLabels(defaultBmhLabels).
			SetConsumerRef(corev1.ObjectReference{Kind: HostClaimKind, Namespace: HostclaimNamespace,
				APIVersion: metal3api.GroupVersion.String(), Name: "other"}).Build()
	)

	DescribeTable("Test chooseBMH",
		func(tc testCaseChooseBMH) {
			objects := []client.Object{}
			objects = append(objects, tc.HostClaim)
			if tc.Namespaces != nil {
				for _, obj := range tc.Namespaces {
					objects = append(objects, obj)
				}
			}
			if tc.HostDeployPolicies != nil {
				for _, obj := range tc.HostDeployPolicies {
					objects = append(objects, obj)
				}
			}
			if tc.BareMetalHosts != nil {
				for _, obj := range tc.BareMetalHosts {
					objects = append(objects, obj)
					objects = append(objects, NewHardwareData(obj).Build())
				}
			}
			fakeClient := fake.NewClientBuilder().WithScheme(setupScheme()).WithObjects(objects...).Build()
			hostMgr, err := NewHostManager(fakeClient, GinkgoLogr, tc.HostClaim, fakeClient)
			Expect(err).NotTo(HaveOccurred())
			bmh, err := hostMgr.chooseBMH(context.TODO())
			if tc.ExpectedBmhName == "" {
				Expect(bmh).To(BeNil())
				if tc.ExpectFail {
					Expect(err).To(HaveOccurred())
					var requeueAfterError RequeueAfterError
					Expect(errors.As(err, &requeueAfterError)).To(BeFalse())
				} else if tc.ExpectRequeue {
					Expect(err).To(HaveOccurred())
					var requeueAfterError RequeueAfterError
					Expect(errors.As(err, &requeueAfterError)).To(BeTrue())
				} else {
					Expect(errors.Is(err, ErrNoAvailableBMH)).To(BeTrue())
				}
			} else {
				Expect(err).NotTo(HaveOccurred())
				Expect(bmh).NotTo(BeNil())
				Expect(bmh.Name).To(Equal(tc.ExpectedBmhName))
			}
		},
		Entry("no policies", testCaseChooseBMH{
			HostClaim:      NewHostclaim(HostclaimName).Build(),
			Namespaces:     []*corev1.Namespace{hcNs, ns1, ns2},
			BareMetalHosts: []*metal3api.BareMetalHost{bmhns1, bmhns2},
		}),
		Entry("bad namespace name", testCaseChooseBMH{
			HostClaim:  NewHostclaim(HostclaimName).Build(),
			Namespaces: []*corev1.Namespace{hcNs, ns1, ns2},
			HostDeployPolicies: []*metal3api.HostDeployPolicy{
				NewHostdeploypolicy("hdp", "ns1").AcceptNames([]string{"otherNs"}).Build()},
			BareMetalHosts: []*metal3api.BareMetalHost{bmhns1, bmhns2},
		}),
		Entry("with policy in ns1", testCaseChooseBMH{
			HostClaim:  NewHostclaim(HostclaimName).Build(),
			Namespaces: []*corev1.Namespace{hcNs, ns1, ns2},
			HostDeployPolicies: []*metal3api.HostDeployPolicy{
				NewHostdeploypolicy("hdp", "ns1").AcceptNames([]string{HostclaimNamespace}).Build()},
			BareMetalHosts:  []*metal3api.BareMetalHost{bmhns1, bmhns2},
			ExpectedBmhName: "bmh1",
		}),
		Entry("HostClaim targets ns1 (positive)", testCaseChooseBMH{
			HostClaim:  NewHostclaim(HostclaimName).SetTargetNamespace("ns1").Build(),
			Namespaces: []*corev1.Namespace{hcNs, ns1, ns2},
			HostDeployPolicies: []*metal3api.HostDeployPolicy{
				NewHostdeploypolicy("hdp", "ns1").AcceptNames([]string{HostclaimNamespace}).Build(),
				NewHostdeploypolicy("hdp", "ns2").AcceptNames([]string{HostclaimNamespace}).Build()},
			BareMetalHosts:  []*metal3api.BareMetalHost{bmhns1, bmhns2},
			ExpectedBmhName: "bmh1",
		}),
		Entry("HostClaim targets ns1 (negative)", testCaseChooseBMH{
			HostClaim:  NewHostclaim(HostclaimName).SetTargetNamespace("ns1").Build(),
			Namespaces: []*corev1.Namespace{hcNs, ns1, ns2},
			HostDeployPolicies: []*metal3api.HostDeployPolicy{
				NewHostdeploypolicy("hdp", "ns1").AcceptNames([]string{HostclaimNamespace}).Build(),
				NewHostdeploypolicy("hdp", "ns2").AcceptNames([]string{HostclaimNamespace}).Build()},
			BareMetalHosts: []*metal3api.BareMetalHost{bmhns2},
		}),
		Entry("with criteriums (positive)", testCaseChooseBMH{
			HostClaim:  NewHostclaim(HostclaimName).SetLabelSelector(map[string]string{"default-selector": "default-value"}).Build(),
			Namespaces: []*corev1.Namespace{hcNs, ns1},
			HostDeployPolicies: []*metal3api.HostDeployPolicy{
				NewHostdeploypolicy("hdp", "ns1").AcceptNames([]string{HostclaimNamespace}).Build()},
			BareMetalHosts:  []*metal3api.BareMetalHost{bmhns1, bmhns1BadLabel, bmhns1ConsOther, bmhns1NotAvail, bmhns1Paused},
			ExpectedBmhName: "bmh1",
		}),
		Entry("with criteriums (negative)", testCaseChooseBMH{
			HostClaim:  NewHostclaim(HostclaimName).SetLabelSelector(map[string]string{"default-selector": "default-value"}).Build(),
			Namespaces: []*corev1.Namespace{hcNs, ns1},
			HostDeployPolicies: []*metal3api.HostDeployPolicy{
				NewHostdeploypolicy("hdp", "ns1").AcceptNames([]string{HostclaimNamespace}).Build()},
			BareMetalHosts: []*metal3api.BareMetalHost{bmhns1BadLabel, bmhns1ConsOther, bmhns1NotAvail, bmhns1Paused},
		}),
		Entry("with bad label value", testCaseChooseBMH{
			HostClaim:  NewHostclaim(HostclaimName).SetLabelSelector(map[string]string{"default-selector": "other-value"}).Build(),
			Namespaces: []*corev1.Namespace{hcNs, ns1},
			HostDeployPolicies: []*metal3api.HostDeployPolicy{
				NewHostdeploypolicy("hdp", "ns1").AcceptNames([]string{HostclaimNamespace}).Build()},
			BareMetalHosts:  []*metal3api.BareMetalHost{bmhns1, bmhns1BadLabel, bmhns1ConsOther, bmhns1NotAvail, bmhns1Paused},
			ExpectedBmhName: "",
		}),
		Entry("with eroneous labels", testCaseChooseBMH{
			HostClaim:  NewHostclaim(HostclaimName).SetLabelSelector(map[string]string{"*/123": "default-value"}).Build(),
			Namespaces: []*corev1.Namespace{hcNs, ns1},
			HostDeployPolicies: []*metal3api.HostDeployPolicy{
				NewHostdeploypolicy("hdp", "ns1").AcceptNames([]string{HostclaimNamespace}).Build()},
			BareMetalHosts: []*metal3api.BareMetalHost{bmhns1BadLabel, bmhns1ConsOther, bmhns1NotAvail, bmhns1Paused},
			ExpectFail:     true,
		}),
		Entry("with consumerRef", testCaseChooseBMH{
			HostClaim:  NewHostclaim(HostclaimName).Build(),
			Namespaces: []*corev1.Namespace{hcNs, ns1},
			HostDeployPolicies: []*metal3api.HostDeployPolicy{
				NewHostdeploypolicy("hdp", "ns1").AcceptNames([]string{HostclaimNamespace}).Build()},
			BareMetalHosts:  []*metal3api.BareMetalHost{bmhns1Consumed, bmhns1, bmhns1ConsOther},
			ExpectedBmhName: "bmh-consumed",
		}),
		Entry("with expr (positive)", testCaseChooseBMH{
			HostClaim: NewHostclaim(HostclaimName).SetMatchExpressions(
				[]metal3api.HostSelectorRequirement{{
					Key: "default-selector", Values: []string{"other", "default-value"},
					Operator: selection.In}}).Build(),
			Namespaces: []*corev1.Namespace{hcNs, ns1},
			HostDeployPolicies: []*metal3api.HostDeployPolicy{
				NewHostdeploypolicy("hdp", "ns1").AcceptNames([]string{HostclaimNamespace}).Build()},
			BareMetalHosts:  []*metal3api.BareMetalHost{bmhns1, bmhns1BadLabel, bmhns1ConsOther, bmhns1NotAvail},
			ExpectedBmhName: "bmh1",
		}),
		Entry("with labeled hostclaim namespace", testCaseChooseBMH{
			HostClaim:  NewHostclaim(HostclaimName).Build(),
			Namespaces: []*corev1.Namespace{hcNsLabelled, ns1},
			HostDeployPolicies: []*metal3api.HostDeployPolicy{
				NewHostdeploypolicy("hdp", "ns1").AcceptLabels([]metal3api.NameValuePair{{Name: "l", Value: "v"}}).Build()},
			BareMetalHosts:  []*metal3api.BareMetalHost{bmhns1, bmhns2},
			ExpectedBmhName: "bmh1",
		}),
		Entry("with labeled hostclaim namespace (bad label)", testCaseChooseBMH{
			HostClaim:  NewHostclaim(HostclaimName).Build(),
			Namespaces: []*corev1.Namespace{hcNsLabelled, ns1},
			HostDeployPolicies: []*metal3api.HostDeployPolicy{
				NewHostdeploypolicy("hdp", "ns1").AcceptLabels([]metal3api.NameValuePair{{Name: "l", Value: "w"}}).Build()},
			BareMetalHosts:  []*metal3api.BareMetalHost{bmhns1, bmhns2},
			ExpectedBmhName: "",
		}),
		Entry("with labeled hostclaim namespace (negative)", testCaseChooseBMH{
			HostClaim:  NewHostclaim(HostclaimName).Build(),
			Namespaces: []*corev1.Namespace{hcNs, ns1},
			HostDeployPolicies: []*metal3api.HostDeployPolicy{
				NewHostdeploypolicy("hdp", "ns1").AcceptLabels([]metal3api.NameValuePair{{Name: "l", Value: "v"}}).Build()},
			BareMetalHosts: []*metal3api.BareMetalHost{bmhns1, bmhns2},
		}),
		Entry("with regexp on hostclaim namespace", testCaseChooseBMH{
			HostClaim:  NewHostclaim(HostclaimName).Build(),
			Namespaces: []*corev1.Namespace{hcNsLabelled, ns1},
			HostDeployPolicies: []*metal3api.HostDeployPolicy{
				NewHostdeploypolicy("hdp", "ns1").AcceptRegexp("hc.*").Build()},
			BareMetalHosts:  []*metal3api.BareMetalHost{bmhns1, bmhns2},
			ExpectedBmhName: "bmh1",
		}),
		Entry("with bad regexp on hostclaim namespace", testCaseChooseBMH{
			HostClaim:  NewHostclaim(HostclaimName).Build(),
			Namespaces: []*corev1.Namespace{hcNsLabelled, ns1},
			HostDeployPolicies: []*metal3api.HostDeployPolicy{
				NewHostdeploypolicy("hdp", "ns1").AcceptRegexp("hc[").Build()},
			BareMetalHosts:  []*metal3api.BareMetalHost{bmhns1, bmhns2},
			ExpectedBmhName: "",
			ExpectFail:      true,
		}),
		Entry("with Failure Domain (available bmh)", testCaseChooseBMH{
			HostClaim: NewHostclaim(HostclaimName).
				SetLabelSelector(map[string]string{"default-selector": "default-value"}).
				SetFailureDomain("zone").Build(),
			Namespaces: []*corev1.Namespace{hcNs, ns1},
			HostDeployPolicies: []*metal3api.HostDeployPolicy{
				NewHostdeploypolicy("hdp", "ns1").AcceptNames([]string{HostclaimNamespace}).Build()},
			BareMetalHosts:  []*metal3api.BareMetalHost{bmhns1, bmh2ns1FailureDomain},
			ExpectedBmhName: "bmh2",
		}),
		Entry("with Failure Domain (no available bmh in zone)", testCaseChooseBMH{
			HostClaim: NewHostclaim(HostclaimName).
				SetLabelSelector(map[string]string{"default-selector": "default-value"}).
				SetFailureDomain("zone").Build(),
			Namespaces: []*corev1.Namespace{hcNs, ns1},
			HostDeployPolicies: []*metal3api.HostDeployPolicy{
				NewHostdeploypolicy("hdp", "ns1").AcceptNames([]string{HostclaimNamespace}).Build()},
			BareMetalHosts:  []*metal3api.BareMetalHost{bmhns1},
			ExpectedBmhName: "bmh1",
		}),
	)

	type testCaseAssociate struct {
		HostClaim     *metal3api.HostClaim
		ExpectFails   bool
		ExpectRequeue bool
	}

	It("test hide conflict error",
		func() {
			ctx := context.TODO()
			bmh := NewBaremetalhost("bmh", "ns", metal3api.StateAvailable).Build()
			oldBmh := bmh.DeepCopy()
			fakeClient := fake.NewClientBuilder().WithScheme(setupScheme()).WithObjects(bmh).Build()
			bmh.Spec.Description = "v0"
			err := fakeClient.Update(ctx, bmh)
			Expect(err).NotTo(HaveOccurred())
			helper, err := patch.NewHelper(bmh, fakeClient)
			Expect(err).NotTo(HaveOccurred())
			bmh.Spec.Description = "v1"
			err = hideConflictError(helper.Patch(ctx, bmh))
			Expect(err).NotTo(HaveOccurred(), "Patch succeeds")
			helper, err = patch.NewHelper(oldBmh, fakeClient)
			Expect(err).NotTo(HaveOccurred())
			oldBmh.ResourceVersion = "234"
			oldBmh.Spec.Description = "v2"
			err = hideConflictError(helper.Patch(ctx, oldBmh))
			Expect(err).To(HaveOccurred(), "Conflict error becomes requeue")
			var requeueAfterError RequeueAfterError
			Expect(errors.As(err, &requeueAfterError)).To(BeTrue())
		})

	DescribeTable("test Associate",
		func(tc testCaseAssociate) {
			sec := NewSecret("sec-user-data", HostclaimNamespace).SetData(map[string][]byte{"user-data": []byte("v")}).Build()
			bmh := NewBaremetalhost("bmh", "ns", metal3api.StateAvailable).Build()
			objects := []client.Object{
				tc.HostClaim, bmh, sec,
				NewHostdeploypolicy("hdp", "ns").AcceptNames([]string{HostclaimNamespace}).Build(),
				NewNamespace("hcNs").Build(), NewNamespace("ns").Build(),
			}
			// We patch the status during associate to set the annotation.
			fakeClient := fake.NewClientBuilder().WithScheme(setupScheme()).WithObjects(objects...).WithStatusSubresource(tc.HostClaim).Build()
			hostMgr, err := NewHostManager(fakeClient, GinkgoLogr, tc.HostClaim, fakeClient)
			Expect(err).NotTo(HaveOccurred())
			err = hostMgr.Associate(context.TODO())
			if tc.ExpectFails {
				Expect(err).To(HaveOccurred())
				var requeueAfterError RequeueAfterError
				Expect(errors.As(err, &requeueAfterError)).To(Equal(tc.ExpectRequeue))
				return
			}
			Expect(err).NotTo(HaveOccurred())
			updatedBmh := &metal3api.BareMetalHost{}
			err = fakeClient.Get(context.TODO(), client.ObjectKeyFromObject(bmh), updatedBmh)
			Expect(err).NotTo(HaveOccurred())
			Expect(updatedBmh.Spec.ConsumerRef).ToNot(BeNil())
			Expect(updatedBmh.Spec.ConsumerRef.Name).To(Equal(tc.HostClaim.Name))
			Expect(updatedBmh.Spec.ConsumerRef.Namespace).To(Equal(tc.HostClaim.Namespace))
			Expect(tc.HostClaim.Status.BareMetalHost).ToNot(BeNil())
			Expect(tc.HostClaim.Status.BareMetalHost.Name).To(Equal(bmh.Name))
			Expect(tc.HostClaim.Status.BareMetalHost.Namespace).To(Equal(bmh.Namespace))
		},
		Entry("Regular case", testCaseAssociate{
			HostClaim: NewHostclaim(HostclaimName).SetImage(defaultImage).SetUserData("sec-user-data").Build(),
		}),
		Entry("Bad Selector, True failure", testCaseAssociate{
			HostClaim: NewHostclaim(HostclaimName).
				SetMatchExpressions([]metal3api.HostSelectorRequirement{{
					Key: "k", Operator: selection.Exists, Values: []string{"a", "b"}}}).Build(),
			ExpectFails: true,
		}),
		Entry("Incompatible selector", testCaseAssociate{
			HostClaim: NewHostclaim(HostclaimName).
				SetMatchExpressions([]metal3api.HostSelectorRequirement{{
					Key: "k", Operator: selection.Exists, Values: []string{}}}).Build(),
			ExpectFails:   true,
			ExpectRequeue: true,
		}),
	)

	type testCaseSetBMHSpec struct {
		UserData        *corev1.Secret
		NetworkData     *corev1.Secret
		MetaData        *corev1.Secret
		BMHUserData     *corev1.Secret
		BMHNetworkData  *corev1.Secret
		BMHMetaData     *corev1.Secret
		SetImage        bool
		SetCustomDeploy bool
		SetPoweredOn    bool
	}

	DescribeTable("Test setBMHspec",
		func(tc testCaseSetBMHSpec) {
			hcBuilder := NewHostclaim(HostclaimName)
			ctx := context.TODO()
			objects := []client.Object{}
			numSecrets := 0
			if tc.UserData != nil {
				hcBuilder = hcBuilder.SetUserData(tc.UserData.Name)
				if !strings.HasPrefix(tc.UserData.Name, "removed") {
					objects = append(objects, tc.UserData)
					numSecrets++
				}
			}
			if tc.MetaData != nil {
				hcBuilder = hcBuilder.SetMetaData(tc.MetaData.Name)
				if !strings.HasPrefix(tc.MetaData.Name, "removed") {
					objects = append(objects, tc.MetaData)
					numSecrets++
				}
			}
			if tc.NetworkData != nil {
				hcBuilder = hcBuilder.SetNetworkData(tc.NetworkData.Name)
				if !strings.HasPrefix(tc.NetworkData.Name, "removed") {
					objects = append(objects, tc.NetworkData)
					numSecrets++
				}
			}
			if tc.SetImage {
				hcBuilder = hcBuilder.SetImage(defaultImage)
			}
			if tc.SetCustomDeploy {
				hcBuilder = hcBuilder.SetCustomDeploy("custom")
			}
			hostClaim := hcBuilder.Build()
			bmhBuilder := NewBaremetalhost("bmh", "ns", metal3api.StateAvailable)
			if tc.BMHUserData != nil {
				bmhBuilder = bmhBuilder.SetUserData(tc.BMHUserData.Name)
				objects = append(objects, tc.BMHUserData)
			}
			if tc.BMHMetaData != nil {
				bmhBuilder = bmhBuilder.SetMetaData(tc.BMHMetaData.Name)
				objects = append(objects, tc.BMHMetaData)
			}
			if tc.BMHNetworkData != nil {
				bmhBuilder = bmhBuilder.SetNetworkData(tc.BMHNetworkData.Name)
				objects = append(objects, tc.BMHNetworkData)
			}
			bmh := bmhBuilder.Build()
			objects = append(objects, hostClaim, bmh)
			// Add secrets if they exists
			fakeClient := fake.NewClientBuilder().WithScheme(setupScheme()).WithObjects(objects...).Build()
			hostMgr, err := NewHostManager(fakeClient, GinkgoLogr, hostClaim, fakeClient)
			Expect(err).NotTo(HaveOccurred())
			updated, err := hostMgr.setBmhSpec(ctx, bmh)
			errorExpected := false
			var checkSecret = func(ref *corev1.SecretReference, source *corev1.Secret, message string) {
				if source == nil {
					Expect(ref).To(BeNil(), message)
				} else if strings.HasPrefix(source.Name, "removed") {
					Expect(ref).To(BeNil(), message)
					errorExpected = true
				} else {
					Expect(ref).NotTo(BeNil(), message)
					sec := &corev1.Secret{}
					key := client.ObjectKey{Name: ref.Name, Namespace: "ns"}
					err = fakeClient.Get(ctx, key, sec)
					Expect(err).NotTo(HaveOccurred(), message)
					Expect(reflect.DeepEqual(sec.Data, source.Data)).To(BeTrue(), message)
				}
			}
			checkSecret(bmh.Spec.UserData, tc.UserData, "userdata coherence")
			checkSecret(bmh.Spec.MetaData, tc.MetaData, "metadata coherence")
			checkSecret(bmh.Spec.NetworkData, tc.NetworkData, "networkdata coherence")
			if errorExpected {
				Expect(err).To(HaveOccurred())
			} else {
				Expect(err).NotTo(HaveOccurred())
			}
			secrets := &corev1.SecretList{}
			err = fakeClient.List(ctx, secrets, client.InNamespace("ns"))
			Expect(err).NotTo(HaveOccurred())
			Expect(secrets.Items).To(HaveLen(numSecrets))
			if tc.SetImage {
				Expect(bmh.Spec.Image).NotTo(BeNil())
				Expect(*bmh.Spec.Image).To(Equal(defaultImage))
			}
			if tc.SetCustomDeploy {
				Expect(bmh.Spec.CustomDeploy).NotTo(BeNil())
				Expect(bmh.Spec.CustomDeploy.Method).To(Equal("custom"))
			}
			if !errorExpected {
				Expect(updated).To(BeTrue())
				updated, _ = hostMgr.setBmhSpec(ctx, bmh)
				Expect(updated).To(BeFalse())
			}
		},
		Entry("set user-data (initialize)", testCaseSetBMHSpec{
			UserData: NewSecret("s1", HostclaimNamespace).SetData(map[string][]byte{"f": []byte("udt")}).Build(),
		}),
		Entry("set user-data (override)", testCaseSetBMHSpec{
			UserData:    NewSecret("s1", HostclaimNamespace).SetData(map[string][]byte{"f": []byte("udt")}).Build(),
			BMHUserData: NewSecret("bmh-userdata", "ns").SetData(map[string][]byte{"f": []byte("other")}).Build(),
		}),
		Entry("reset user-data (override)", testCaseSetBMHSpec{
			BMHUserData: NewSecret("bmh-userdata", "ns").SetData(map[string][]byte{"f": []byte("other")}).Build(),
		}),
		Entry("set meta-data/network-data (initialize)", testCaseSetBMHSpec{
			MetaData:    NewSecret("s1", HostclaimNamespace).SetData(map[string][]byte{"f": []byte("mdt")}).Build(),
			NetworkData: NewSecret("s2", HostclaimNamespace).SetData(map[string][]byte{"f": []byte("nwdt")}).Build(),
		}),
		Entry("set meta-data/network-data (overide)", testCaseSetBMHSpec{
			MetaData:       NewSecret("s1", HostclaimNamespace).SetData(map[string][]byte{"f": []byte("mdt")}).Build(),
			NetworkData:    NewSecret("s2", HostclaimNamespace).SetData(map[string][]byte{"f": []byte("nwdt")}).Build(),
			BMHMetaData:    NewSecret("bmh-metadata", "ns").SetData(map[string][]byte{"f": []byte("other")}).Build(),
			BMHNetworkData: NewSecret("bmh-networkdata", "ns").SetData(map[string][]byte{"f": []byte("other")}).Build(),
		}),
		Entry("reset meta-data/network-data (overide)", testCaseSetBMHSpec{
			BMHMetaData:    NewSecret("bmh-metadata", "ns").SetData(map[string][]byte{"f": []byte("other")}).Build(),
			BMHNetworkData: NewSecret("bmh-networkdata", "ns").SetData(map[string][]byte{"f": []byte("other")}).Build(),
		}),
		Entry("set meta-data (initialize/not yet available)", testCaseSetBMHSpec{
			MetaData: NewSecret("removed-secret", HostclaimNamespace).Build(),
		}),
		Entry("set image", testCaseSetBMHSpec{
			SetImage: true,
		}),
		Entry("set custom deploy", testCaseSetBMHSpec{
			SetCustomDeploy: true,
		}),
	)

	It("Test updateHostClaimStatus",
		func() {
			hostClaim := NewHostclaim(HostclaimName).SetAssociatedBMH("ns", "bmh").Build()
			bmh := NewBaremetalhost("bmh", "ns", metal3api.StateAvailable).
				SetCondition(metal3api.ProvisionedCondition, false, "reason").
				SetCondition(metal3api.AvailableForProvisioningCondition, true, "reason").
				Build()
			bmh.Status.PoweredOn = true
			fakeClient := fake.NewClientBuilder().WithScheme(setupScheme()).Build()
			hostMgr, err := NewHostManager(fakeClient, GinkgoLogr, hostClaim, fakeClient)
			Expect(err).NotTo(HaveOccurred())
			hostMgr.updateHostClaimStatus(bmh)
			Expect(hostClaim.Status.PoweredOn).To(BeTrue())
			Expect(hostClaim.Status.HardwareData).NotTo(BeNil())
			Expect(hostClaim.Status.HardwareData.Name).To(Equal("bmh"))
			Expect(hostClaim.Status.HardwareData.Namespace).To(Equal("ns"))
			Expect(conditions.IsFalse(hostClaim, metal3api.ProvisionedCondition)).To(BeTrue())
			Expect(conditions.IsTrue(hostClaim, metal3api.AvailableForProvisioningCondition)).To(BeTrue())
		})

	It("test syncReboot",
		func() {
			opt := map[string]string{"a": "w1"}
			saved := maps.Clone(opt)
			bmh := NewBaremetalhost("bmh", "ns", metal3api.StateAvailable).SetAnnotations(opt).Build()
			annot := rebootDomain + "/test"
			hostClaim := NewHostclaim(HostclaimName).SetAnnotations(map[string]string{annot: "v0"}).SetAssociatedBMH("ns", "bmh").Build()
			b := syncReboot(hostClaim.Annotations, bmh.Annotations)
			Expect(bmh.Annotations[annot]).To(Equal("v0"), "reboot propagated")
			Expect(b).To(BeTrue(), "change occurred")
			b = syncReboot(hostClaim.Annotations, bmh.Annotations)
			Expect(b).To(BeFalse(), "idempotent")
			// Remove reboot/stop Annotation
			delete(hostClaim.Annotations, annot)
			b = syncReboot(hostClaim.Annotations, bmh.Annotations)
			Expect(maps.Equal(bmh.Annotations, saved)).To(BeTrue(), "removal propagated")
			Expect(b).To(BeTrue(), "change occurred")
			b = syncReboot(hostClaim.Annotations, bmh.Annotations)
			Expect(b).To(BeFalse(), "idempotent")
			// Set transient reboot.
			hostClaim.Annotations[rebootDomain] = "v1"
			b = syncReboot(hostClaim.Annotations, bmh.Annotations)
			Expect(b).To(BeTrue(), "transient reboot propagated")
			// Check reboot propagated to save
			if saved == nil {
				saved = map[string]string{}
			}
			saved[rebootDomain] = "v1"
			Expect(maps.Equal(bmh.Annotations, saved)).To(BeTrue())
		},
	)

	type testCaseUpdate struct {
		HostClaim  *metal3api.HostClaim
		ExpectFail bool
	}
	DescribeTable("test Update",
		func(tc testCaseUpdate) {
			ctx := context.TODO()
			hc := tc.HostClaim
			bmh := NewBaremetalhost("bmh", "ns", metal3api.StateAvailable).SetConsumerRef(defaultConsumerRef).Build()
			objects := []client.Object{
				hc, bmh,
				NewHostdeploypolicy("hdp", "ns").AcceptNames([]string{HostclaimNamespace}).Build(),
				NewNamespace("hcNs").Build(), NewNamespace("ns").Build(),
			}
			fakeClient := fake.NewClientBuilder().WithScheme(setupScheme()).WithObjects(objects...).Build()
			hostMgr, err := NewHostManager(fakeClient, GinkgoLogr, hc, fakeClient)
			Expect(err).NotTo(HaveOccurred())
			err = hostMgr.Update(ctx)
			if tc.ExpectFail {
				Expect(err).To(HaveOccurred())
			} else {
				Expect(err).NotTo(HaveOccurred())
			}
		},
		Entry("Regular case", testCaseUpdate{HostClaim: NewHostclaim(HostclaimName).SetAssociatedBMH("ns", "bmh").Build()}),
		Entry("badly associated case", testCaseUpdate{HostClaim: NewHostclaim("other").SetAssociatedBMH("ns", "bmh").Build(), ExpectFail: true}),
		Entry("no bmh", testCaseUpdate{HostClaim: NewHostclaim(HostclaimName).SetAssociatedBMH("ns", "other").Build(), ExpectFail: true}),
	)

})

func TestManagers(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(t, "Manager Suite")
}
