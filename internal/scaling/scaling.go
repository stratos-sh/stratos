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

package scaling

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	stratosv1alpha1 "github.com/stratos-sh/stratos/api/v1alpha1"
	"github.com/stratos-sh/stratos/internal/controller/nodepool/nodestate"
)

// CheckDemand evaluates unschedulable pods to determine how many standby nodes to start.
func (s *Scaler) CheckDemand(ctx context.Context, nodePool *stratosv1alpha1.NodePool,
	standbyCount, runningCount, poolSize int) (ScalingDemand, error) {
	logger := log.FromContext(ctx)

	// Get unschedulable pods that could use this pool
	pods, err := s.getUnschedulablePods(ctx, nodePool)
	if err != nil {
		return ScalingDemand{}, fmt.Errorf("failed to get unschedulable pods: %w", err)
	}

	if len(pods) == 0 {
		return ScalingDemand{}, nil
	}

	logger.Info("Found unschedulable pods", "count", len(pods))

	// Filter out pods that already have a valid assignment
	unassignedPods := s.filterAssignedPods(ctx, pods, nodePool)
	if len(unassignedPods) == 0 {
		logger.Info("All unschedulable pods already have valid assignments",
			"totalPending", len(pods),
			"assigned", len(pods)-len(unassignedPods))
		return ScalingDemand{}, nil
	}

	logger.Info("Unassigned pods after filtering",
		"totalPending", len(pods),
		"unassigned", len(unassignedPods))

	// Get existing nodes for capacity lookup
	existingNodes, err := s.getNodesForPool(ctx, nodePool.Name)
	if err != nil {
		return ScalingDemand{}, fmt.Errorf("failed to get existing nodes: %w", err)
	}

	// Get instance type from NodeClass for capacity lookup
	instanceType := ""
	nodeClass, err := s.getNodeClass(ctx, nodePool.Spec.Template.NodeClassRef)
	if err == nil && nodeClass != nil {
		instanceType = nodeClass.GetInstanceType()
	} else {
		logger.V(1).Info("Could not fetch NodeClass, will use existing node capacity", "error", err)
	}

	// Calculate nodes needed based on resource requests of unassigned pods only
	calculator := NewScaleCalculator(nodePool, instanceType, s.capacityProvider)
	nodesNeeded := calculator.CalculateNodesNeeded(unassignedPods, existingNodes)

	logger.Info("Calculated nodes needed from resources",
		"unassignedPods", len(unassignedPods),
		"nodesNeeded", nodesNeeded)

	// Adjust for pool capacity, in-flight starts, and standby availability
	nodesNeeded = s.adjustForCapacity(ctx, nodePool.Name, nodesNeeded, poolSize, runningCount, standbyCount)

	logger.Info("Final scale-up decision",
		"unassignedPods", len(unassignedPods),
		"nodesNeeded", nodesNeeded,
		"standbyAvailable", standbyCount)

	return ScalingDemand{
		NodesNeeded: nodesNeeded,
		Pods:        unassignedPods,
	}, nil
}

// adjustForCapacity caps nodesNeeded based on pool capacity, in-flight starts, and standby availability.
func (s *Scaler) adjustForCapacity(ctx context.Context, poolName string, nodesNeeded, poolSize, runningCount, standbyCount int) int {
	logger := log.FromContext(ctx)

	canStart := poolSize - runningCount
	if canStart <= 0 {
		logger.Info("Pool at capacity, cannot scale up",
			"poolSize", poolSize,
			"running", runningCount)
		return 0
	}

	startingCount, err := s.countStartingNodes(ctx, poolName)
	if err != nil {
		logger.Error(err, "Failed to count starting nodes")
	} else if startingCount > 0 {
		nodesNeeded -= startingCount
		if nodesNeeded < 0 {
			nodesNeeded = 0
		}
	}

	if nodesNeeded > standbyCount {
		nodesNeeded = standbyCount
	}
	if nodesNeeded > canStart {
		nodesNeeded = canStart
	}

	return nodesNeeded
}

// OnScaleUp creates pod assignments after nodes have been started.
func (s *Scaler) OnScaleUp(ctx context.Context, nodePool *stratosv1alpha1.NodePool,
	startedNodes []corev1.Node, demand ScalingDemand) error {
	if len(startedNodes) == 0 {
		return nil
	}

	if len(demand.Pods) == 0 {
		return nil
	}

	s.createPodAssignments(ctx, nodePool, startedNodes, demand.Pods)
	return nil
}

// FindScaleDownCandidates returns running nodes that are empty of workload pods
// for longer than the configured TTL.
func (s *Scaler) FindScaleDownCandidates(ctx context.Context,
	nodePool *stratosv1alpha1.NodePool) ([]ScaleDownCandidate, error) {
	logger := log.FromContext(ctx)

	if !nodePool.Spec.ScaleDown.GetEnabled() {
		return nil, nil
	}

	nodes, err := s.getRunningNodes(ctx, nodePool.Name)
	if err != nil {
		return nil, err
	}

	emptyNodeTTL := nodePool.Spec.ScaleDown.GetEmptyNodeTTL().Duration
	var candidates []ScaleDownCandidate

	for i := range nodes {
		candidate, evalErr := s.evaluateScaleDownNode(ctx, &nodes[i], emptyNodeTTL)
		if evalErr != nil {
			logger.Error(evalErr, "Failed to evaluate node for scale-down", "node", nodes[i].Name)
			continue
		}
		if candidate != nil {
			candidates = append(candidates, *candidate)
		}
	}

	return candidates, nil
}

// evaluateScaleDownNode checks whether a single node qualifies as a scale-down candidate.
// Returns nil if the node is not a candidate (busy, newly empty, or TTL not elapsed).
func (s *Scaler) evaluateScaleDownNode(ctx context.Context, node *corev1.Node, emptyNodeTTL time.Duration) (*ScaleDownCandidate, error) {
	logger := log.FromContext(ctx)

	empty, err := isNodeEmpty(ctx, s.client, node.Name)
	if err != nil {
		return nil, err
	}

	if !empty {
		s.clearScaleDownAnnotation(ctx, node)
		return nil, nil
	}

	// Node is empty - check or set candidate timestamp
	candidateSince := parseScaleDownTimestamp(node)

	if candidateSince.IsZero() {
		s.markScaleDownCandidate(ctx, node)
		return nil, nil
	}

	if time.Since(candidateSince) >= emptyNodeTTL {
		logger.Info("Node is a scale-down candidate",
			"node", node.Name,
			"emptySince", candidateSince,
			"ttl", emptyNodeTTL)
		return &ScaleDownCandidate{Node: *node}, nil
	}

	return nil, nil
}

// clearScaleDownAnnotation removes the scale-down candidate annotation if present.
func (s *Scaler) clearScaleDownAnnotation(ctx context.Context, node *corev1.Node) {
	if node.Annotations == nil {
		return
	}
	if _, ok := node.Annotations[nodestate.AnnotationScaleDownCandidateSince]; !ok {
		return
	}
	logger := log.FromContext(ctx)
	patch := client.MergeFrom(node.DeepCopy())
	delete(node.Annotations, nodestate.AnnotationScaleDownCandidateSince)
	if err := s.client.Patch(ctx, node, patch); err != nil {
		logger.Error(err, "Failed to remove scale-down annotation")
	}
}

// markScaleDownCandidate sets the scale-down candidate annotation with the current time.
func (s *Scaler) markScaleDownCandidate(ctx context.Context, node *corev1.Node) {
	logger := log.FromContext(ctx)
	patch := client.MergeFrom(node.DeepCopy())
	if node.Annotations == nil {
		node.Annotations = make(map[string]string)
	}
	node.Annotations[nodestate.AnnotationScaleDownCandidateSince] = time.Now().Format(time.RFC3339)
	if err := s.client.Patch(ctx, node, patch); err != nil {
		logger.Error(err, "Failed to set scale-down annotation")
	}
}

// parseScaleDownTimestamp extracts the scale-down candidate timestamp from node annotations.
func parseScaleDownTimestamp(node *corev1.Node) time.Time {
	if node.Annotations == nil {
		return time.Time{}
	}
	ts, ok := node.Annotations[nodestate.AnnotationScaleDownCandidateSince]
	if !ok {
		return time.Time{}
	}
	parsed, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return time.Time{}
	}
	return parsed
}

// countStartingNodes counts nodes that are in the process of starting (in-flight scale-up).
func (s *Scaler) countStartingNodes(ctx context.Context, poolName string) (int, error) {
	nodes, err := s.getNodesForPool(ctx, poolName)
	if err != nil {
		return 0, err
	}

	count := 0
	now := time.Now()

	for i := range nodes {
		node := &nodes[i]
		ts, ok := node.Annotations[nodestate.AnnotationScaleUpStarted]
		if !ok {
			continue
		}

		startedAt, err := time.Parse(time.RFC3339, ts)
		if err != nil {
			continue
		}

		if now.Sub(startedAt) < nodestate.ScaleUpStartedTTL && !nodestate.IsNodeReady(node) {
			count++
		}
	}

	return count, nil
}
