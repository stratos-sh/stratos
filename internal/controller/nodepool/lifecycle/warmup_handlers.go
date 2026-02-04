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

// handleWarmupTimeout handles a warmup timeout according to the configured action.
func (m *Manager) handleWarmupTimeout(ctx context.Context, pool *stratosv1alpha1.NodePool, node *corev1.Node, instanceID string) error {
	logger := log.FromContext(ctx)

	action := pool.Spec.PreWarm.GetTimeoutAction()
	metrics.RecordWarmupFailure(pool.Name, "timeout")

	switch action {
	case stratosv1alpha1.TimeoutActionStop:
		logger.Info("Stopping instance due to warmup timeout", "node", node.Name)
		if err := m.cloudProvider.StopInstance(ctx, instanceID, true); err != nil {
			return fmt.Errorf("failed to stop instance: %w", err)
		}
		// Will transition to standby on next reconcile when stopped
		if m.recorder != nil {
			m.recorder.Eventf(pool, nil, corev1.EventTypeWarning, "WarmupTimeout", "HandleWarmupTimeout",
				"Node %s warmup timed out, forcing stop", node.Name)
		}

	case stratosv1alpha1.TimeoutActionTerminate:
		logger.Info("Terminating instance due to warmup timeout", "node", node.Name)
		if err := m.TransitionState(ctx, node, nodestate.NodeStateTerminating); err != nil {
			return err
		}
		if err := m.cloudProvider.TerminateInstance(ctx, instanceID); err != nil {
			return fmt.Errorf("failed to terminate instance: %w", err)
		}
		if err := m.deleteNode(ctx, node); err != nil {
			return err
		}
		if m.recorder != nil {
			m.recorder.Eventf(pool, nil, corev1.EventTypeWarning, "WarmupTimeout", "HandleWarmupTimeout",
				"Node %s warmup timed out, terminated", node.Name)
		}
	}

	return nil
}

// handleControllerStopWarmup handles warmup completion.
// It checks if the node is ready and stops it when conditions are met.
func (m *Manager) handleControllerStopWarmup(ctx context.Context, pool *stratosv1alpha1.NodePool, node *corev1.Node, instanceID string, startTime time.Time) error {
	logger := log.FromContext(ctx)

	// Use scaler-provided readiness checker
	if m.hooks != nil {
		ready, err := m.hooks.IsReady(ctx, pool, node)
		if err != nil {
			return err
		}
		if !ready {
			logger.V(1).Info("Instance not ready (scaler check), waiting for warmup to complete", "node", node.Name)
			return nil
		}
	} else {
		// No hooks: fallback to basic K8s readiness check
		if !nodestate.IsNodeReady(node) {
			logger.V(1).Info("Node not ready, waiting for warmup to complete", "node", node.Name)
			return nil
		}
	}

	// Node is ready (and network ready if required) — stop and transition to standby
	logger.Info("Node ready, stopping instance", "node", node.Name, "instanceID", instanceID)

	// Stop the instance
	if err := m.cloudProvider.StopInstance(ctx, instanceID, false); err != nil {
		return fmt.Errorf("failed to stop instance in ControllerStop mode: %w", err)
	}

	// Record warmup duration
	metrics.RecordWarmupDuration(pool.Name, "controller_stop", time.Since(startTime).Seconds())

	// Prepare node for standby (cordon + taint + re-add startup taints)
	if m.hooks != nil {
		if err := m.hooks.PrepareForStandby(ctx, pool, node); err != nil {
			return err
		}
	}

	if err := m.TransitionState(ctx, node, nodestate.NodeStateStandby); err != nil {
		return err
	}

	// Update cloud provider tags
	if err := m.cloudProvider.UpdateInstanceTags(ctx, instanceID, map[string]string{
		nodestate.TagState: string(nodestate.NodeStateStandby),
	}); err != nil {
		logger.Error(err, "Failed to update instance tags", "instanceID", instanceID)
	}

	// Record event
	if m.recorder != nil {
		m.recorder.Eventf(pool, nil, corev1.EventTypeNormal, "WarmupCompleted", "HandleControllerStopWarmup",
			"Node %s completed warmup and is now standby", node.Name)
	}

	// Set warmup completed annotation
	patch := client.MergeFrom(node.DeepCopy())
	if node.Annotations == nil {
		node.Annotations = make(map[string]string)
	}
	node.Annotations[nodestate.AnnotationWarmupCompleted] = time.Now().Format(time.RFC3339)
	if err := m.client.Patch(ctx, node, patch); err != nil {
		logger.Error(err, "Failed to set warmup completed annotation")
	}

	return nil
}

// handleWarmupFailure handles the case where warmup failed because the node never registered.
// This terminates the orphaned cloud instance and records appropriate metrics.
func (m *Manager) handleWarmupFailure(ctx context.Context, pool *stratosv1alpha1.NodePool, instanceID string, reason string) error {
	logger := log.FromContext(ctx)

	logger.Info("Handling warmup failure", "instanceID", instanceID, "reason", reason)
	metrics.RecordWarmupFailure(pool.Name, reason)

	// Terminate the orphaned instance
	if err := m.cloudProvider.TerminateInstance(ctx, instanceID); err != nil {
		return fmt.Errorf("failed to terminate orphaned instance %s: %w", instanceID, err)
	}

	// Record event
	if m.recorder != nil {
		m.recorder.Eventf(pool, nil, corev1.EventTypeWarning, "WarmupFailed", "HandleWarmupFailure",
			"Instance %s warmup failed (%s), terminated", instanceID, reason)
	}

	logger.Info("Terminated orphaned warmup instance", "instanceID", instanceID)
	return nil
}

// handleCloudWarmupTimeout handles warmup timeout for instances that never registered as K8s nodes.
func (m *Manager) handleCloudWarmupTimeout(ctx context.Context, pool *stratosv1alpha1.NodePool, instanceID string) error {
	logger := log.FromContext(ctx)

	action := pool.Spec.PreWarm.GetTimeoutAction()
	metrics.RecordWarmupFailure(pool.Name, "timeout_no_node")

	switch action {
	case stratosv1alpha1.TimeoutActionStop:
		logger.Info("Force stopping instance due to warmup timeout (no K8s node)", "instanceID", instanceID)
		if err := m.cloudProvider.StopInstance(ctx, instanceID, true); err != nil {
			return fmt.Errorf("failed to stop instance: %w", err)
		}

		// Update instance tags to mark as failed warmup
		if err := m.cloudProvider.UpdateInstanceTags(ctx, instanceID, map[string]string{
			nodestate.TagState:           "warmup-failed",
			nodestate.TagState + "-note": "timeout_no_node",
		}); err != nil {
			logger.Error(err, "Failed to update instance tags", "instanceID", instanceID)
		}

		if m.recorder != nil {
			m.recorder.Eventf(pool, nil, corev1.EventTypeWarning, "WarmupTimeout", "HandleCloudWarmupTimeout",
				"Instance %s warmup timed out (no K8s node registration), stopped", instanceID)
		}

	case stratosv1alpha1.TimeoutActionTerminate:
		logger.Info("Terminating instance due to warmup timeout (no K8s node)", "instanceID", instanceID)
		if err := m.cloudProvider.TerminateInstance(ctx, instanceID); err != nil {
			return fmt.Errorf("failed to terminate instance: %w", err)
		}
		if m.recorder != nil {
			m.recorder.Eventf(pool, nil, corev1.EventTypeWarning, "WarmupTimeout", "HandleCloudWarmupTimeout",
				"Instance %s warmup timed out (no K8s node registration), terminated", instanceID)
		}
	}

	return nil
}
