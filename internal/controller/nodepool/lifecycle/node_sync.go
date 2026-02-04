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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	stratosv1alpha1 "github.com/stratos-sh/stratos/api/v1alpha1"
	"github.com/stratos-sh/stratos/internal/cloudprovider"
	"github.com/stratos-sh/stratos/internal/controller/nodepool/nodestate"
)

// SyncNodeState synchronizes the node state with the cloud provider instance nodestate.
func (m *Manager) SyncNodeState(ctx context.Context, pool *stratosv1alpha1.NodePool, node *corev1.Node) error {
	logger := log.FromContext(ctx)

	instanceID := node.Labels[nodestate.LabelInstanceID]
	if instanceID == "" {
		return nil
	}

	cloudState, err := m.cloudProvider.GetInstanceState(ctx, instanceID)
	if err != nil {
		return fmt.Errorf("failed to get instance state: %w", err)
	}

	nodeState := nodestate.ParseNodeState(node.Labels[nodestate.LabelState])

	// Handle terminated instances
	if cloudState == cloudprovider.InstanceStateTerminated {
		logger.Info("Instance terminated externally, cleaning up node", "node", node.Name)
		return m.deleteNode(ctx, node)
	}

	// Handle state mismatches
	switch {
	case cloudState == cloudprovider.InstanceStateStopped && nodeState == nodestate.NodeStateRunning:
		// Instance was stopped externally
		logger.Info("Instance stopped externally, updating node state", "node", node.Name)
		if m.hooks != nil {
			if err := m.hooks.PrepareForStandby(ctx, pool, node); err != nil {
				return err
			}
		}
		return m.TransitionState(ctx, node, nodestate.NodeStateStandby)

	case cloudState == cloudprovider.InstanceStateRunning && nodeState == nodestate.NodeStateStandby:
		// Instance was started externally
		logger.Info("Instance started externally, updating node state", "node", node.Name)
		if m.hooks != nil {
			if err := m.hooks.PrepareForRunning(ctx, pool, node); err != nil {
				return err
			}
		}
		// Set the last started annotation (for startup taint grace period tracking)
		if err := m.setLastStartedAnnotation(ctx, node); err != nil {
			logger.Error(err, "Failed to set last started annotation", "node", node.Name)
			// Continue anyway - this is not fatal
		}
		return m.TransitionState(ctx, node, nodestate.NodeStateRunning)
	}

	return nil
}

// FindNodeByInstanceID finds a Kubernetes node by its cloud provider instance ID.
// It searches by ProviderID (aws:///region/instance-id format) and the instance-id label.
func (m *Manager) FindNodeByInstanceID(ctx context.Context, instanceID string) (*corev1.Node, error) {
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
		if node.Labels != nil && node.Labels[nodestate.LabelInstanceID] == instanceID {
			return node, nil
		}
	}

	return nil, nil // Node not found, but not an error
}

// deleteNode deletes a Kubernetes node object.
func (m *Manager) deleteNode(ctx context.Context, node *corev1.Node) error {
	logger := log.FromContext(ctx)

	if err := m.client.Delete(ctx, node); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete node: %w", err)
	}

	logger.Info("Deleted node", "node", node.Name)
	return nil
}

// setLastStartedAnnotation sets the AnnotationLastStarted annotation on a node if not already set.
// This is used for tracking when the node started (for startup taint grace period).
func (m *Manager) setLastStartedAnnotation(ctx context.Context, node *corev1.Node) error {
	// Only set if not already present (allows pre-setting for tests or manual override)
	if node.Annotations != nil && node.Annotations[nodestate.AnnotationLastStarted] != "" {
		return nil
	}
	patch := client.MergeFrom(node.DeepCopy())
	if node.Annotations == nil {
		node.Annotations = make(map[string]string)
	}
	node.Annotations[nodestate.AnnotationLastStarted] = time.Now().Format(time.RFC3339)
	return m.client.Patch(ctx, node, patch)
}
