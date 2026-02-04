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
	"github.com/stratos-sh/stratos/internal/metrics"
)

// IsReady checks if a node is ready for standby (Node Ready + CNI ready).
func (s *Scaler) IsReady(ctx context.Context, nodePool *stratosv1alpha1.NodePool,
	node *corev1.Node) (bool, error) {
	// Check if node is Ready
	if !nodestate.IsNodeReady(node) {
		return false, nil
	}

	// Check network readiness if the network readiness taint is enabled
	if nodePool.Spec.Template.IsNetworkReadinessTaintEnabled() {
		if !s.networkChecker.IsReady(node) {
			return false, nil
		}
		// For EKS, also verify CNI pod is Ready
		if s.networkChecker.HasNetworkingReadyCondition(node) && !s.networkChecker.IsCNIPodReady(ctx, node.Name) {
			return false, nil
		}
	}

	return true, nil
}

// PrepareForRunning uncordons a node and removes the standby taint.
// Startup taints are preserved and removed later by RunMaintenance.
func (s *Scaler) PrepareForRunning(ctx context.Context, nodePool *stratosv1alpha1.NodePool,
	node *corev1.Node) error {
	logger := log.FromContext(ctx)

	// Check if already uncordoned and untainted
	if !node.Spec.Unschedulable && !nodestate.HasTaint(node.Spec.Taints, nodestate.TaintKeyStandby) {
		return nil
	}

	patch := client.MergeFrom(node.DeepCopy())

	// Uncordon the node
	node.Spec.Unschedulable = false

	// Remove standby taint if present
	node.Spec.Taints = nodestate.RemoveTaint(node.Spec.Taints, nodestate.TaintKeyStandby)

	if err := s.client.Patch(ctx, node, patch); err != nil {
		return fmt.Errorf("failed to uncordon and untaint node: %w", err)
	}

	logger.Info("Prepared node for running (uncordoned + standby taint removed)",
		"node", node.Name, "networkReadinessTaintEnabled", nodePool.Spec.Template.IsNetworkReadinessTaintEnabled())
	return nil
}

// PrepareForStandby cordons a node, adds the NoExecute standby taint,
// and re-adds any configured startup taints.
func (s *Scaler) PrepareForStandby(ctx context.Context, nodePool *stratosv1alpha1.NodePool,
	node *corev1.Node) error {
	logger := log.FromContext(ctx)

	patch := client.MergeFrom(node.DeepCopy())
	modified := false

	// Cordon the node
	if !node.Spec.Unschedulable {
		node.Spec.Unschedulable = true
		modified = true
	}

	// Add standby taint if not present
	if !nodestate.HasTaint(node.Spec.Taints, nodestate.TaintKeyStandby) {
		node.Spec.Taints = append(node.Spec.Taints, corev1.Taint{
			Key:    nodestate.TaintKeyStandby,
			Effect: corev1.TaintEffectNoExecute,
		})
		modified = true
	}

	// Re-add network readiness taint so it's present when node starts from standby.
	if nodePool.Spec.Template.IsNetworkReadinessTaintEnabled() {
		nrt := nodestate.NetworkReadinessTaint()
		if !nodestate.HasTaintWithKeyAndEffect(node.Spec.Taints, nrt.Key, nrt.Effect) {
			node.Spec.Taints = append(node.Spec.Taints, nrt)
			modified = true
		}
	}

	if !modified {
		return nil
	}

	if err := s.client.Patch(ctx, node, patch); err != nil {
		return fmt.Errorf("failed to prepare node for standby: %w", err)
	}

	logger.Info("Prepared node for standby (cordoned + tainted)", "node", node.Name)
	return nil
}

// processStartupTaints handles network readiness taint removal for a running node.
func (s *Scaler) processStartupTaints(ctx context.Context, pool *stratosv1alpha1.NodePool, node *corev1.Node) (bool, error) {
	if !pool.Spec.Template.IsNetworkReadinessTaintEnabled() {
		return false, nil
	}

	nrt := nodestate.NetworkReadinessTaint()
	if !nodestate.HasTaintWithKeyAndEffect(node.Spec.Taints, nrt.Key, nrt.Effect) {
		return false, nil
	}

	startedAt := getNodeStartTime(node)

	timedOut := !startedAt.IsZero() && time.Since(startedAt) > nodestate.StartupTaintRemovalTimeout
	if timedOut {
		return s.forceRemoveNetworkReadinessTaint(ctx, pool, node, startedAt)
	}

	return s.removeNetworkReadinessTaintWhenReady(ctx, pool, node, startedAt)
}

// getNodeStartTime returns the time the node was started (from annotation).
func getNodeStartTime(node *corev1.Node) time.Time {
	startedAtStr := node.Annotations[nodestate.AnnotationLastStarted]
	if startedAtStr == "" {
		return time.Time{}
	}
	startedAt, err := time.Parse(time.RFC3339, startedAtStr)
	if err != nil {
		return time.Time{}
	}
	return startedAt
}

// forceRemoveNetworkReadinessTaint forcibly removes the network readiness taint after timeout.
func (s *Scaler) forceRemoveNetworkReadinessTaint(ctx context.Context, pool *stratosv1alpha1.NodePool, node *corev1.Node, startedAt time.Time) (bool, error) {
	logger := log.FromContext(ctx)

	logger.Info("Startup taint timeout, removing taint despite CNI not ready",
		"node", node.Name, "timeout", nodestate.StartupTaintRemovalTimeout)

	if s.recorder != nil {
		s.recorder.Eventf(pool, nil, corev1.EventTypeWarning, "StartupTaintTimeout", "ForceRemoveNetworkReadinessTaint",
			"Node %s network readiness taint removed after timeout (CNI may not be ready)", node.Name)
	}

	nrt := nodestate.NetworkReadinessTaint()
	patch := client.MergeFrom(node.DeepCopy())
	node.Spec.Taints = nodestate.RemoveTaintByKeyAndEffect(node.Spec.Taints, nrt.Key, nrt.Effect)
	if err := s.client.Patch(ctx, node, patch); err != nil {
		metrics.RecordStartupTaintRemoval(pool.Name, "timeout", "error")
		return false, fmt.Errorf("failed to remove network readiness taint: %w", err)
	}

	if !startedAt.IsZero() {
		metrics.RecordStartupTaintDuration(pool.Name, time.Since(startedAt).Seconds())
	}
	metrics.RecordStartupTaintRemoval(pool.Name, "timeout", "success")
	return true, nil
}

// removeNetworkReadinessTaintWhenReady removes the network readiness taint when CNI is ready.
func (s *Scaler) removeNetworkReadinessTaintWhenReady(ctx context.Context, pool *stratosv1alpha1.NodePool, node *corev1.Node, startedAt time.Time) (bool, error) {
	logger := log.FromContext(ctx)

	if !s.networkChecker.IsReady(node) {
		logger.V(1).Info("Network not ready (node condition), waiting to remove network readiness taint", "node", node.Name)
		return false, nil
	}

	if s.networkChecker.HasNetworkingReadyCondition(node) && !s.networkChecker.IsCNIPodReady(ctx, node.Name) {
		logger.V(1).Info("Network not ready (CNI pod not ready), waiting to remove network readiness taint", "node", node.Name)
		return false, nil
	}

	reason := s.networkChecker.GetNetworkConditionReason(node)

	logger.Info("Network ready, removing network readiness taint", "node", node.Name, "reason", reason)

	nrt := nodestate.NetworkReadinessTaint()
	patch := client.MergeFrom(node.DeepCopy())
	node.Spec.Taints = nodestate.RemoveTaintByKeyAndEffect(node.Spec.Taints, nrt.Key, nrt.Effect)
	if err := s.client.Patch(ctx, node, patch); err != nil {
		metrics.RecordStartupTaintRemoval(pool.Name, "network_ready", "error")
		return false, fmt.Errorf("failed to remove network readiness taint: %w", err)
	}

	if !startedAt.IsZero() {
		metrics.RecordStartupTaintDuration(pool.Name, time.Since(startedAt).Seconds())
	}
	metrics.RecordStartupTaintRemoval(pool.Name, "network_ready", "success")

	if s.recorder != nil {
		s.recorder.Eventf(pool, nil, corev1.EventTypeNormal, "StartupTaintsRemoved", "RemoveNetworkReadinessTaintWhenReady",
			"Removed network readiness taint from node %s after network ready (%s)", node.Name, reason)
	}

	return true, nil
}
