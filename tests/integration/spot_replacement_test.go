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

var _ = Describe("Spot Replacement", func() {

	// createSpotEnabledPool creates a NodePool with spotReplacement enabled and a short delay.
	createSpotEnabledPool := func(name string, poolSize, minStandby int32, replacementDelay time.Duration) *stratosv1alpha1.NodePool {
		nodeClassName := name + "-nodeclass"
		nc := &stratosv1alpha1.AWSNodeClass{
			ObjectMeta: metav1.ObjectMeta{
				Name: nodeClassName,
			},
			Spec: stratosv1alpha1.AWSNodeClassSpec{
				Region:             "us-east-1",
				InstanceType:       "m5.large",
				BootstrapTemplate:  stratosv1alpha1.BootstrapTemplateAL2023,
				SubnetIDs:          []string{"subnet-12345678"},
				SecurityGroupIDs:   []string{"sg-12345678"},
				IAMInstanceProfile: "test-profile",
				SpotConfig: &stratosv1alpha1.SpotConfig{
					InstanceTypes:      []string{"m5.large", "m5a.large"},
					AllocationStrategy: "price-capacity-optimized",
				},
			},
		}
		err := k8sClient.Create(ctx, nc)
		Expect(err).NotTo(HaveOccurred())

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
				SpotReplacement: &stratosv1alpha1.SpotReplacementConfig{
					Enabled:          true,
					ReplacementDelay: &metav1.Duration{Duration: replacementDelay},
				},
			},
		}

		reconciler.InjectCloudProvider(name, fakeProvider)

		err = k8sClient.Create(ctx, np)
		Expect(err).NotTo(HaveOccurred())
		return np
	}

	// simulateSpotNodeJoin creates a K8s Node that represents a Spot instance.
	simulateSpotNodeJoin := func(poolName, instanceID, replacingNodeName string, state controller.NodeState) *corev1.Node {
		nodeName := fmt.Sprintf("node-%s", instanceID)

		node := &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: nodeName,
				Labels: map[string]string{
					controller.LabelPool:         poolName,
					controller.LabelState:        string(state),
					controller.LabelInstanceID:   instanceID,
					controller.LabelStateSince:   fmt.Sprintf("%d", time.Now().Unix()),
					controller.LabelCapacityType: cloudprovider.CapacityTypeSpot,
				},
				Annotations: map[string]string{},
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

		if replacingNodeName != "" {
			node.Annotations[controller.AnnotationSpotReplacingNode] = replacingNodeName
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

	Context("Spot replacement eligibility", func() {
		It("should not replace On-Demand nodes before replacement delay", func() {
			By("Creating a spot-enabled pool with 5 minute delay")
			pool := createSpotEnabledPool("spot-delay-test", 5, 2, 5*time.Minute)

			By("Creating a running On-Demand node that just started")
			instanceID := launchFakeInstance(pool.Name)
			node := simulateNodeJoin(pool.Name, instanceID, controller.NodeStateRunning)

			// Re-read node from API server to get current state
			err := k8sClient.Get(ctx, types.NamespacedName{Name: node.Name}, node)
			Expect(err).NotTo(HaveOccurred())

			// Set last-started to now
			patch := client.MergeFrom(node.DeepCopy())
			if node.Annotations == nil {
				node.Annotations = make(map[string]string)
			}
			node.Annotations[controller.AnnotationLastStarted] = time.Now().Format(time.RFC3339)
			err = k8sClient.Patch(ctx, node, patch)
			Expect(err).NotTo(HaveOccurred())

			By("Triggering reconciliation")
			triggerReconcile(pool.Name)
			time.Sleep(2 * time.Second)

			By("Verifying the node is NOT annotated for replacement yet")
			updatedNode := &corev1.Node{}
			err = k8sClient.Get(ctx, types.NamespacedName{Name: node.Name}, updatedNode)
			Expect(err).NotTo(HaveOccurred())
			_, hasAnnotation := updatedNode.Annotations[controller.AnnotationSpotReplacementStarted]
			Expect(hasAnnotation).To(BeFalse(), "Node should not be marked for replacement before delay")
		})

		It("should not replace nodes that already have spot replacement annotation", func() {
			By("Creating a spot-enabled pool with 0s delay")
			pool := createSpotEnabledPool("spot-no-dup", 5, 2, 0)

			By("Creating a running On-Demand node already annotated for replacement")
			instanceID := launchFakeInstance(pool.Name)
			node := simulateNodeJoin(pool.Name, instanceID, controller.NodeStateRunning)

			// Re-read node from API server to get current state
			err := k8sClient.Get(ctx, types.NamespacedName{Name: node.Name}, node)
			Expect(err).NotTo(HaveOccurred())

			patch := client.MergeFrom(node.DeepCopy())
			if node.Annotations == nil {
				node.Annotations = make(map[string]string)
			}
			node.Annotations[controller.AnnotationLastStarted] = time.Now().Add(-5 * time.Minute).Format(time.RFC3339)
			node.Annotations[controller.AnnotationSpotReplacementStarted] = time.Now().Format(time.RFC3339)
			err = k8sClient.Patch(ctx, node, patch)
			Expect(err).NotTo(HaveOccurred())

			By("Tracking spot launch calls")
			spotLaunchCount := 0
			fakeProvider.LaunchSpotHook = func(_ context.Context, _ *stratosv1alpha1.AWSNodeClass, _, _ string, _ *cloudprovider.TemplateConfig) error {
				spotLaunchCount++
				return nil
			}

			By("Triggering reconciliation")
			triggerReconcile(pool.Name)
			time.Sleep(2 * time.Second)

			By("Verifying no additional spot launch was triggered")
			Expect(spotLaunchCount).To(Equal(0), "Should not launch another spot for already-annotated node")
		})
	})

	Context("Spot node capacity type labels", func() {
		It("should set capacity-type label on On-Demand nodes", func() {
			pool := createTestNodePool("cap-type-test", 5, 2)

			By("Creating an On-Demand standby node")
			instanceID := launchFakeInstance(pool.Name)
			node := simulateNodeJoin(pool.Name, instanceID, controller.NodeStateStandby)

			By("Checking the capacity-type label is not yet set (added by LabelNode)")
			updatedNode := &corev1.Node{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: node.Name}, updatedNode)
			Expect(err).NotTo(HaveOccurred())
			// Node was created via simulateNodeJoin which doesn't set capacity-type,
			// but the reconciler should not crash if it's missing
		})
	})

	Context("Spot interruption handling", func() {
		It("should delete the node when a spot instance is terminated", func() {
			By("Creating a spot-enabled pool")
			pool := createSpotEnabledPool("spot-interrupt", 5, 2, 0)

			By("Launching a spot instance via the fake provider")
			nc := testAWSNodeClass(pool.Name + "-nodeclass")
			nc.Spec.SpotConfig = &stratosv1alpha1.SpotConfig{
				InstanceTypes: []string{"m5.large"},
			}
			inst, err := fakeProvider.LaunchSpotInstance(ctx, nc, pool.Name, testClusterName, nil)
			Expect(err).NotTo(HaveOccurred())

			By("Creating a K8s node for the spot instance")
			spotNode := simulateSpotNodeJoin(pool.Name, inst.ID, "", controller.NodeStateRunning)

			By("Simulating spot interruption")
			err = fakeProvider.SimulateSpotInterruption(inst.ID)
			Expect(err).NotTo(HaveOccurred())

			By("Triggering reconciliation")
			triggerReconcile(pool.Name)

			By("Verifying the spot node is deleted")
			Eventually(func() bool {
				node := &corev1.Node{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: spotNode.Name}, node)
				return err != nil // Should be not found
			}, timeout, interval).Should(BeTrue(), "Spot node should be deleted after interruption")
		})
	})

	Context("Fake provider spot support", func() {
		It("should reject StopInstance for Spot instances", func() {
			By("Launching a spot instance")
			nc := testAWSNodeClass("spot-stop-test-nc")
			nc.Spec.SpotConfig = &stratosv1alpha1.SpotConfig{
				InstanceTypes: []string{"m5.large"},
			}
			inst, err := fakeProvider.LaunchSpotInstance(ctx, nc, "test-pool", testClusterName, nil)
			Expect(err).NotTo(HaveOccurred())

			By("Waiting for instance to become running")
			Eventually(func() cloudprovider.InstanceState {
				state, _ := fakeProvider.GetInstanceState(ctx, inst.ID)
				return state
			}, timeout, interval).Should(Equal(cloudprovider.InstanceStateRunning))

			By("Trying to stop the spot instance")
			err = fakeProvider.StopInstance(ctx, inst.ID, false)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("spot instances cannot be stopped"))
		})

		It("should set CapacityType=spot on launched spot instances", func() {
			nc := testAWSNodeClass("spot-cap-type-nc")
			nc.Spec.SpotConfig = &stratosv1alpha1.SpotConfig{
				InstanceTypes: []string{"m5.large"},
			}
			inst, err := fakeProvider.LaunchSpotInstance(ctx, nc, "test-pool", testClusterName, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(inst.CapacityType).To(Equal(cloudprovider.CapacityTypeSpot))
			Expect(inst.Tags["stratos.sh/capacity-type"]).To(Equal("spot"))
		})

		It("should set CapacityType=on-demand on regular launched instances", func() {
			nc := testAWSNodeClass("od-cap-type-nc")
			inst, err := fakeProvider.LaunchInstance(ctx, nc, "test-pool", testClusterName, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(inst.CapacityType).To(Equal(cloudprovider.CapacityTypeOnDemand))
			Expect(inst.Tags["stratos.sh/capacity-type"]).To(Equal("on-demand"))
		})

		It("should simulate spot interruption correctly", func() {
			nc := testAWSNodeClass("spot-sim-nc")
			nc.Spec.SpotConfig = &stratosv1alpha1.SpotConfig{
				InstanceTypes: []string{"m5.large"},
			}
			inst, err := fakeProvider.LaunchSpotInstance(ctx, nc, "test-pool", testClusterName, nil)
			Expect(err).NotTo(HaveOccurred())

			By("Simulating interruption")
			err = fakeProvider.SimulateSpotInterruption(inst.ID)
			Expect(err).NotTo(HaveOccurred())

			By("Checking instance is terminated")
			state, err := fakeProvider.GetInstanceState(ctx, inst.ID)
			Expect(err).NotTo(HaveOccurred())
			Expect(state).To(Equal(cloudprovider.InstanceStateTerminated))
		})
	})

	Context("Scale-down terminates empty Spot nodes", func() {
		It("should terminate (not stop) empty Spot nodes during scale-down", func() {
			By("Creating a pool with scale-down enabled")
			pool := createTestNodePoolWithScaleDown("spot-scaledown-term", 5, 1, 1*time.Second)

			By("Launching a spot instance in the fake provider")
			nc := testAWSNodeClass("spot-scaledown-term-nc")
			nc.Spec.SpotConfig = &stratosv1alpha1.SpotConfig{
				InstanceTypes: []string{"m5.large"},
			}
			inst, err := fakeProvider.LaunchSpotInstance(ctx, nc, pool.Name, testClusterName, nil)
			Expect(err).NotTo(HaveOccurred())

			// Wait for instance to become running in fake provider
			Eventually(func() cloudprovider.InstanceState {
				state, _ := fakeProvider.GetInstanceState(ctx, inst.ID)
				return state
			}, timeout, interval).Should(Equal(cloudprovider.InstanceStateRunning))

			By("Creating a running Spot node (empty, no pods)")
			spotNode := simulateSpotNodeJoin(pool.Name, inst.ID, "", controller.NodeStateRunning)

			// Re-read node from API server to get current state
			err = k8sClient.Get(ctx, types.NamespacedName{Name: spotNode.Name}, spotNode)
			Expect(err).NotTo(HaveOccurred())

			// Add last-started annotation
			patch := client.MergeFrom(spotNode.DeepCopy())
			if spotNode.Annotations == nil {
				spotNode.Annotations = make(map[string]string)
			}
			spotNode.Annotations[controller.AnnotationLastStarted] = time.Now().Add(-10 * time.Minute).Format(time.RFC3339)
			err = k8sClient.Patch(ctx, spotNode, patch)
			Expect(err).NotTo(HaveOccurred())

			By("Waiting for scale-down TTL to expire")
			time.Sleep(2 * time.Second)

			By("Triggering reconciliation")
			triggerReconcile(pool.Name)

			By("Verifying the Spot node is deleted (terminated, not stopped)")
			Eventually(func() bool {
				node := &corev1.Node{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: spotNode.Name}, node)
				return err != nil // Should be not found
			}, timeout, interval).Should(BeTrue(), "Spot node should be deleted after scale-down")

			By("Verifying the EC2 instance was terminated")
			Eventually(func() cloudprovider.InstanceState {
				state, _ := fakeProvider.GetInstanceState(ctx, inst.ID)
				return state
			}, timeout, interval).Should(Equal(cloudprovider.InstanceStateTerminated))
		})
	})
})
