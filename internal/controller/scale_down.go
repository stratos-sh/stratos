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
	"github.com/stratos-sh/stratos/internal/cloudprovider"
	"github.com/stratos-sh/stratos/internal/metrics"
)

// findScaleDownCandidates finds nodes that are candidates for scale-down.
func (r *NodePoolReconciler) findScaleDownCandidates(ctx context.Context, nodePool *stratosv1alpha1.NodePool) ([]corev1.Node, error) {
	logger := log.FromContext(ctx)

	// Check if scale-down is enabled
	if !nodePool.Spec.ScaleDown.GetEnabled() {
		return nil, nil
	}

	// Get running nodes
	nodes, err := r.getRunningNodes(ctx, nodePool.Name)
	if err != nil {
		return nil, err
	}

	emptyNodeTTL := nodePool.Spec.ScaleDown.GetEmptyNodeTTL().Duration
	var candidates []corev1.Node

	for _, node := range nodes {
		// Check if node is empty
		empty, err := IsNodeEmpty(ctx, r.Client, node.Name)
		if err != nil {
			logger.Error(err, "Failed to check if node is empty", "node", node.Name)
			continue
		}

		if !empty {
			// Remove scale-down candidate annotation if present
			if node.Annotations != nil {
				if _, ok := node.Annotations[AnnotationScaleDownCandidateSince]; ok {
					patch := client.MergeFrom(node.DeepCopy())
					delete(node.Annotations, AnnotationScaleDownCandidateSince)
					if err := r.Patch(ctx, &node, patch); err != nil {
						logger.Error(err, "Failed to remove scale-down annotation")
					}
				}
			}
			continue
		}

		// Node is empty - check or set candidate timestamp
		var candidateSince time.Time
		if node.Annotations != nil {
			if ts, ok := node.Annotations[AnnotationScaleDownCandidateSince]; ok {
				candidateSince, _ = time.Parse(time.RFC3339, ts)
			}
		}

		if candidateSince.IsZero() {
			// Mark as scale-down candidate
			patch := client.MergeFrom(node.DeepCopy())
			if node.Annotations == nil {
				node.Annotations = make(map[string]string)
			}
			node.Annotations[AnnotationScaleDownCandidateSince] = time.Now().Format(time.RFC3339)
			if err := r.Patch(ctx, &node, patch); err != nil {
				logger.Error(err, "Failed to set scale-down annotation")
			}
			continue
		}

		// Check if TTL has elapsed
		if time.Since(candidateSince) >= emptyNodeTTL {
			logger.Info("Node is a scale-down candidate",
				"node", node.Name,
				"emptySince", candidateSince,
				"ttl", emptyNodeTTL)
			candidates = append(candidates, node)
		}
	}

	return candidates, nil
}

// scaleDown drains and stops nodes that have been empty for longer than the TTL.
func (r *NodePoolReconciler) scaleDown(ctx context.Context, nodePool *stratosv1alpha1.NodePool, candidates []corev1.Node) error {
	logger := log.FromContext(ctx)

	if len(candidates) == 0 {
		return nil
	}

	logger.Info("Scaling down", "pool", nodePool.Name, "count", len(candidates))

	// Get cloud provider
	provider := r.getCloudProvider(nodePool.Name)
	if provider == nil {
		return fmt.Errorf("no cloud provider for pool %s", nodePool.Name)
	}

	// Create drain helper
	drainConfig := &DrainConfig{
		GracePeriodSeconds:  -1,
		IgnoreAllDaemonSets: true,
		DeleteEmptyDirData:  false,
		Force:               false,
		Timeout:             nodePool.Spec.ScaleDown.GetDrainTimeout().Duration,
	}
	drainHelper := NewDrainHelper(r.Client, r.Recorder, drainConfig)

	// Create node manager
	nodeMgr := NewNodeManager(r.Client, r.Recorder, provider, r.ClusterName)

	stopped := 0
	terminated := 0
	for _, node := range candidates {
		nodeCopy := node.DeepCopy()
		isSpot := nodeCopy.Labels[LabelCapacityType] == cloudprovider.CapacityTypeSpot

		// Transition to terminating state
		if err := nodeMgr.TransitionState(ctx, nodeCopy, NodeStateTerminating); err != nil {
			logger.Error(err, "Failed to transition node to terminating", "node", node.Name)
			continue
		}

		// Drain the node
		startTime := time.Now()
		if err := drainHelper.DrainNode(ctx, nodeCopy); err != nil {
			logger.Error(err, "Failed to drain node", "node", node.Name)
			continue
		}
		drainDuration := time.Since(startTime).Seconds()
		metrics.RecordDrainDuration(nodePool.Name, drainDuration)

		if isSpot {
			// Spot nodes can't be stopped — terminate and delete K8s node
			instanceID := nodeCopy.Labels[LabelInstanceID]
			if instanceID != "" {
				if err := provider.TerminateInstance(ctx, instanceID); err != nil {
					logger.Error(err, "Failed to terminate spot instance", "node", node.Name, "instanceID", instanceID)
					continue
				}
			}
			if err := nodeMgr.deleteNode(ctx, nodeCopy); err != nil {
				logger.Error(err, "Failed to delete spot node", "node", node.Name)
				continue
			}
			terminated++
		} else {
			// On-Demand nodes are stopped and returned to standby
			if err := nodeMgr.StopNode(ctx, nodePool, nodeCopy); err != nil {
				logger.Error(err, "Failed to stop node", "node", node.Name)
				continue
			}
			stopped++
		}
	}

	if stopped > 0 {
		r.recordEvent(nodePool, corev1.EventTypeNormal, "ScaleDown",
			fmt.Sprintf("Stopped %d on-demand nodes for scale-down", stopped))
	}
	if terminated > 0 {
		r.recordEvent(nodePool, corev1.EventTypeNormal, "ScaleDown",
			fmt.Sprintf("Terminated %d spot nodes for scale-down", terminated))
	}

	return nil
}
