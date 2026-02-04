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
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	stratosv1alpha1 "github.com/stratos-sh/stratos/api/v1alpha1"
	"github.com/stratos-sh/stratos/internal/cloudprovider"
	"github.com/stratos-sh/stratos/internal/controller/nodepool/nodestate"
)

var _ = Describe("Startup Taints", func() {
	const (
		poolName = "startup-taints-test"
	)

	Context("Network readiness taint enabled (default)", func() {
		It("should preserve network readiness taint when node starts and remove it when network is ready", func() {
			By("Creating a NodePool with network readiness taint enabled (default)")
			np := createTestNodePoolWithNetworkReadinessTaint(poolName, 3, 2, nil)
			Expect(np).NotTo(BeNil())

			By("Simulating a running node with network readiness taint (past grace period)")
			instanceID := launchFakeInstance(poolName)
			setFakeInstanceState(instanceID, cloudprovider.InstanceStateRunning)
			startTime := time.Now().Add(-45 * time.Second)
			node := simulateNodeWithStartupTaint(poolName, instanceID, nodestate.NodeStateRunning,
				nodestate.TaintKeyNotReady, nodestate.TaintValueNotReady, corev1.TaintEffectNoSchedule, startTime)

			By("Verifying network readiness taint is preserved initially (before network ready)")
			Eventually(func() bool {
				n := &corev1.Node{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: node.Name}, n)
				if err != nil {
					return false
				}
				return hasTaint(n.Spec.Taints, nodestate.TaintKeyNotReady, corev1.TaintEffectNoSchedule)
			}, timeout, interval).Should(BeTrue())

			By("Simulating CNI ready (NetworkingReady condition)")
			updateNodeNetworkCondition(node.Name, true)

			By("Triggering reconciliation")
			triggerReconcile(poolName)

			By("Verifying network readiness taint is removed after network ready")
			Eventually(func() bool {
				n := &corev1.Node{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: node.Name}, n)
				if err != nil {
					return true // Node might be gone, which is also a failure
				}
				return !hasTaint(n.Spec.Taints, nodestate.TaintKeyNotReady, corev1.TaintEffectNoSchedule)
			}, timeout, interval).Should(BeTrue())
		})

		It("should not remove network readiness taint when network is not ready", func() {
			By("Creating a NodePool with network readiness taint enabled")
			np := createTestNodePoolWithNetworkReadinessTaint(poolName+"-network-not-ready", 3, 2, nil)
			Expect(np).NotTo(BeNil())

			By("Simulating a running node with network readiness taint but network not ready")
			instanceID := launchFakeInstance(poolName + "-network-not-ready")
			setFakeInstanceState(instanceID, cloudprovider.InstanceStateRunning)
			startTime := time.Now().Add(-45 * time.Second)
			node := simulateNodeWithStartupTaint(poolName+"-network-not-ready", instanceID, nodestate.NodeStateRunning,
				nodestate.TaintKeyNotReady, nodestate.TaintValueNotReady, corev1.TaintEffectNoSchedule, startTime)

			By("Triggering reconciliation")
			triggerReconcile(poolName + "-network-not-ready")

			By("Verifying network readiness taint is NOT removed (network not ready)")
			Consistently(func() bool {
				n := &corev1.Node{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: node.Name}, n)
				if err != nil {
					return false
				}
				return hasTaint(n.Spec.Taints, nodestate.TaintKeyNotReady, corev1.TaintEffectNoSchedule)
			}, 2*time.Second, 200*time.Millisecond).Should(BeTrue())
		})
	})

	Context("Network readiness taint disabled", func() {
		It("should not manage taints when networkReadinessStrategy is None", func() {
			By("Creating a NodePool with network readiness strategy None")
			none := stratosv1alpha1.NetworkReadinessStrategyNone
			np := createTestNodePoolWithNetworkReadinessTaint(poolName+"-disabled", 3, 2, &none)
			Expect(np).NotTo(BeNil())

			By("Simulating a standby node")
			instanceID := launchFakeInstance(poolName + "-disabled")
			setFakeInstanceState(instanceID, cloudprovider.InstanceStateStopped)
			node := simulateNodeJoin(poolName+"-disabled", instanceID, nodestate.NodeStateStandby)

			By("Triggering scale-up")
			setFakeInstanceState(instanceID, cloudprovider.InstanceStateRunning)

			// Update to running state
			patchNode := client.MergeFrom(node.DeepCopy())
			node.Labels[nodestate.LabelState] = string(nodestate.NodeStateRunning)
			node.Spec.Unschedulable = false
			node.Spec.Taints = nil // No taints
			err := k8sClient.Patch(ctx, node, patchNode)
			Expect(err).NotTo(HaveOccurred())

			By("Triggering reconciliation")
			triggerReconcile(poolName + "-disabled")

			By("Verifying node is running without issues")
			Eventually(func() string {
				n := &corev1.Node{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: node.Name}, n)
				if err != nil {
					return ""
				}
				return n.Labels[nodestate.LabelState]
			}, timeout, interval).Should(Equal(string(nodestate.NodeStateRunning)))
		})
	})

	Context("Backward compatibility", func() {
		It("should work with default networkReadinessStrategy (nil = Taint)", func() {
			By("Creating a NodePool without explicit networkReadinessStrategy")
			np := createTestNodePool(poolName+"-default", 3, 2)
			Expect(np).NotTo(BeNil())

			By("Simulating a standby node")
			instanceID := launchFakeInstance(poolName + "-default")
			setFakeInstanceState(instanceID, cloudprovider.InstanceStateStopped)
			node := simulateNodeJoin(poolName+"-default", instanceID, nodestate.NodeStateStandby)

			By("Triggering scale-up")
			setFakeInstanceState(instanceID, cloudprovider.InstanceStateRunning)

			// Update to running state
			patchNode := client.MergeFrom(node.DeepCopy())
			node.Labels[nodestate.LabelState] = string(nodestate.NodeStateRunning)
			node.Spec.Unschedulable = false
			node.Spec.Taints = nil
			err := k8sClient.Patch(ctx, node, patchNode)
			Expect(err).NotTo(HaveOccurred())

			By("Triggering reconciliation")
			triggerReconcile(poolName + "-default")

			By("Verifying node is running without issues")
			Eventually(func() string {
				n := &corev1.Node{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: node.Name}, n)
				if err != nil {
					return ""
				}
				return n.Labels[nodestate.LabelState]
			}, timeout, interval).Should(Equal(string(nodestate.NodeStateRunning)))
		})
	})
})

// createTestNodePoolWithNetworkReadinessTaint creates a NodePool with the networkReadinessStrategy field.
func createTestNodePoolWithNetworkReadinessTaint(name string, poolSize, minStandby int32, strategy *stratosv1alpha1.NetworkReadinessStrategy) *stratosv1alpha1.NodePool {
	// Create an AWSNodeClass for this pool
	nodeClassName := name + "-nodeclass"
	createTestAWSNodeClass(nodeClassName)

	np := &stratosv1alpha1.NodePool{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: stratosv1alpha1.NodePoolSpec{
			PoolSize:   poolSize,
			MinStandby: minStandby,
			Template: stratosv1alpha1.NodeTemplate{
				NetworkReadinessStrategy: strategy,
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

// simulateNodeWithStartupTaint creates a node with a startup taint.
// If startedAt is provided (non-zero), it sets the AnnotationLastStarted annotation.
func simulateNodeWithStartupTaint(poolName, instanceID string, nodeState nodestate.NodeState, taintKey, taintValue string, taintEffect corev1.TaintEffect, startedAt ...time.Time) *corev1.Node {
	nodeName := fmt.Sprintf("node-%s", instanceID)

	annotations := make(map[string]string)
	if len(startedAt) > 0 && !startedAt[0].IsZero() {
		annotations[nodestate.AnnotationLastStarted] = startedAt[0].Format(time.RFC3339)
	}

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: nodeName,
			Labels: map[string]string{
				nodestate.LabelPool:       poolName,
				nodestate.LabelState:      string(nodeState),
				nodestate.LabelInstanceID: instanceID,
				nodestate.LabelStateSince: fmt.Sprintf("%d", time.Now().Unix()),
			},
			Annotations: annotations,
		},
		Spec: corev1.NodeSpec{
			ProviderID:    fmt.Sprintf("aws:///us-east-1a/%s", instanceID),
			Unschedulable: nodeState != nodestate.NodeStateRunning,
			Taints: []corev1.Taint{
				{
					Key:    taintKey,
					Value:  taintValue,
					Effect: taintEffect,
				},
			},
		},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{
					Type:   corev1.NodeReady,
					Status: corev1.ConditionTrue,
				},
			},
		},
	}

	// Add standby taint for standby nodes
	if nodeState == nodestate.NodeStateStandby {
		node.Spec.Taints = append(node.Spec.Taints, corev1.Taint{
			Key:    nodestate.TaintKeyStandby,
			Effect: corev1.TaintEffectNoExecute,
		})
	}

	err := k8sClient.Create(ctx, node)
	Expect(err).NotTo(HaveOccurred())
	return node
}

// updateNodeNetworkCondition updates the NetworkingReady condition on a node (EKS-style).
func updateNodeNetworkCondition(nodeName string, ready bool) {
	node := &corev1.Node{}
	err := k8sClient.Get(ctx, types.NamespacedName{Name: nodeName}, node)
	Expect(err).NotTo(HaveOccurred())

	patch := client.MergeFrom(node.DeepCopy())

	status := corev1.ConditionFalse
	reason := "NetworkingNotReady"
	if ready {
		status = corev1.ConditionTrue
		reason = "NetworkingIsReady"
	}

	// Add or update the NetworkingReady condition
	found := false
	for i, cond := range node.Status.Conditions {
		if cond.Type == nodestate.NetworkingReadyCondition {
			node.Status.Conditions[i].Status = status
			node.Status.Conditions[i].Reason = reason
			found = true
			break
		}
	}
	if !found {
		node.Status.Conditions = append(node.Status.Conditions, corev1.NodeCondition{
			Type:   nodestate.NetworkingReadyCondition,
			Status: status,
			Reason: reason,
		})
	}

	err = k8sClient.Status().Patch(ctx, node, patch)
	Expect(err).NotTo(HaveOccurred())
}

// hasTaint checks if a taint with the given key and effect exists.
func hasTaint(taints []corev1.Taint, key string, effect corev1.TaintEffect) bool {
	for _, t := range taints {
		if t.Key == key && t.Effect == effect {
			return true
		}
	}
	return false
}
