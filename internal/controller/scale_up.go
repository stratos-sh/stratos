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
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	stratosv1alpha1 "github.com/stratos-sh/stratos/api/v1alpha1"
	"github.com/stratos-sh/stratos/internal/metrics"
)

// getUnschedulablePods returns pods that are unschedulable and could use nodes from this pool.
func (r *NodePoolReconciler) getUnschedulablePods(ctx context.Context, nodePool *stratosv1alpha1.NodePool) ([]corev1.Pod, error) {
	podList := &corev1.PodList{}
	if err := r.List(ctx, podList); err != nil {
		return nil, err
	}

	var unschedulable []corev1.Pod
	for _, pod := range podList.Items {
		if isPodUnschedulable(&pod) && couldSatisfyPod(nodePool, &pod) {
			unschedulable = append(unschedulable, pod)
		}
	}
	return unschedulable, nil
}

// countStartingNodes counts nodes that are in the process of starting (in-flight scale-up).
// A node is considered "starting" if it has the scale-up-started annotation within the TTL
// and is not yet Ready.
func (r *NodePoolReconciler) countStartingNodes(ctx context.Context, poolName string) (int, error) {
	nodes, err := r.getNodesForPool(ctx, poolName)
	if err != nil {
		return 0, err
	}

	count := 0
	now := time.Now()

	for i := range nodes {
		node := &nodes[i]
		ts, ok := node.Annotations[AnnotationScaleUpStarted]
		if !ok {
			continue
		}

		startedAt, err := time.Parse(time.RFC3339, ts)
		if err != nil {
			continue
		}

		// Check if within TTL and node is not yet Ready
		if now.Sub(startedAt) < ScaleUpStartedTTL && !IsNodeReady(node) {
			count++
		}
	}

	return count, nil
}

// clearStaleScaleUpAnnotations removes scale-up-started annotations from nodes that are:
// 1. Ready (scale-up completed successfully)
// 2. Past the TTL (stale annotation)
func (r *NodePoolReconciler) clearStaleScaleUpAnnotations(ctx context.Context, poolName string) error {
	logger := log.FromContext(ctx)

	nodes, err := r.getNodesForPool(ctx, poolName)
	if err != nil {
		return err
	}

	now := time.Now()
	cleared := 0

	for i := range nodes {
		node := &nodes[i]
		ts, ok := node.Annotations[AnnotationScaleUpStarted]
		if !ok {
			continue
		}

		startedAt, err := time.Parse(time.RFC3339, ts)
		if err != nil {
			// Invalid timestamp, remove it
			if err := r.removeScaleUpAnnotation(ctx, node); err != nil {
				logger.Error(err, "Failed to remove invalid scale-up annotation", "node", node.Name)
			}
			cleared++
			continue
		}

		// Remove annotation if: node is Ready OR past TTL
		shouldClear := IsNodeReady(node) || now.Sub(startedAt) >= ScaleUpStartedTTL
		if shouldClear {
			if err := r.removeScaleUpAnnotation(ctx, node); err != nil {
				logger.Error(err, "Failed to remove scale-up annotation", "node", node.Name)
				continue
			}
			cleared++
		}
	}

	if cleared > 0 {
		logger.V(1).Info("Cleared stale scale-up annotations", "pool", poolName, "cleared", cleared)
	}

	return nil
}

// removeScaleUpAnnotation removes the scale-up-started annotation from a node.
func (r *NodePoolReconciler) removeScaleUpAnnotation(ctx context.Context, node *corev1.Node) error {
	if node.Annotations == nil {
		return nil
	}
	if _, ok := node.Annotations[AnnotationScaleUpStarted]; !ok {
		return nil
	}

	patch := client.MergeFrom(node.DeepCopy())
	delete(node.Annotations, AnnotationScaleUpStarted)
	return r.Patch(ctx, node, patch)
}

// filterAssignedPods returns only pods that do NOT have a valid assignment in the NodePool status.
// An assignment is valid if it is within the TTL and the assigned node still exists and is not terminating.
func (r *NodePoolReconciler) filterAssignedPods(ctx context.Context, pods []corev1.Pod, nodePool *stratosv1alpha1.NodePool) []corev1.Pod {
	if len(nodePool.Status.PodAssignments) == 0 {
		return pods
	}

	now := time.Now()

	// Build a set of valid assigned pod keys (namespace/name)
	assignedPods := make(map[string]bool)
	for _, a := range nodePool.Status.PodAssignments {
		// Skip expired assignments
		if now.Sub(a.AssignedAt.Time) >= PodAssignmentTTL {
			continue
		}
		// Check that assigned node still exists and isn't terminating
		node := &corev1.Node{}
		if err := r.Get(ctx, types.NamespacedName{Name: a.NodeName}, node); err != nil {
			continue // Node gone — assignment invalid
		}
		state := ParseNodeState(node.Labels[LabelState])
		if state == NodeStateTerminating {
			continue // Node terminating — assignment invalid
		}
		assignedPods[a.PodNamespace+"/"+a.PodName] = true
	}

	var unassigned []corev1.Pod
	for _, pod := range pods {
		key := pod.Namespace + "/" + pod.Name
		if !assignedPods[key] {
			unassigned = append(unassigned, pod)
		}
	}
	return unassigned
}

// calculateScaleUpNeeded determines how many nodes to start for scale-up.
// Returns the number of nodes needed and the unassigned pods that triggered the need.
// Pods with valid assignments are filtered out to prevent duplicate scale-ups.
func (r *NodePoolReconciler) calculateScaleUpNeeded(ctx context.Context, nodePool *stratosv1alpha1.NodePool) (int, []corev1.Pod, error) {
	logger := log.FromContext(ctx)

	// Get unschedulable pods that could use this pool
	pods, err := r.getUnschedulablePods(ctx, nodePool)
	if err != nil {
		return 0, nil, fmt.Errorf("failed to get unschedulable pods: %w", err)
	}

	if len(pods) == 0 {
		return 0, nil, nil
	}

	logger.Info("Found unschedulable pods", "count", len(pods))

	// Filter out pods that already have a valid assignment
	unassignedPods := r.filterAssignedPods(ctx, pods, nodePool)
	if len(unassignedPods) == 0 {
		logger.Info("All unschedulable pods already have valid assignments",
			"totalPending", len(pods),
			"assigned", len(pods)-len(unassignedPods))
		return 0, nil, nil
	}

	logger.Info("Unassigned pods after filtering",
		"totalPending", len(pods),
		"unassigned", len(unassignedPods))

	// Get existing nodes for capacity lookup
	existingNodes, err := r.getNodesForPool(ctx, nodePool.Name)
	if err != nil {
		return 0, nil, fmt.Errorf("failed to get existing nodes: %w", err)
	}

	// Get instance type from NodeClass for capacity lookup
	instanceType := ""
	nodeClass, err := r.getNodeClass(ctx, nodePool.Spec.Template.NodeClassRef)
	if err == nil && nodeClass != nil {
		instanceType = nodeClass.Spec.InstanceType
	} else {
		logger.V(1).Info("Could not fetch NodeClass, will use existing node capacity", "error", err)
	}

	// Calculate nodes needed based on resource requests of unassigned pods only
	calculator := NewScaleCalculator(nodePool, instanceType)
	nodesNeeded := calculator.CalculateNodesNeeded(unassignedPods, existingNodes)

	logger.Info("Calculated nodes needed from resources",
		"unassignedPods", len(unassignedPods),
		"nodesNeeded", nodesNeeded)

	// Get current node counts
	_, standby, running, _, err := r.countNodesByState(ctx, nodePool.Name)
	if err != nil {
		return 0, nil, err
	}

	// Check pool capacity
	maxRunning := int(nodePool.Spec.PoolSize)
	canStart := maxRunning - running
	if canStart <= 0 {
		logger.Info("Pool at capacity, cannot scale up",
			"poolSize", nodePool.Spec.PoolSize,
			"running", running)
		return 0, nil, nil
	}

	// Cap at available standby and capacity
	if nodesNeeded > standby {
		nodesNeeded = standby
	}
	if nodesNeeded > canStart {
		nodesNeeded = canStart
	}

	logger.Info("Final scale-up decision",
		"unassignedPods", len(unassignedPods),
		"nodesNeeded", nodesNeeded,
		"standbyAvailable", standby)

	return nodesNeeded, unassignedPods, nil
}

// scaleUp starts standby nodes to handle unschedulable pods and records pod assignments.
func (r *NodePoolReconciler) scaleUp(ctx context.Context, nodePool *stratosv1alpha1.NodePool, count int, pods []corev1.Pod) error {
	logger := log.FromContext(ctx)

	if count <= 0 {
		return nil
	}

	logger.Info("Scaling up", "pool", nodePool.Name, "count", count)

	// Get standby nodes
	nodes, err := r.getStandbyNodes(ctx, nodePool.Name)
	if err != nil {
		return fmt.Errorf("failed to get standby nodes: %w", err)
	}

	if len(nodes) < count {
		count = len(nodes)
	}

	// Get cloud provider
	provider := r.getCloudProvider(nodePool.Name)
	if provider == nil {
		return fmt.Errorf("no cloud provider for pool %s", nodePool.Name)
	}

	// Create node manager
	nodeMgr := NewNodeManager(r.Client, r.Recorder, provider, r.ClusterName)

	// Start nodes and track which ones succeeded
	startTime := time.Now()
	var startedNodes []corev1.Node
	for i := 0; i < count && i < len(nodes); i++ {
		node := &nodes[i]
		if err := nodeMgr.StartNode(ctx, nodePool, node); err != nil {
			logger.Error(err, "Failed to start node", "node", node.Name)
			continue
		}
		startedNodes = append(startedNodes, *node)
	}

	// Record metrics
	if len(startedNodes) > 0 {
		duration := time.Since(startTime).Seconds()
		metrics.RecordScaleUpDuration(nodePool.Name, duration/float64(len(startedNodes)))
		r.recordEvent(nodePool, corev1.EventTypeNormal, "ScaleUp",
			fmt.Sprintf("Started %d nodes for scale-up", len(startedNodes)))
	}

	// Create pod assignments: assign only as many pods as the started nodes can handle
	if len(startedNodes) > 0 && len(pods) > 0 {
		r.createPodAssignments(ctx, nodePool, startedNodes, pods)
	}

	return nil
}

// createPodAssignments assigns pending pods to started nodes round-robin,
// limited by per-node capacity from the ScaleCalculator.
func (r *NodePoolReconciler) createPodAssignments(ctx context.Context, nodePool *stratosv1alpha1.NodePool, startedNodes []corev1.Node, pods []corev1.Pod) {
	logger := log.FromContext(ctx)
	now := metav1.Now()

	// Estimate pods-per-node capacity
	podsPerNode := r.estimatePodsPerNode(ctx, nodePool, pods)
	totalCapacity := podsPerNode * len(startedNodes)
	assignCount := len(pods)
	if assignCount > totalCapacity {
		assignCount = totalCapacity
	}

	// Build new assignments round-robin across started nodes
	var newAssignments []stratosv1alpha1.PodAssignment
	for i := 0; i < assignCount; i++ {
		pod := pods[i]
		node := startedNodes[i%len(startedNodes)]
		newAssignments = append(newAssignments, stratosv1alpha1.PodAssignment{
			PodName:      pod.Name,
			PodNamespace: pod.Namespace,
			NodeName:     node.Name,
			AssignedAt:   now,
		})
	}

	if len(newAssignments) == 0 {
		return
	}

	// Update NodePool status with assignments
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		latest := &stratosv1alpha1.NodePool{}
		if err := r.Get(ctx, types.NamespacedName{Name: nodePool.Name}, latest); err != nil {
			return err
		}
		latest.Status.PodAssignments = append(latest.Status.PodAssignments, newAssignments...)
		return r.Status().Update(ctx, latest)
	})
	if err != nil {
		logger.Error(err, "Failed to write pod assignments to status")
		return
	}

	logger.Info("Created pod assignments",
		"count", len(newAssignments),
		"startedNodes", len(startedNodes),
		"podsPerNode", podsPerNode)
}

// estimatePodsPerNode returns how many of the given pods fit on one node.
// Falls back to len(pods) (i.e., all pods fit) if capacity is unknown.
func (r *NodePoolReconciler) estimatePodsPerNode(ctx context.Context, nodePool *stratosv1alpha1.NodePool, pods []corev1.Pod) int {
	instanceType := ""
	nodeClass, err := r.getNodeClass(ctx, nodePool.Spec.Template.NodeClassRef)
	if err == nil && nodeClass != nil {
		instanceType = nodeClass.Spec.InstanceType
	}

	existingNodes, _ := r.getNodesForPool(ctx, nodePool.Name)
	calculator := NewScaleCalculator(nodePool, instanceType)
	nodesNeeded := calculator.CalculateNodesNeeded(pods, existingNodes)
	if nodesNeeded <= 0 {
		return len(pods)
	}
	// pods-per-node = ceil(len(pods) / nodesNeeded)
	podsPerNode := (len(pods) + nodesNeeded - 1) / nodesNeeded
	if podsPerNode < 1 {
		podsPerNode = 1
	}
	return podsPerNode
}

// cleanupPodAssignments removes stale or resolved pod assignments from NodePool status.
// An assignment is removed if:
// 1. The pod is no longer unschedulable (scheduled or deleted)
// 2. The assigned node doesn't exist or is in terminating state
// 3. The assignment is older than PodAssignmentTTL
func (r *NodePoolReconciler) cleanupPodAssignments(ctx context.Context, nodePool *stratosv1alpha1.NodePool) error {
	logger := log.FromContext(ctx)

	// Re-fetch latest to avoid stale data
	latest := &stratosv1alpha1.NodePool{}
	if err := r.Get(ctx, types.NamespacedName{Name: nodePool.Name}, latest); err != nil {
		return err
	}

	if len(latest.Status.PodAssignments) == 0 {
		return nil
	}

	now := time.Now()
	var kept []stratosv1alpha1.PodAssignment
	removed := 0

	for _, a := range latest.Status.PodAssignments {
		// Check TTL
		if now.Sub(a.AssignedAt.Time) >= PodAssignmentTTL {
			removed++
			continue
		}

		// Check if pod is still unschedulable
		pod := &corev1.Pod{}
		podKey := types.NamespacedName{Name: a.PodName, Namespace: a.PodNamespace}
		if err := r.Get(ctx, podKey, pod); err != nil {
			if apierrors.IsNotFound(err) {
				removed++ // Pod deleted
				continue
			}
			// Transient error — keep the assignment
			kept = append(kept, a)
			continue
		}
		if !isPodUnschedulable(pod) {
			removed++ // Pod scheduled or no longer pending
			continue
		}

		// Check if assigned node still exists and isn't terminating
		node := &corev1.Node{}
		if err := r.Get(ctx, types.NamespacedName{Name: a.NodeName}, node); err != nil {
			if apierrors.IsNotFound(err) {
				removed++ // Node gone
				continue
			}
			kept = append(kept, a)
			continue
		}
		state := ParseNodeState(node.Labels[LabelState])
		if state == NodeStateTerminating {
			removed++ // Node terminating
			continue
		}

		kept = append(kept, a)
	}

	if removed == 0 {
		return nil
	}

	logger.V(1).Info("Cleaning up pod assignments",
		"pool", nodePool.Name,
		"removed", removed,
		"remaining", len(kept))

	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		fresh := &stratosv1alpha1.NodePool{}
		if err := r.Get(ctx, types.NamespacedName{Name: nodePool.Name}, fresh); err != nil {
			return err
		}
		fresh.Status.PodAssignments = kept
		return r.Status().Update(ctx, fresh)
	})
}
