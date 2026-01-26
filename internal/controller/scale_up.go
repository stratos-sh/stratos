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

// calculateScaleUpNeeded determines how many nodes to start for scale-up.
// Uses resource-based calculation to determine optimal number of nodes needed,
// and tracks in-flight scale-ups to prevent duplicate starts.
func (r *NodePoolReconciler) calculateScaleUpNeeded(ctx context.Context, nodePool *stratosv1alpha1.NodePool) (int, error) {
	logger := log.FromContext(ctx)

	// Get unschedulable pods that could use this pool
	pods, err := r.getUnschedulablePods(ctx, nodePool)
	if err != nil {
		return 0, fmt.Errorf("failed to get unschedulable pods: %w", err)
	}

	if len(pods) == 0 {
		return 0, nil
	}

	logger.Info("Found unschedulable pods", "count", len(pods))

	// Get existing nodes for capacity lookup
	existingNodes, err := r.getNodesForPool(ctx, nodePool.Name)
	if err != nil {
		return 0, fmt.Errorf("failed to get existing nodes: %w", err)
	}

	// Get instance type from NodeClass for capacity lookup
	instanceType := ""
	nodeClass, err := r.getNodeClass(ctx, nodePool.Spec.Template.NodeClassRef)
	if err == nil && nodeClass != nil {
		instanceType = nodeClass.Spec.InstanceType
	} else {
		logger.V(1).Info("Could not fetch NodeClass, will use existing node capacity", "error", err)
	}

	// Calculate nodes needed based on resource requests
	calculator := NewScaleCalculator(nodePool, instanceType)
	nodesNeeded := calculator.CalculateNodesNeeded(pods, existingNodes)

	logger.Info("Calculated nodes needed from resources",
		"pendingPods", len(pods),
		"nodesNeeded", nodesNeeded)

	// Get current node counts
	_, standby, running, _, err := r.countNodesByState(ctx, nodePool.Name)
	if err != nil {
		return 0, err
	}

	// Count nodes that are currently starting (in-flight scale-up)
	starting, err := r.countStartingNodes(ctx, nodePool.Name)
	if err != nil {
		logger.Error(err, "Failed to count starting nodes")
		starting = 0
	}

	// Record starting nodes metric
	metrics.RecordStartingNodes(nodePool.Name, starting)

	// Subtract starting nodes - they will satisfy some demand once ready
	calculatedNeed := nodesNeeded
	nodesNeeded = nodesNeeded - starting
	if nodesNeeded <= 0 {
		logger.Info("Scale-up already in progress",
			"calculatedNeed", calculatedNeed,
			"startingNodes", starting)
		return 0, nil
	}

	// Check pool capacity
	maxRunning := int(nodePool.Spec.PoolSize)
	canStart := maxRunning - running - starting
	if canStart <= 0 {
		logger.Info("Pool at capacity, cannot scale up",
			"poolSize", nodePool.Spec.PoolSize,
			"running", running,
			"starting", starting)
		return 0, nil
	}

	// Cap at available standby and capacity
	if nodesNeeded > standby {
		nodesNeeded = standby
	}
	if nodesNeeded > canStart {
		nodesNeeded = canStart
	}

	logger.Info("Final scale-up decision",
		"pendingPods", len(pods),
		"nodesNeeded", nodesNeeded,
		"startingNodes", starting,
		"standbyAvailable", standby)

	return nodesNeeded, nil
}

// scaleUp starts standby nodes to handle unschedulable pods.
func (r *NodePoolReconciler) scaleUp(ctx context.Context, nodePool *stratosv1alpha1.NodePool, count int) error {
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

	// Start nodes
	startTime := time.Now()
	started := 0
	for i := 0; i < count && i < len(nodes); i++ {
		node := &nodes[i]
		if err := nodeMgr.StartNode(ctx, nodePool, node); err != nil {
			logger.Error(err, "Failed to start node", "node", node.Name)
			continue
		}
		started++
	}

	// Record metrics
	if started > 0 {
		duration := time.Since(startTime).Seconds()
		metrics.RecordScaleUpDuration(nodePool.Name, duration/float64(started))
		r.recordEvent(nodePool, corev1.EventTypeNormal, "ScaleUp",
			fmt.Sprintf("Started %d nodes for scale-up", started))
	}

	return nil
}
