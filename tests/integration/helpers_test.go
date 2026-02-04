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

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	stratosv1alpha1 "github.com/stratos-sh/stratos/api/v1alpha1"
	"github.com/stratos-sh/stratos/internal/cloudprovider"
	"github.com/stratos-sh/stratos/internal/controller/nodepool/nodestate"
)

// testAWSNodeClass returns a minimal AWSNodeClass for testing
func testAWSNodeClass(name string) *stratosv1alpha1.AWSNodeClass {
	return &stratosv1alpha1.AWSNodeClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: stratosv1alpha1.AWSNodeClassSpec{
			Region:             "us-east-1",
			InstanceType:       "m5.large",
			BootstrapTemplate: stratosv1alpha1.BootstrapTemplateAL2023,
			SubnetIDs:          []string{"subnet-12345678"},
			SecurityGroupIDs:   []string{"sg-12345678"},
			IAMInstanceProfile: "test-profile",
		},
	}
}

// createTestAWSNodeClass creates an AWSNodeClass for testing.
func createTestAWSNodeClass(name string) *stratosv1alpha1.AWSNodeClass {
	nc := testAWSNodeClass(name)
	err := k8sClient.Create(ctx, nc)
	Expect(err).NotTo(HaveOccurred())
	return nc
}

// createTestNodePool creates a NodePool for testing with default values.
func createTestNodePool(name string, poolSize, minStandby int32) *stratosv1alpha1.NodePool {
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

// createTestNodePoolWithScaleDown creates a NodePool with scale-down configuration.
func createTestNodePoolWithScaleDown(name string, poolSize, minStandby int32, emptyNodeTTL time.Duration) *stratosv1alpha1.NodePool {
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
				NodeClassRef: stratosv1alpha1.NodeClassRef{
					Kind: "AWSNodeClass",
					Name: nodeClassName,
				},
			},
			ScaleDown: &stratosv1alpha1.ScaleDownConfig{
				Enabled:      boolPtr(true),
				EmptyNodeTTL: &metav1.Duration{Duration: emptyNodeTTL},
			},
		},
	}

	// Inject fake provider for this pool
	reconciler.InjectCloudProvider(name, fakeProvider)

	err := k8sClient.Create(ctx, np)
	Expect(err).NotTo(HaveOccurred())
	return np
}

// simulateNodeJoin creates a K8s Node object that simulates an EC2 instance joining the cluster.
// This bridges the gap between the fake EC2 provider and Kubernetes.
func simulateNodeJoin(poolName, instanceID string, nodeState nodestate.NodeState) *corev1.Node {
	nodeName := fmt.Sprintf("node-%s", instanceID)

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: nodeName,
			Labels: map[string]string{
				nodestate.LabelPool:       poolName,
				nodestate.LabelState:      string(nodeState),
				nodestate.LabelInstanceID: instanceID,
				nodestate.LabelStateSince: fmt.Sprintf("%d", time.Now().Unix()),
			},
			Annotations: make(map[string]string),
		},
		Spec: corev1.NodeSpec{
			ProviderID:    fmt.Sprintf("aws:///us-east-1a/%s", instanceID),
			Unschedulable: nodeState != nodestate.NodeStateRunning,
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

	// Add standby taint for non-running nodes
	if nodeState == nodestate.NodeStateStandby || nodeState == nodestate.NodeStateWarmup {
		node.Spec.Taints = []corev1.Taint{
			{
				Key:    nodestate.TaintKeyStandby,
				Effect: corev1.TaintEffectNoExecute,
			},
		}
	}

	err := k8sClient.Create(ctx, node)
	Expect(err).NotTo(HaveOccurred())
	return node
}

// simulateNodeJoinWithResources creates a Node with custom resource capacity.
func simulateNodeJoinWithResources(poolName, instanceID string, nodeState nodestate.NodeState, cpu, memory string) *corev1.Node {
	node := simulateNodeJoin(poolName, instanceID, nodeState)

	// Update with custom resources
	patch := client.MergeFrom(node.DeepCopy())
	node.Status.Allocatable = corev1.ResourceList{
		corev1.ResourceCPU:    resource.MustParse(cpu),
		corev1.ResourceMemory: resource.MustParse(memory),
		corev1.ResourcePods:   resource.MustParse("110"),
	}
	node.Status.Capacity = node.Status.Allocatable

	err := k8sClient.Status().Patch(ctx, node, patch)
	Expect(err).NotTo(HaveOccurred())
	return node
}

// createUnschedulablePod creates a pending pod that cannot be scheduled.
func createUnschedulablePod(namespace, name, cpu, memory string) *corev1.Pod {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "test-container",
					Image: "nginx:latest",
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse(cpu),
							corev1.ResourceMemory: resource.MustParse(memory),
						},
					},
				},
			},
		},
	}

	err := k8sClient.Create(ctx, pod)
	Expect(err).NotTo(HaveOccurred())

	// Mark as unschedulable by setting the PodScheduled condition to false
	// with reason Unschedulable
	pod.Status.Phase = corev1.PodPending
	pod.Status.Conditions = []corev1.PodCondition{
		{
			Type:    corev1.PodScheduled,
			Status:  corev1.ConditionFalse,
			Reason:  corev1.PodReasonUnschedulable,
			Message: "0/0 nodes are available: insufficient cpu",
		},
	}

	err = k8sClient.Status().Update(ctx, pod)
	Expect(err).NotTo(HaveOccurred())
	return pod
}

// createPodOnNode creates a pod scheduled to a specific node.
func createPodOnNode(namespace, name, nodeName string) *corev1.Pod {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: corev1.PodSpec{
			NodeName: nodeName,
			Containers: []corev1.Container{
				{
					Name:  "test-container",
					Image: "nginx:latest",
				},
			},
		},
	}

	err := k8sClient.Create(ctx, pod)
	Expect(err).NotTo(HaveOccurred())

	// Mark as running
	pod.Status.Phase = corev1.PodRunning
	err = k8sClient.Status().Update(ctx, pod)
	Expect(err).NotTo(HaveOccurred())
	return pod
}

// eventuallyNodeState waits for a node to reach the expected nodestate.
func eventuallyNodeState(nodeName string, expectedState nodestate.NodeState) {
	Eventually(func() string {
		node := &corev1.Node{}
		err := k8sClient.Get(ctx, types.NamespacedName{Name: nodeName}, node)
		if err != nil {
			return ""
		}
		return node.Labels[nodestate.LabelState]
	}, timeout, interval).Should(Equal(string(expectedState)))
}

// eventuallyNodePoolStatus waits for a NodePool to have the expected status counts.
func eventuallyNodePoolStatus(name string, standby, running int32) {
	Eventually(func() bool {
		np := &stratosv1alpha1.NodePool{}
		err := k8sClient.Get(ctx, types.NamespacedName{Name: name}, np)
		if err != nil {
			return false
		}
		return np.Status.Standby == standby && np.Status.Running == running
	}, timeout, interval).Should(BeTrue())
}

// eventuallyNodePoolReady waits for a NodePool to become ready.
func eventuallyNodePoolReady(name string) {
	Eventually(func() bool {
		np := &stratosv1alpha1.NodePool{}
		err := k8sClient.Get(ctx, types.NamespacedName{Name: name}, np)
		if err != nil {
			return false
		}
		for _, cond := range np.Status.Conditions {
			if cond.Type == stratosv1alpha1.ConditionTypeReady && cond.Status == metav1.ConditionTrue {
				return true
			}
		}
		return false
	}, timeout, interval).Should(BeTrue())
}

// getNodePool retrieves a NodePool by name.
func getNodePool(name string) *stratosv1alpha1.NodePool {
	np := &stratosv1alpha1.NodePool{}
	err := k8sClient.Get(ctx, types.NamespacedName{Name: name}, np)
	Expect(err).NotTo(HaveOccurred())
	return np
}

// getNode retrieves a Node by name.
func getNode(name string) *corev1.Node {
	node := &corev1.Node{}
	err := k8sClient.Get(ctx, types.NamespacedName{Name: name}, node)
	Expect(err).NotTo(HaveOccurred())
	return node
}

// updateNodeState updates a node's state label.
func updateNodeState(nodeName string, newState nodestate.NodeState) {
	node := &corev1.Node{}
	err := k8sClient.Get(ctx, types.NamespacedName{Name: nodeName}, node)
	Expect(err).NotTo(HaveOccurred())

	patch := client.MergeFrom(node.DeepCopy())
	node.Labels[nodestate.LabelState] = string(newState)
	err = k8sClient.Patch(ctx, node, patch)
	Expect(err).NotTo(HaveOccurred())
}

// setFakeInstanceState sets the state of a fake instance in the provider.
func setFakeInstanceState(instanceID string, instanceState cloudprovider.InstanceState) {
	err := fakeProvider.SetInstanceState(instanceID, instanceState)
	Expect(err).NotTo(HaveOccurred())
}

// launchFakeInstance launches a fake instance and returns its ID.
func launchFakeInstance(poolName string) string {
	nodeClass := testAWSNodeClass(poolName + "-test-class")
	instance, err := fakeProvider.LaunchInstance(ctx, nodeClass, poolName, testClusterName, nil)
	Expect(err).NotTo(HaveOccurred())
	return instance.ID
}

// boolPtr returns a pointer to a bool.
func boolPtr(b bool) *bool {
	return &b
}

// ensureNamespace creates a namespace if it doesn't exist.
func ensureNamespace(name string) {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
	err := k8sClient.Create(ctx, ns)
	if err != nil {
		err = client.IgnoreAlreadyExists(err)
		if err != nil {
			Expect(err).NotTo(HaveOccurred())
		}
	}
}

// triggerReconcile forces a reconciliation for a NodePool by updating an annotation.
func triggerReconcile(name string) {
	np := &stratosv1alpha1.NodePool{}
	err := k8sClient.Get(ctx, types.NamespacedName{Name: name}, np)
	Expect(err).NotTo(HaveOccurred())

	patch := client.MergeFrom(np.DeepCopy())
	if np.Annotations == nil {
		np.Annotations = make(map[string]string)
	}
	np.Annotations["test.stratos.sh/trigger"] = fmt.Sprintf("%d", time.Now().UnixNano())
	err = k8sClient.Patch(ctx, np, patch)
	Expect(err).NotTo(HaveOccurred())
}

// countNodesForPool counts nodes belonging to a specific pool.
func countNodesForPool(poolName string) int {
	nodeList := &corev1.NodeList{}
	err := k8sClient.List(ctx, nodeList, client.MatchingLabels{
		nodestate.LabelPool: poolName,
	})
	Expect(err).NotTo(HaveOccurred())
	return len(nodeList.Items)
}

// countNodesByStateForPool counts nodes by state for a specific pool.
func countNodesByStateForPool(poolName string, nodeState nodestate.NodeState) int {
	nodeList := &corev1.NodeList{}
	err := k8sClient.List(ctx, nodeList, client.MatchingLabels{
		nodestate.LabelPool:  poolName,
		nodestate.LabelState: string(nodeState),
	})
	Expect(err).NotTo(HaveOccurred())
	return len(nodeList.Items)
}
