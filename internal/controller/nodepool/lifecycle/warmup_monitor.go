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
	"github.com/stratos-sh/stratos/internal/cloudprovider"
	"github.com/stratos-sh/stratos/internal/controller/nodepool/nodestate"
	"github.com/stratos-sh/stratos/internal/metrics"
)

// MonitorWarmup monitors a node in warmup state and transitions it when the instance self-stops.
func (m *Manager) MonitorWarmup(ctx context.Context, pool *stratosv1alpha1.NodePool, node *corev1.Node) error {
	logger := log.FromContext(ctx)

	instanceID := node.Labels[nodestate.LabelInstanceID]
	if instanceID == "" {
		return fmt.Errorf("node %s has no instance ID label", node.Name)
	}

	cloudState, err := m.cloudProvider.GetInstanceState(ctx, instanceID)
	if err != nil {
		return fmt.Errorf("failed to get instance state: %w", err)
	}

	logger.V(1).Info("Checking warmup instance state", "node", node.Name, "instanceID", instanceID, "cloudState", cloudState)

	switch cloudState {
	case cloudprovider.InstanceStateStopped:
		return m.handleWarmupStopped(ctx, pool, node, instanceID)

	case cloudprovider.InstanceStateRunning:
		return m.handleWarmupRunning(ctx, pool, node, instanceID)

	case cloudprovider.InstanceStateTerminated:
		logger.Info("Instance terminated during warmup", "node", node.Name)
		metrics.RecordWarmupFailure(pool.Name, "terminated")
		return m.deleteNode(ctx, node)

	case cloudprovider.InstanceStateStopping:
		logger.Info("Instance is stopping", "node", node.Name)
	}

	return nil
}

// handleWarmupStopped transitions a stopped warmup node to standby state.
func (m *Manager) handleWarmupStopped(ctx context.Context, pool *stratosv1alpha1.NodePool, node *corev1.Node, instanceID string) error {
	logger := log.FromContext(ctx)
	logger.Info("Instance stopped, transitioning to standby", "node", node.Name)

	// Record warmup completion
	startTime := parseUnixTimestamp(node.Labels[nodestate.LabelStateSince])
	if !startTime.IsZero() {
		duration := time.Since(startTime).Seconds()
		metrics.RecordWarmupDuration(pool.Name, "controller_stop", duration)
	}

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
		m.recorder.Eventf(pool, nil, corev1.EventTypeNormal, "WarmupCompleted", "MonitorWarmup",
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

// handleWarmupRunning checks timeout and readiness for a running warmup node.
func (m *Manager) handleWarmupRunning(ctx context.Context, pool *stratosv1alpha1.NodePool, node *corev1.Node, instanceID string) error {
	logger := log.FromContext(ctx)

	startTime := parseUnixTimestamp(node.Labels[nodestate.LabelStateSince])
	if startTime.IsZero() {
		startTime = time.Now()
	}

	timeout := pool.Spec.PreWarm.GetTimeout().Duration
	if time.Since(startTime) > timeout {
		logger.Info("Warmup timeout exceeded", "node", node.Name, "timeout", timeout)
		return m.handleWarmupTimeout(ctx, pool, node, instanceID)
	}

	return m.handleControllerStopWarmup(ctx, pool, node, instanceID, startTime)
}

// MonitorCloudWarmup monitors a cloud instance in warmup state and handles the case
// where the instance stops before the K8s node registers.
// This is critical for detecting warmup failures when nodes don't join the cluster.
func (m *Manager) MonitorCloudWarmup(ctx context.Context, pool *stratosv1alpha1.NodePool, instance *cloudprovider.Instance) error {
	logger := log.FromContext(ctx)

	instanceID := instance.ID

	node, err := m.FindNodeByInstanceID(ctx, instanceID)
	if err != nil {
		return fmt.Errorf("failed to find node for instance %s: %w", instanceID, err)
	}

	switch instance.State {
	case cloudprovider.InstanceStateStopped:
		return m.handleCloudWarmupStopped(ctx, pool, node, instanceID)

	case cloudprovider.InstanceStateTerminated, cloudprovider.InstanceStateShuttingDown:
		return m.handleCloudWarmupTerminated(ctx, pool, node, instanceID)

	case cloudprovider.InstanceStateRunning, cloudprovider.InstanceStatePending:
		return m.handleCloudWarmupRunning(ctx, pool, node, instance)

	case cloudprovider.InstanceStateStopping:
		logger.Info("Instance is stopping", "instanceID", instanceID)
	}

	return nil
}

// handleCloudWarmupStopped handles a cloud warmup instance that has stopped.
func (m *Manager) handleCloudWarmupStopped(ctx context.Context, pool *stratosv1alpha1.NodePool, node *corev1.Node, instanceID string) error {
	logger := log.FromContext(ctx)

	if node != nil {
		// Instance stopped and node exists - check if it has Stratos labels
		if node.Labels[nodestate.LabelPool] == pool.Name && node.Labels[nodestate.LabelState] != "" {
			logger.Info("Instance stopped with labeled K8s node, will be handled by MonitorWarmup",
				"instanceID", instanceID, "node", node.Name, "state", node.Labels[nodestate.LabelState])
			return nil
		}

		// Node exists but doesn't have Stratos labels - label it and transition to standby
		logger.Info("Instance stopped with unlabeled K8s node, labeling and transitioning to standby",
			"instanceID", instanceID, "node", node.Name)
		return m.adoptAndTransitionToStandby(ctx, pool, node, instanceID)
	}

	// Instance stopped but node never registered - warmup failure
	logger.Info("Instance stopped before K8s node registration, treating as warmup failure",
		"instanceID", instanceID)
	return m.handleWarmupFailure(ctx, pool, instanceID, "node_never_registered")
}

// handleCloudWarmupTerminated handles a cloud warmup instance that was terminated.
func (m *Manager) handleCloudWarmupTerminated(ctx context.Context, pool *stratosv1alpha1.NodePool, node *corev1.Node, instanceID string) error {
	logger := log.FromContext(ctx)

	logger.Info("Instance terminated during warmup", "instanceID", instanceID)
	metrics.RecordWarmupFailure(pool.Name, "terminated")

	if node != nil {
		logger.Info("Cleaning up K8s node for terminated instance", "node", node.Name)
		return m.deleteNode(ctx, node)
	}
	return nil
}

// handleCloudWarmupRunning handles a cloud warmup instance that is still running.
func (m *Manager) handleCloudWarmupRunning(ctx context.Context, pool *stratosv1alpha1.NodePool, node *corev1.Node, instance *cloudprovider.Instance) error {
	logger := log.FromContext(ctx)
	instanceID := instance.ID

	// If K8s node exists but isn't tracked by warmup workflow, label it
	if node != nil && node.Labels[nodestate.LabelState] == "" {
		logger.Info("Labeling unlabeled K8s node as warmup", "instanceID", instanceID, "node", node.Name)
		if err := m.LabelNode(ctx, node, pool.Name, instanceID, nodestate.NodeStateWarmup, pool.Spec.Template.Labels); err != nil {
			return fmt.Errorf("failed to label warmup node: %w", err)
		}
	}

	// Determine launch time for timeout check
	launchTime := instance.LaunchTime
	if launchTime.IsZero() {
		if ts, ok := instance.Tags[nodestate.TagState+"-since"]; ok {
			if parsed, parseErr := time.Parse(time.RFC3339, ts); parseErr == nil {
				launchTime = parsed
			}
		}
	}

	if launchTime.IsZero() {
		return nil
	}

	timeout := pool.Spec.PreWarm.GetTimeout().Duration
	if time.Since(launchTime) <= timeout {
		return nil
	}

	logger.Info("Cloud warmup timeout exceeded", "instanceID", instanceID, "timeout", timeout)
	if node != nil {
		return m.handleWarmupTimeout(ctx, pool, node, instanceID)
	}
	return m.handleCloudWarmupTimeout(ctx, pool, instanceID)
}
