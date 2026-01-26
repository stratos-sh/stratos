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
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	stratosv1alpha1 "github.com/stratos-sh/stratos/api/v1alpha1"
	"github.com/stratos-sh/stratos/internal/cloudprovider"
	"github.com/stratos-sh/stratos/internal/controller"
)

var _ = Describe("ControllerStop Mode", func() {
	var (
		poolName string
	)

	BeforeEach(func() {
		poolName = fmt.Sprintf("controller-stop-test-%d", time.Now().UnixNano())
	})

	Describe("Warmup Completion", func() {
		It("should stop instance when node is Ready in ControllerStop mode", func() {
			// Track if StopInstance was called
			var stopCalled bool
			var stoppedInstanceID string
			fakeProvider.StopHook = func(ctx context.Context, instanceID string, force bool) error {
				stopCalled = true
				stoppedInstanceID = instanceID
				return nil
			}

			// Create NodePool with ControllerStop mode
			np := createTestNodePoolWithControllerStop(poolName, 3, 2)
			Expect(np).NotTo(BeNil())

			// Launch instance and simulate it joining the cluster
			instanceID := launchFakeInstance(poolName)
			setFakeInstanceState(instanceID, cloudprovider.InstanceStateRunning)

			// Simulate node joining with Ready condition
			node := simulateNodeJoinWithReady(poolName, instanceID, controller.NodeStateWarmup, true)
			Expect(node).NotTo(BeNil())

			// Trigger reconciliation
			triggerReconcile(poolName)

			// Wait for the node to transition to standby
			Eventually(func() string {
				n := &corev1.Node{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: node.Name}, n)
				if err != nil {
					return ""
				}
				return n.Labels[controller.LabelState]
			}, timeout, interval).Should(Equal(string(controller.NodeStateStandby)))

			// Verify StopInstance was called
			Expect(stopCalled).To(BeTrue())
			Expect(stoppedInstanceID).To(Equal(instanceID))
		})

		It("should not stop instance when node is not Ready in ControllerStop mode", func() {
			// Track if StopInstance was called
			var stopCalled bool
			fakeProvider.StopHook = func(ctx context.Context, instanceID string, force bool) error {
				stopCalled = true
				return nil
			}

			// Create NodePool with ControllerStop mode
			np := createTestNodePoolWithControllerStop(poolName, 3, 2)
			Expect(np).NotTo(BeNil())

			// Launch instance
			instanceID := launchFakeInstance(poolName)
			setFakeInstanceState(instanceID, cloudprovider.InstanceStateRunning)

			// Simulate node joining but NOT ready
			node := simulateNodeJoinWithReady(poolName, instanceID, controller.NodeStateWarmup, false)
			Expect(node).NotTo(BeNil())

			// Trigger reconciliation multiple times
			for i := 0; i < 3; i++ {
				triggerReconcile(poolName)
				time.Sleep(100 * time.Millisecond)
			}

			// Verify StopInstance was NOT called
			Expect(stopCalled).To(BeFalse())

			// Verify node is still in warmup state
			updatedNode := &corev1.Node{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: node.Name}, updatedNode)
			Expect(err).NotTo(HaveOccurred())
			Expect(updatedNode.Labels[controller.LabelState]).To(Equal(string(controller.NodeStateWarmup)))
		})

		It("should wait for network ready when WhenNetworkReady is configured", func() {
			// Track if StopInstance was called
			var stopCalled bool
			fakeProvider.StopHook = func(ctx context.Context, instanceID string, force bool) error {
				stopCalled = true
				return nil
			}

			// Create NodePool with ControllerStop mode and WhenNetworkReady
			np := createTestNodePoolWithControllerStopAndNetworkReady(poolName, 3, 2)
			Expect(np).NotTo(BeNil())

			// Launch instance
			instanceID := launchFakeInstance(poolName)
			setFakeInstanceState(instanceID, cloudprovider.InstanceStateRunning)

			// Simulate node joining - Ready but network NOT ready
			node := simulateNodeJoinWithReadyNoNetwork(poolName, instanceID, controller.NodeStateWarmup)
			Expect(node).NotTo(BeNil())

			// Trigger reconciliation
			triggerReconcile(poolName)
			time.Sleep(200 * time.Millisecond)

			// Verify StopInstance was NOT called (waiting for network)
			Expect(stopCalled).To(BeFalse())

			// Now add NetworkingReady condition
			patchNode := &corev1.Node{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: node.Name}, patchNode)
			Expect(err).NotTo(HaveOccurred())

			patch := client.MergeFrom(patchNode.DeepCopy())
			patchNode.Status.Conditions = append(patchNode.Status.Conditions, corev1.NodeCondition{
				Type:   controller.NetworkingReadyCondition,
				Status: corev1.ConditionTrue,
				Reason: "NetworkingIsReady",
			})
			err = k8sClient.Status().Patch(ctx, patchNode, patch)
			Expect(err).NotTo(HaveOccurred())

			// Trigger reconciliation again
			triggerReconcile(poolName)

			// Now StopInstance should be called
			Eventually(func() bool {
				return stopCalled
			}, timeout, interval).Should(BeTrue())
		})
	})

	Describe("SelfStop Backward Compatibility", func() {
		It("should use SelfStop mode by default", func() {
			// Track if StopInstance was called
			var stopCalled bool
			fakeProvider.StopHook = func(ctx context.Context, instanceID string, force bool) error {
				stopCalled = true
				return nil
			}

			// Create NodePool without specifying CompletionMode (should default to SelfStop)
			np := createTestNodePool(poolName, 3, 2)
			Expect(np).NotTo(BeNil())

			// Launch instance
			instanceID := launchFakeInstance(poolName)
			setFakeInstanceState(instanceID, cloudprovider.InstanceStateRunning)

			// Simulate node joining and becoming Ready
			node := simulateNodeJoinWithReady(poolName, instanceID, controller.NodeStateWarmup, true)
			Expect(node).NotTo(BeNil())

			// Trigger reconciliation multiple times
			for i := 0; i < 3; i++ {
				triggerReconcile(poolName)
				time.Sleep(100 * time.Millisecond)
			}

			// Verify StopInstance was NOT called (waiting for self-stop)
			Expect(stopCalled).To(BeFalse())

			// Node should still be in warmup state
			updatedNode := &corev1.Node{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: node.Name}, updatedNode)
			Expect(err).NotTo(HaveOccurred())
			Expect(updatedNode.Labels[controller.LabelState]).To(Equal(string(controller.NodeStateWarmup)))
		})

		It("should transition to standby when instance self-stops in SelfStop mode", func() {
			// Create NodePool without specifying CompletionMode (should default to SelfStop)
			np := createTestNodePool(poolName, 3, 2)
			Expect(np).NotTo(BeNil())

			// Launch instance
			instanceID := launchFakeInstance(poolName)
			setFakeInstanceState(instanceID, cloudprovider.InstanceStateRunning)

			// Simulate node joining
			node := simulateNodeJoinWithReady(poolName, instanceID, controller.NodeStateWarmup, true)
			Expect(node).NotTo(BeNil())

			// Simulate instance self-stopping (what userdata would do)
			setFakeInstanceState(instanceID, cloudprovider.InstanceStateStopped)

			// Trigger reconciliation
			triggerReconcile(poolName)

			// Wait for the node to transition to standby
			Eventually(func() string {
				n := &corev1.Node{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: node.Name}, n)
				if err != nil {
					return ""
				}
				return n.Labels[controller.LabelState]
			}, timeout, interval).Should(Equal(string(controller.NodeStateStandby)))
		})
	})

	Describe("Warmup Timeout", func() {
		It("should handle timeout in ControllerStop mode", func() {
			// Create NodePool with ControllerStop mode and short timeout
			timeout := 1 * time.Second
			np := createTestNodePoolWithControllerStopAndTimeout(poolName, 3, 2, timeout)
			Expect(np).NotTo(BeNil())

			// Launch instance
			instanceID := launchFakeInstance(poolName)
			setFakeInstanceState(instanceID, cloudprovider.InstanceStateRunning)

			// Simulate node joining but NOT ready (will timeout)
			node := simulateNodeJoinWithReadyAndOldTimestamp(poolName, instanceID, controller.NodeStateWarmup, false)
			Expect(node).NotTo(BeNil())

			// Trigger reconciliation
			triggerReconcile(poolName)

			// Wait for timeout handling (either stop or terminate based on timeoutAction)
			// Default timeoutAction is stop, so node should transition to standby
			Eventually(func() bool {
				state, err := fakeProvider.GetInstanceState(ctx, instanceID)
				if err != nil {
					return false
				}
				// Instance should be stopping or stopped
				return state == cloudprovider.InstanceStateStopping || state == cloudprovider.InstanceStateStopped
			}, 5*time.Second, interval).Should(BeTrue())
		})
	})

	Describe("Bottlerocket Node Adoption", func() {
		It("should adopt and label Bottlerocket-like node with pool-only label", func() {
			// Track if StopInstance was called
			var stopCalled bool
			var stoppedInstanceID string
			fakeProvider.StopHook = func(ctx context.Context, instanceID string, force bool) error {
				stopCalled = true
				stoppedInstanceID = instanceID
				return nil
			}

			// Create NodePool with ControllerStop mode
			np := createTestNodePoolWithControllerStop(poolName, 3, 2)
			Expect(np).NotTo(BeNil())

			// Launch instance (running state)
			instanceID := launchFakeInstance(poolName)
			setFakeInstanceState(instanceID, cloudprovider.InstanceStateRunning)

			// Simulate node joining with ONLY pool label (Bottlerocket scenario)
			node := simulateNodeJoinWithPoolLabelOnly(poolName, instanceID, true)
			Expect(node).NotTo(BeNil())

			// Trigger reconciliation
			triggerReconcile(poolName)

			// Wait for the controller to adopt the node (add state label and instance-id)
			// In ControllerStop mode with a ready node, adoption may go directly to standby
			// in a single reconciliation, so we accept either warmup or standby here
			Eventually(func() string {
				n := &corev1.Node{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: node.Name}, n)
				if err != nil {
					return ""
				}
				return n.Labels[controller.LabelState]
			}, timeout, interval).Should(SatisfyAny(
				Equal(string(controller.NodeStateWarmup)),
				Equal(string(controller.NodeStateStandby)),
			))

			// Verify instance-id was set
			updatedNode := &corev1.Node{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: node.Name}, updatedNode)
			Expect(err).NotTo(HaveOccurred())
			Expect(updatedNode.Labels[controller.LabelInstanceID]).To(Equal(instanceID))
			Expect(updatedNode.Labels[controller.LabelStateSince]).NotTo(BeEmpty())

			// Wait for the node to transition to standby (controller stops it when Ready)
			Eventually(func() string {
				n := &corev1.Node{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: node.Name}, n)
				if err != nil {
					return ""
				}
				return n.Labels[controller.LabelState]
			}, timeout, interval).Should(Equal(string(controller.NodeStateStandby)))

			// Verify StopInstance was called
			Expect(stopCalled).To(BeTrue())
			Expect(stoppedInstanceID).To(Equal(instanceID))
		})

		It("should adopt Bottlerocket-like node directly to standby when instance already stopped", func() {
			// Create NodePool with ControllerStop mode
			np := createTestNodePoolWithControllerStop(poolName, 3, 2)
			Expect(np).NotTo(BeNil())

			// Launch instance and set to stopped state
			instanceID := launchFakeInstance(poolName)
			setFakeInstanceState(instanceID, cloudprovider.InstanceStateStopped)

			// Simulate node joining with ONLY pool label
			node := simulateNodeJoinWithPoolLabelOnly(poolName, instanceID, true)
			Expect(node).NotTo(BeNil())

			// Trigger reconciliation
			triggerReconcile(poolName)

			// Wait for the controller to adopt directly to standby
			Eventually(func() string {
				n := &corev1.Node{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: node.Name}, n)
				if err != nil {
					return ""
				}
				return n.Labels[controller.LabelState]
			}, timeout, interval).Should(Equal(string(controller.NodeStateStandby)))

			// Verify all labels, annotations, and taints are set
			updatedNode := &corev1.Node{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: node.Name}, updatedNode)
			Expect(err).NotTo(HaveOccurred())
			Expect(updatedNode.Labels[controller.LabelInstanceID]).To(Equal(instanceID))
			Expect(updatedNode.Labels[controller.LabelStateSince]).NotTo(BeEmpty())
			Expect(updatedNode.Annotations[controller.AnnotationWarmupCompleted]).NotTo(BeEmpty())

			// Verify standby taint
			hasTaint := false
			for _, taint := range updatedNode.Spec.Taints {
				if taint.Key == controller.TaintKeyStandby {
					hasTaint = true
					break
				}
			}
			Expect(hasTaint).To(BeTrue())
		})

		It("should adopt Bottlerocket-like node in SelfStop mode", func() {
			// Track if StopInstance was called
			var stopCalled bool
			fakeProvider.StopHook = func(ctx context.Context, instanceID string, force bool) error {
				stopCalled = true
				return nil
			}

			// Create NodePool without specifying CompletionMode (defaults to SelfStop)
			np := createTestNodePool(poolName, 3, 2)
			Expect(np).NotTo(BeNil())

			// Launch instance (running state)
			instanceID := launchFakeInstance(poolName)
			setFakeInstanceState(instanceID, cloudprovider.InstanceStateRunning)

			// Simulate node joining with ONLY pool label (Bottlerocket scenario)
			node := simulateNodeJoinWithPoolLabelOnly(poolName, instanceID, true)
			Expect(node).NotTo(BeNil())

			// Trigger reconciliation
			triggerReconcile(poolName)

			// Wait for the controller to adopt the node (add state=warmup, instance-id)
			Eventually(func() string {
				n := &corev1.Node{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: node.Name}, n)
				if err != nil {
					return ""
				}
				return n.Labels[controller.LabelState]
			}, timeout, interval).Should(Equal(string(controller.NodeStateWarmup)))

			// Verify instance-id was set
			updatedNode := &corev1.Node{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: node.Name}, updatedNode)
			Expect(err).NotTo(HaveOccurred())
			Expect(updatedNode.Labels[controller.LabelInstanceID]).To(Equal(instanceID))

			// Give the controller time to potentially call StopInstance
			time.Sleep(200 * time.Millisecond)
			triggerReconcile(poolName)
			time.Sleep(200 * time.Millisecond)

			// Verify StopInstance was NOT called (SelfStop mode waits for self-stop)
			Expect(stopCalled).To(BeFalse())

			// Verify node is still in warmup state
			err = k8sClient.Get(ctx, types.NamespacedName{Name: node.Name}, updatedNode)
			Expect(err).NotTo(HaveOccurred())
			Expect(updatedNode.Labels[controller.LabelState]).To(Equal(string(controller.NodeStateWarmup)))

			// Simulate instance self-stopping
			setFakeInstanceState(instanceID, cloudprovider.InstanceStateStopped)

			// Trigger reconciliation
			triggerReconcile(poolName)

			// Wait for the node to transition to standby
			Eventually(func() string {
				n := &corev1.Node{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: node.Name}, n)
				if err != nil {
					return ""
				}
				return n.Labels[controller.LabelState]
			}, timeout, interval).Should(Equal(string(controller.NodeStateStandby)))
		})
	})
})

// createTestNodePoolWithControllerStop creates a NodePool with ControllerStop completion mode.
func createTestNodePoolWithControllerStop(name string, poolSize, minStandby int32) *stratosv1alpha1.NodePool {
	// Create an AWSNodeClass for this pool
	nodeClassName := name + "-nodeclass"
	createTestAWSNodeClass(nodeClassName)

	controllerStop := stratosv1alpha1.WarmupCompletionModeControllerStop
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
			PreWarm: &stratosv1alpha1.PreWarmConfig{
				CompletionMode: &controllerStop,
			},
		},
	}

	reconciler.InjectCloudProvider(name, fakeProvider)
	err := k8sClient.Create(ctx, np)
	Expect(err).NotTo(HaveOccurred())
	return np
}

// createTestNodePoolWithControllerStopAndNetworkReady creates a NodePool with ControllerStop mode
// and WhenNetworkReady startup taint removal mode.
func createTestNodePoolWithControllerStopAndNetworkReady(name string, poolSize, minStandby int32) *stratosv1alpha1.NodePool {
	// Create an AWSNodeClass for this pool
	nodeClassName := name + "-nodeclass"
	createTestAWSNodeClass(nodeClassName)

	controllerStop := stratosv1alpha1.WarmupCompletionModeControllerStop
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
				StartupTaintRemoval: stratosv1alpha1.StartupTaintRemovalWhenNetworkReady,
			},
			PreWarm: &stratosv1alpha1.PreWarmConfig{
				CompletionMode: &controllerStop,
			},
		},
	}

	reconciler.InjectCloudProvider(name, fakeProvider)
	err := k8sClient.Create(ctx, np)
	Expect(err).NotTo(HaveOccurred())
	return np
}

// createTestNodePoolWithControllerStopAndTimeout creates a NodePool with ControllerStop mode
// and a custom timeout.
func createTestNodePoolWithControllerStopAndTimeout(name string, poolSize, minStandby int32, timeout time.Duration) *stratosv1alpha1.NodePool {
	// Create an AWSNodeClass for this pool
	nodeClassName := name + "-nodeclass"
	createTestAWSNodeClass(nodeClassName)

	controllerStop := stratosv1alpha1.WarmupCompletionModeControllerStop
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
			PreWarm: &stratosv1alpha1.PreWarmConfig{
				CompletionMode: &controllerStop,
				Timeout:        &metav1.Duration{Duration: timeout},
			},
		},
	}

	reconciler.InjectCloudProvider(name, fakeProvider)
	err := k8sClient.Create(ctx, np)
	Expect(err).NotTo(HaveOccurred())
	return np
}

// simulateNodeJoinWithReady creates a Node with the specified Ready condition status.
// When ready=true, also sets NodeNetworkUnavailable=False for WhenNetworkReady mode.
func simulateNodeJoinWithReady(poolName, instanceID string, state controller.NodeState, ready bool) *corev1.Node {
	nodeName := fmt.Sprintf("node-%s", instanceID)
	readyStatus := corev1.ConditionFalse
	if ready {
		readyStatus = corev1.ConditionTrue
	}

	conditions := []corev1.NodeCondition{
		{
			Type:   corev1.NodeReady,
			Status: readyStatus,
		},
	}
	// Add network readiness condition when node is ready
	if ready {
		conditions = append(conditions, corev1.NodeCondition{
			Type:   corev1.NodeNetworkUnavailable,
			Status: corev1.ConditionFalse,
		})
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
			Annotations: make(map[string]string),
		},
		Spec: corev1.NodeSpec{
			ProviderID:    fmt.Sprintf("aws:///us-east-1a/%s", instanceID),
			Unschedulable: state != controller.NodeStateRunning,
		},
		Status: corev1.NodeStatus{
			Conditions: conditions,
			Allocatable: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("4"),
				corev1.ResourceMemory: resource.MustParse("8Gi"),
				corev1.ResourcePods:   resource.MustParse("110"),
			},
			Capacity: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("4"),
				corev1.ResourceMemory: resource.MustParse("8Gi"),
				corev1.ResourcePods:   resource.MustParse("110"),
			},
		},
	}

	if state == controller.NodeStateStandby || state == controller.NodeStateWarmup {
		node.Spec.Taints = []corev1.Taint{
			{
				Key:    controller.TaintKeyStandby,
				Effect: corev1.TaintEffectNoExecute,
			},
		}
	}

	err := k8sClient.Create(ctx, node)
	Expect(err).NotTo(HaveOccurred())
	return node
}

// simulateNodeJoinWithReadyNoNetwork creates a Node that is Ready but without any network readiness
// conditions (no NetworkingReady and no NodeNetworkUnavailable=False). Used to test network waiting behavior.
func simulateNodeJoinWithReadyNoNetwork(poolName, instanceID string, state controller.NodeState) *corev1.Node {
	nodeName := fmt.Sprintf("node-%s", instanceID)

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: nodeName,
			Labels: map[string]string{
				controller.LabelPool:       poolName,
				controller.LabelState:      string(state),
				controller.LabelInstanceID: instanceID,
				controller.LabelStateSince: fmt.Sprintf("%d", time.Now().Unix()),
			},
			Annotations: make(map[string]string),
		},
		Spec: corev1.NodeSpec{
			ProviderID:    fmt.Sprintf("aws:///us-east-1a/%s", instanceID),
			Unschedulable: state != controller.NodeStateRunning,
		},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{
					Type:   corev1.NodeReady,
					Status: corev1.ConditionTrue,
				},
				// Intentionally no NetworkingReady or NodeNetworkUnavailable=False
			},
			Allocatable: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("4"),
				corev1.ResourceMemory: resource.MustParse("8Gi"),
				corev1.ResourcePods:   resource.MustParse("110"),
			},
			Capacity: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("4"),
				corev1.ResourceMemory: resource.MustParse("8Gi"),
				corev1.ResourcePods:   resource.MustParse("110"),
			},
		},
	}

	if state == controller.NodeStateStandby || state == controller.NodeStateWarmup {
		node.Spec.Taints = []corev1.Taint{
			{
				Key:    controller.TaintKeyStandby,
				Effect: corev1.TaintEffectNoExecute,
			},
		}
	}

	err := k8sClient.Create(ctx, node)
	Expect(err).NotTo(HaveOccurred())
	return node
}

// simulateNodeJoinWithReadyAndOldTimestamp creates a Node with an old state-since timestamp
// to simulate timeout scenarios.
// When ready=true, also sets NodeNetworkUnavailable=False for WhenNetworkReady mode.
func simulateNodeJoinWithReadyAndOldTimestamp(poolName, instanceID string, state controller.NodeState, ready bool) *corev1.Node {
	nodeName := fmt.Sprintf("node-%s", instanceID)
	readyStatus := corev1.ConditionFalse
	if ready {
		readyStatus = corev1.ConditionTrue
	}

	// Set timestamp to 5 minutes ago to trigger timeout
	oldTimestamp := time.Now().Add(-5 * time.Minute).Unix()

	conditions := []corev1.NodeCondition{
		{
			Type:   corev1.NodeReady,
			Status: readyStatus,
		},
	}
	// Add network readiness condition when node is ready
	if ready {
		conditions = append(conditions, corev1.NodeCondition{
			Type:   corev1.NodeNetworkUnavailable,
			Status: corev1.ConditionFalse,
		})
	}

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: nodeName,
			Labels: map[string]string{
				controller.LabelPool:       poolName,
				controller.LabelState:      string(state),
				controller.LabelInstanceID: instanceID,
				controller.LabelStateSince: fmt.Sprintf("%d", oldTimestamp),
			},
			Annotations: make(map[string]string),
		},
		Spec: corev1.NodeSpec{
			ProviderID:    fmt.Sprintf("aws:///us-east-1a/%s", instanceID),
			Unschedulable: state != controller.NodeStateRunning,
		},
		Status: corev1.NodeStatus{
			Conditions: conditions,
			Allocatable: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("4"),
				corev1.ResourceMemory: resource.MustParse("8Gi"),
				corev1.ResourcePods:   resource.MustParse("110"),
			},
			Capacity: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("4"),
				corev1.ResourceMemory: resource.MustParse("8Gi"),
				corev1.ResourcePods:   resource.MustParse("110"),
			},
		},
	}

	if state == controller.NodeStateStandby || state == controller.NodeStateWarmup {
		node.Spec.Taints = []corev1.Taint{
			{
				Key:    controller.TaintKeyStandby,
				Effect: corev1.TaintEffectNoExecute,
			},
		}
	}

	err := k8sClient.Create(ctx, node)
	Expect(err).NotTo(HaveOccurred())
	return node
}

// simulateNodeJoinWithPoolLabelOnly creates a Node with only the pool label set,
// mimicking Bottlerocket user data behavior where the pool label is set but not
// the state, instance-id, or state-since labels.
func simulateNodeJoinWithPoolLabelOnly(poolName, instanceID string, ready bool) *corev1.Node {
	nodeName := fmt.Sprintf("node-%s", instanceID)
	readyStatus := corev1.ConditionFalse
	if ready {
		readyStatus = corev1.ConditionTrue
	}

	conditions := []corev1.NodeCondition{
		{
			Type:   corev1.NodeReady,
			Status: readyStatus,
		},
	}
	// Add network readiness condition when node is ready
	if ready {
		conditions = append(conditions, corev1.NodeCondition{
			Type:   corev1.NodeNetworkUnavailable,
			Status: corev1.ConditionFalse,
		})
	}

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: nodeName,
			Labels: map[string]string{
				// Only pool label set - no state, instance-id, or state-since
				controller.LabelPool: poolName,
			},
			Annotations: make(map[string]string),
		},
		Spec: corev1.NodeSpec{
			ProviderID:    fmt.Sprintf("aws:///us-east-1a/%s", instanceID),
			Unschedulable: false,
		},
		Status: corev1.NodeStatus{
			Conditions: conditions,
			Allocatable: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("4"),
				corev1.ResourceMemory: resource.MustParse("8Gi"),
				corev1.ResourcePods:   resource.MustParse("110"),
			},
			Capacity: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("4"),
				corev1.ResourceMemory: resource.MustParse("8Gi"),
				corev1.ResourcePods:   resource.MustParse("110"),
			},
		},
	}

	err := k8sClient.Create(ctx, node)
	Expect(err).NotTo(HaveOccurred())
	return node
}
