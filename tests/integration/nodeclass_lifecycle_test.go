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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	stratosv1alpha1 "github.com/stratos-sh/stratos/api/v1alpha1"
)

var _ = Describe("AWSNodeClass Lifecycle", func() {
	Context("When NodePool references an AWSNodeClass", func() {
		It("should add finalizer to AWSNodeClass when referenced", func() {
			// Create an AWSNodeClass first
			nodeClass := createTestAWSNodeClass("finalizer-test-class")

			// Create a NodePool that references it
			np := createTestNodePoolWithNodeClass("finalizer-test-pool", 3, 1, "finalizer-test-class")
			Expect(np).NotTo(BeNil())

			// Wait for reconciliation and trigger it
			time.Sleep(500 * time.Millisecond)
			triggerReconcile(np.Name)

			// Verify the finalizer was added to the NodeClass
			Eventually(func() bool {
				updated := &stratosv1alpha1.AWSNodeClass{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: nodeClass.Name}, updated)
				if err != nil {
					return false
				}
				return controllerutil.ContainsFinalizer(updated, stratosv1alpha1.AWSNodeClassFinalizerInUse)
			}, timeout, interval).Should(BeTrue(), "Expected finalizer to be added to AWSNodeClass")
		})

		It("should update nodePoolCount in AWSNodeClass status", func() {
			// Create an AWSNodeClass
			nodeClass := createTestAWSNodeClass("status-test-class")

			// Create first NodePool
			np1 := createTestNodePoolWithNodeClass("status-test-pool-1", 3, 1, "status-test-class")
			Expect(np1).NotTo(BeNil())

			time.Sleep(500 * time.Millisecond)
			triggerReconcile(np1.Name)

			// Verify nodePoolCount is 1
			Eventually(func() int32 {
				updated := &stratosv1alpha1.AWSNodeClass{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: nodeClass.Name}, updated)
				if err != nil {
					return -1
				}
				return updated.Status.NodePoolCount
			}, timeout, interval).Should(Equal(int32(1)), "Expected nodePoolCount to be 1")

			// Create second NodePool referencing same class
			np2 := createTestNodePoolWithNodeClass("status-test-pool-2", 3, 1, "status-test-class")
			Expect(np2).NotTo(BeNil())

			time.Sleep(500 * time.Millisecond)
			triggerReconcile(np2.Name)

			// Verify nodePoolCount is 2
			Eventually(func() int32 {
				updated := &stratosv1alpha1.AWSNodeClass{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: nodeClass.Name}, updated)
				if err != nil {
					return -1
				}
				return updated.Status.NodePoolCount
			}, timeout, interval).Should(Equal(int32(2)), "Expected nodePoolCount to be 2")
		})

		It("should set InUse condition to True when referenced", func() {
			// Create an AWSNodeClass
			nodeClass := createTestAWSNodeClass("inuse-test-class")

			// Create a NodePool that references it
			np := createTestNodePoolWithNodeClass("inuse-test-pool", 3, 1, "inuse-test-class")
			Expect(np).NotTo(BeNil())

			time.Sleep(500 * time.Millisecond)
			triggerReconcile(np.Name)

			// Verify InUse condition is True
			Eventually(func() bool {
				updated := &stratosv1alpha1.AWSNodeClass{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: nodeClass.Name}, updated)
				if err != nil {
					return false
				}
				for _, cond := range updated.Status.Conditions {
					if cond.Type == stratosv1alpha1.AWSNodeClassConditionTypeInUse &&
						cond.Status == metav1.ConditionTrue {
						return true
					}
				}
				return false
			}, timeout, interval).Should(BeTrue(), "Expected InUse condition to be True")
		})
	})

	Context("When NodePool is deleted", func() {
		It("should remove finalizer when last referencing NodePool is deleted", func() {
			// Create an AWSNodeClass
			nodeClass := createTestAWSNodeClass("cleanup-test-class")

			// Create a NodePool that references it
			np := createTestNodePoolWithNodeClass("cleanup-test-pool", 3, 1, "cleanup-test-class")
			Expect(np).NotTo(BeNil())

			time.Sleep(500 * time.Millisecond)
			triggerReconcile(np.Name)

			// Verify finalizer is added
			Eventually(func() bool {
				updated := &stratosv1alpha1.AWSNodeClass{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: nodeClass.Name}, updated)
				if err != nil {
					return false
				}
				return controllerutil.ContainsFinalizer(updated, stratosv1alpha1.AWSNodeClassFinalizerInUse)
			}, timeout, interval).Should(BeTrue())

			// Delete the NodePool
			err := k8sClient.Delete(ctx, np)
			Expect(err).NotTo(HaveOccurred())

			// Wait for NodePool to be deleted
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: np.Name}, &stratosv1alpha1.NodePool{})
				return err != nil
			}, timeout, interval).Should(BeTrue())

			// Verify finalizer is removed from NodeClass
			Eventually(func() bool {
				updated := &stratosv1alpha1.AWSNodeClass{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: nodeClass.Name}, updated)
				if err != nil {
					return true // NodeClass might be deleted too
				}
				return !controllerutil.ContainsFinalizer(updated, stratosv1alpha1.AWSNodeClassFinalizerInUse)
			}, timeout, interval).Should(BeTrue(), "Expected finalizer to be removed from AWSNodeClass")
		})

		It("should keep finalizer when other NodePools still reference the class", func() {
			// Create an AWSNodeClass
			nodeClass := createTestAWSNodeClass("multi-pool-class")

			// Create two NodePools referencing the same class
			np1 := createTestNodePoolWithNodeClass("multi-pool-1", 3, 1, "multi-pool-class")
			np2 := createTestNodePoolWithNodeClass("multi-pool-2", 3, 1, "multi-pool-class")
			Expect(np1).NotTo(BeNil())
			Expect(np2).NotTo(BeNil())

			time.Sleep(500 * time.Millisecond)
			triggerReconcile(np1.Name)
			triggerReconcile(np2.Name)

			// Verify finalizer is present
			Eventually(func() bool {
				updated := &stratosv1alpha1.AWSNodeClass{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: nodeClass.Name}, updated)
				if err != nil {
					return false
				}
				return controllerutil.ContainsFinalizer(updated, stratosv1alpha1.AWSNodeClassFinalizerInUse)
			}, timeout, interval).Should(BeTrue())

			// Delete first NodePool
			err := k8sClient.Delete(ctx, np1)
			Expect(err).NotTo(HaveOccurred())

			// Wait for first pool to be deleted
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: np1.Name}, &stratosv1alpha1.NodePool{})
				return err != nil
			}, timeout, interval).Should(BeTrue())

			// Give controller time to reconcile
			time.Sleep(1 * time.Second)
			triggerReconcile(np2.Name)
			time.Sleep(500 * time.Millisecond)

			// Verify finalizer is STILL present (np2 still references it)
			Eventually(func() bool {
				updated := &stratosv1alpha1.AWSNodeClass{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: nodeClass.Name}, updated)
				if err != nil {
					return false
				}
				return controllerutil.ContainsFinalizer(updated, stratosv1alpha1.AWSNodeClassFinalizerInUse)
			}, timeout, interval).Should(BeTrue(), "Expected finalizer to be retained while other pool exists")

			// Verify nodePoolCount is now 1
			Eventually(func() int32 {
				updated := &stratosv1alpha1.AWSNodeClass{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: nodeClass.Name}, updated)
				if err != nil {
					return -1
				}
				return updated.Status.NodePoolCount
			}, timeout, interval).Should(Equal(int32(1)), "Expected nodePoolCount to be 1 after one pool deleted")
		})
	})

	Context("AWSNodeClass deletion protection", func() {
		It("should prevent deletion when NodePool references it", func() {
			// Create an AWSNodeClass
			nodeClass := createTestAWSNodeClass("protected-class")

			// Create a NodePool that references it
			np := createTestNodePoolWithNodeClass("protect-test-pool", 3, 1, "protected-class")
			Expect(np).NotTo(BeNil())

			time.Sleep(500 * time.Millisecond)
			triggerReconcile(np.Name)

			// Wait for finalizer to be added
			Eventually(func() bool {
				updated := &stratosv1alpha1.AWSNodeClass{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: nodeClass.Name}, updated)
				if err != nil {
					return false
				}
				return controllerutil.ContainsFinalizer(updated, stratosv1alpha1.AWSNodeClassFinalizerInUse)
			}, timeout, interval).Should(BeTrue())

			// Try to delete the NodeClass
			err := k8sClient.Delete(ctx, nodeClass)
			Expect(err).NotTo(HaveOccurred()) // Delete request succeeds but object is not removed

			// Verify NodeClass still exists (blocked by finalizer)
			time.Sleep(500 * time.Millisecond)
			updated := &stratosv1alpha1.AWSNodeClass{}
			err = k8sClient.Get(ctx, types.NamespacedName{Name: nodeClass.Name}, updated)
			Expect(err).NotTo(HaveOccurred(), "NodeClass should still exist due to finalizer")
			Expect(updated.DeletionTimestamp).NotTo(BeNil(), "NodeClass should have deletion timestamp")
		})
	})
})

// createTestNodePoolWithNodeClass creates a NodePool referencing a specific AWSNodeClass.
func createTestNodePoolWithNodeClass(name string, poolSize, minStandby int32, nodeClassName string) *stratosv1alpha1.NodePool {
	np := &stratosv1alpha1.NodePool{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: stratosv1alpha1.NodePoolSpec{
			PoolSize:   poolSize,
			MinStandby: minStandby,
			Template: stratosv1alpha1.NodeTemplate{
				NodeClassRef: stratosv1alpha1.NodeClassRef{
					Kind: "AWSNodeClass",
					Name: nodeClassName,
				},
			},
		},
	}

	// Inject fake provider for this pool
	reconciler.InjectCloudProvider(name, fakeProvider)

	err := k8sClient.Create(ctx, np)
	Expect(err).NotTo(HaveOccurred())
	return np
}
