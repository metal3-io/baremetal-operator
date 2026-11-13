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
	"time"

	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	. "github.com/metal3-io/baremetal-operator/internal/testutil"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	hdpName      = "hdp"
	hdpNamespace = "hdp-ns"
)

var _ = Describe("Test HostDeployPolicy Controller", func() {
	var (
		ctx   context.Context
		req   ctrl.Request
		key   types.NamespacedName
		recon HostDeployPolicyReconciler
	)

	BeforeEach(func() {
		ctx = context.TODO()
		key = types.NamespacedName{Namespace: hdpNamespace, Name: hdpName}
		req = ctrl.Request{NamespacedName: key}
	})

	build := func(hdp *metal3api.HostDeployPolicy) client.Client {
		scheme := setupScheme()
		b := fake.NewClientBuilder().WithScheme(scheme)
		if hdp != nil {
			b = b.WithObjects(hdp).WithStatusSubresource(hdp)
		}
		fakeClient := b.Build()
		recon = HostDeployPolicyReconciler{Client: fakeClient, Log: GinkgoLogr}
		return fakeClient
	}

	It("Adds the finalizer and set observed Generation on first pass", func() {
		hdp := NewHostdeploypolicy(hdpName, hdpNamespace).AcceptNames([]string{"tenant"}).Build()
		hdp.Generation = 1
		cli := build(hdp)

		_, err := recon.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())

		got := &metal3api.HostDeployPolicy{}
		Expect(cli.Get(ctx, key, got)).To(Succeed())
		Expect(got.Finalizers).To(ContainElement(metal3api.HostDeployPolicyFinalizer))
		Expect(got.Status.ObservedGeneration).To(Equal(got.Generation))
	})

	It("Patches ObservedGeneration once the finalizer is in place", func() {
		hdp := NewHostdeploypolicy(hdpName, hdpNamespace).AcceptNames([]string{"tenant"}).AddFinalizer().Build()
		hdp.Generation = 3
		cli := build(hdp)

		result, err := recon.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal(ctrl.Result{}))

		got := &metal3api.HostDeployPolicy{}
		Expect(cli.Get(ctx, key, got)).To(Succeed())
		Expect(got.Status.ObservedGeneration).To(Equal(int64(3)))
	})

	It("Is a no-op when ObservedGeneration already matches", func() {
		hdp := NewHostdeploypolicy(hdpName, hdpNamespace).AddFinalizer().Build()
		hdp.Generation = 5
		hdp.Status.ObservedGeneration = 5
		cli := build(hdp)

		result, err := recon.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal(ctrl.Result{}))

		got := &metal3api.HostDeployPolicy{}
		Expect(cli.Get(ctx, key, got)).To(Succeed())
		Expect(got.Status.ObservedGeneration).To(Equal(int64(5)))
		// ResourceVersion unchanged proves no write happened.
		Expect(got.ResourceVersion).To(Equal(hdp.ResourceVersion))
	})

	It("Returns no error when the HostDeployPolicy is gone", func() {
		build(nil)
		result, err := recon.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal(ctrl.Result{}))
	})

	It("Catches up when the spec is bumped again", func() {
		hdp := NewHostdeploypolicy(hdpName, hdpNamespace).AddFinalizer().Build()
		hdp.Generation = 1
		cli := build(hdp)

		_, err := recon.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())

		got := &metal3api.HostDeployPolicy{}
		Expect(cli.Get(ctx, key, got)).To(Succeed())
		Expect(got.Status.ObservedGeneration).To(Equal(int64(1)))

		got.Generation = 2
		Expect(cli.Update(ctx, got)).To(Succeed())

		_, err = recon.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())

		Expect(cli.Get(ctx, key, got)).To(Succeed())
		Expect(got.Status.ObservedGeneration).To(Equal(int64(2)))
	})

	It("Removes the finalizer when the HostDeployPolicy is being deleted", func() {
		now := metav1.NewTime(time.Now())
		hdp := NewHostdeploypolicy(hdpName, hdpNamespace).AddFinalizer().AcceptNames([]string{"tenant"}).Build()
		hdp.DeletionTimestamp = &now
		cli := build(hdp)

		_, err := recon.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())

		// With the only finalizer gone, the fake client removes the object.
		got := &metal3api.HostDeployPolicy{}
		err = cli.Get(ctx, key, got)
		Expect(err).To(HaveOccurred())
	})
})
