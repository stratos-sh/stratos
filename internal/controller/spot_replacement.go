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

// SpotLauncher defines the interface for launching Spot instances with cloud-specific NodeClass.
type SpotLauncher interface {
	LaunchSpotInstance(ctx context.Context, nodeClass *stratosv1alpha1.AWSNodeClass, poolName, clusterName string, templateConfig *cloudprovider.TemplateConfig) (*cloudprovider.Instance, error)
}

// processSpotReplacements finds running On-Demand nodes eligible for Spot replacement
// and launches Spot instances to replace them.
func (r *NodePoolReconciler) processSpotReplacements(ctx context.Context, nodePool *stratosv1alpha1.NodePool) error {
	logger := log.FromContext(ctx)

	// Check if spot replacement is enabled
	if nodePool.Spec.SpotReplacement == nil || !nodePool.Spec.SpotReplacement.Enabled {
		return nil
	}

	// Get cloud provider and check it supports spot launches
	provider := r.getCloudProvider(nodePool.Name)
	if provider == nil {
		return nil
	}

	spotLauncher, ok := provider.(SpotLauncher)
	if !ok {
		logger.V(1).Info("Cloud provider does not support Spot launches")
		return nil
	}

	// Fetch the NodeClass to check for spotConfig
	nodeClass, err := r.getNodeClass(ctx, nodePool.Spec.Template.NodeClassRef)
	if err != nil {
		return fmt.Errorf("failed to fetch nodeClass for spot replacement: %w", err)
	}
	if nodeClass.Spec.SpotConfig == nil {
		logger.V(1).Info("AWSNodeClass has no spotConfig, skipping spot replacement")
		return nil
	}

	// Get running On-Demand nodes
	runningNodes, err := r.getRunningNodes(ctx, nodePool.Name)
	if err != nil {
		return fmt.Errorf("failed to get running nodes: %w", err)
	}

	replacementDelay := nodePool.Spec.SpotReplacement.GetReplacementDelay().Duration
	now := time.Now()

	logger.Info("Checking spot replacement eligibility",
		"pool", nodePool.Name,
		"runningNodes", len(runningNodes),
		"replacementDelay", replacementDelay)

	for i := range runningNodes {
		node := &runningNodes[i]

		// Skip Spot nodes — only replace On-Demand
		if node.Labels[LabelCapacityType] == cloudprovider.CapacityTypeSpot {
			logger.V(1).Info("Skipping spot node", "node", node.Name)
			continue
		}

		// Skip if already being replaced
		if _, ok := node.Annotations[AnnotationSpotReplacementStarted]; ok {
			logger.V(1).Info("Skipping node already being replaced", "node", node.Name)
			continue
		}

		// Check replacement delay — node must be running long enough
		startedAtStr := node.Annotations[AnnotationLastStarted]
		if startedAtStr == "" {
			logger.Info("Skipping node without last-started annotation", "node", node.Name)
			continue
		}
		startedAt, err := time.Parse(time.RFC3339, startedAtStr)
		if err != nil {
			logger.Info("Skipping node with invalid last-started annotation", "node", node.Name, "value", startedAtStr)
			continue
		}
		if now.Sub(startedAt) < replacementDelay {
			logger.V(1).Info("Skipping node not running long enough",
				"node", node.Name,
				"runningSince", startedAt,
				"remaining", replacementDelay-now.Sub(startedAt))
			continue
		}

		// Check if a Spot replacement is already warming up for this node
		if r.hasSpotReplacementInFlight(ctx, nodePool.Name, node.Name) {
			continue
		}

		// Launch Spot replacement
		logger.Info("Launching spot replacement for On-Demand node",
			"node", node.Name,
			"runningSince", startedAt)

		if err := r.launchSpotReplacement(ctx, nodePool, nodeClass, node, spotLauncher); err != nil {
			logger.Error(err, "Failed to launch spot replacement", "node", node.Name)
			// Continue with other nodes
			continue
		}
	}

	return nil
}

// launchSpotReplacement launches a Spot instance to replace an On-Demand node.
func (r *NodePoolReconciler) launchSpotReplacement(ctx context.Context, pool *stratosv1alpha1.NodePool, nodeClass *stratosv1alpha1.AWSNodeClass, onDemandNode *corev1.Node, spotLauncher SpotLauncher) error {
	logger := log.FromContext(ctx)

	// Build template config
	templateConfig := &cloudprovider.TemplateConfig{
		Labels:                      pool.Spec.Template.Labels,
		Taints:                      pool.Spec.Template.Taints,
		EnableNetworkReadinessTaint: pool.Spec.Template.IsNetworkReadinessTaintEnabled(),
		CapacityType:                cloudprovider.CapacityTypeSpot,
		ReplacingNode:               onDemandNode.Name,
	}

	instance, err := spotLauncher.LaunchSpotInstance(ctx, nodeClass, pool.Name, r.ClusterName, templateConfig)
	if err != nil {
		return fmt.Errorf("failed to launch spot instance: %w", err)
	}

	logger.Info("Spot instance launched", "instanceID", instance.ID, "replacingNode", onDemandNode.Name)

	// Mark the On-Demand node as being replaced
	patch := client.MergeFrom(onDemandNode.DeepCopy())
	if onDemandNode.Annotations == nil {
		onDemandNode.Annotations = make(map[string]string)
	}
	onDemandNode.Annotations[AnnotationSpotReplacementStarted] = time.Now().Format(time.RFC3339)
	if err := r.Patch(ctx, onDemandNode, patch); err != nil {
		return fmt.Errorf("failed to annotate on-demand node: %w", err)
	}

	r.recordEvent(pool, corev1.EventTypeNormal, "SpotReplacementStarted",
		fmt.Sprintf("Launched spot instance %s to replace on-demand node %s", instance.ID, onDemandNode.Name))

	return nil
}

// hasSpotReplacementInFlight checks if a Spot node is already warming up for the given On-Demand node.
func (r *NodePoolReconciler) hasSpotReplacementInFlight(ctx context.Context, poolName, onDemandNodeName string) bool {
	nodeList := &corev1.NodeList{}
	if err := r.List(ctx, nodeList, client.MatchingLabels{
		LabelPool:         poolName,
		LabelCapacityType: cloudprovider.CapacityTypeSpot,
	}); err != nil {
		return false
	}

	for _, node := range nodeList.Items {
		state := ParseNodeState(node.Labels[LabelState])
		if state == NodeStateWarmup || state == NodeStateStandby {
			if node.Annotations[AnnotationSpotReplacingNode] == onDemandNodeName {
				return true
			}
		}
	}
	return false
}

// processSpotWarmupComplete handles Spot nodes that have completed warmup and are ready
// to take over from their On-Demand counterpart.
func (r *NodePoolReconciler) processSpotWarmupComplete(ctx context.Context, nodePool *stratosv1alpha1.NodePool) error {
	logger := log.FromContext(ctx)

	if nodePool.Spec.SpotReplacement == nil || !nodePool.Spec.SpotReplacement.Enabled {
		return nil
	}

	provider := r.getCloudProvider(nodePool.Name)
	if provider == nil {
		return nil
	}

	// Find Spot nodes in warmup state that have joined the cluster and are ready
	nodeList := &corev1.NodeList{}
	if err := r.List(ctx, nodeList, client.MatchingLabels{
		LabelPool:         nodePool.Name,
		LabelCapacityType: cloudprovider.CapacityTypeSpot,
		LabelState:        string(NodeStateWarmup),
	}); err != nil {
		return fmt.Errorf("failed to list spot warmup nodes: %w", err)
	}

	nodeMgr := NewNodeManager(r.Client, r.Recorder, provider, r.ClusterName)

	for i := range nodeList.Items {
		node := &nodeList.Items[i]

		// Check if node is Ready
		if !IsNodeReady(node) {
			continue
		}

		// Check network readiness if enabled
		if nodePool.Spec.Template.IsNetworkReadinessTaintEnabled() {
			checker := NewNetworkReadinessChecker(r.Client)
			if !checker.IsReady(node) {
				continue
			}
		}

		// Spot warmup complete — transition to running directly (no stop for Spot)
		logger.Info("Spot node warmup complete, transitioning to running", "node", node.Name)

		// Prepare for running (uncordon + remove standby taint)
		if err := nodeMgr.prepareNodeForRunning(ctx, nodePool, node); err != nil {
			logger.Error(err, "Failed to prepare spot node for running", "node", node.Name)
			continue
		}

		// Transition warmup → standby (required intermediate state)
		if err := nodeMgr.TransitionState(ctx, node, NodeStateStandby); err != nil {
			logger.Error(err, "Failed to transition spot node to standby", "node", node.Name)
			continue
		}

		// Then standby → running
		if err := nodeMgr.TransitionState(ctx, node, NodeStateRunning); err != nil {
			logger.Error(err, "Failed to transition spot node to running", "node", node.Name)
			continue
		}

		instanceID := node.Labels[LabelInstanceID]
		if err := provider.UpdateInstanceTags(ctx, instanceID, map[string]string{
			TagState: string(NodeStateRunning),
		}); err != nil {
			logger.Error(err, "Failed to update spot instance tags", "instanceID", instanceID)
		}

		// Find the On-Demand node this Spot node is replacing
		replacingNodeName := node.Annotations[AnnotationSpotReplacingNode]
		if replacingNodeName != "" {
			if err := r.migrateToSpot(ctx, nodePool, replacingNodeName, node, provider); err != nil {
				logger.Error(err, "Failed to migrate to spot", "onDemandNode", replacingNodeName, "spotNode", node.Name)
				continue
			}
		}
	}

	return nil
}

// migrateToSpot drains the On-Demand node and returns it to standby.
func (r *NodePoolReconciler) migrateToSpot(ctx context.Context, pool *stratosv1alpha1.NodePool, onDemandNodeName string, spotNode *corev1.Node, provider cloudprovider.CloudProvider) error {
	logger := log.FromContext(ctx)

	// Get the On-Demand node
	onDemandNode := &corev1.Node{}
	if err := r.Get(ctx, client.ObjectKey{Name: onDemandNodeName}, onDemandNode); err != nil {
		return fmt.Errorf("failed to get on-demand node %s: %w", onDemandNodeName, err)
	}

	nodeMgr := NewNodeManager(r.Client, r.Recorder, provider, r.ClusterName)

	// Transition On-Demand to terminating
	if err := nodeMgr.TransitionState(ctx, onDemandNode, NodeStateTerminating); err != nil {
		return fmt.Errorf("failed to transition on-demand node to terminating: %w", err)
	}

	// Drain the On-Demand node
	drainConfig := &DrainConfig{
		GracePeriodSeconds:  -1,
		IgnoreAllDaemonSets: true,
		DeleteEmptyDirData:  false,
		Force:               false,
		Timeout:             5 * time.Minute,
	}
	if pool.Spec.ScaleDown != nil {
		drainConfig.Timeout = pool.Spec.ScaleDown.GetDrainTimeout().Duration
	}
	drainHelper := NewDrainHelper(r.Client, r.Recorder, drainConfig)

	drainStart := time.Now()
	if err := drainHelper.DrainNode(ctx, onDemandNode); err != nil {
		logger.Error(err, "Failed to drain on-demand node during spot migration", "node", onDemandNodeName)
		// Continue anyway — best effort
	}
	metrics.RecordDrainDuration(pool.Name, time.Since(drainStart).Seconds())

	// Stop the On-Demand node → returns to standby
	if err := nodeMgr.StopNode(ctx, pool, onDemandNode); err != nil {
		return fmt.Errorf("failed to stop on-demand node: %w", err)
	}

	// Clean up spot replacement annotation on On-Demand node
	patch := client.MergeFrom(onDemandNode.DeepCopy())
	delete(onDemandNode.Annotations, AnnotationSpotReplacementStarted)
	if err := r.Patch(ctx, onDemandNode, patch); err != nil {
		logger.Error(err, "Failed to clean up spot replacement annotation")
	}

	// Record spot replacement duration
	spotReplacementStarted := spotNode.Annotations[AnnotationSpotReplacementStarted]
	if startedAt, err := time.Parse(time.RFC3339, spotReplacementStarted); err == nil {
		metrics.RecordSpotReplacementDuration(pool.Name, time.Since(startedAt).Seconds())
	}

	metrics.RecordSpotReplacement(pool.Name)

	logger.Info("Successfully migrated from On-Demand to Spot",
		"onDemandNode", onDemandNodeName, "spotNode", spotNode.Name)

	r.recordEvent(pool, corev1.EventTypeNormal, "SpotReplacementComplete",
		fmt.Sprintf("Migrated workload from %s (On-Demand) to %s (Spot)", onDemandNodeName, spotNode.Name))

	return nil
}

// handleSpotInterruption handles a terminated Spot instance by cleaning up the node
// and letting the existing scale-up path handle fallback to On-Demand.
func (r *NodePoolReconciler) handleSpotInterruption(ctx context.Context, pool *stratosv1alpha1.NodePool, node *corev1.Node) error {
	logger := log.FromContext(ctx)

	logger.Info("Handling spot interruption", "node", node.Name)

	metrics.RecordSpotInterruption(pool.Name)
	metrics.RecordSpotFallback(pool.Name)

	r.recordEvent(pool, corev1.EventTypeWarning, "SpotInterruption",
		fmt.Sprintf("Spot node %s was interrupted, falling back to On-Demand", node.Name))

	// Delete the K8s node — existing scale-up path detects unschedulable pods
	// and starts On-Demand standby nodes
	if err := r.Delete(ctx, node); err != nil {
		return fmt.Errorf("failed to delete interrupted spot node: %w", err)
	}

	return nil
}
