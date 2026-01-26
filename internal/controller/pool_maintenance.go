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
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	stratosv1alpha1 "github.com/stratos-sh/stratos/api/v1alpha1"
	"github.com/stratos-sh/stratos/internal/cloudprovider"
	"github.com/stratos-sh/stratos/internal/metrics"
)

// getStandbyNodes returns standby nodes for a pool.
func (r *NodePoolReconciler) getStandbyNodes(ctx context.Context, poolName string) ([]corev1.Node, error) {
	nodeList := &corev1.NodeList{}
	if err := r.List(ctx, nodeList, client.MatchingLabels{
		LabelPool:  poolName,
		LabelState: string(NodeStateStandby),
	}); err != nil {
		return nil, err
	}
	return nodeList.Items, nil
}

// getRunningNodes returns running nodes for a pool.
func (r *NodePoolReconciler) getRunningNodes(ctx context.Context, poolName string) ([]corev1.Node, error) {
	nodeList := &corev1.NodeList{}
	if err := r.List(ctx, nodeList, client.MatchingLabels{
		LabelPool:  poolName,
		LabelState: string(NodeStateRunning),
	}); err != nil {
		return nil, err
	}
	return nodeList.Items, nil
}

// countCloudInstances counts cloud instances for a pool (including those not yet in K8s).
func (r *NodePoolReconciler) countCloudInstances(ctx context.Context, nodePool *stratosv1alpha1.NodePool, provider cloudprovider.CloudProvider) (int, error) {
	instances, err := provider.ListInstances(ctx, map[string]string{
		"stratos.sh/pool": nodePool.Name,
	})
	if err != nil {
		return 0, fmt.Errorf("failed to list cloud instances: %w", err)
	}

	// Count non-terminated instances
	count := 0
	for _, inst := range instances {
		if inst.State != cloudprovider.InstanceStateTerminated &&
			inst.State != cloudprovider.InstanceStateShuttingDown {
			count++
		}
	}

	return count, nil
}

// getWarmupNodes returns warmup nodes for a pool.
func (r *NodePoolReconciler) getWarmupNodes(ctx context.Context, poolName string) ([]corev1.Node, error) {
	nodeList := &corev1.NodeList{}
	if err := r.List(ctx, nodeList, client.MatchingLabels{
		LabelPool:  poolName,
		LabelState: string(NodeStateWarmup),
	}); err != nil {
		return nil, err
	}
	return nodeList.Items, nil
}

// syncNodesWithCloud syncs node states with cloud provider instance states.
// This detects externally terminated instances and cleans up stale nodes.
func (r *NodePoolReconciler) syncNodesWithCloud(ctx context.Context, nodePool *stratosv1alpha1.NodePool, provider cloudprovider.CloudProvider) error {
	logger := log.FromContext(ctx)

	nodes, err := r.getNodesForPool(ctx, nodePool.Name)
	if err != nil {
		return fmt.Errorf("failed to get nodes: %w", err)
	}

	nodeMgr := NewNodeManager(r.Client, r.Recorder, provider, r.ClusterName)

	for i := range nodes {
		node := &nodes[i]
		if err := nodeMgr.SyncNodeState(ctx, nodePool, node); err != nil {
			logger.Error(err, "Failed to sync node state", "node", node.Name)
			// Continue with other nodes
		}
	}

	return nil
}

// processRunningNodesStartupTaints processes startup taint removal for all running nodes in a pool.
func (r *NodePoolReconciler) processRunningNodesStartupTaints(ctx context.Context, nodePool *stratosv1alpha1.NodePool, provider cloudprovider.CloudProvider) error {
	logger := log.FromContext(ctx)

	// Skip if no startup taints configured
	if len(nodePool.Spec.Template.StartupTaints) == 0 {
		return nil
	}

	nodes, err := r.getRunningNodes(ctx, nodePool.Name)
	if err != nil {
		return fmt.Errorf("failed to get running nodes: %w", err)
	}

	if len(nodes) == 0 {
		return nil
	}

	nodeMgr := NewNodeManager(r.Client, r.Recorder, provider, r.ClusterName)

	for i := range nodes {
		node := &nodes[i]
		removed, err := nodeMgr.ProcessStartupTaints(ctx, nodePool, node)
		if err != nil {
			logger.Error(err, "Failed to process startup taints", "node", node.Name)
			// Continue with other nodes
			continue
		}
		if removed {
			logger.V(1).Info("Startup taints removed", "node", node.Name)
		}
	}

	return nil
}

// monitorWarmupNodes monitors nodes in warmup state and transitions them when ready.
func (r *NodePoolReconciler) monitorWarmupNodes(ctx context.Context, nodePool *stratosv1alpha1.NodePool, provider cloudprovider.CloudProvider) error {
	logger := log.FromContext(ctx)

	nodes, err := r.getWarmupNodes(ctx, nodePool.Name)
	if err != nil {
		return fmt.Errorf("failed to get warmup nodes: %w", err)
	}

	if len(nodes) == 0 {
		return nil
	}

	nodeMgr := NewNodeManager(r.Client, r.Recorder, provider, r.ClusterName)

	for i := range nodes {
		node := &nodes[i]
		if err := nodeMgr.MonitorWarmup(ctx, nodePool, node); err != nil {
			logger.Error(err, "Failed to monitor warmup node", "node", node.Name)
			// Continue with other nodes
		}
	}

	return nil
}

// monitorCloudWarmupInstances monitors cloud instances in warmup state directly from the cloud provider.
// This catches instances that self-stop before registering as K8s nodes - a critical gap in the
// existing monitorWarmupNodes() which only sees nodes that have already joined Kubernetes.
func (r *NodePoolReconciler) monitorCloudWarmupInstances(ctx context.Context, nodePool *stratosv1alpha1.NodePool, provider cloudprovider.CloudProvider) error {
	logger := log.FromContext(ctx)

	// List all instances for this pool that are tagged as warmup state
	instances, err := provider.ListInstances(ctx, map[string]string{
		TagPool:  nodePool.Name,
		TagState: string(NodeStateWarmup),
	})
	if err != nil {
		return fmt.Errorf("failed to list cloud warmup instances: %w", err)
	}

	if len(instances) == 0 {
		return nil
	}

	// logger.V(1).Info("Monitoring cloud warmup instances", "count", len(instances))

	nodeMgr := NewNodeManager(r.Client, r.Recorder, provider, r.ClusterName)

	for _, instance := range instances {
		// Skip terminated instances
		if instance.State == cloudprovider.InstanceStateTerminated ||
			instance.State == cloudprovider.InstanceStateShuttingDown {
			continue
		}

		if err := nodeMgr.MonitorCloudWarmup(ctx, nodePool, instance); err != nil {
			logger.Error(err, "Failed to monitor cloud warmup instance", "instanceID", instance.ID)
			// Continue with other instances
		}
	}

	return nil
}

// checkMaxNodeRuntime checks for nodes that have exceeded their maximum runtime.
func (r *NodePoolReconciler) checkMaxNodeRuntime(ctx context.Context, nodePool *stratosv1alpha1.NodePool) ([]corev1.Node, error) {
	logger := log.FromContext(ctx)

	// Check if maxNodeRuntime is configured
	if nodePool.Spec.MaxNodeRuntime == nil || nodePool.Spec.MaxNodeRuntime.Duration == 0 {
		return nil, nil
	}

	maxRuntime := nodePool.Spec.MaxNodeRuntime.Duration
	warningThreshold := maxRuntime - (5 * time.Minute) // Warn 5 minutes before

	nodes, err := r.getRunningNodes(ctx, nodePool.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to get running nodes: %w", err)
	}

	var exceededNodes []corev1.Node
	now := time.Now()

	for i := range nodes {
		node := &nodes[i]

		// Get last started time from annotation
		lastStartedStr := node.Annotations[AnnotationLastStarted]
		if lastStartedStr == "" {
			continue
		}

		lastStarted, err := time.Parse(time.RFC3339, lastStartedStr)
		if err != nil {
			logger.Error(err, "Failed to parse last-started annotation", "node", node.Name)
			continue
		}

		runtime := now.Sub(lastStarted)

		// Check if exceeded
		if runtime >= maxRuntime {
			logger.Info("Node exceeded max runtime",
				"node", node.Name,
				"runtime", runtime,
				"maxRuntime", maxRuntime)
			exceededNodes = append(exceededNodes, nodes[i])
			continue
		}

		// Check if approaching warning threshold
		if runtime >= warningThreshold {
			logger.Info("Node approaching max runtime",
				"node", node.Name,
				"runtime", runtime,
				"remaining", maxRuntime-runtime)
			r.recordEvent(nodePool, corev1.EventTypeWarning, "MaxRuntimeApproaching",
				fmt.Sprintf("Node %s will reach max runtime in %v", node.Name, maxRuntime-runtime))
		}
	}

	return exceededNodes, nil
}

// recycleNodesForMaxRuntime drains and stops nodes that have exceeded their maximum runtime.
func (r *NodePoolReconciler) recycleNodesForMaxRuntime(ctx context.Context, nodePool *stratosv1alpha1.NodePool, nodes []corev1.Node) error {
	logger := log.FromContext(ctx)

	if len(nodes) == 0 {
		return nil
	}

	logger.Info("Recycling nodes that exceeded max runtime",
		"pool", nodePool.Name,
		"count", len(nodes))

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
		Timeout:             5 * time.Minute,
	}
	if nodePool.Spec.ScaleDown != nil {
		drainConfig.Timeout = nodePool.Spec.ScaleDown.GetDrainTimeout().Duration
	}
	drainHelper := NewDrainHelper(r.Client, r.Recorder, drainConfig)

	// Create node manager
	nodeMgr := NewNodeManager(r.Client, r.Recorder, provider, r.ClusterName)

	recycled := 0
	for i := range nodes {
		node := nodes[i].DeepCopy()

		// Record event
		r.recordEvent(nodePool, corev1.EventTypeNormal, "MaxRuntimeExceeded",
			fmt.Sprintf("Recycling node %s due to max runtime exceeded", node.Name))

		// Transition to terminating state
		if err := nodeMgr.TransitionState(ctx, node, NodeStateTerminating); err != nil {
			logger.Error(err, "Failed to transition node to terminating", "node", node.Name)
			continue
		}

		// Drain the node
		if err := drainHelper.DrainNode(ctx, node); err != nil {
			logger.Error(err, "Failed to drain node", "node", node.Name)
			continue
		}

		// Stop the node
		if err := nodeMgr.StopNode(ctx, nodePool, node); err != nil {
			logger.Error(err, "Failed to stop node", "node", node.Name)
			continue
		}

		recycled++
		metrics.RecordScaleDown(nodePool.Name)
	}

	if recycled > 0 {
		logger.Info("Recycled nodes due to max runtime",
			"pool", nodePool.Name,
			"recycled", recycled)
	}

	return nil
}

// replenishStandby launches new nodes to maintain minStandby count.
func (r *NodePoolReconciler) replenishStandby(ctx context.Context, nodePool *stratosv1alpha1.NodePool, count int) error {
	logger := log.FromContext(ctx)

	if count <= 0 {
		return nil
	}

	provider := r.getCloudProvider(nodePool.Name)
	if provider == nil {
		return fmt.Errorf("no cloud provider for pool %s", nodePool.Name)
	}

	// Get the NodeLauncher interface from the provider
	launcher, ok := provider.(NodeLauncher)
	if !ok {
		return fmt.Errorf("cloud provider does not implement NodeLauncher interface")
	}

	// Fetch the NodeClass
	nodeClass, err := r.getNodeClass(ctx, nodePool.Spec.Template.NodeClassRef)
	if err != nil {
		if apierrors.IsNotFound(err) {
			r.setDegradedCondition(ctx, nodePool, stratosv1alpha1.ReasonNodeClassNotFound,
				fmt.Sprintf("NodeClass %s not found", nodePool.Spec.Template.NodeClassRef.Name))
			r.recordEvent(nodePool, corev1.EventTypeWarning, "NodeClassNotFound",
				fmt.Sprintf("AWSNodeClass %s not found", nodePool.Spec.Template.NodeClassRef.Name))
			return fmt.Errorf("nodeClass %s not found: %w", nodePool.Spec.Template.NodeClassRef.Name, err)
		}
		return fmt.Errorf("failed to fetch nodeClass: %w", err)
	}

	nodeMgr := NewNodeManager(r.Client, r.Recorder, provider, r.ClusterName)

	// Launch new nodes
	launched := 0
	for i := 0; i < count; i++ {
		_, err := nodeMgr.LaunchNode(ctx, nodePool, nodeClass, launcher)
		if err != nil {
			logger.Error(err, "Failed to launch node for replenishment")
			continue
		}
		launched++
	}

	if launched > 0 {
		logger.Info("Launched nodes for standby replenishment",
			"pool", nodePool.Name,
			"nodeClass", nodeClass.Name,
			"requested", count,
			"launched", launched)
		r.recordEvent(nodePool, corev1.EventTypeNormal, "Replenishing",
			fmt.Sprintf("Launched %d nodes to maintain minStandby", launched))
	}

	return nil
}
