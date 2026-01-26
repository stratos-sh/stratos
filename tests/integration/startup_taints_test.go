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
	"github.com/stratos-sh/stratos/internal/controller"
)

var _ = Describe("Startup Taints", func() {
	const (
		poolName = "startup-taints-test"
	)

	Context("WhenNetworkReady mode", func() {
		It("should preserve startup taints when node starts and remove them when network is ready", func() {
			By("Creating a NodePool with startup taints")
			np := createTestNodePoolWithStartupTaints(poolName, 3, 2,
				[]corev1.Taint{{
					Key:    "node.eks.amazonaws.com/not-ready",
					Value:  "true",
					Effect: corev1.TaintEffectNoSchedule,
				}},
				stratosv1alpha1.StartupTaintRemovalWhenNetworkReady,
			)
			Expect(np).NotTo(BeNil())

			By("Simulating a running node with startup taint (past grace period)")
			instanceID := launchFakeInstance(poolName)
			setFakeInstanceState(instanceID, cloudprovider.InstanceStateRunning)
			// Create node directly in running state with annotation set to past grace period
			startTime := time.Now().Add(-45 * time.Second)
			node := simulateNodeWithStartupTaint(poolName, instanceID, controller.NodeStateRunning,
				"node.eks.amazonaws.com/not-ready", "true", corev1.TaintEffectNoSchedule, startTime)

			By("Verifying startup taint is preserved initially (before network ready)")
			Eventually(func() bool {
				n := &corev1.Node{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: node.Name}, n)
				if err != nil {
					return false
				}
				return hasTaint(n.Spec.Taints, "node.eks.amazonaws.com/not-ready", corev1.TaintEffectNoSchedule)
			}, timeout, interval).Should(BeTrue())

			By("Simulating CNI ready (NetworkingReady condition)")
			updateNodeNetworkCondition(node.Name, true)

			By("Triggering reconciliation")
			triggerReconcile(poolName)

			By("Verifying startup taint is removed after network ready")
			Eventually(func() bool {
				n := &corev1.Node{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: node.Name}, n)
				if err != nil {
					return true // Node might be gone, which is also a failure
				}
				return !hasTaint(n.Spec.Taints, "node.eks.amazonaws.com/not-ready", corev1.TaintEffectNoSchedule)
			}, timeout, interval).Should(BeTrue())
		})

		It("should not remove startup taints when network is not ready", func() {
			By("Creating a NodePool with startup taints")
			np := createTestNodePoolWithStartupTaints(poolName+"-network-not-ready", 3, 2,
				[]corev1.Taint{{
					Key:    "node.eks.amazonaws.com/not-ready",
					Value:  "true",
					Effect: corev1.TaintEffectNoSchedule,
				}},
				stratosv1alpha1.StartupTaintRemovalWhenNetworkReady,
			)
			Expect(np).NotTo(BeNil())

			By("Simulating a running node with startup taint but network not ready")
			instanceID := launchFakeInstance(poolName + "-network-not-ready")
			setFakeInstanceState(instanceID, cloudprovider.InstanceStateRunning)
			// Create with start time past grace period
			startTime := time.Now().Add(-45 * time.Second)
			node := simulateNodeWithStartupTaint(poolName+"-network-not-ready", instanceID, controller.NodeStateRunning,
				"node.eks.amazonaws.com/not-ready", "true", corev1.TaintEffectNoSchedule, startTime)

			By("Triggering reconciliation")
			triggerReconcile(poolName + "-network-not-ready")

			By("Verifying startup taint is NOT removed (network not ready)")
			Consistently(func() bool {
				n := &corev1.Node{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: node.Name}, n)
				if err != nil {
					return false
				}
				return hasTaint(n.Spec.Taints, "node.eks.amazonaws.com/not-ready", corev1.TaintEffectNoSchedule)
			}, 2*time.Second, 200*time.Millisecond).Should(BeTrue())
		})
	})

	Context("External mode", func() {
		It("should not remove startup taints in External mode", func() {
			By("Creating a NodePool with startup taints in External mode")
			np := createTestNodePoolWithStartupTaints(poolName+"-external", 3, 2,
				[]corev1.Taint{{
					Key:    "node.cilium.io/agent-not-ready",
					Value:  "true",
					Effect: corev1.TaintEffectNoSchedule,
				}},
				stratosv1alpha1.StartupTaintRemovalExternal,
			)
			Expect(np).NotTo(BeNil())

			By("Simulating a running node with startup taint")
			instanceID := launchFakeInstance(poolName + "-external")
			setFakeInstanceState(instanceID, cloudprovider.InstanceStateRunning)
			// Create with start time past grace period
			startTime := time.Now().Add(-45 * time.Second)
			node := simulateNodeWithStartupTaint(poolName+"-external", instanceID, controller.NodeStateRunning,
				"node.cilium.io/agent-not-ready", "true", corev1.TaintEffectNoSchedule, startTime)

			// Simulate network ready
			updateNodeNetworkConditionCilium(node.Name, true)

			By("Triggering reconciliation")
			triggerReconcile(poolName + "-external")

			By("Verifying startup taint is NOT removed by Stratos (External mode)")
			Consistently(func() bool {
				n := &corev1.Node{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: node.Name}, n)
				if err != nil {
					return false
				}
				return hasTaint(n.Spec.Taints, "node.cilium.io/agent-not-ready", corev1.TaintEffectNoSchedule)
			}, 2*time.Second, 200*time.Millisecond).Should(BeTrue())
		})

		It("should detect when external controller removes startup taints", func() {
			By("Creating a NodePool with startup taints in External mode")
			np := createTestNodePoolWithStartupTaints(poolName+"-external-detect", 3, 2,
				[]corev1.Taint{{
					Key:    "node.cilium.io/agent-not-ready",
					Value:  "true",
					Effect: corev1.TaintEffectNoSchedule,
				}},
				stratosv1alpha1.StartupTaintRemovalExternal,
			)
			Expect(np).NotTo(BeNil())

			By("Simulating a running node with startup taint")
			instanceID := launchFakeInstance(poolName + "-external-detect")
			setFakeInstanceState(instanceID, cloudprovider.InstanceStateRunning)
			node := simulateNodeWithStartupTaint(poolName+"-external-detect", instanceID, controller.NodeStateRunning,
				"node.cilium.io/agent-not-ready", "true", corev1.TaintEffectNoSchedule)

			By("Simulating external controller (Cilium) removing the taint")
			patchNode := client.MergeFrom(node.DeepCopy())
			node.Spec.Taints = nil // Remove all taints
			err := k8sClient.Patch(ctx, node, patchNode)
			Expect(err).NotTo(HaveOccurred())

			By("Verifying startup taint was removed externally")
			Eventually(func() bool {
				n := &corev1.Node{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: node.Name}, n)
				if err != nil {
					return false
				}
				return !hasTaint(n.Spec.Taints, "node.cilium.io/agent-not-ready", corev1.TaintEffectNoSchedule)
			}, timeout, interval).Should(BeTrue())
		})
	})

	Context("Backward compatibility", func() {
		It("should work without startup taints configured", func() {
			By("Creating a NodePool without startup taints")
			np := createTestNodePool(poolName+"-no-taints", 3, 2)
			Expect(np).NotTo(BeNil())

			By("Simulating a standby node")
			instanceID := launchFakeInstance(poolName + "-no-taints")
			setFakeInstanceState(instanceID, cloudprovider.InstanceStateStopped)
			node := simulateNodeJoin(poolName+"-no-taints", instanceID, controller.NodeStateStandby)

			By("Triggering scale-up")
			setFakeInstanceState(instanceID, cloudprovider.InstanceStateRunning)

			// Update to running state
			patchNode := client.MergeFrom(node.DeepCopy())
			node.Labels[controller.LabelState] = string(controller.NodeStateRunning)
			node.Spec.Unschedulable = false
			node.Spec.Taints = nil // No taints
			err := k8sClient.Patch(ctx, node, patchNode)
			Expect(err).NotTo(HaveOccurred())

			By("Triggering reconciliation")
			triggerReconcile(poolName + "-no-taints")

			By("Verifying node is running without issues")
			Eventually(func() string {
				n := &corev1.Node{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: node.Name}, n)
				if err != nil {
					return ""
				}
				return n.Labels[controller.LabelState]
			}, timeout, interval).Should(Equal(string(controller.NodeStateRunning)))
		})
	})
})

// createTestNodePoolWithStartupTaints creates a NodePool with startup taints configuration.
func createTestNodePoolWithStartupTaints(name string, poolSize, minStandby int32, startupTaints []corev1.Taint, removalMode stratosv1alpha1.StartupTaintRemovalMode) *stratosv1alpha1.NodePool {
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
				StartupTaints:       startupTaints,
				StartupTaintRemoval: removalMode,
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
func simulateNodeWithStartupTaint(poolName, instanceID string, state controller.NodeState, taintKey, taintValue string, taintEffect corev1.TaintEffect, startedAt ...time.Time) *corev1.Node {
	nodeName := fmt.Sprintf("node-%s", instanceID)

	annotations := make(map[string]string)
	if len(startedAt) > 0 && !startedAt[0].IsZero() {
		annotations[controller.AnnotationLastStarted] = startedAt[0].Format(time.RFC3339)
	}

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: nodeName,
			Labels: map[string]string{
				controller.LabelPool:       poolName,
				controller.LabelState:      string(state),
				controller.LabelInstanceID: instanceID,
				controller.LabelStateSince: fmt.Sprintf("%d", time.Now().Unix()),
			},
			Annotations: annotations,
		},
		Spec: corev1.NodeSpec{
			ProviderID:    fmt.Sprintf("aws:///us-east-1a/%s", instanceID),
			Unschedulable: state != controller.NodeStateRunning,
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
	if state == controller.NodeStateStandby {
		node.Spec.Taints = append(node.Spec.Taints, corev1.Taint{
			Key:    controller.TaintKeyStandby,
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
		if cond.Type == controller.NetworkingReadyCondition {
			node.Status.Conditions[i].Status = status
			node.Status.Conditions[i].Reason = reason
			found = true
			break
		}
	}
	if !found {
		node.Status.Conditions = append(node.Status.Conditions, corev1.NodeCondition{
			Type:   controller.NetworkingReadyCondition,
			Status: status,
			Reason: reason,
		})
	}

	err = k8sClient.Status().Patch(ctx, node, patch)
	Expect(err).NotTo(HaveOccurred())
}

// updateNodeNetworkConditionCilium updates the NetworkUnavailable condition on a node (Cilium-style).
func updateNodeNetworkConditionCilium(nodeName string, ready bool) {
	node := &corev1.Node{}
	err := k8sClient.Get(ctx, types.NamespacedName{Name: nodeName}, node)
	Expect(err).NotTo(HaveOccurred())

	patch := client.MergeFrom(node.DeepCopy())

	// For Cilium, NetworkUnavailable=False means ready
	status := corev1.ConditionTrue
	reason := "NetworkPluginNotReady"
	if ready {
		status = corev1.ConditionFalse
		reason = "CiliumIsUp"
	}

	// Add or update the NetworkUnavailable condition
	found := false
	for i, cond := range node.Status.Conditions {
		if cond.Type == corev1.NodeNetworkUnavailable {
			node.Status.Conditions[i].Status = status
			node.Status.Conditions[i].Reason = reason
			found = true
			break
		}
	}
	if !found {
		node.Status.Conditions = append(node.Status.Conditions, corev1.NodeCondition{
			Type:   corev1.NodeNetworkUnavailable,
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
