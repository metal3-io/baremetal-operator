//go:build e2e
// +build e2e

package e2e

import (
	"context"
	"path"
	"time"

	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/cluster-api/test/framework"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func checkNoAssociation(specName string, hostClaim *metal3api.HostClaim) {
	Consistently(func(g Gomega) {
		currentHC := metal3api.HostClaim{}
		key := client.ObjectKeyFromObject(hostClaim)
		g.Expect(clusterProxy.GetClient().Get(ctx, key, &currentHC)).To(Succeed())
		g.Expect(currentHC.Status.BareMetalHost).To(BeNil())
	}, e2eConfig.GetIntervals(specName, "wait-associated")...).Should(Succeed())
}

func createEmptyHostclaim(namespace string) *metal3api.HostClaim {
	hostClaim := &metal3api.HostClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "claim",
			Namespace: namespace,
		},
		Spec: metal3api.HostClaimSpec{
			HostSelector: metal3api.HostSelector{
				MatchLabels: map[string]string{selectorLabel: selectorValue},
			},
		},
	}
	err := clusterProxy.GetClient().Create(ctx, hostClaim)
	Expect(err).NotTo(HaveOccurred())
	return hostClaim
}

var _ = Describe("Restrict HostClaims association with HostDeployPolicies.", Label("required", "hostclaim", "hostdeploypolicy"),
	func() {
		var (
			specName        = "hdp"
			secretName      = "bmc-credentials"
			namespaceBMH    *corev1.Namespace
			namespaceClaimA *corev1.Namespace
			namespaceClaimB *corev1.Namespace
			cancelWatches   context.CancelFunc
			toCleanupBMH    []client.Object
			toCleanupClaimA []client.Object
			toCleanupClaimB []client.Object
		)

		BeforeEach(func() {
			toCleanupBMH = nil
			toCleanupClaimA = nil
			toCleanupClaimB = nil
			namespaceBMH, cancelWatches = framework.CreateNamespaceAndWatchEvents(ctx, framework.CreateNamespaceAndWatchEventsInput{
				Creator:             clusterProxy.GetClient(),
				ClientSet:           clusterProxy.GetClientSet(),
				Name:                specName + "-infra",
				LogFolder:           artifactFolder,
				IgnoreAlreadyExists: true,
			})
			namespaceClaimA = framework.CreateNamespace(ctx, framework.CreateNamespaceInput{
				Creator:             clusterProxy.GetClient(),
				Name:                specName + "-tenanta",
				Labels:              map[string]string{"tenant": "a"},
				IgnoreAlreadyExists: true,
			})
			namespaceClaimB = framework.CreateNamespace(ctx, framework.CreateNamespaceInput{
				Creator:             clusterProxy.GetClient(),
				Name:                specName + "-tenantb",
				Labels:              map[string]string{"tenant": "b"},
				IgnoreAlreadyExists: true,
			})
		})

		It("Create Policies and check association", func() {
			By("Creating a secret with BMH credentials")
			bmcCredentialsData := map[string]string{
				"username": bmc.User,
				"password": bmc.Password,
			}
			secret := CreateSecret(ctx, clusterProxy.GetClient(), namespaceBMH.Name, secretName, bmcCredentialsData)
			toCleanupBMH = append(toCleanupBMH, secret)

			By("Creating a BMH with inspection disabled and hardware details added")
			hardwareDetails := hardwareDetailsFor(&bmc)
			bmh := createBmh("bmh", namespaceBMH.Name, map[string]string{selectorLabel: selectorValue}, hardwareDetails)
			err := clusterProxy.GetClient().Create(ctx, bmh)
			Expect(err).NotTo(HaveOccurred())
			toCleanupBMH = append(toCleanupBMH, bmh)

			By("Waiting for the BMH to become available")
			WaitForBmhInProvisioningState(ctx, WaitForBmhInProvisioningStateInput{
				Client: clusterProxy.GetClient(),
				Bmh:    *bmh,
				State:  metal3api.StateAvailable,
			}, e2eConfig.GetIntervals(specName, "wait-available")...)

			By("Creating hostDeployPolicies on namespace")
			hostDeployPolicy := createHostDeployPolicy(namespaceBMH.Name, namespaceClaimB.Name)
			toCleanupBMH = append(toCleanupBMH, hostDeployPolicy)
			for _, input := range []struct {
				title                string
				hostDeployPolicySpec metal3api.HostDeployPolicySpec
				AuthorizeA           bool // Used to alternate namespace that binds.
			}{
				{
					title: "Check Namespace selection on explicit list (authorize B)",
					hostDeployPolicySpec: metal3api.HostDeployPolicySpec{
						HostClaimNamespaces: &metal3api.HostClaimNamespaces{
							Names: []string{namespaceClaimB.Name, "other"},
						},
					},
				},
				{
					title: "Check Namespace selection on labels (authorize A)",
					hostDeployPolicySpec: metal3api.HostDeployPolicySpec{
						HostClaimNamespaces: &metal3api.HostClaimNamespaces{
							HasLabels: []metal3api.NameValuePair{
								{Name: "tenant", Value: "a"},
							},
						},
					},
					AuthorizeA: true,
				},
				{
					title: "Check Namespace selection on regular expression (authorize B)",
					hostDeployPolicySpec: metal3api.HostDeployPolicySpec{
						HostClaimNamespaces: &metal3api.HostClaimNamespaces{
							NameMatches: "b$",
						},
					},
				},
			} {
				By(input.title)
				hostDeployPolicy.Spec = input.hostDeployPolicySpec
				err = clusterProxy.GetClient().Update(ctx, hostDeployPolicy)
				Expect(err).NotTo(HaveOccurred())
				// For performance reason, the HostClaim controller reads
				// HostDeployPolicies from its informer cache.
				// Wait before creating claims, otherwise the controller may
				// reconcile the new claim against the previous policy.
				time.Sleep(3 * time.Second)
				var bindNamespace, rejectNamespace *corev1.Namespace
				if input.AuthorizeA {
					bindNamespace = namespaceClaimA
					rejectNamespace = namespaceClaimB
				} else {
					bindNamespace = namespaceClaimB
					rejectNamespace = namespaceClaimA
				}

				By("Creating rejected HostClaim and checking it cannot bind")
				rejectedClaim := createEmptyHostclaim(rejectNamespace.Name)
				if input.AuthorizeA {
					toCleanupClaimB = append(toCleanupClaimB, rejectedClaim)
				} else {
					toCleanupClaimA = append(toCleanupClaimA, rejectedClaim)
				}
				checkNoAssociation(specName, rejectedClaim)

				By("Creating authorized HostClaim and checking association succeeds")
				boundClaim := createEmptyHostclaim(bindNamespace.Name)
				if input.AuthorizeA {
					toCleanupClaimA = append(toCleanupClaimA, boundClaim)
				} else {
					toCleanupClaimB = append(toCleanupClaimB, boundClaim)
				}
				checkAssociation(specName, boundClaim, bmh)

				By("Deleting the hostclaims")
				Expect(clusterProxy.GetClient().Delete(ctx, rejectedClaim)).To(Succeed())
				Expect(clusterProxy.GetClient().Delete(ctx, boundClaim)).To(Succeed())
				WaitForHostClaimDeleted(ctx, clusterProxy.GetClient(), rejectedClaim, e2eConfig.GetIntervals(specName, "wait-deleted")...)
				WaitForHostClaimDeleted(ctx, clusterProxy.GetClient(), boundClaim, e2eConfig.GetIntervals(specName, "wait-deleted")...)
				checkNoConsumer(specName, bmh)
			}
		})

		AfterEach(func() {
			DumpResources(ctx, e2eConfig, clusterProxy, path.Join(artifactFolder, specName))
			if !skipCleanup {
				Cleanup(ctx, clusterProxy, namespaceBMH, cancelWatches, e2eConfig, toCleanupBMH)
				Cleanup(ctx, clusterProxy, namespaceClaimA, nil, e2eConfig, toCleanupClaimA)
				Cleanup(ctx, clusterProxy, namespaceClaimB, nil, e2eConfig, toCleanupClaimB)
			}
		})
	},
)
