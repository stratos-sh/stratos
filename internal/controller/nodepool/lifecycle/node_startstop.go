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

package lifecycle

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

// TransitionState transitions a node to a new nodestate.
func (m *Manager) TransitionState(ctx context.Context, node *corev1.Node, newState nodestate.NodeState) error {
	logger := log.FromContext(ctx)

	currentState := nodestate.ParseNodeState(node.Labels[nodestate.LabelState])
	if !nodestate.IsValidTransition(currentState, newState) {
		return &nodestate.InvalidTransitionError{From: currentState, To: newState}
	}

	// Create a copy for patching
	patch := client.MergeFrom(node.DeepCopy())

	// Update state labels
	node.Labels[nodestate.LabelState] = string(newState)
	node.Labels[nodestate.LabelStateSince] = fmt.Sprintf("%d", time.Now().Unix())

	// Apply patch
	if err := m.client.Patch(ctx, node, patch); err != nil {
		return fmt.Errorf("failed to update node state: %w", err)
	}

	logger.Info("Transitioned node state", "node", node.Name, "from", currentState, "to", newState)
	return nil
}

// StartNode starts a standby node for scale-up.
func (m *Manager) StartNode(ctx context.Context, pool *stratosv1alpha1.NodePool, node *corev1.Node) error {
	logger := log.FromContext(ctx)

	instanceID := node.Labels[nodestate.LabelInstanceID]
	if instanceID == "" {
		return fmt.Errorf("node %s has no instance ID label", node.Name)
	}

	// Prepare node for running (uncordon + remove standby taint) before starting
	// Note: Startup taints are preserved and removed later after CNI is ready
	if m.hooks != nil {
		if err := m.hooks.PrepareForRunning(ctx, pool, node); err != nil {
			return err
		}
	}

	// Start the instance
	logger.Info("Starting instance", "node", node.Name, "instanceID", instanceID)
	if err := m.cloudProvider.StartInstance(ctx, instanceID); err != nil {
		return fmt.Errorf("failed to start instance: %w", err)
	}

	// Transition state
	if err := m.TransitionState(ctx, node, nodestate.NodeStateRunning); err != nil {
		return err
	}

	// Update cloud provider tags
	if err := m.cloudProvider.UpdateInstanceTags(ctx, instanceID, map[string]string{
		nodestate.TagState: string(nodestate.NodeStateRunning),
	}); err != nil {
		logger.Error(err, "Failed to update instance tags", "instanceID", instanceID)
	}

	// Set scale-up started annotation (for in-flight tracking) and last started annotation
	now := time.Now().Format(time.RFC3339)
	patch := client.MergeFrom(node.DeepCopy())
	if node.Annotations == nil {
		node.Annotations = make(map[string]string)
	}
	node.Annotations[nodestate.AnnotationScaleUpStarted] = now
	node.Annotations[nodestate.AnnotationLastStarted] = now
	if err := m.client.Patch(ctx, node, patch); err != nil {
		logger.Error(err, "Failed to set scale-up annotations")
	}

	// Record event
	if m.recorder != nil {
		m.recorder.Eventf(pool, nil, corev1.EventTypeNormal, "NodeStarted", "StartNode",
			"Started node %s for scale-up", node.Name)
	}

	metrics.RecordScaleUp(pool.Name)

	return nil
}

// StopNode stops a running node for scale-down (after draining).
func (m *Manager) StopNode(ctx context.Context, pool *stratosv1alpha1.NodePool, node *corev1.Node) error {
	logger := log.FromContext(ctx)

	instanceID := node.Labels[nodestate.LabelInstanceID]
	if instanceID == "" {
		return fmt.Errorf("node %s has no instance ID label", node.Name)
	}

	// Prepare node for standby (cordon + taint + re-add startup taints) before stopping
	if m.hooks != nil {
		if err := m.hooks.PrepareForStandby(ctx, pool, node); err != nil {
			return err
		}
	}

	// Stop the instance
	logger.Info("Stopping instance", "node", node.Name, "instanceID", instanceID)
	if err := m.cloudProvider.StopInstance(ctx, instanceID, false); err != nil {
		return fmt.Errorf("failed to stop instance: %w", err)
	}

	// Transition state
	if err := m.TransitionState(ctx, node, nodestate.NodeStateStandby); err != nil {
		return err
	}

	// Update cloud provider tags
	if err := m.cloudProvider.UpdateInstanceTags(ctx, instanceID, map[string]string{
		nodestate.TagState: string(nodestate.NodeStateStandby),
	}); err != nil {
		logger.Error(err, "Failed to update instance tags", "instanceID", instanceID)
	}

	// Remove scale-down candidate annotation
	patch := client.MergeFrom(node.DeepCopy())
	delete(node.Annotations, nodestate.AnnotationScaleDownCandidateSince)
	if err := m.client.Patch(ctx, node, patch); err != nil {
		logger.Error(err, "Failed to remove scale-down annotation")
	}

	// Record event
	if m.recorder != nil {
		m.recorder.Eventf(pool, nil, corev1.EventTypeNormal, "NodeStopped", "StopNode",
			"Stopped node %s after scale-down", node.Name)
	}

	metrics.RecordScaleDown(pool.Name)

	return nil
}
