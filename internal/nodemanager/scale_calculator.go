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

package nodemanager

import (
	"math"

	corev1 "k8s.io/api/core/v1"

	stratosv1alpha1 "github.com/stratos-sh/stratos/api/v1alpha1"
	"github.com/stratos-sh/stratos/internal/cloudprovider/aws"
)

const (
	// NodeCapacityUsagePercent is the percentage of node capacity to use
	// in scale-up calculations. Accounts for DaemonSets and system overhead.
	NodeCapacityUsagePercent = 0.80
)

// ScaleCalculator calculates how many nodes are needed for pending pods
// based on resource requests and node capacity.
type ScaleCalculator struct {
	nodePool *stratosv1alpha1.NodePool
}

// NewScaleCalculator creates a new scale calculator for the given NodePool.
func NewScaleCalculator(pool *stratosv1alpha1.NodePool) *ScaleCalculator {
	return &ScaleCalculator{nodePool: pool}
}

// CalculateNodesNeeded returns the number of nodes needed for the given pods
// based on their resource requests and the node capacity.
// If the instance type is unknown, falls back to 1:1 pod-to-node mapping.
func (c *ScaleCalculator) CalculateNodesNeeded(pods []corev1.Pod, existingNodes []corev1.Node) int {
	if len(pods) == 0 {
		return 0
	}

	// Get node capacity using hybrid lookup
	nodeCapacity := c.getNodeCapacity(existingNodes)
	if nodeCapacity.CPU.IsZero() || nodeCapacity.Memory.IsZero() {
		// Unknown instance type - fall back to 1:1 pod-to-node
		return len(pods)
	}

	// Apply capacity usage factor (80%) to account for DaemonSets/system overhead
	usableCPUMillis := float64(nodeCapacity.CPU.MilliValue()) * NodeCapacityUsagePercent
	usableMemoryBytes := float64(nodeCapacity.Memory.Value()) * NodeCapacityUsagePercent

	// Sum pod resource requests
	totalCPUMillis, totalMemoryBytes := c.sumPodRequests(pods)

	// If pods have no resource requests and no defaults, fall back to 1:1
	if totalCPUMillis == 0 && totalMemoryBytes == 0 {
		return len(pods)
	}

	// Calculate nodes needed based on each resource (ceiling division)
	var nodesByCPU, nodesByMemory int
	if totalCPUMillis > 0 && usableCPUMillis > 0 {
		nodesByCPU = int(math.Ceil(float64(totalCPUMillis) / usableCPUMillis))
	}
	if totalMemoryBytes > 0 && usableMemoryBytes > 0 {
		nodesByMemory = int(math.Ceil(float64(totalMemoryBytes) / usableMemoryBytes))
	}

	// Return the larger of the two (bottleneck resource)
	if nodesByCPU > nodesByMemory {
		return nodesByCPU
	}
	return nodesByMemory
}

// getNodeCapacity returns the capacity of nodes in this pool using hybrid lookup:
// 1. Try existing node's .status.allocatable (real data from actual nodes)
// 2. Fall back to static mapping for AWS instance type
func (c *ScaleCalculator) getNodeCapacity(existingNodes []corev1.Node) aws.InstanceCapacity {
	// Priority 1: Use real data from existing nodes
	for _, node := range existingNodes {
		if node.Status.Allocatable != nil {
			cpu := node.Status.Allocatable[corev1.ResourceCPU]
			mem := node.Status.Allocatable[corev1.ResourceMemory]
			if !cpu.IsZero() && !mem.IsZero() {
				return aws.InstanceCapacity{CPU: cpu, Memory: mem}
			}
		}
	}

	// Priority 2: Static mapping for AWS instance type
	if c.nodePool.Spec.Template.CloudProvider.AWS != nil {
		instanceType := c.nodePool.Spec.Template.CloudProvider.AWS.InstanceType
		return aws.GetInstanceCapacity(instanceType)
	}

	return aws.InstanceCapacity{} // Unknown - will fall back to 1:1
}

// sumPodRequests sums the resource requests of all pods.
// Uses default resources from NodePool config for pods without explicit requests.
func (c *ScaleCalculator) sumPodRequests(pods []corev1.Pod) (cpuMillis int64, memoryBytes int64) {
	defaultCPU, defaultMemory := c.getDefaultResources()

	for i := range pods {
		podCPU, podMemory := c.getPodRequests(&pods[i])

		// Use defaults if pod has no requests
		if podCPU == 0 {
			podCPU = defaultCPU
		}
		if podMemory == 0 {
			podMemory = defaultMemory
		}

		cpuMillis += podCPU
		memoryBytes += podMemory
	}

	return cpuMillis, memoryBytes
}

// getPodRequests returns the total resource requests for a pod
// by summing the requests of all containers.
func (c *ScaleCalculator) getPodRequests(pod *corev1.Pod) (cpuMillis int64, memoryBytes int64) {
	for _, container := range pod.Spec.Containers {
		if container.Resources.Requests != nil {
			if cpu, ok := container.Resources.Requests[corev1.ResourceCPU]; ok {
				cpuMillis += cpu.MilliValue()
			}
			if mem, ok := container.Resources.Requests[corev1.ResourceMemory]; ok {
				memoryBytes += mem.Value()
			}
		}
	}

	// For init containers, we should take the max (they run sequentially),
	// not the sum. But for simplicity and to be conservative, we only
	// consider main containers for scale-up calculation.

	return cpuMillis, memoryBytes
}

// getDefaultResources returns the default resource requests from NodePool config.
// Returns zeros if no defaults are configured.
func (c *ScaleCalculator) getDefaultResources() (cpuMillis int64, memoryBytes int64) {
	if c.nodePool.Spec.ScaleUp == nil || c.nodePool.Spec.ScaleUp.DefaultPodResources == nil {
		return 0, 0
	}

	defaults := c.nodePool.Spec.ScaleUp.DefaultPodResources.Requests
	if defaults == nil {
		return 0, 0
	}

	if cpu, ok := defaults[corev1.ResourceCPU]; ok {
		cpuMillis = cpu.MilliValue()
	}
	if mem, ok := defaults[corev1.ResourceMemory]; ok {
		memoryBytes = mem.Value()
	}

	return cpuMillis, memoryBytes
}
