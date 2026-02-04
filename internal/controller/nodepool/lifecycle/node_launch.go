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
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	stratosv1alpha1 "github.com/stratos-sh/stratos/api/v1alpha1"
	"github.com/stratos-sh/stratos/internal/cloudprovider"
	"github.com/stratos-sh/stratos/internal/controller/nodepool/nodestate"
)

// LaunchNode launches a new node for the given pool using the provided NodeClass.
// The launcher must be a cloud-specific provider that implements NodeLauncher (e.g., AWSProvider, FakeProvider).
// Subnet selection is handled internally by the cloud provider using round-robin distribution.
func (m *Manager) LaunchNode(ctx context.Context, pool *stratosv1alpha1.NodePool, nodeClass stratosv1alpha1.NodeClass, launcher NodeLauncher) (*corev1.Node, error) {
	logger := log.FromContext(ctx)

	// Build template config from NodePool spec
	templateConfig := &cloudprovider.TemplateConfig{
		Labels:                      pool.Spec.Template.Labels,
		Taints:                      pool.Spec.Template.Taints,
		EnableNetworkReadinessTaint: pool.Spec.Template.IsNetworkReadinessTaintEnabled(),
	}

	// Launch the instance using the cloud-specific provider
	logger.Info("Launching instance", "pool", pool.Name, "nodeClass", nodeClass.GetName(), "instanceType", nodeClass.GetInstanceType())
	instance, err := launcher.LaunchInstance(ctx, nodeClass, pool.Name, m.clusterName, templateConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to launch instance: %w", err)
	}

	logger.Info("Instance launched", "instanceID", instance.ID, "state", instance.State, "subnet", instance.SubnetID)

	// Record event
	if m.recorder != nil {
		m.recorder.Eventf(pool, nil, corev1.EventTypeNormal, "WarmupStarted", "LaunchNode",
			"Started warming up node %s", instance.ID)
	}

	// Note: The Kubernetes node object will be created by the kubelet when it joins.
	// We'll wait for it to appear and then label it.
	return nil, nil
}

// LabelNode applies Stratos labels and template labels to a node.
// If the node is entering warmup state, it is also cordoned to prevent scheduling.
// Template labels from the NodePool spec are applied to the node, skipping any
// labels with the stratos.sh/ prefix to prevent conflicts with system labels.
func (m *Manager) LabelNode(ctx context.Context, node *corev1.Node, poolName, instanceID string, nodeState nodestate.NodeState, templateLabels map[string]string) error {
	logger := log.FromContext(ctx)

	// Create a copy for patching
	patch := client.MergeFrom(node.DeepCopy())

	// Ensure labels map exists
	if node.Labels == nil {
		node.Labels = make(map[string]string)
	}

	// Set Stratos labels
	node.Labels[nodestate.LabelPool] = poolName
	node.Labels[nodestate.LabelState] = string(nodeState)
	node.Labels[nodestate.LabelInstanceID] = instanceID
	node.Labels[nodestate.LabelStateSince] = fmt.Sprintf("%d", time.Now().Unix())

	// Apply template labels (skip stratos.sh/ prefix to prevent conflicts)
	for k, v := range templateLabels {
		if !strings.HasPrefix(k, "stratos.sh/") {
			node.Labels[k] = v
		}
	}

	// Ensure annotations map exists
	if node.Annotations == nil {
		node.Annotations = make(map[string]string)
	}

	// Cordon warmup nodes to prevent scheduling during warmup phase
	if nodeState == nodestate.NodeStateWarmup {
		node.Spec.Unschedulable = true
	}

	// Apply patch
	if err := m.client.Patch(ctx, node, patch); err != nil {
		return fmt.Errorf("failed to label node: %w", err)
	}

	logger.Info("Labeled node", "node", node.Name, "pool", poolName, "state", nodeState, "cordoned", nodeState == nodestate.NodeStateWarmup)
	return nil
}
