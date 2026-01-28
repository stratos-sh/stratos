//go:build integration
// +build integration

/*
Copyright 2026 Stratos Authors.

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

package integration

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	stratosv1alpha1 "github.com/stratos-sh/stratos/api/v1alpha1"
)

var _ = Describe("AWSNodeClass Resolution", func() {
	Context("Static resource IDs", func() {
		It("should populate resolved status fields and set all conditions True", func() {
			// Create an AWSNodeClass with static IDs
			nc := &stratosv1alpha1.AWSNodeClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "static-ids-test",
				},
				Spec: stratosv1alpha1.AWSNodeClassSpec{
					Region:             "us-east-1",
					InstanceType:       "m5.large",
					AMI:                "ami-12345678",
					SubnetIDs:          []string{"subnet-aaa", "subnet-bbb"},
					SecurityGroupIDs:   []string{"sg-xxx"},
					IAMInstanceProfile: "arn:aws:iam::123456789012:instance-profile/test-profile",
				},
			}
			err := k8sClient.Create(ctx, nc)
			Expect(err).NotTo(HaveOccurred())

			// Wait for AWSNodeClass reconciler to resolve and set status
			Eventually(func() string {
				updated := &stratosv1alpha1.AWSNodeClass{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: nc.Name}, updated)
				if err != nil {
					return ""
				}
				return updated.Status.ResolvedAMI
			}, timeout, interval).Should(Equal("ami-12345678"))

			// Verify all resolved status fields
			updated := &stratosv1alpha1.AWSNodeClass{}
			err = k8sClient.Get(ctx, types.NamespacedName{Name: nc.Name}, updated)
			Expect(err).NotTo(HaveOccurred())

			Expect(updated.Status.ResolvedAMI).To(Equal("ami-12345678"))
			Expect(updated.Status.ResolvedSubnets).To(HaveLen(2))
			Expect(updated.Status.ResolvedSubnets[0].ID).To(Equal("subnet-aaa"))
			Expect(updated.Status.ResolvedSubnets[1].ID).To(Equal("subnet-bbb"))
			Expect(updated.Status.ResolvedSecurityGroups).To(HaveLen(1))
			Expect(updated.Status.ResolvedSecurityGroups[0].ID).To(Equal("sg-xxx"))
			Expect(updated.Status.ResolvedInstanceProfile).To(Equal("arn:aws:iam::123456789012:instance-profile/test-profile"))

			// Verify all conditions are True
			expectConditionTrue(updated, stratosv1alpha1.AWSNodeClassConditionTypeAMIReady)
			expectConditionTrue(updated, stratosv1alpha1.AWSNodeClassConditionTypeSubnetsReady)
			expectConditionTrue(updated, stratosv1alpha1.AWSNodeClassConditionTypeSecurityGroupsReady)
			expectConditionTrue(updated, stratosv1alpha1.AWSNodeClassConditionTypeInstanceProfileReady)
		})
	})

	Context("Selector-based resolution", func() {
		It("should resolve via FakeResolver, populate status, and set conditions True", func() {
			nc := &stratosv1alpha1.AWSNodeClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "selector-test",
				},
				Spec: stratosv1alpha1.AWSNodeClassSpec{
					Region:       "us-east-1",
					InstanceType: "m5.large",
					AMISelector: &stratosv1alpha1.AMISelector{
						Tags: map[string]string{"env": "test"},
					},
					SubnetSelector: &stratosv1alpha1.SubnetSelector{
						Tags: map[string]string{"kubernetes.io/cluster/test": "owned"},
					},
					SecurityGroupSelector: &stratosv1alpha1.SecurityGroupSelector{
						Tags: map[string]string{"app": "stratos"},
					},
					Role: "test-node-role",
				},
			}
			err := k8sClient.Create(ctx, nc)
			Expect(err).NotTo(HaveOccurred())

			// Wait for resolution to complete
			Eventually(func() string {
				updated := &stratosv1alpha1.AWSNodeClass{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: nc.Name}, updated)
				if err != nil {
					return ""
				}
				return updated.Status.ResolvedAMI
			}, timeout, interval).Should(Equal("ami-fake-12345"))

			// Verify resolved status from FakeResolver defaults
			updated := &stratosv1alpha1.AWSNodeClass{}
			err = k8sClient.Get(ctx, types.NamespacedName{Name: nc.Name}, updated)
			Expect(err).NotTo(HaveOccurred())

			Expect(updated.Status.ResolvedAMI).To(Equal("ami-fake-12345"))
			Expect(updated.Status.ResolvedSubnets).To(HaveLen(2))
			Expect(updated.Status.ResolvedSubnets[0].ID).To(Equal("subnet-fake-aaa"))
			Expect(updated.Status.ResolvedSecurityGroups).To(HaveLen(1))
			Expect(updated.Status.ResolvedSecurityGroups[0].ID).To(Equal("sg-fake-xxx"))
			Expect(updated.Status.ResolvedInstanceProfile).To(Equal("arn:aws:iam::123456789012:instance-profile/fake-profile"))

			// Verify all conditions are True
			expectConditionTrue(updated, stratosv1alpha1.AWSNodeClassConditionTypeAMIReady)
			expectConditionTrue(updated, stratosv1alpha1.AWSNodeClassConditionTypeSubnetsReady)
			expectConditionTrue(updated, stratosv1alpha1.AWSNodeClassConditionTypeSecurityGroupsReady)
			expectConditionTrue(updated, stratosv1alpha1.AWSNodeClassConditionTypeInstanceProfileReady)
		})
	})

	Context("Selector resolves to nothing", func() {
		It("should set condition False and block NodePool launch", func() {
			// Configure FakeResolver to return an error (simulating no match)
			fakeResolver.ResolveSubnetsHook = func(ctx context.Context, selector *stratosv1alpha1.SubnetSelector) ([]stratosv1alpha1.ResolvedSubnet, error) {
				return nil, fmt.Errorf("no subnets matching selector")
			}
			defer func() { fakeResolver.ResolveSubnetsHook = nil }()

			nc := &stratosv1alpha1.AWSNodeClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "no-match-test",
				},
				Spec: stratosv1alpha1.AWSNodeClassSpec{
					Region:       "us-east-1",
					InstanceType: "m5.large",
					AMI:          "ami-12345678",
					SubnetSelector: &stratosv1alpha1.SubnetSelector{
						Tags: map[string]string{"nonexistent": "tag"},
					},
					SecurityGroupIDs:   []string{"sg-xxx"},
					IAMInstanceProfile: "test-profile",
				},
			}
			err := k8sClient.Create(ctx, nc)
			Expect(err).NotTo(HaveOccurred())

			// Wait for SubnetsReady condition to be False
			Eventually(func() bool {
				updated := &stratosv1alpha1.AWSNodeClass{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: nc.Name}, updated)
				if err != nil {
					return false
				}
				return hasConditionWithStatus(updated, stratosv1alpha1.AWSNodeClassConditionTypeSubnetsReady, metav1.ConditionFalse)
			}, timeout, interval).Should(BeTrue(), "Expected SubnetsReady condition to be False")

			// Create a NodePool referencing this AWSNodeClass
			np := createTestNodePoolWithNodeClass("no-match-pool", 3, 1, nc.Name)
			Expect(np).NotTo(BeNil())

			time.Sleep(500 * time.Millisecond)
			triggerReconcile(np.Name)

			// Verify NodePool has Degraded condition with reason NodeClassNotReady
			Eventually(func() bool {
				updated := &stratosv1alpha1.NodePool{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: np.Name}, updated)
				if err != nil {
					return false
				}
				for _, cond := range updated.Status.Conditions {
					if cond.Type == stratosv1alpha1.ConditionTypeDegraded &&
						cond.Status == metav1.ConditionTrue &&
						cond.Reason == "NodeClassNotReady" {
						return true
					}
				}
				return false
			}, timeout, interval).Should(BeTrue(), "Expected NodePool Degraded=True with reason NodeClassNotReady")
		})
	})

	Context("Transient resolution failure", func() {
		It("should preserve last-known-good values and keep condition True", func() {
			// First, create an AWSNodeClass with selectors that resolve successfully
			nc := &stratosv1alpha1.AWSNodeClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "transient-fail-test",
				},
				Spec: stratosv1alpha1.AWSNodeClassSpec{
					Region:       "us-east-1",
					InstanceType: "m5.large",
					AMISelector: &stratosv1alpha1.AMISelector{
						Tags: map[string]string{"env": "test"},
					},
					SubnetSelector: &stratosv1alpha1.SubnetSelector{
						Tags: map[string]string{"env": "test"},
					},
					SecurityGroupSelector: &stratosv1alpha1.SecurityGroupSelector{
						Tags: map[string]string{"env": "test"},
					},
					Role: "test-role",
				},
			}
			err := k8sClient.Create(ctx, nc)
			Expect(err).NotTo(HaveOccurred())

			// Wait for initial resolution to succeed
			Eventually(func() string {
				updated := &stratosv1alpha1.AWSNodeClass{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: nc.Name}, updated)
				if err != nil {
					return ""
				}
				return updated.Status.ResolvedAMI
			}, timeout, interval).Should(Equal("ami-fake-12345"))

			// Now inject a transient failure for AMI resolution
			fakeResolver.ResolveAMIHook = func(ctx context.Context, selector *stratosv1alpha1.AMISelector) (string, error) {
				return "", fmt.Errorf("transient AWS API error")
			}
			defer func() { fakeResolver.ResolveAMIHook = nil }()

			// Trigger re-reconciliation
			triggerNodeClassReconcile(nc.Name)

			// Wait a bit for reconciliation
			time.Sleep(1 * time.Second)

			// Verify the resolved AMI is preserved (last-known-good)
			updated := &stratosv1alpha1.AWSNodeClass{}
			err = k8sClient.Get(ctx, types.NamespacedName{Name: nc.Name}, updated)
			Expect(err).NotTo(HaveOccurred())
			Expect(updated.Status.ResolvedAMI).To(Equal("ami-fake-12345"), "Last-known-good AMI should be preserved")

			// The AMIReady condition should still be True (cached value still valid)
			expectConditionTrue(updated, stratosv1alpha1.AWSNodeClassConditionTypeAMIReady)
		})
	})

	Context("NodePool readiness check", func() {
		It("should check AWSNodeClass readiness conditions before launching", func() {
			// Configure FakeResolver to fail SecurityGroup resolution
			fakeResolver.ResolveSecurityGroupsHook = func(ctx context.Context, selector *stratosv1alpha1.SecurityGroupSelector) ([]stratosv1alpha1.ResolvedSecurityGroup, error) {
				return nil, fmt.Errorf("no security groups found")
			}
			defer func() { fakeResolver.ResolveSecurityGroupsHook = nil }()

			nc := &stratosv1alpha1.AWSNodeClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "readiness-check-class",
				},
				Spec: stratosv1alpha1.AWSNodeClassSpec{
					Region:       "us-east-1",
					InstanceType: "m5.large",
					AMI:          "ami-12345678",
					SubnetIDs:    []string{"subnet-aaa"},
					SecurityGroupSelector: &stratosv1alpha1.SecurityGroupSelector{
						Tags: map[string]string{"missing": "tag"},
					},
					IAMInstanceProfile: "test-profile",
				},
			}
			err := k8sClient.Create(ctx, nc)
			Expect(err).NotTo(HaveOccurred())

			// Wait for SecurityGroupsReady=False
			Eventually(func() bool {
				updated := &stratosv1alpha1.AWSNodeClass{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: nc.Name}, updated)
				if err != nil {
					return false
				}
				return hasConditionWithStatus(updated, stratosv1alpha1.AWSNodeClassConditionTypeSecurityGroupsReady, metav1.ConditionFalse)
			}, timeout, interval).Should(BeTrue())

			// Create NodePool referencing the unready AWSNodeClass
			np := createTestNodePoolWithNodeClass("readiness-check-pool", 3, 1, nc.Name)

			time.Sleep(500 * time.Millisecond)
			triggerReconcile(np.Name)

			// NodePool should be degraded with NodeClassNotReady
			Eventually(func() bool {
				updated := &stratosv1alpha1.NodePool{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: np.Name}, updated)
				if err != nil {
					return false
				}
				for _, cond := range updated.Status.Conditions {
					if cond.Type == stratosv1alpha1.ConditionTypeDegraded &&
						cond.Status == metav1.ConditionTrue &&
						cond.Reason == "NodeClassNotReady" {
						return true
					}
				}
				return false
			}, timeout, interval).Should(BeTrue(), "Expected NodePool Degraded with NodeClassNotReady")

			// Verify no instances were launched
			Expect(countNodesForPool(np.Name)).To(Equal(0))
		})
	})

	Context("Instance profile cleanup finalizer", func() {
		It("should clean up instance profile on AWSNodeClass deletion after in-use finalizer", func() {
			var deleteProfileCalled bool
			fakeResolver.DeleteInstanceProfileHook = func(ctx context.Context, profileName string) error {
				deleteProfileCalled = true
				Expect(profileName).To(Equal(fmt.Sprintf("stratos-%s-finalizer-ip-class", testClusterName)))
				return nil
			}
			defer func() { fakeResolver.DeleteInstanceProfileHook = nil }()

			nc := &stratosv1alpha1.AWSNodeClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "finalizer-ip-class",
				},
				Spec: stratosv1alpha1.AWSNodeClassSpec{
					Region:       "us-east-1",
					InstanceType: "m5.large",
					AMI:          "ami-12345678",
					SubnetIDs:    []string{"subnet-aaa"},
					SecurityGroupIDs: []string{"sg-xxx"},
					Role:         "test-node-role",
				},
			}
			err := k8sClient.Create(ctx, nc)
			Expect(err).NotTo(HaveOccurred())

			// Wait for instance-profile finalizer to be added
			Eventually(func() bool {
				updated := &stratosv1alpha1.AWSNodeClass{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: nc.Name}, updated)
				if err != nil {
					return false
				}
				return controllerutil.ContainsFinalizer(updated, stratosv1alpha1.AWSNodeClassFinalizerInstanceProfile)
			}, timeout, interval).Should(BeTrue(), "Expected instance-profile finalizer to be added")

			// Delete the AWSNodeClass
			err = k8sClient.Delete(ctx, nc)
			Expect(err).NotTo(HaveOccurred())

			// The AWSNodeClass should eventually be fully deleted
			// (the instance-profile finalizer should be processed since there's no in-use finalizer)
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: nc.Name}, &stratosv1alpha1.AWSNodeClass{})
				return err != nil // Not found = deleted
			}, timeout, interval).Should(BeTrue(), "Expected AWSNodeClass to be deleted after finalizer cleanup")

			Expect(deleteProfileCalled).To(BeTrue(), "Expected DeleteInstanceProfile to be called")
		})

		It("should wait for in-use finalizer before processing instance-profile finalizer", func() {
			nc := &stratosv1alpha1.AWSNodeClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "finalizer-order-class",
				},
				Spec: stratosv1alpha1.AWSNodeClassSpec{
					Region:           "us-east-1",
					InstanceType:     "m5.large",
					AMI:              "ami-12345678",
					SubnetIDs:        []string{"subnet-aaa"},
					SecurityGroupIDs: []string{"sg-xxx"},
					Role:             "test-role",
				},
			}
			err := k8sClient.Create(ctx, nc)
			Expect(err).NotTo(HaveOccurred())

			// Wait for instance-profile finalizer
			Eventually(func() bool {
				updated := &stratosv1alpha1.AWSNodeClass{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: nc.Name}, updated)
				if err != nil {
					return false
				}
				return controllerutil.ContainsFinalizer(updated, stratosv1alpha1.AWSNodeClassFinalizerInstanceProfile)
			}, timeout, interval).Should(BeTrue())

			// Create a NodePool to add the in-use finalizer
			np := createTestNodePoolWithNodeClass("finalizer-order-pool", 3, 1, nc.Name)
			time.Sleep(500 * time.Millisecond)
			triggerReconcile(np.Name)

			// Wait for in-use finalizer to be added
			Eventually(func() bool {
				updated := &stratosv1alpha1.AWSNodeClass{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: nc.Name}, updated)
				if err != nil {
					return false
				}
				return controllerutil.ContainsFinalizer(updated, stratosv1alpha1.AWSNodeClassFinalizerInUse)
			}, timeout, interval).Should(BeTrue())

			// Delete the AWSNodeClass
			err = k8sClient.Delete(ctx, nc)
			Expect(err).NotTo(HaveOccurred())

			// AWSNodeClass should still exist because in-use finalizer is present
			time.Sleep(1 * time.Second)
			updated := &stratosv1alpha1.AWSNodeClass{}
			err = k8sClient.Get(ctx, types.NamespacedName{Name: nc.Name}, updated)
			Expect(err).NotTo(HaveOccurred(), "AWSNodeClass should still exist while in-use finalizer is present")
			Expect(updated.DeletionTimestamp).NotTo(BeNil())
			Expect(controllerutil.ContainsFinalizer(updated, stratosv1alpha1.AWSNodeClassFinalizerInstanceProfile)).To(BeTrue(),
				"Instance-profile finalizer should not be processed while in-use finalizer exists")
		})
	})

	// CEL validation tests - these test admission-time validation.
	// Note: envtest may not enforce CEL validation rules depending on the version
	// of the API server. These tests verify the behavior when CEL is enforced.
	Context("CEL validation", func() {
		It("should reject both ami and amiSelector set simultaneously", func() {
			nc := &stratosv1alpha1.AWSNodeClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "cel-both-ami-test",
				},
				Spec: stratosv1alpha1.AWSNodeClassSpec{
					Region:       "us-east-1",
					InstanceType: "m5.large",
					AMI:          "ami-12345678",
					AMISelector: &stratosv1alpha1.AMISelector{
						Tags: map[string]string{"env": "test"},
					},
					SubnetIDs:          []string{"subnet-aaa"},
					SecurityGroupIDs:   []string{"sg-xxx"},
					IAMInstanceProfile: "test-profile",
				},
			}
			err := k8sClient.Create(ctx, nc)
			// CEL validation should reject this - if envtest supports CEL
			if err != nil {
				Expect(err.Error()).To(ContainSubstring("mutually exclusive"))
			} else {
				// If envtest doesn't enforce CEL, clean up and skip
				_ = k8sClient.Delete(ctx, nc)
				Skip("envtest does not enforce CEL validation rules")
			}
		})

		It("should reject neither ami nor amiSelector set", func() {
			nc := &stratosv1alpha1.AWSNodeClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "cel-neither-ami-test",
				},
				Spec: stratosv1alpha1.AWSNodeClassSpec{
					Region:             "us-east-1",
					InstanceType:       "m5.large",
					SubnetIDs:          []string{"subnet-aaa"},
					SecurityGroupIDs:   []string{"sg-xxx"},
					IAMInstanceProfile: "test-profile",
				},
			}
			err := k8sClient.Create(ctx, nc)
			// CEL validation should reject this - if envtest supports CEL
			if err != nil {
				Expect(err.Error()).To(ContainSubstring("one of ami or amiSelector is required"))
			} else {
				// If envtest doesn't enforce CEL, clean up and skip
				_ = k8sClient.Delete(ctx, nc)
				Skip("envtest does not enforce CEL validation rules")
			}
		})
	})
})

// triggerNodeClassReconcile forces a reconciliation for an AWSNodeClass by updating an annotation.
func triggerNodeClassReconcile(name string) {
	nc := &stratosv1alpha1.AWSNodeClass{}
	err := k8sClient.Get(ctx, types.NamespacedName{Name: name}, nc)
	Expect(err).NotTo(HaveOccurred())

	patch := nc.DeepCopy()
	if patch.Annotations == nil {
		patch.Annotations = make(map[string]string)
	}
	patch.Annotations["test.stratos.sh/trigger"] = fmt.Sprintf("%d", time.Now().UnixNano())
	err = k8sClient.Patch(ctx, patch, client.MergeFrom(nc))
	Expect(err).NotTo(HaveOccurred())
}

// expectConditionTrue asserts that the given condition type is True on the AWSNodeClass.
func expectConditionTrue(nc *stratosv1alpha1.AWSNodeClass, condType string) {
	found := false
	for _, cond := range nc.Status.Conditions {
		if cond.Type == condType {
			Expect(cond.Status).To(Equal(metav1.ConditionTrue),
				"Expected condition %s to be True, got %s: %s", condType, cond.Status, cond.Message)
			found = true
			break
		}
	}
	Expect(found).To(BeTrue(), "Expected condition %s to be present", condType)
}

// hasConditionWithStatus checks if an AWSNodeClass has a condition with the given type and status.
func hasConditionWithStatus(nc *stratosv1alpha1.AWSNodeClass, condType string, status metav1.ConditionStatus) bool {
	for _, cond := range nc.Status.Conditions {
		if cond.Type == condType && cond.Status == status {
			return true
		}
	}
	return false
}
