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
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	stratosv1alpha1 "github.com/stratos-sh/stratos/api/v1alpha1"
	"github.com/stratos-sh/stratos/internal/cloudprovider"
	"github.com/stratos-sh/stratos/internal/metrics"
)

// NodeLauncher defines the interface for launching instances with cloud-specific NodeClass.
// This interface is implemented by cloud-specific providers (AWSProvider, FakeProvider).
type NodeLauncher interface {
	LaunchInstance(ctx context.Context, nodeClass *stratosv1alpha1.AWSNodeClass, poolName, clusterName string) (*cloudprovider.Instance, error)
}

// NodeManager handles the lifecycle of Stratos-managed nodes.
type NodeManager struct {
	client        client.Client
	recorder      record.EventRecorder
	cloudProvider cloudprovider.CloudProvider
	clusterName   string
}

// NewNodeManager creates a new NodeManager.
func NewNodeManager(c client.Client, recorder record.EventRecorder, provider cloudprovider.CloudProvider, clusterName string) *NodeManager {
	return &NodeManager{
		client:        c,
		recorder:      recorder,
		cloudProvider: provider,
		clusterName:   clusterName,
	}
}

// LaunchNode launches a new node for the given pool using the provided NodeClass.
// The launcher must be a cloud-specific provider that implements NodeLauncher (e.g., AWSProvider, FakeProvider).
// Subnet selection is handled internally by the cloud provider using round-robin distribution.
func (m *NodeManager) LaunchNode(ctx context.Context, pool *stratosv1alpha1.NodePool, nodeClass *stratosv1alpha1.AWSNodeClass, launcher NodeLauncher) (*corev1.Node, error) {
	logger := log.FromContext(ctx)

	// Launch the instance using the cloud-specific provider
	logger.Info("Launching instance", "pool", pool.Name, "nodeClass", nodeClass.Name, "instanceType", nodeClass.Spec.InstanceType)
	instance, err := launcher.LaunchInstance(ctx, nodeClass, pool.Name, m.clusterName)
	if err != nil {
		return nil, fmt.Errorf("failed to launch instance: %w", err)
	}

	logger.Info("Instance launched", "instanceID", instance.ID, "state", instance.State, "subnet", instance.SubnetID)

	// Record event
	if m.recorder != nil {
		m.recorder.Eventf(pool, corev1.EventTypeNormal, "WarmupStarted",
			"Started warming up node %s", instance.ID)
	}

	// Note: The Kubernetes node object will be created by the kubelet when it joins.
	// We'll wait for it to appear and then label it.
	return nil, nil
}

// WaitForNodeJoin waits for a node with the given instance ID to join the cluster.
func (m *NodeManager) WaitForNodeJoin(ctx context.Context, pool *stratosv1alpha1.NodePool, instanceID string, timeout time.Duration) (*corev1.Node, error) {
	logger := log.FromContext(ctx)

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		// List nodes and look for one with matching instance ID
		nodeList := &corev1.NodeList{}
		if err := m.client.List(ctx, nodeList); err != nil {
			logger.Error(err, "Failed to list nodes")
			time.Sleep(5 * time.Second)
			continue
		}

		for _, node := range nodeList.Items {
			// Check provider ID for AWS instance ID
			if containsInstanceID(node.Spec.ProviderID, instanceID) {
				logger.Info("Node joined cluster", "node", node.Name, "instanceID", instanceID)
				return &node, nil
			}
		}

		time.Sleep(5 * time.Second)
	}

	return nil, fmt.Errorf("timeout waiting for node %s to join", instanceID)
}

// LabelNode applies Stratos labels to a node.
// If the node is entering warmup state, it is also cordoned to prevent scheduling.
func (m *NodeManager) LabelNode(ctx context.Context, node *corev1.Node, poolName, instanceID string, state NodeState) error {
	logger := log.FromContext(ctx)

	// Create a copy for patching
	patch := client.MergeFrom(node.DeepCopy())

	// Ensure labels map exists
	if node.Labels == nil {
		node.Labels = make(map[string]string)
	}

	// Set Stratos labels
	node.Labels[LabelPool] = poolName
	node.Labels[LabelState] = string(state)
	node.Labels[LabelInstanceID] = instanceID
	node.Labels[LabelStateSince] = fmt.Sprintf("%d", time.Now().Unix())

	// Ensure annotations map exists
	if node.Annotations == nil {
		node.Annotations = make(map[string]string)
	}

	// Cordon warmup nodes to prevent scheduling during warmup phase
	if state == NodeStateWarmup {
		node.Spec.Unschedulable = true
	}

	// Apply patch
	if err := m.client.Patch(ctx, node, patch); err != nil {
		return fmt.Errorf("failed to label node: %w", err)
	}

	logger.Info("Labeled node", "node", node.Name, "pool", poolName, "state", state, "cordoned", state == NodeStateWarmup)
	return nil
}

// TransitionState transitions a node to a new state.
func (m *NodeManager) TransitionState(ctx context.Context, node *corev1.Node, newState NodeState) error {
	logger := log.FromContext(ctx)

	currentState := ParseNodeState(node.Labels[LabelState])
	if !IsValidTransition(currentState, newState) {
		return &InvalidTransitionError{From: currentState, To: newState}
	}

	// Create a copy for patching
	patch := client.MergeFrom(node.DeepCopy())

	// Update state labels
	node.Labels[LabelState] = string(newState)
	node.Labels[LabelStateSince] = fmt.Sprintf("%d", time.Now().Unix())

	// Apply patch
	if err := m.client.Patch(ctx, node, patch); err != nil {
		return fmt.Errorf("failed to update node state: %w", err)
	}

	logger.Info("Transitioned node state", "node", node.Name, "from", currentState, "to", newState)
	return nil
}

// MonitorWarmup monitors a node in warmup state and transitions it when the instance self-stops.
func (m *NodeManager) MonitorWarmup(ctx context.Context, pool *stratosv1alpha1.NodePool, node *corev1.Node) error {
	logger := log.FromContext(ctx)

	instanceID := node.Labels[LabelInstanceID]
	if instanceID == "" {
		return fmt.Errorf("node %s has no instance ID label", node.Name)
	}

	// Get instance state from cloud provider
	state, err := m.cloudProvider.GetInstanceState(ctx, instanceID)
	if err != nil {
		return fmt.Errorf("failed to get instance state: %w", err)
	}

	logger.V(1).Info("Checking warmup instance state", "node", node.Name, "instanceID", instanceID, "cloudState", state)

	switch state {
	case cloudprovider.InstanceStateStopped:
		// Instance has self-stopped - transition to standby
		logger.Info("Instance self-stopped, transitioning to standby", "node", node.Name)

		// Record warmup completion
		startTime := parseUnixTimestamp(node.Labels[LabelStateSince])
		if !startTime.IsZero() {
			duration := time.Since(startTime).Seconds()
			metrics.RecordWarmupDuration(pool.Name, "self_stop", duration)
		}

		// Prepare node for standby (cordon + taint + re-add startup taints)
		if err := m.prepareNodeForStandby(ctx, pool, node); err != nil {
			return err
		}

		if err := m.TransitionState(ctx, node, NodeStateStandby); err != nil {
			return err
		}

		// Update cloud provider tags
		if err := m.cloudProvider.UpdateInstanceTags(ctx, instanceID, map[string]string{
			TagState: string(NodeStateStandby),
		}); err != nil {
			logger.Error(err, "Failed to update instance tags", "instanceID", instanceID)
		}

		// Record event
		if m.recorder != nil {
			m.recorder.Eventf(pool, corev1.EventTypeNormal, "WarmupCompleted",
				"Node %s completed warmup and is now standby", node.Name)
		}

		// Set warmup completed annotation
		patch := client.MergeFrom(node.DeepCopy())
		if node.Annotations == nil {
			node.Annotations = make(map[string]string)
		}
		node.Annotations[AnnotationWarmupCompleted] = time.Now().Format(time.RFC3339)
		if err := m.client.Patch(ctx, node, patch); err != nil {
			logger.Error(err, "Failed to set warmup completed annotation")
		}

	case cloudprovider.InstanceStateRunning:
		// Instance still running - behavior depends on completion mode
		startTime := parseUnixTimestamp(node.Labels[LabelStateSince])
		if startTime.IsZero() {
			startTime = time.Now()
		}

		// Check timeout first (applies to both modes)
		timeout := pool.Spec.PreWarm.GetTimeout().Duration
		if time.Since(startTime) > timeout {
			logger.Info("Warmup timeout exceeded", "node", node.Name, "timeout", timeout)
			return m.handleWarmupTimeout(ctx, pool, node, instanceID)
		}

		// In ControllerStop mode, check if node is ready and stop it
		if pool.Spec.PreWarm.GetCompletionMode() == stratosv1alpha1.WarmupCompletionModeControllerStop {
			return m.handleControllerStopWarmup(ctx, pool, node, instanceID, startTime)
		}
		// In SelfStop mode, just wait for instance to self-stop (no action needed)

	case cloudprovider.InstanceStateTerminated:
		// Instance was terminated - clean up node
		logger.Info("Instance terminated during warmup", "node", node.Name)
		metrics.RecordWarmupFailure(pool.Name, "terminated")
		return m.deleteNode(ctx, node)

	case cloudprovider.InstanceStateStopping:
		// Instance is stopping - wait for it to stop
		logger.Info("Instance is stopping", "node", node.Name)
	}

	return nil
}

// handleWarmupTimeout handles a warmup timeout according to the configured action.
func (m *NodeManager) handleWarmupTimeout(ctx context.Context, pool *stratosv1alpha1.NodePool, node *corev1.Node, instanceID string) error {
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
			m.recorder.Eventf(pool, corev1.EventTypeWarning, "WarmupTimeout",
				"Node %s warmup timed out, forcing stop", node.Name)
		}

	case stratosv1alpha1.TimeoutActionTerminate:
		logger.Info("Terminating instance due to warmup timeout", "node", node.Name)
		if err := m.TransitionState(ctx, node, NodeStateTerminating); err != nil {
			return err
		}
		if err := m.cloudProvider.TerminateInstance(ctx, instanceID); err != nil {
			return fmt.Errorf("failed to terminate instance: %w", err)
		}
		if err := m.deleteNode(ctx, node); err != nil {
			return err
		}
		if m.recorder != nil {
			m.recorder.Eventf(pool, corev1.EventTypeWarning, "WarmupTimeout",
				"Node %s warmup timed out, terminated", node.Name)
		}
	}

	return nil
}

// handleControllerStopWarmup handles warmup in ControllerStop mode.
// It checks if the node is ready and stops it when conditions are met.
func (m *NodeManager) handleControllerStopWarmup(ctx context.Context, pool *stratosv1alpha1.NodePool, node *corev1.Node, instanceID string, startTime time.Time) error {
	logger := log.FromContext(ctx)

	// Check if node is Ready
	if !IsNodeReady(node) {
		logger.V(1).Info("Node not ready, waiting for warmup to complete",
			"node", node.Name, "mode", "ControllerStop")
		return nil
	}

	// Check network readiness if WhenNetworkReady is configured
	if pool.Spec.Template.StartupTaintRemoval == stratosv1alpha1.StartupTaintRemovalWhenNetworkReady {
		checker := NewNetworkReadinessChecker(m.client)
		if !checker.IsReady(node) {
			logger.V(1).Info("Node ready but network not ready, waiting",
				"node", node.Name, "mode", "ControllerStop")
			return nil
		}
		// For EKS, also verify aws-node pod is Ready
		if checker.HasNetworkingReadyCondition(node) && !checker.IsAwsNodePodReady(ctx, node.Name) {
			logger.V(1).Info("Node ready but aws-node pod not ready, waiting",
				"node", node.Name, "mode", "ControllerStop")
			return nil
		}
	}

	// Node is ready (and network ready if required) - stop the instance
	logger.Info("Node ready, stopping instance in ControllerStop mode",
		"node", node.Name, "instanceID", instanceID)

	// Stop the instance
	if err := m.cloudProvider.StopInstance(ctx, instanceID, false); err != nil {
		return fmt.Errorf("failed to stop instance in ControllerStop mode: %w", err)
	}

	// Record warmup duration with mode label
	duration := time.Since(startTime).Seconds()
	metrics.RecordWarmupDuration(pool.Name, "controller_stop", duration)

	// Prepare node for standby (cordon + taint + re-add startup taints)
	if err := m.prepareNodeForStandby(ctx, pool, node); err != nil {
		return err
	}

	if err := m.TransitionState(ctx, node, NodeStateStandby); err != nil {
		return err
	}

	// Update cloud provider tags
	if err := m.cloudProvider.UpdateInstanceTags(ctx, instanceID, map[string]string{
		TagState: string(NodeStateStandby),
	}); err != nil {
		logger.Error(err, "Failed to update instance tags", "instanceID", instanceID)
	}

	// Record event
	if m.recorder != nil {
		m.recorder.Eventf(pool, corev1.EventTypeNormal, "WarmupCompleted",
			"Node %s completed warmup (ControllerStop mode) and is now standby", node.Name)
	}

	// Set warmup completed annotation
	patch := client.MergeFrom(node.DeepCopy())
	if node.Annotations == nil {
		node.Annotations = make(map[string]string)
	}
	node.Annotations[AnnotationWarmupCompleted] = time.Now().Format(time.RFC3339)
	if err := m.client.Patch(ctx, node, patch); err != nil {
		logger.Error(err, "Failed to set warmup completed annotation")
	}

	return nil
}

// StartNode starts a standby node for scale-up.
func (m *NodeManager) StartNode(ctx context.Context, pool *stratosv1alpha1.NodePool, node *corev1.Node) error {
	logger := log.FromContext(ctx)

	instanceID := node.Labels[LabelInstanceID]
	if instanceID == "" {
		return fmt.Errorf("node %s has no instance ID label", node.Name)
	}

	// Prepare node for running (uncordon + remove standby taint) before starting
	// Note: Startup taints are preserved and removed later after CNI is ready
	if err := m.prepareNodeForRunning(ctx, pool, node); err != nil {
		return err
	}

	// Start the instance
	logger.Info("Starting instance", "node", node.Name, "instanceID", instanceID)
	if err := m.cloudProvider.StartInstance(ctx, instanceID); err != nil {
		return fmt.Errorf("failed to start instance: %w", err)
	}

	// Transition state
	if err := m.TransitionState(ctx, node, NodeStateRunning); err != nil {
		return err
	}

	// Update cloud provider tags
	if err := m.cloudProvider.UpdateInstanceTags(ctx, instanceID, map[string]string{
		TagState: string(NodeStateRunning),
	}); err != nil {
		logger.Error(err, "Failed to update instance tags", "instanceID", instanceID)
	}

	// Set scale-up started annotation (for in-flight tracking) and last started annotation
	now := time.Now().Format(time.RFC3339)
	patch := client.MergeFrom(node.DeepCopy())
	if node.Annotations == nil {
		node.Annotations = make(map[string]string)
	}
	node.Annotations[AnnotationScaleUpStarted] = now
	node.Annotations[AnnotationLastStarted] = now
	if err := m.client.Patch(ctx, node, patch); err != nil {
		logger.Error(err, "Failed to set scale-up annotations")
	}

	// Record event
	if m.recorder != nil {
		m.recorder.Eventf(pool, corev1.EventTypeNormal, "NodeStarted",
			"Started node %s for scale-up", node.Name)
	}

	metrics.RecordScaleUp(pool.Name)

	return nil
}

// StopNode stops a running node for scale-down (after draining).
func (m *NodeManager) StopNode(ctx context.Context, pool *stratosv1alpha1.NodePool, node *corev1.Node) error {
	logger := log.FromContext(ctx)

	instanceID := node.Labels[LabelInstanceID]
	if instanceID == "" {
		return fmt.Errorf("node %s has no instance ID label", node.Name)
	}

	// Prepare node for standby (cordon + taint + re-add startup taints) before stopping
	if err := m.prepareNodeForStandby(ctx, pool, node); err != nil {
		return err
	}

	// Stop the instance
	logger.Info("Stopping instance", "node", node.Name, "instanceID", instanceID)
	if err := m.cloudProvider.StopInstance(ctx, instanceID, false); err != nil {
		return fmt.Errorf("failed to stop instance: %w", err)
	}

	// Transition state
	if err := m.TransitionState(ctx, node, NodeStateStandby); err != nil {
		return err
	}

	// Update cloud provider tags
	if err := m.cloudProvider.UpdateInstanceTags(ctx, instanceID, map[string]string{
		TagState: string(NodeStateStandby),
	}); err != nil {
		logger.Error(err, "Failed to update instance tags", "instanceID", instanceID)
	}

	// Remove scale-down candidate annotation
	patch := client.MergeFrom(node.DeepCopy())
	delete(node.Annotations, AnnotationScaleDownCandidateSince)
	if err := m.client.Patch(ctx, node, patch); err != nil {
		logger.Error(err, "Failed to remove scale-down annotation")
	}

	// Record event
	if m.recorder != nil {
		m.recorder.Eventf(pool, corev1.EventTypeNormal, "NodeStopped",
			"Stopped node %s after scale-down", node.Name)
	}

	metrics.RecordScaleDown(pool.Name)

	return nil
}

// deleteNode deletes a Kubernetes node object.
func (m *NodeManager) deleteNode(ctx context.Context, node *corev1.Node) error {
	logger := log.FromContext(ctx)

	if err := m.client.Delete(ctx, node); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete node: %w", err)
	}

	logger.Info("Deleted node", "node", node.Name)
	return nil
}

// GetStandbyNodes returns all standby nodes for a pool.
func (m *NodeManager) GetStandbyNodes(ctx context.Context, poolName string) ([]corev1.Node, error) {
	nodeList := &corev1.NodeList{}
	if err := m.client.List(ctx, nodeList, client.MatchingLabels{
		LabelPool:  poolName,
		LabelState: string(NodeStateStandby),
	}); err != nil {
		return nil, err
	}
	return nodeList.Items, nil
}

// GetRunningNodes returns all running nodes for a pool.
func (m *NodeManager) GetRunningNodes(ctx context.Context, poolName string) ([]corev1.Node, error) {
	nodeList := &corev1.NodeList{}
	if err := m.client.List(ctx, nodeList, client.MatchingLabels{
		LabelPool:  poolName,
		LabelState: string(NodeStateRunning),
	}); err != nil {
		return nil, err
	}
	return nodeList.Items, nil
}

// SyncNodeState synchronizes the node state with the cloud provider instance state.
func (m *NodeManager) SyncNodeState(ctx context.Context, pool *stratosv1alpha1.NodePool, node *corev1.Node) error {
	logger := log.FromContext(ctx)

	instanceID := node.Labels[LabelInstanceID]
	if instanceID == "" {
		return nil
	}

	cloudState, err := m.cloudProvider.GetInstanceState(ctx, instanceID)
	if err != nil {
		return fmt.Errorf("failed to get instance state: %w", err)
	}

	nodeState := ParseNodeState(node.Labels[LabelState])

	// Handle terminated instances
	if cloudState == cloudprovider.InstanceStateTerminated {
		logger.Info("Instance terminated externally, cleaning up node", "node", node.Name)
		return m.deleteNode(ctx, node)
	}

	// Handle state mismatches
	switch {
	case cloudState == cloudprovider.InstanceStateStopped && nodeState == NodeStateRunning:
		// Instance was stopped externally
		logger.Info("Instance stopped externally, updating node state", "node", node.Name)
		if err := m.prepareNodeForStandby(ctx, pool, node); err != nil {
			return err
		}
		return m.TransitionState(ctx, node, NodeStateStandby)

	case cloudState == cloudprovider.InstanceStateRunning && nodeState == NodeStateStandby:
		// Instance was started externally
		logger.Info("Instance started externally, updating node state", "node", node.Name)
		if err := m.prepareNodeForRunning(ctx, pool, node); err != nil {
			return err
		}
		// Set the last started annotation (for startup taint grace period tracking)
		if err := m.setLastStartedAnnotation(ctx, node); err != nil {
			logger.Error(err, "Failed to set last started annotation", "node", node.Name)
			// Continue anyway - this is not fatal
		}
		return m.TransitionState(ctx, node, NodeStateRunning)
	}

	return nil
}

// FindNodeByInstanceID finds a Kubernetes node by its cloud provider instance ID.
// It searches by ProviderID (aws:///region/instance-id format) and the instance-id label.
func (m *NodeManager) FindNodeByInstanceID(ctx context.Context, instanceID string) (*corev1.Node, error) {
	nodeList := &corev1.NodeList{}
	if err := m.client.List(ctx, nodeList); err != nil {
		return nil, fmt.Errorf("failed to list nodes: %w", err)
	}

	for i := range nodeList.Items {
		node := &nodeList.Items[i]

		// Check provider ID (e.g., aws:///us-east-1a/i-1234567890abcdef0)
		if containsInstanceID(node.Spec.ProviderID, instanceID) {
			return node, nil
		}

		// Check instance ID label
		if node.Labels != nil && node.Labels[LabelInstanceID] == instanceID {
			return node, nil
		}
	}

	return nil, nil // Node not found, but not an error
}

// MonitorCloudWarmup monitors a cloud instance in warmup state and handles the case
// where the instance stops before the K8s node registers.
// This is critical for detecting warmup failures when nodes don't join the cluster.
func (m *NodeManager) MonitorCloudWarmup(ctx context.Context, pool *stratosv1alpha1.NodePool, instance *cloudprovider.Instance) error {
	logger := log.FromContext(ctx)

	instanceID := instance.ID
	logger.V(1).Info("Monitoring cloud warmup instance", "instanceID", instanceID, "cloudState", instance.State)

	// Check if K8s node exists for this instance
	node, err := m.FindNodeByInstanceID(ctx, instanceID)
	if err != nil {
		return fmt.Errorf("failed to find node for instance %s: %w", instanceID, err)
	}

	switch instance.State {
	case cloudprovider.InstanceStateStopped:
		if node != nil {
			// Instance stopped and node exists - check if it has Stratos labels
			if node.Labels[LabelPool] == pool.Name && node.Labels[LabelState] != "" {
				// Node has Stratos labels - MonitorWarmup() will handle the transition
				logger.Info("Instance stopped with labeled K8s node, will be handled by MonitorWarmup",
					"instanceID", instanceID, "node", node.Name, "state", node.Labels[LabelState])
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

	case cloudprovider.InstanceStateTerminated, cloudprovider.InstanceStateShuttingDown:
		// Instance was terminated unexpectedly
		logger.Info("Instance terminated during warmup", "instanceID", instanceID)
		metrics.RecordWarmupFailure(pool.Name, "terminated")

		// If K8s node exists, clean it up
		if node != nil {
			logger.Info("Cleaning up K8s node for terminated instance", "node", node.Name)
			return m.deleteNode(ctx, node)
		}
		return nil

	case cloudprovider.InstanceStateRunning, cloudprovider.InstanceStatePending:
		// If K8s node exists but isn't tracked by warmup workflow, label it
		if node != nil && node.Labels[LabelState] == "" {
			logger.Info("Labeling unlabeled K8s node as warmup", "instanceID", instanceID, "node", node.Name)
			if err := m.LabelNode(ctx, node, pool.Name, instanceID, NodeStateWarmup); err != nil {
				return fmt.Errorf("failed to label warmup node: %w", err)
			}
		}

		// Instance still running - check timeout
		launchTime := instance.LaunchTime
		if launchTime.IsZero() {
			// If we don't have launch time, check the state-since tag
			if ts, ok := instance.Tags[TagState+"-since"]; ok {
				launchTime, _ = time.Parse(time.RFC3339, ts)
			}
		}

		if !launchTime.IsZero() {
			timeout := pool.Spec.PreWarm.GetTimeout().Duration
			if time.Since(launchTime) > timeout {
				logger.Info("Cloud warmup timeout exceeded", "instanceID", instanceID, "timeout", timeout)

				// If node exists, delegate to MonitorWarmup for proper handling
				if node != nil {
					return m.handleWarmupTimeout(ctx, pool, node, instanceID)
				}

				// Node doesn't exist, handle as timeout failure
				return m.handleCloudWarmupTimeout(ctx, pool, instanceID)
			}
		}

	case cloudprovider.InstanceStateStopping:
		// Instance is stopping - wait for it to stop
		logger.Info("Instance is stopping", "instanceID", instanceID)
	}

	return nil
}

// handleWarmupFailure handles the case where warmup failed because the node never registered.
// This terminates the orphaned cloud instance and records appropriate metrics.
func (m *NodeManager) handleWarmupFailure(ctx context.Context, pool *stratosv1alpha1.NodePool, instanceID string, reason string) error {
	logger := log.FromContext(ctx)

	logger.Info("Handling warmup failure", "instanceID", instanceID, "reason", reason)
	metrics.RecordWarmupFailure(pool.Name, reason)

	// Terminate the orphaned instance
	if err := m.cloudProvider.TerminateInstance(ctx, instanceID); err != nil {
		return fmt.Errorf("failed to terminate orphaned instance %s: %w", instanceID, err)
	}

	// Record event
	if m.recorder != nil {
		m.recorder.Eventf(pool, corev1.EventTypeWarning, "WarmupFailed",
			"Instance %s warmup failed (%s), terminated", instanceID, reason)
	}

	logger.Info("Terminated orphaned warmup instance", "instanceID", instanceID)
	return nil
}

// handleCloudWarmupTimeout handles warmup timeout for instances that never registered as K8s nodes.
func (m *NodeManager) handleCloudWarmupTimeout(ctx context.Context, pool *stratosv1alpha1.NodePool, instanceID string) error {
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
			TagState:           "warmup-failed",
			TagState + "-note": "timeout_no_node",
		}); err != nil {
			logger.Error(err, "Failed to update instance tags", "instanceID", instanceID)
		}

		if m.recorder != nil {
			m.recorder.Eventf(pool, corev1.EventTypeWarning, "WarmupTimeout",
				"Instance %s warmup timed out (no K8s node registration), stopped", instanceID)
		}

	case stratosv1alpha1.TimeoutActionTerminate:
		logger.Info("Terminating instance due to warmup timeout (no K8s node)", "instanceID", instanceID)
		if err := m.cloudProvider.TerminateInstance(ctx, instanceID); err != nil {
			return fmt.Errorf("failed to terminate instance: %w", err)
		}
		if m.recorder != nil {
			m.recorder.Eventf(pool, corev1.EventTypeWarning, "WarmupTimeout",
				"Instance %s warmup timed out (no K8s node registration), terminated", instanceID)
		}
	}

	return nil
}

// adoptAndTransitionToStandby adopts an unlabeled K8s node that was created by warmup
// and transitions it directly to standby state.
func (m *NodeManager) adoptAndTransitionToStandby(ctx context.Context, pool *stratosv1alpha1.NodePool, node *corev1.Node, instanceID string) error {
	logger := log.FromContext(ctx)

	// Create a copy for patching
	patch := client.MergeFrom(node.DeepCopy())

	// Ensure labels map exists
	if node.Labels == nil {
		node.Labels = make(map[string]string)
	}

	// Set Stratos labels - directly to standby since instance already stopped successfully
	node.Labels[LabelPool] = pool.Name
	node.Labels[LabelState] = string(NodeStateStandby)
	node.Labels[LabelInstanceID] = instanceID
	node.Labels[LabelStateSince] = fmt.Sprintf("%d", time.Now().Unix())

	// Ensure annotations map exists
	if node.Annotations == nil {
		node.Annotations = make(map[string]string)
	}
	node.Annotations[AnnotationWarmupCompleted] = time.Now().Format(time.RFC3339)

	// Cordon the node and add standby taint
	node.Spec.Unschedulable = true
	if !hasTaint(node.Spec.Taints, TaintKeyStandby) {
		node.Spec.Taints = append(node.Spec.Taints, corev1.Taint{
			Key:    TaintKeyStandby,
			Effect: corev1.TaintEffectNoExecute,
		})
	}

	// Apply patch
	if err := m.client.Patch(ctx, node, patch); err != nil {
		return fmt.Errorf("failed to adopt and label node: %w", err)
	}

	// Update cloud provider tags to standby
	if err := m.cloudProvider.UpdateInstanceTags(ctx, instanceID, map[string]string{
		TagState: string(NodeStateStandby),
	}); err != nil {
		logger.Error(err, "Failed to update instance tags", "instanceID", instanceID)
	}

	logger.Info("Adopted unlabeled node and transitioned to standby",
		"node", node.Name, "pool", pool.Name, "instanceID", instanceID)

	// Record event
	if m.recorder != nil {
		m.recorder.Eventf(pool, corev1.EventTypeNormal, "NodeAdopted",
			"Adopted unlabeled node %s and transitioned to standby", node.Name)
	}

	return nil
}

// containsInstanceID checks if a provider ID contains the instance ID.
func containsInstanceID(providerID, instanceID string) bool {
	// AWS provider ID format: aws:///region/instance-id
	return len(providerID) > 0 && len(instanceID) > 0 &&
		(providerID == instanceID || strings.Contains(providerID, instanceID))
}

// hasTaint checks if a taint with the given key exists in the slice.
func hasTaint(taints []corev1.Taint, key string) bool {
	for _, t := range taints {
		if t.Key == key {
			return true
		}
	}
	return false
}

// removeTaint returns a new slice with the taint matching the key removed.
func removeTaint(taints []corev1.Taint, key string) []corev1.Taint {
	result := make([]corev1.Taint, 0, len(taints))
	for _, t := range taints {
		if t.Key != key {
			result = append(result, t)
		}
	}
	return result
}

// hasTaintWithKeyAndEffect checks if a taint with the given key and effect exists.
func hasTaintWithKeyAndEffect(taints []corev1.Taint, key string, effect corev1.TaintEffect) bool {
	for _, t := range taints {
		if t.Key == key && t.Effect == effect {
			return true
		}
	}
	return false
}

// removeTaintByKeyAndEffect returns a new slice with the taint matching key and effect removed.
func removeTaintByKeyAndEffect(taints []corev1.Taint, key string, effect corev1.TaintEffect) []corev1.Taint {
	result := make([]corev1.Taint, 0, len(taints))
	for _, t := range taints {
		if t.Key != key || t.Effect != effect {
			result = append(result, t)
		}
	}
	return result
}

// prepareNodeForStandby cordons a node, adds the NoExecute standby taint,
// and re-adds any configured startup taints.
// This prevents scheduling, evicts stale pods, and ensures startup taints
// are present when the node starts from standby (since kubelet doesn't
// re-apply --register-with-taints on restart).
func (m *NodeManager) prepareNodeForStandby(ctx context.Context, pool *stratosv1alpha1.NodePool, node *corev1.Node) error {
	logger := log.FromContext(ctx)

	patch := client.MergeFrom(node.DeepCopy())
	modified := false

	// Cordon the node
	if !node.Spec.Unschedulable {
		node.Spec.Unschedulable = true
		modified = true
	}

	// Add standby taint if not present
	if !hasTaint(node.Spec.Taints, TaintKeyStandby) {
		node.Spec.Taints = append(node.Spec.Taints, corev1.Taint{
			Key:    TaintKeyStandby,
			Effect: corev1.TaintEffectNoExecute,
		})
		modified = true
	}

	// Re-add startup taints so they're present when node starts from standby.
	// EKS removes these during warmup when CNI becomes ready, but kubelet
	// doesn't re-apply --register-with-taints on restart from standby.
	for _, st := range pool.Spec.Template.StartupTaints {
		if !hasTaintWithKeyAndEffect(node.Spec.Taints, st.Key, st.Effect) {
			node.Spec.Taints = append(node.Spec.Taints, st)
			modified = true
		}
	}

	if !modified {
		return nil
	}

	if err := m.client.Patch(ctx, node, patch); err != nil {
		return fmt.Errorf("failed to prepare node for standby: %w", err)
	}

	logger.Info("Prepared node for standby (cordoned + tainted)",
		"node", node.Name, "startupTaintsAdded", len(pool.Spec.Template.StartupTaints))
	return nil
}

// prepareNodeForRunning uncordons a node and removes the standby taint.
// Startup taints are preserved and will be removed later by ProcessStartupTaints
// after the CNI is ready.
func (m *NodeManager) prepareNodeForRunning(ctx context.Context, pool *stratosv1alpha1.NodePool, node *corev1.Node) error {
	logger := log.FromContext(ctx)

	// Check if already uncordoned and untainted
	if !node.Spec.Unschedulable && !hasTaint(node.Spec.Taints, TaintKeyStandby) {
		return nil
	}

	patch := client.MergeFrom(node.DeepCopy())

	// Uncordon the node
	node.Spec.Unschedulable = false

	// Remove standby taint if present
	node.Spec.Taints = removeTaint(node.Spec.Taints, TaintKeyStandby)

	// NOTE: Do NOT remove startup taints here
	// They will be removed by ProcessStartupTaints() after CNI is ready

	if err := m.client.Patch(ctx, node, patch); err != nil {
		return fmt.Errorf("failed to uncordon and untaint node: %w", err)
	}

	hasStartupTaints := len(pool.Spec.Template.StartupTaints) > 0
	logger.Info("Prepared node for running (uncordoned + standby taint removed)",
		"node", node.Name, "hasStartupTaints", hasStartupTaints)
	return nil
}

// setLastStartedAnnotation sets the AnnotationLastStarted annotation on a node if not already set.
// This is used for tracking when the node started (for startup taint grace period).
func (m *NodeManager) setLastStartedAnnotation(ctx context.Context, node *corev1.Node) error {
	// Only set if not already present (allows pre-setting for tests or manual override)
	if node.Annotations != nil && node.Annotations[AnnotationLastStarted] != "" {
		return nil
	}
	patch := client.MergeFrom(node.DeepCopy())
	if node.Annotations == nil {
		node.Annotations = make(map[string]string)
	}
	node.Annotations[AnnotationLastStarted] = time.Now().Format(time.RFC3339)
	return m.client.Patch(ctx, node, patch)
}

// parseUnixTimestamp parses a Unix timestamp string and returns the corresponding time.
// Returns zero time if parsing fails.
func parseUnixTimestamp(ts string) time.Time {
	if ts == "" {
		return time.Time{}
	}
	var unix int64
	if _, err := fmt.Sscanf(ts, "%d", &unix); err != nil {
		return time.Time{}
	}
	return time.Unix(unix, 0)
}

// ProcessStartupTaints handles startup taint removal based on the configured mode.
// Returns (removed, error) where removed is true ONLY if we actually removed taints this call.
// Returns false if taints are already gone, waiting for conditions, or in External mode.
func (m *NodeManager) ProcessStartupTaints(ctx context.Context, pool *stratosv1alpha1.NodePool, node *corev1.Node) (bool, error) {
	logger := log.FromContext(ctx)
	startupTaints := pool.Spec.Template.StartupTaints
	if len(startupTaints) == 0 {
		return false, nil // No startup taints configured
	}

	// Check if node still has any startup taints
	hasStartupTaints := m.nodeHasStartupTaints(node, startupTaints)
	if !hasStartupTaints {
		return false, nil // Already removed (by Stratos or external controller)
	}

	removalMode := pool.Spec.Template.StartupTaintRemoval
	if removalMode == "" {
		removalMode = stratosv1alpha1.StartupTaintRemovalWhenNetworkReady // default
	}

	// Get the node start time for timeout check
	startedAt := m.getNodeStartTime(node)

	// Check for timeout
	timedOut := !startedAt.IsZero() && time.Since(startedAt) > StartupTaintRemovalTimeout
	if timedOut {
		return m.handleStartupTaintTimeout(ctx, pool, node, removalMode, startedAt)
	}

	switch removalMode {
	case stratosv1alpha1.StartupTaintRemovalWhenNetworkReady:
		return m.removeStartupTaintsWhenNetworkReady(ctx, pool, node, startupTaints, startedAt)

	case stratosv1alpha1.StartupTaintRemovalExternal:
		// In External mode, just log that we're waiting
		// We don't remove them - external controller should
		logger.V(1).Info("Waiting for external controller to remove startup taints",
			"node", node.Name, "mode", "External")
		return false, nil
	}

	return false, nil
}

// nodeHasStartupTaints checks if the node has any of the configured startup taints.
func (m *NodeManager) nodeHasStartupTaints(node *corev1.Node, startupTaints []corev1.Taint) bool {
	for _, st := range startupTaints {
		if hasTaintWithKeyAndEffect(node.Spec.Taints, st.Key, st.Effect) {
			return true
		}
	}
	return false
}

// getNodeStartTime returns the time the node was started (from annotation).
// Returns zero time if annotation is missing or invalid.
func (m *NodeManager) getNodeStartTime(node *corev1.Node) time.Time {
	startedAtStr := node.Annotations[AnnotationLastStarted]
	if startedAtStr == "" {
		return time.Time{}
	}

	startedAt, err := time.Parse(time.RFC3339, startedAtStr)
	if err != nil {
		return time.Time{}
	}

	return startedAt
}

// checkStartupTaintTimeout checks if the node has exceeded the startup taint removal timeout.
// Returns (timedOut, startedAt) where startedAt is the time the node was started.
func (m *NodeManager) checkStartupTaintTimeout(node *corev1.Node) (bool, time.Time) {
	startedAt := m.getNodeStartTime(node)
	if startedAt.IsZero() {
		return false, time.Time{}
	}
	return time.Since(startedAt) > StartupTaintRemovalTimeout, startedAt
}

// handleStartupTaintTimeout handles the case where startup taint removal has timed out.
func (m *NodeManager) handleStartupTaintTimeout(ctx context.Context, pool *stratosv1alpha1.NodePool, node *corev1.Node, mode stratosv1alpha1.StartupTaintRemovalMode, startedAt time.Time) (bool, error) {
	logger := log.FromContext(ctx)

	switch mode {
	case stratosv1alpha1.StartupTaintRemovalWhenNetworkReady:
		// In WhenNetworkReady mode, force remove taints after timeout
		logger.Info("Startup taint timeout, removing taints despite CNI not ready",
			"node", node.Name, "timeout", StartupTaintRemovalTimeout)

		// Emit warning event
		if m.recorder != nil {
			m.recorder.Eventf(pool, corev1.EventTypeWarning, "StartupTaintTimeout",
				"Node %s startup taints removed after timeout (CNI may not be ready)", node.Name)
		}

		if err := m.forceRemoveStartupTaints(ctx, pool, node); err != nil {
			metrics.RecordStartupTaintRemoval(pool.Name, "timeout", "error")
			return false, err
		}

		// Record metrics
		if !startedAt.IsZero() {
			metrics.RecordStartupTaintDuration(pool.Name, time.Since(startedAt).Seconds())
		}
		metrics.RecordStartupTaintRemoval(pool.Name, "timeout", "success")
		return true, nil

	case stratosv1alpha1.StartupTaintRemovalExternal:
		// In External mode, emit warning but do NOT remove taints
		logger.Info("Startup taint timeout in External mode, waiting for external controller",
			"node", node.Name, "timeout", StartupTaintRemovalTimeout)

		if m.recorder != nil {
			m.recorder.Eventf(pool, corev1.EventTypeWarning, "StartupTaintTimeoutExternal",
				"Node %s waiting for external controller to remove startup taints (timeout exceeded)", node.Name)
		}
		return false, nil
	}

	return false, nil
}

// removeStartupTaintsWhenNetworkReady removes startup taints when CNI is actually ready.
// For EKS, we check both the NetworkingReady condition AND the aws-node pod status,
// because the condition may be stale after node restart while the pod status is accurate.
// For other CNIs (Cilium, Calico), we trust the NetworkUnavailable condition.
func (m *NodeManager) removeStartupTaintsWhenNetworkReady(ctx context.Context, pool *stratosv1alpha1.NodePool, node *corev1.Node, startupTaints []corev1.Taint, startedAt time.Time) (bool, error) {
	logger := log.FromContext(ctx)

	checker := NewNetworkReadinessChecker(m.client)

	// First check node conditions (fast, works for all CNIs)
	if !checker.IsReady(node) {
		logger.V(1).Info("Network not ready (node condition), waiting to remove startup taints", "node", node.Name)
		return false, nil
	}

	// For EKS (detected by presence of NetworkingReady condition), also verify
	// aws-node pod is Ready. This is needed because NetworkingReady may be stale
	// after node restart, but aws-node pod only becomes Ready when IPAMD is listening.
	if checker.HasNetworkingReadyCondition(node) && !checker.IsAwsNodePodReady(ctx, node.Name) {
		logger.V(1).Info("Network not ready (aws-node pod not ready), waiting to remove startup taints", "node", node.Name)
		return false, nil
	}

	reason := checker.GetNetworkConditionReason(node)

	// Network is ready with fresh condition - remove startup taints
	logger.Info("Network ready, removing startup taints", "node", node.Name, "reason", reason)

	patch := client.MergeFrom(node.DeepCopy())
	for _, st := range startupTaints {
		node.Spec.Taints = removeTaintByKeyAndEffect(node.Spec.Taints, st.Key, st.Effect)
	}
	if err := m.client.Patch(ctx, node, patch); err != nil {
		metrics.RecordStartupTaintRemoval(pool.Name, "network_ready", "error")
		return false, fmt.Errorf("failed to remove startup taints: %w", err)
	}

	// Record metrics
	if !startedAt.IsZero() {
		metrics.RecordStartupTaintDuration(pool.Name, time.Since(startedAt).Seconds())
	}
	metrics.RecordStartupTaintRemoval(pool.Name, "network_ready", "success")

	// Emit event
	if m.recorder != nil {
		m.recorder.Eventf(pool, corev1.EventTypeNormal, "StartupTaintsRemoved",
			"Removed startup taints from node %s after network ready (%s)", node.Name, reason)
	}

	return true, nil
}

// forceRemoveStartupTaints forcibly removes all startup taints from a node.
func (m *NodeManager) forceRemoveStartupTaints(ctx context.Context, pool *stratosv1alpha1.NodePool, node *corev1.Node) error {
	startupTaints := pool.Spec.Template.StartupTaints
	if len(startupTaints) == 0 {
		return nil
	}

	patch := client.MergeFrom(node.DeepCopy())
	for _, st := range startupTaints {
		node.Spec.Taints = removeTaintByKeyAndEffect(node.Spec.Taints, st.Key, st.Effect)
	}
	return m.client.Patch(ctx, node, patch)
}
