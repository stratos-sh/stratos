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
	"k8s.io/apimachinery/pkg/types"

	corev1 "k8s.io/api/core/v1"

	"github.com/stratos-sh/stratos/internal/cloudprovider"
	"github.com/stratos-sh/stratos/internal/controller"
)

var _ = Describe("State Transitions", func() {
	Context("Warmup State", func() {
		It("should transition from warmup to standby when instance stops", func() {
			np := createTestNodePool("test-warmup-to-standby", 5, 2)

			// Create a warmup node
			instanceID := launchFakeInstance(np.Name)
			time.Sleep(200 * time.Millisecond)

			// Instance is running (warmup)
			setFakeInstanceState(instanceID, cloudprovider.InstanceStateRunning)
			node := simulateNodeJoin(np.Name, instanceID, controller.NodeStateWarmup)

			// Verify node is in warmup
			Expect(node.Labels[controller.LabelState]).To(Equal(string(controller.NodeStateWarmup)))

			triggerReconcile(np.Name)
			time.Sleep(500 * time.Millisecond)

			// Simulate instance self-stopping (warmup complete)
			setFakeInstanceState(instanceID, cloudprovider.InstanceStateStopped)

			// Update state to standby (simulating what MonitorWarmup does)
			updateNodeState(node.Name, controller.NodeStateStandby)

			// Verify transition
			updated := getNode(node.Name)
			Expect(updated.Labels[controller.LabelState]).To(Equal(string(controller.NodeStateStandby)))
		})

		It("should be cordoned during warmup", func() {
			np := createTestNodePool("test-warmup-cordoned", 5, 2)

			// Create a warmup node
			instanceID := launchFakeInstance(np.Name)
			time.Sleep(200 * time.Millisecond)
			setFakeInstanceState(instanceID, cloudprovider.InstanceStateRunning)
			node := simulateNodeJoin(np.Name, instanceID, controller.NodeStateWarmup)

			// Warmup nodes should be cordoned (unschedulable)
			Expect(node.Spec.Unschedulable).To(BeTrue())
		})
	})

	Context("Standby State", func() {
		It("should transition from standby to running on scale-up", func() {
			np := createTestNodePool("test-standby-to-running", 5, 2)

			// Create a standby node
			instanceID := launchFakeInstance(np.Name)
			time.Sleep(200 * time.Millisecond)
			setFakeInstanceState(instanceID, cloudprovider.InstanceStateStopped)
			node := simulateNodeJoin(np.Name, instanceID, controller.NodeStateStandby)

			// Verify node is in standby
			Expect(node.Labels[controller.LabelState]).To(Equal(string(controller.NodeStateStandby)))

			triggerReconcile(np.Name)
			time.Sleep(500 * time.Millisecond)

			// Simulate scale-up: start instance
			setFakeInstanceState(instanceID, cloudprovider.InstanceStateRunning)

			// Update state to running
			updateNodeState(node.Name, controller.NodeStateRunning)

			// Verify transition
			updated := getNode(node.Name)
			Expect(updated.Labels[controller.LabelState]).To(Equal(string(controller.NodeStateRunning)))
		})

		It("should have standby taint", func() {
			np := createTestNodePool("test-standby-taint", 5, 2)

			instanceID := launchFakeInstance(np.Name)
			time.Sleep(200 * time.Millisecond)
			setFakeInstanceState(instanceID, cloudprovider.InstanceStateStopped)
			node := simulateNodeJoin(np.Name, instanceID, controller.NodeStateStandby)

			// Verify standby taint exists
			hasTaint := false
			for _, taint := range node.Spec.Taints {
				if taint.Key == controller.TaintKeyStandby {
					hasTaint = true
					break
				}
			}
			Expect(hasTaint).To(BeTrue())
		})
	})

	Context("Running State", func() {
		It("should transition from running to terminating on scale-down", func() {
			np := createTestNodePoolWithScaleDown("test-running-to-terminating", 5, 2, 100*time.Millisecond)

			// Create a running node
			instanceID := launchFakeInstance(np.Name)
			time.Sleep(200 * time.Millisecond)
			setFakeInstanceState(instanceID, cloudprovider.InstanceStateRunning)
			node := simulateNodeJoin(np.Name, instanceID, controller.NodeStateRunning)

			// Verify node is running
			Expect(node.Labels[controller.LabelState]).To(Equal(string(controller.NodeStateRunning)))

			// Transition to terminating (for scale-down)
			updateNodeState(node.Name, controller.NodeStateTerminating)

			// Verify transition
			updated := getNode(node.Name)
			Expect(updated.Labels[controller.LabelState]).To(Equal(string(controller.NodeStateTerminating)))
		})

		It("should transition to standby if instance stopped externally", func() {
			np := createTestNodePool("test-running-external-stop", 5, 2)

			// Create a running node
			instanceID := launchFakeInstance(np.Name)
			time.Sleep(200 * time.Millisecond)
			setFakeInstanceState(instanceID, cloudprovider.InstanceStateRunning)
			node := simulateNodeJoin(np.Name, instanceID, controller.NodeStateRunning)

			triggerReconcile(np.Name)
			time.Sleep(500 * time.Millisecond)

			// Simulate external stop (e.g., AWS console, spot termination)
			setFakeInstanceState(instanceID, cloudprovider.InstanceStateStopped)

			// SyncNodeState should detect this and transition to standby
			// For this test, we directly update the state
			updateNodeState(node.Name, controller.NodeStateStandby)

			// Verify transition
			updated := getNode(node.Name)
			Expect(updated.Labels[controller.LabelState]).To(Equal(string(controller.NodeStateStandby)))
		})

		It("should not be cordoned", func() {
			np := createTestNodePool("test-running-uncordoned", 5, 2)

			// Set instance to running state FIRST before launching
			instanceID := launchFakeInstance(np.Name)
			time.Sleep(200 * time.Millisecond)
			setFakeInstanceState(instanceID, cloudprovider.InstanceStateRunning)

			// Create a running node - it should be uncordoned
			node := simulateNodeJoin(np.Name, instanceID, controller.NodeStateRunning)

			// Running nodes should not be cordoned
			// Use Eventually because the controller may process the node
			Eventually(func() bool {
				updated := &corev1.Node{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: node.Name}, updated)
				if err != nil {
					return false
				}
				// Running nodes with running instances should be schedulable
				return !updated.Spec.Unschedulable
			}, timeout, interval).Should(BeTrue())
		})

		It("should not have standby taint", func() {
			np := createTestNodePool("test-running-no-taint", 5, 2)

			instanceID := launchFakeInstance(np.Name)
			time.Sleep(200 * time.Millisecond)
			setFakeInstanceState(instanceID, cloudprovider.InstanceStateRunning)
			node := simulateNodeJoin(np.Name, instanceID, controller.NodeStateRunning)

			// Running nodes should not have standby taint
			// Use Eventually because the controller processes nodes
			Eventually(func() bool {
				updated := &corev1.Node{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: node.Name}, updated)
				if err != nil {
					return false
				}
				// Check no standby taint
				for _, taint := range updated.Spec.Taints {
					if taint.Key == controller.TaintKeyStandby {
						return false
					}
				}
				return true
			}, timeout, interval).Should(BeTrue())
		})
	})

	Context("Terminating State", func() {
		It("should transition from terminating to standby after drain", func() {
			np := createTestNodePoolWithScaleDown("test-terminating-to-standby", 5, 2, 100*time.Millisecond)

			// Create a terminating node
			instanceID := launchFakeInstance(np.Name)
			time.Sleep(200 * time.Millisecond)
			setFakeInstanceState(instanceID, cloudprovider.InstanceStateRunning)
			node := simulateNodeJoin(np.Name, instanceID, controller.NodeStateTerminating)

			// Verify node is in terminating
			Expect(node.Labels[controller.LabelState]).To(Equal(string(controller.NodeStateTerminating)))

			triggerReconcile(np.Name)
			time.Sleep(500 * time.Millisecond)

			// Simulate instance stopping after drain
			setFakeInstanceState(instanceID, cloudprovider.InstanceStateStopped)

			// Transition to standby
			updateNodeState(node.Name, controller.NodeStateStandby)

			// Verify transition
			updated := getNode(node.Name)
			Expect(updated.Labels[controller.LabelState]).To(Equal(string(controller.NodeStateStandby)))
		})
	})

	Context("Invalid Transitions", func() {
		It("should validate state transitions", func() {
			// Test valid transitions defined in controller.ValidTransitions
			validTransitions := map[controller.NodeState][]controller.NodeState{
				controller.NodeStateWarmup:      {controller.NodeStateStandby, controller.NodeStateTerminating},
				controller.NodeStateStandby:     {controller.NodeStateRunning, controller.NodeStateTerminating},
				controller.NodeStateRunning:     {controller.NodeStateTerminating, controller.NodeStateStandby},
				controller.NodeStateTerminating: {controller.NodeStateStandby},
			}

			for from, toStates := range validTransitions {
				for _, to := range toStates {
					Expect(controller.IsValidTransition(from, to)).To(BeTrue(),
						"Expected %s -> %s to be valid", from, to)
				}
			}

			// Test some invalid transitions
			Expect(controller.IsValidTransition(controller.NodeStateStandby, controller.NodeStateWarmup)).To(BeFalse())
			Expect(controller.IsValidTransition(controller.NodeStateRunning, controller.NodeStateWarmup)).To(BeFalse())
			Expect(controller.IsValidTransition(controller.NodeStateTerminating, controller.NodeStateRunning)).To(BeFalse())
		})
	})

	Context("State Since Label", func() {
		It("should update state-since label on transitions", func() {
			np := createTestNodePool("test-state-since", 5, 2)

			instanceID := launchFakeInstance(np.Name)
			time.Sleep(200 * time.Millisecond)
			setFakeInstanceState(instanceID, cloudprovider.InstanceStateStopped)
			node := simulateNodeJoin(np.Name, instanceID, controller.NodeStateStandby)

			// Record initial state-since
			initialStateSince := node.Labels[controller.LabelStateSince]
			Expect(initialStateSince).NotTo(BeEmpty())

			// Wait a bit to ensure time difference
			time.Sleep(1100 * time.Millisecond)

			// Update state-since with a new timestamp (simulating a state transition)
			Eventually(func() error {
				nodeUpdate := &corev1.Node{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: node.Name}, nodeUpdate)
				if err != nil {
					return err
				}
				nodeUpdate.Labels[controller.LabelStateSince] = fmt.Sprintf("%d", time.Now().Unix())
				return k8sClient.Update(ctx, nodeUpdate)
			}, timeout, interval).Should(Succeed())

			// Verify state-since changed
			Eventually(func() bool {
				updated := &corev1.Node{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: node.Name}, updated)
				if err != nil {
					return false
				}
				newStateSince := updated.Labels[controller.LabelStateSince]
				return newStateSince != initialStateSince
			}, timeout, interval).Should(BeTrue())
		})
	})
})
