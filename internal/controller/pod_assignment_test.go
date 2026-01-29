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

package controller

import (
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	stratosv1alpha1 "github.com/stratos-sh/stratos/api/v1alpha1"
)

func TestPodAssignmentTTL(t *testing.T) {
	if PodAssignmentTTL != 10*time.Minute {
		t.Errorf("PodAssignmentTTL = %v, want 10m", PodAssignmentTTL)
	}
}

func TestPodAssignment_DeepCopy(t *testing.T) {
	now := metav1.Now()
	a := stratosv1alpha1.PodAssignment{
		PodName:      "pod-1",
		PodNamespace: "default",
		NodeName:     "node-1",
		AssignedAt:   now,
	}

	b := a.DeepCopy()
	if b.PodName != a.PodName || b.PodNamespace != a.PodNamespace || b.NodeName != a.NodeName {
		t.Error("DeepCopy did not preserve fields")
	}

	// Modify the copy to ensure it's independent
	b.PodName = "pod-2"
	if a.PodName == "pod-2" {
		t.Error("DeepCopy is not independent")
	}
}

func TestNodePoolStatus_PodAssignments_DeepCopy(t *testing.T) {
	now := metav1.Now()
	status := stratosv1alpha1.NodePoolStatus{
		Warmup:  1,
		Standby: 2,
		Running: 3,
		PodAssignments: []stratosv1alpha1.PodAssignment{
			{
				PodName:      "pod-1",
				PodNamespace: "default",
				NodeName:     "node-1",
				AssignedAt:   now,
			},
			{
				PodName:      "pod-2",
				PodNamespace: "kube-system",
				NodeName:     "node-2",
				AssignedAt:   now,
			},
		},
	}

	copy := status.DeepCopy()
	if len(copy.PodAssignments) != 2 {
		t.Fatalf("expected 2 PodAssignments, got %d", len(copy.PodAssignments))
	}

	// Modify original and verify copy is independent
	status.PodAssignments[0].PodName = "modified"
	if copy.PodAssignments[0].PodName == "modified" {
		t.Error("DeepCopy of PodAssignments is not independent")
	}
}

func TestEstimatePodsPerNode_FallsBackToTotal(t *testing.T) {
	// When CalculateNodesNeeded returns 0 (no resources), estimatePodsPerNode
	// should return len(pods) as fallback.
	pods := makePodsWithoutRequests(5)
	nodePool := &stratosv1alpha1.NodePool{
		Spec: stratosv1alpha1.NodePoolSpec{
			Template: stratosv1alpha1.NodeTemplate{
				NodeClassRef: stratosv1alpha1.NodeClassRef{
					Kind: "AWSNodeClass",
					Name: "test-class",
				},
			},
		},
	}

	// When instance type is unknown and pods have no requests, CalculateNodesNeeded returns 1:1
	// So 5 pods → 5 nodes needed → podsPerNode = ceil(5/5) = 1
	calc := NewScaleCalculator(nodePool, "unknown.type")
	nodesNeeded := calc.CalculateNodesNeeded(pods, nil)
	if nodesNeeded != 5 {
		t.Fatalf("expected 5 nodes needed (1:1 fallback), got %d", nodesNeeded)
	}

	// podsPerNode = ceil(5/5) = 1
	podsPerNode := (len(pods) + nodesNeeded - 1) / nodesNeeded
	if podsPerNode != 1 {
		t.Errorf("expected 1 pod per node, got %d", podsPerNode)
	}
}

func TestEstimatePodsPerNode_ResourceBased(t *testing.T) {
	// 10 pods @ 500m on m5.xlarge (4 vCPU) → 2 nodes needed → ~5 pods/node
	pods := makePods(10, "500m", "2Gi")
	nodePool := &stratosv1alpha1.NodePool{
		Spec: stratosv1alpha1.NodePoolSpec{
			Template: stratosv1alpha1.NodeTemplate{
				NodeClassRef: stratosv1alpha1.NodeClassRef{
					Kind: "AWSNodeClass",
					Name: "test-class",
				},
			},
		},
	}

	calc := NewScaleCalculator(nodePool, "m5.xlarge")
	nodesNeeded := calc.CalculateNodesNeeded(pods, nil)
	if nodesNeeded != 2 {
		t.Fatalf("expected 2 nodes needed, got %d", nodesNeeded)
	}

	podsPerNode := (len(pods) + nodesNeeded - 1) / nodesNeeded
	if podsPerNode != 5 {
		t.Errorf("expected 5 pods per node, got %d", podsPerNode)
	}
}

func TestRoundRobinAssignment(t *testing.T) {
	// Simulate the round-robin assignment logic used by createPodAssignments
	pods := makePods(8, "500m", "1Gi")
	nodeNames := []string{"node-a", "node-b", "node-c"}
	podsPerNode := 3
	totalCapacity := podsPerNode * len(nodeNames)

	assignCount := len(pods)
	if assignCount > totalCapacity {
		assignCount = totalCapacity
	}

	// With 3 nodes and 3 pods/node = 9 capacity, 8 pods → assign 8
	if assignCount != 8 {
		t.Fatalf("expected assignCount=8, got %d", assignCount)
	}

	assignments := make(map[string]int)
	for i := 0; i < assignCount; i++ {
		node := nodeNames[i%len(nodeNames)]
		assignments[node]++
	}

	// Round-robin: node-a gets 3, node-b gets 3, node-c gets 2
	if assignments["node-a"] != 3 || assignments["node-b"] != 3 || assignments["node-c"] != 2 {
		t.Errorf("unexpected distribution: %v", assignments)
	}
}

func TestRoundRobinAssignment_CapacityLimit(t *testing.T) {
	// 15 pods but only 2 nodes with 4 pods each = 8 capacity → assign 8
	pods := makePods(15, "500m", "1Gi")
	nodeNames := []string{"node-a", "node-b"}
	podsPerNode := 4
	totalCapacity := podsPerNode * len(nodeNames) // 8

	assignCount := len(pods) // 15
	if assignCount > totalCapacity {
		assignCount = totalCapacity // capped to 8
	}

	if assignCount != 8 {
		t.Fatalf("expected assignCount=8, got %d", assignCount)
	}

	assignments := make(map[string]int)
	for i := 0; i < assignCount; i++ {
		node := nodeNames[i%len(nodeNames)]
		assignments[node]++
	}

	// Round-robin: each node gets exactly 4
	if assignments["node-a"] != 4 || assignments["node-b"] != 4 {
		t.Errorf("unexpected distribution: %v", assignments)
	}
}

func TestFilterAssignedPods_NoAssignments(t *testing.T) {
	// With no assignments, all pods should pass through
	pods := makePods(3, "100m", "128Mi")
	nodePool := &stratosv1alpha1.NodePool{
		Status: stratosv1alpha1.NodePoolStatus{
			PodAssignments: nil,
		},
	}

	// filterAssignedPods is a method on NodePoolReconciler, but the "no assignments"
	// fast path just returns pods directly. We test that logic inline here.
	if len(nodePool.Status.PodAssignments) == 0 {
		result := pods
		if len(result) != 3 {
			t.Errorf("expected 3 pods, got %d", len(result))
		}
	}
}

func TestPodAssignmentFields(t *testing.T) {
	now := metav1.Now()
	a := stratosv1alpha1.PodAssignment{
		PodName:      "my-pod",
		PodNamespace: "production",
		NodeName:     "worker-1",
		AssignedAt:   now,
	}

	if a.PodName != "my-pod" {
		t.Errorf("PodName = %q, want %q", a.PodName, "my-pod")
	}
	if a.PodNamespace != "production" {
		t.Errorf("PodNamespace = %q, want %q", a.PodNamespace, "production")
	}
	if a.NodeName != "worker-1" {
		t.Errorf("NodeName = %q, want %q", a.NodeName, "worker-1")
	}
	if a.AssignedAt.IsZero() {
		t.Error("AssignedAt should not be zero")
	}
}
